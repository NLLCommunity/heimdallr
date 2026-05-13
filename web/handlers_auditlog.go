package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"github.com/NLLCommunity/heimdallr/audit"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
)

const auditLogPageSize = 50

// auditLogDefaultLookback is applied when the user hasn't specified a `from`
// filter — keeps the first page snappy on busy servers without preventing
// older entries being reached via explicit From / pagination.
const auditLogDefaultLookback = 7 * 24 * time.Hour

// handleAuditLog renders the audit log viewer. The filter form and the
// pagination links both issue hx-get requests targeting #auditlog-table,
// so we render the table partial when the request carries HX-Request +
// HX-Target=auditlog-table, and the full page otherwise.
func handleAuditLog(client *bot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guildIDStr := r.PathValue("id")
		guildID, ok := checkGuildAdmin(w, r, client, guildIDStr)
		if !ok {
			return
		}

		settings, err := model.GetGuildSettings(guildID)
		if err != nil {
			http.Error(w, "failed to load settings", http.StatusInternalServerError)
			return
		}

		filters := parseAuditLogFilters(r)
		page := parsePage(r.URL.Query().Get("page"))

		// Query regardless of the enabled flag. The flag gates *writes*
		// (new entries are dropped at audit.Log / LogPending via shouldLog);
		// reads are harmless and the settings UI tells admins existing
		// rows "remain until pruned" — they should be visible whether the
		// feature is currently on or off. The page renders a "currently
		// disabled" banner above the table when applicable.
		modelFilter := buildAuditLogFilter(client, guildID, filters)
		entries, total, err := model.ListAuditLogEntries(guildID, modelFilter, auditLogPageSize, (page-1)*auditLogPageSize)
		if err != nil {
			http.Error(w, "failed to query audit log", http.StatusInternalServerError)
			return
		}

		rows := buildAuditLogRows(client, guildID, entries)
		filterQuery := auditLogFilterQuery(filters)

		// HTMX form submits target the table only — return just the partial
		// so we don't repaint the filter form's input state.
		if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "auditlog-table" {
			renderSafe(w, r, partials.AuditLogTable(partials.AuditLogTableData{
				GuildID:     guildIDStr,
				Rows:        rows,
				Total:       total,
				Page:        page,
				PageSize:    auditLogPageSize,
				FilterQuery: filterQuery,
			}))
			return
		}

		session := sessionFromContext(r.Context())
		guild, _ := client.Caches.Guild(guildID)
		nav := layouts.NavData{
			User:      session,
			GuildID:   guildIDStr,
			GuildName: guild.Name,
			IsAdmin:   true,
		}

		renderSafe(w, r, pages.AuditLog(nav, pages.AuditLogData{
			GuildID:      guildIDStr,
			Enabled:      settings.AuditLogEnabled,
			Filters:      filters,
			FilterQuery:  filterQuery,
			Rows:         rows,
			Total:        total,
			Page:         page,
			PageSize:     auditLogPageSize,
			EventOptions: auditLogEventOptions(),
		}))
	}
}

// buildAuditLogRows turns persisted AuditLogEntry rows into render-ready
// rows with names resolved from the disgo cache. Done here (not in templ)
// so the template stays free of cache lookups and unit tests aren't
// blocked on a live bot client.
//
// Each entry's Details JSON is decoded exactly once per row, then passed
// to FormatActor / FormatTarget / summariseDetail / refineMemberUpdateLabel
// as the same map. Previously each of those helpers unmarshalled the
// payload itself — on a 50-row page that meant up to 200 redundant parses.
func buildAuditLogRows(client *bot.Client, guildID snowflake.ID, entries []model.AuditLogEntry) []partials.AuditLogRow {
	rows := make([]partials.AuditLogRow, len(entries))
	for i, e := range entries {
		var details map[string]any
		if e.Details != "" {
			if err := json.Unmarshal([]byte(e.Details), &details); err != nil {
				// Malformed payload still renders the row (with an
				// ID-only target and no summary), but log so an operator
				// can investigate — corruption / manual DB edit / schema
				// drift from a rolled-back writer would otherwise produce
				// silently-blank rows.
				slog.Warn("audit log: unparseable Details JSON",
					"err", err, "entry_id", e.ID, "event_type", e.EventType)
			}
		}

		label := eventLabel(e.EventType)
		// Legacy rows written before member.update was split: re-derive a
		// specific label from the stored details so old entries don't
		// just say "Member updated".
		if e.EventType == string(audit.EventMemberUpdate) {
			label = refineMemberUpdateLabel(label, details)
		}
		summary, sections := summariseDetail(client, guildID, e.EventType, details)
		rows[i] = partials.AuditLogRow{
			CreatedAt:      e.CreatedAt,
			EventType:      e.EventType,
			EventLabel:     label,
			Actor:          audit.FormatActor(client, guildID, audit.ActorKind(e.ActorKind), e.ActorID, details),
			Target:         audit.FormatTarget(client, guildID, audit.TargetKind(e.TargetKind), e.TargetID, details),
			Reason:         e.Reason,
			DetailSummary:  summary,
			DetailSections: sections,
		}
	}
	return rows
}

// summariseDetail extracts a one-line human summary plus optional
// sections from the already-decoded details map for a given event type.
//
// Sections are headed text blocks that render inside an expandable
// <details> disclosure when present. Events that produce only a short
// summary (no sections) render the summary as a small subtitle under
// the event label rather than as a sub-row, since a sub-row containing
// just a label feels visually disconnected.
//
// client + guildID are used to resolve embedded channel/role IDs in the
// details payload (web settings updates can carry these as referenced
// values).
//
// Returns ("", nil) when there's nothing useful to surface. A nil details
// map (legacy or malformed-JSON row) is treated as "nothing to surface".
func summariseDetail(client *bot.Client, guildID snowflake.ID, eventType string, d map[string]any) (summary string, sections []partials.DetailSection) {
	if d == nil {
		return "", nil
	}

	switch eventType {
	case string(audit.EventMessageDelete):
		full := stringField(d, "before_content")
		if full == "" {
			return "", nil
		}
		return truncate(full, 120), []partials.DetailSection{
			{Heading: "Deleted message", Body: full},
		}

	case string(audit.EventMessageEdit):
		before := stringField(d, "before_content")
		after := stringField(d, "after_content")
		if before == "" && after == "" {
			return "", nil
		}
		summary := truncate(before, 60) + "  →  " + truncate(after, 60)
		return summary, []partials.DetailSection{
			{Heading: "Before", Body: before},
			{Heading: "After", Body: after},
		}

	case string(audit.EventMemberNickChange):
		before := stringField(d, "nick_before")
		after := stringField(d, "nick_after")
		if before == "" {
			before = "—"
		}
		if after == "" {
			after = "—"
		}
		return before + " → " + after, nil

	case string(audit.EventMemberRoleChange):
		return roleChangeSummary(d), nil

	case string(audit.EventMemberTimeoutAdd):
		if v, ok := d["timeout_until"].(string); ok {
			return "until " + v, nil
		}

	case string(audit.EventBotWarn):
		if w, ok := d["weight"].(float64); ok {
			return "severity " + strconv.FormatFloat(w, 'f', -1, 64), nil
		}

	case string(audit.EventGuildPrune):
		removed := stringField(d, "members_removed")
		days := stringField(d, "delete_member_days")
		switch {
		case removed != "" && days != "":
			return removed + " removed (inactive ≥ " + days + " days)", nil
		case removed != "":
			return removed + " removed", nil
		case days != "":
			return "inactive ≥ " + days + " days", nil
		}

	case string(audit.EventSettingsUpdate),
		string(audit.EventWebSettingsUpdate):
		return formatSettingsUpdate(client, guildID, d)

	case string(audit.EventWebPostCreate),
		string(audit.EventWebPostUpdate),
		string(audit.EventWebPostDelete):
		return stringField(d, "post_name"), nil
	}
	return "", nil
}

// settingsUpdateMetadataKeys are keys present in a settings.update
// Details payload that are NOT user-changed settings — they're
// attribution / routing metadata co-located in the same map (e.g.
// actor_username for cache-miss rendering, section for the form group).
// formatSettingsUpdate skips these when building the "Changed values"
// section so the viewer doesn't claim "Actor username: alice" is a
// setting that was modified.
var settingsUpdateMetadataKeys = map[string]bool{
	"section":         true,
	"actor_username":  true,
	"target_username": true,
}

// formatSettingsUpdate renders a web.settings.update entry's details into
// a humanized summary + a "Changed values" section listing each field as
// "Field name: value". Channel and role IDs are resolved via the cache
// so the disclosure shows "#general" rather than the raw snowflake.
//
// Keys in settingsUpdateMetadataKeys are skipped — they're attribution
// metadata, not actual settings.
func formatSettingsUpdate(client *bot.Client, guildID snowflake.ID, d map[string]any) (string, []partials.DetailSection) {
	section := stringField(d, "section")
	summary := settingsSectionLabel(section)

	type field struct {
		key, label, value string
	}
	var fields []field
	for k, raw := range d {
		if settingsUpdateMetadataKeys[k] {
			continue
		}
		v := formatSettingsValue(client, guildID, k, raw)
		fields = append(fields, field{key: k, label: humanizeFieldName(k), value: v})
	}
	if len(fields) == 0 {
		return summary, nil
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].label < fields[j].label })

	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(f.label)
		b.WriteString(": ")
		b.WriteString(f.value)
	}
	return summary, []partials.DetailSection{
		{Heading: "Changed values", Body: b.String()},
	}
}

// settingsSectionLabel maps the internal section identifier (snake_case
// matching the form route, e.g. "audit_log") to the label moderators see
// in the audit log viewer.
func settingsSectionLabel(section string) string {
	switch section {
	case "mod_channel":
		return "Moderator channel"
	case "infractions":
		return "Infractions"
	case "anti_spam":
		return "Anti-spam"
	case "ban_footer":
		return "Ban footer"
	case "modmail":
		return "Modmail"
	case "gatekeep":
		return "Gatekeep"
	case "join_leave":
		return "Join/leave messages"
	case "audit_log":
		return "Audit log"
	}
	if section == "" {
		return "Settings"
	}
	return section
}

// humanizeFieldName converts a snake_case settings key into Sentence case.
// Used as the label inside the changed-values disclosure body. Empty
// parts (from leading / trailing / consecutive underscores) are dropped
// so we never index into "" and never emit leading/duplicate spaces.
func humanizeFieldName(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "_")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if len(out) == 0 {
			p = strings.ToUpper(p[:1]) + p[1:]
		}
		out = append(out, p)
	}
	return strings.Join(out, " ")
}

// formatSettingsValue renders a setting's stored value for display.
// Booleans become On/Off, channel/role IDs resolve to readable names via
// the cache. Cleared channel/role fields render as "(none)" — the web
// dashboard serializes a cleared snowflake as "" via idStr() while
// command-side updates emit "0", so both shapes funnel to the same label.
// True override-style fields (retention days, etc.) keep "(default)" for
// empty values; an explicit "0" for those fields renders as the literal
// "0". parseRetentionField now collapses 0-when-ceiling-is-0 to nil at
// save time, so a literal "0" in the audit Details payload should only
// appear on legacy rows written before that normalization landed.
func formatSettingsValue(client *bot.Client, guildID snowflake.ID, key string, raw any) string {
	isSnowflakeKey := strings.HasSuffix(key, "_channel") || strings.HasSuffix(key, "_role")
	switch v := raw.(type) {
	case bool:
		if v {
			return "On"
		}
		return "Off"
	case string:
		if v == "" {
			if isSnowflakeKey {
				return "(none)"
			}
			return "(default)"
		}
		if v == "0" && isSnowflakeKey {
			return "(none)"
		}
		if strings.HasSuffix(key, "_channel") {
			if id, err := snowflake.Parse(v); err == nil {
				return "#" + audit.ResolveChannelName(client, id)
			}
		}
		if strings.HasSuffix(key, "_role") {
			if id, err := snowflake.Parse(v); err == nil {
				return "@" + audit.ResolveRoleName(client, guildID, id)
			}
		}
		return v
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		if isSnowflakeKey {
			return "(none)"
		}
		return "(default)"
	}
	return fmt.Sprintf("%v", raw)
}

// stringField pulls a string out of a generic decoded JSON object,
// returning "" for both missing and non-string keys.
func stringField(d map[string]any, key string) string {
	if v, ok := d[key].(string); ok {
		return v
	}
	return ""
}

// truncate shortens long strings to width, appending an ellipsis. width
// is in runes so we don't mid-truncate a UTF-8 sequence.
func truncate(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width]) + "…"
}

// roleChangeSummary builds "+@RoleA, -@RoleB" from a member.role_change
// details payload. The slices are stored as [{id, name}, ...] by the
// listener so we can render readable names without re-fetching.
func roleChangeSummary(d map[string]any) string {
	added := roleNames(d, "roles_added")
	removed := roleNames(d, "roles_removed")
	parts := make([]string, 0, len(added)+len(removed))
	for _, name := range added {
		parts = append(parts, "+@"+name)
	}
	for _, name := range removed {
		parts = append(parts, "-@"+name)
	}
	return strings.Join(parts, ", ")
}

func roleNames(d map[string]any, key string) []string {
	raw, ok := d[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if obj, ok := item.(map[string]any); ok {
			if name, ok := obj["name"].(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// eventLabel returns the human-friendly label for an event type. Falls
// back to the raw type for unmapped values so adding a new event type
// degrades gracefully even before its label is registered here.
func eventLabel(t string) string {
	if label, ok := auditLogEventLabels[t]; ok {
		return label
	}
	return t
}

// refineMemberUpdateLabel returns a more specific label for legacy
// member.update rows by inspecting which keys are in the already-decoded
// details map. Current listeners write split event types (member.nick_change,
// member.role_change, member.timeout_add, member.timeout_clear), so the
// only member.update rows that reach this function are either pre-split
// historical rows or the fallback emitted when memberUpdateEnrichmentTargets
// doesn't recognise any change key — neither path writes timeout fields.
func refineMemberUpdateLabel(fallback string, d map[string]any) string {
	if d == nil {
		return fallback
	}
	// Type-assert against string so a JSON `null` value is treated as
	// absent — matches the old pointer-based decode where `null` decoded
	// to a nil *string. Current writers don't emit null nicks (they go
	// to EventMemberNickChange), but legacy/hand-edited rows might.
	_, hasNickBefore := d["nick_before"].(string)
	_, hasNickAfter := d["nick_after"].(string)
	rolesAdded, _ := d["roles_added"].([]any)
	rolesRemoved, _ := d["roles_removed"].([]any)

	switch {
	case len(rolesAdded) > 0 && len(rolesRemoved) > 0:
		return "Roles changed"
	case len(rolesAdded) > 0:
		return "Role added"
	case len(rolesRemoved) > 0:
		return "Role removed"
	case hasNickAfter || hasNickBefore:
		return "Nickname changed"
	}
	return fallback
}

// auditLogFilterQuery URL-encodes the active filters as a query string
// fragment WITHOUT a leading "?" and WITHOUT the page parameter. Used by
// the pagination links so the href and hx-get URL stand alone: a user
// opening "← Newer" in a new tab, or browsing without HTMX, gets the
// same filtered view they had on-page rather than an unfiltered scan.
//
// hx-include="form" already supplies these values for the in-page HTMX
// request, but it doesn't influence the rendered href, so we still need
// the values embedded explicitly for non-HTMX navigation.
func auditLogFilterQuery(f pages.AuditLogFilters) string {
	v := url.Values{}
	if f.Category != "" {
		v.Set("category", f.Category)
	}
	if f.EventType != "" {
		v.Set("event_type", f.EventType)
	}
	if f.Actor != "" {
		v.Set("actor", f.Actor)
	}
	if f.Target != "" {
		v.Set("target", f.Target)
	}
	if f.From != "" {
		v.Set("from", f.From)
	}
	if f.To != "" {
		v.Set("to", f.To)
	}
	return v.Encode()
}

// parseAuditLogFilters reads the filter values out of the query string into
// the form-state struct used by the template. Empty values mean "no filter".
//
// `from` defaults to today minus auditLogDefaultLookback if missing — but
// only when no actor/target/event/time filter narrows the scope. Category
// alone doesn't suppress the default because it splits the dataset only
// 3 ways; a category-only query on a busy guild would still scan most of
// retention. Actor/target/event are precise enough to bound results.
func parseAuditLogFilters(r *http.Request) pages.AuditLogFilters {
	q := r.URL.Query()
	filters := pages.AuditLogFilters{
		Category:  strings.ToLower(strings.TrimSpace(q.Get("category"))),
		EventType: strings.TrimSpace(q.Get("event_type")),
		Actor:     strings.TrimSpace(q.Get("actor")),
		Target:    strings.TrimSpace(q.Get("target")),
		From:      strings.TrimSpace(q.Get("from")),
		To:        strings.TrimSpace(q.Get("to")),
	}
	if filters.From == "" && filters.To == "" &&
		filters.Actor == "" && filters.Target == "" &&
		filters.EventType == "" {
		filters.From = time.Now().Add(-auditLogDefaultLookback).UTC().Format("2006-01-02")
	}
	return filters
}

// buildAuditLogFilter converts the URL/form values into the model filter
// type. The actor/target inputs accept either a snowflake ID, a username
// (with or without a leading @), or a channel name (with or without #).
//
// Resolution layers OR together:
//   - bare snowflake → exact ID match.
//   - non-snowflake text → cached member/channel substring match (so a
//     name in the disgo cache resolves to a precise ID), AND a fallback
//     LIKE against the username stored in the entry's details JSON for
//     rows whose users aren't currently cached (e.g. after a restart).
//
// Channel names search both target_id (events whose target IS the
// channel) and details.channel_id (message events that happened in the
// channel) so "#general" finds either kind.
func buildAuditLogFilter(client *bot.Client, guildID snowflake.ID, f pages.AuditLogFilters) model.AuditLogFilter {
	mf := model.AuditLogFilter{
		Category:  f.Category,
		EventType: f.EventType,
	}
	if f.Actor != "" {
		ids, query := resolveActorQuery(client, guildID, f.Actor)
		mf.ActorIDs = ids
		mf.ActorQuery = query
	}
	if f.Target != "" {
		ids, channelIDs, query := resolveTargetQuery(client, guildID, f.Target)
		mf.TargetIDs = ids
		mf.TargetChannelIDs = channelIDs
		mf.TargetQuery = query
	}
	if f.From != "" {
		if t, err := time.Parse("2006-01-02", f.From); err == nil {
			mf.From = t.UTC()
		}
	}
	if f.To != "" {
		if t, err := time.Parse("2006-01-02", f.To); err == nil {
			// To is inclusive in the UI, exclusive in the query — add a day.
			mf.To = t.Add(24 * time.Hour).UTC()
		}
	}
	return mf
}

// resolveActorQuery normalises the Actor input into:
//   - ids: cached members whose username substring matches (or the
//     parsed snowflake when input is a bare ID).
//   - text: the query itself, used by the model layer for a LIKE
//     fallback against details.actor_username when the cache is cold.
//
// Snowflake input returns (ids=[id], text="") — exact match only, since
// the user clearly wants that specific ID.
func resolveActorQuery(client *bot.Client, guildID snowflake.ID, query string) (ids []snowflake.ID, text string) {
	// Trim whitespace before stripping the @ — " @user" should be treated
	// the same as "@user", and TrimPrefix only matches at the very start.
	query = strings.TrimPrefix(strings.TrimSpace(query), "@")
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ""
	}
	if id, err := snowflake.Parse(query); err == nil {
		return []snowflake.ID{id}, ""
	}
	return matchMembersByUsername(client, guildID, query), query
}

// resolveTargetQuery normalises the Target input into:
//   - ids: snowflakes that should match target_id directly (cached
//     members and cached channels whose names substring-match).
//   - channelIDs: channel IDs that should match details.channel_id —
//     populated only when the query could be a channel name, since
//     message events store channel context there rather than as the
//     target.
//   - text: query for the LIKE fallback against details.target_username.
//
// Leading "#" or "@" scopes the search; bare input matches both kinds.
// The snowflake fast-path respects the same scoping — an "@id" query
// only matches target_id (treating the ID as a user) while "#id" also
// extends the match to details.channel_id (treating the ID as a channel).
//
// When both user and channel matchers run (bare query), they overflow
// independently: a query that matches too many cached members may drop
// its user-ID contribution while the channel matcher still contributes
// IDs (or vice versa). The text return is always populated for
// non-snowflake queries, so the LIKE fallback against details still
// covers the overflowed side.
func resolveTargetQuery(client *bot.Client, guildID snowflake.ID, query string) (ids []snowflake.ID, channelIDs []snowflake.ID, text string) {
	// Normalise whitespace before deciding scope so " #general" still
	// reads as a channel-only query, then strip the scoping prefix.
	query = strings.TrimSpace(query)
	wantUser := !strings.HasPrefix(query, "#")
	wantChannel := !strings.HasPrefix(query, "@")
	query = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(query, "@"), "#"))
	if query == "" {
		return nil, nil, ""
	}
	if id, err := snowflake.Parse(query); err == nil {
		// target_id stores the user for user-target events and the channel
		// for channel-target events, so any scope wants the snowflake in
		// ids. channelIDs (which matches details.channel_id on message
		// events) is only meaningful when the caller hinted "this is a
		// channel" with the # prefix or didn't scope at all.
		ids = []snowflake.ID{id}
		if wantChannel {
			channelIDs = []snowflake.ID{id}
		}
		return ids, channelIDs, ""
	}
	if wantUser {
		ids = append(ids, matchMembersByUsername(client, guildID, query)...)
	}
	if wantChannel {
		matchedChannels := matchChannelsByName(client, guildID, query)
		ids = append(ids, matchedChannels...)
		channelIDs = append(channelIDs, matchedChannels...)
	}
	return ids, channelIDs, query
}

// matchResolutionCap bounds how many cached IDs a substring match contributes
// to the model filter. Each ID becomes a bound parameter in an IN (?,?,...)
// list, and SQLite caps the total bound parameters per statement at
// SQLITE_MAX_VARIABLE_NUMBER (999 by default, 32766 on 3.32+ — not portable
// to assume the larger value). On a busy guild a permissive query like "a"
// could match thousands of cached members and exceed that limit, erroring
// the whole query. When a matcher overflows we drop the cache-precise IDs
// and let the LIKE fallback against details.{actor,target}_username handle
// it — less precise (only catches events whose details enriched the
// username), but it never blows up the query. Sized well under SQLite's
// budget after accounting for the other clauses in the same statement
// (guild_id, category, event_type, From, To, the sibling actor/target
// person clause, and the second person clause's own ID list).
//
// "Drop all on overflow" is intentional rather than "keep the first N":
// a partial cache-precise match would silently bias the result toward
// whatever subset of members the cache iterator emitted first, which is
// implementation-dependent. Falling back to LIKE gives a result the user
// can reason about.
const matchResolutionCap = 200

func matchMembersByUsername(client *bot.Client, guildID snowflake.ID, query string) []snowflake.ID {
	q := strings.ToLower(query)
	var ids []snowflake.ID
	for member := range client.Caches.Members(guildID) {
		if strings.Contains(strings.ToLower(member.User.Username), q) {
			ids = append(ids, member.User.ID)
			if len(ids) > matchResolutionCap {
				return nil
			}
		}
	}
	return ids
}

func matchChannelsByName(client *bot.Client, guildID snowflake.ID, query string) []snowflake.ID {
	q := strings.ToLower(query)
	var ids []snowflake.ID
	for ch := range client.Caches.ChannelsForGuild(guildID) {
		if strings.Contains(strings.ToLower(ch.Name()), q) {
			ids = append(ids, ch.ID())
			if len(ids) > matchResolutionCap {
				return nil
			}
		}
	}
	return ids
}

// auditLogMaxPage caps the page parameter so a crafted query string can't
// drive an arbitrarily large OFFSET into the DB. At pageSize=50 this still
// reaches 500k rows of history, far beyond any realistic guild's retention.
const auditLogMaxPage = 10_000

func parsePage(s string) int {
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	if n > auditLogMaxPage {
		return auditLogMaxPage
	}
	return n
}

// auditLogEventOptions returns the dropdown options for the EventType
// filter. Listed centrally here rather than hardcoded in the template so
// adding a new audit event type only requires updating the audit package
// and auditLogEventOptionList.
func auditLogEventOptions() []pages.AuditLogEventOption {
	return auditLogEventOptionList
}

// auditLogEventOptionList is built once at package init. eventLabel runs
// per row, so a per-call slice allocation + linear scan would do real
// work on busy pages; auditLogEventLabels backs that lookup as an O(1)
// map derived from this same source of truth.
var auditLogEventOptionList = []pages.AuditLogEventOption{
	{Value: string(audit.EventMessageEdit), Label: "Message edited", Category: string(audit.CategoryMessage)},
	{Value: string(audit.EventMessageDelete), Label: "Message deleted", Category: string(audit.CategoryMessage)},
	{Value: string(audit.EventMemberNickChange), Label: "Nickname changed", Category: string(audit.CategoryMember)},
	{Value: string(audit.EventMemberRoleChange), Label: "Roles changed", Category: string(audit.CategoryMember)},
	{Value: string(audit.EventMemberTimeoutAdd), Label: "Member timed out", Category: string(audit.CategoryMember)},
	{Value: string(audit.EventMemberTimeoutClear), Label: "Timeout cleared", Category: string(audit.CategoryMember)},
	{Value: string(audit.EventGuildBan), Label: "Member banned", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventGuildUnban), Label: "Member unbanned", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventGuildKick), Label: "Member kicked", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventGuildPrune), Label: "Members pruned", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventBotWarn), Label: "Bot warning issued", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventSettingsUpdate), Label: "Settings updated", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventWebPostCreate), Label: "Post created", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventWebPostUpdate), Label: "Post updated", Category: string(audit.CategoryGuild)},
	{Value: string(audit.EventWebPostDelete), Label: "Post deleted", Category: string(audit.CategoryGuild)},
}

var auditLogEventLabels = func() map[string]string {
	m := make(map[string]string, len(auditLogEventOptionList)+2)
	for _, opt := range auditLogEventOptionList {
		m[opt.Value] = opt.Label
	}
	// Legacy event types — kept out of the filter dropdown (new entries
	// use the renamed types) but still possible on DB rows written
	// before the rename, so they get a readable label rather than
	// falling through to the raw event string. refineMemberUpdateLabel
	// may further refine the member.update label when the details
	// payload identifies the specific change.
	m[string(audit.EventMemberUpdate)] = "Member updated"
	m[string(audit.EventWebSettingsUpdate)] = "Settings updated"
	return m
}()
