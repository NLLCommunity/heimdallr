package web

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// handleAuditLog renders the audit log viewer. HTMX POSTs from the filter
// form re-target #auditlog-table so we render the partial-only response
// when the request is HX-Request and the full page otherwise.
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

		var (
			entries []model.AuditLogEntry
			total   int64
		)
		if settings.AuditLogEnabled {
			modelFilter := buildAuditLogFilter(client, guildID, filters)
			entries, total, err = model.ListAuditLogEntries(guildID, modelFilter, auditLogPageSize, (page-1)*auditLogPageSize)
			if err != nil {
				http.Error(w, "failed to query audit log", http.StatusInternalServerError)
				return
			}
		}

		rows := buildAuditLogRows(client, guildID, entries)

		// HTMX form submits target the table only — return just the partial
		// so we don't repaint the filter form's input state.
		if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "auditlog-table" {
			renderSafe(w, r, partials.AuditLogTable(partials.AuditLogTableData{
				GuildID:  guildIDStr,
				Rows:     rows,
				Total:    total,
				Page:     page,
				PageSize: auditLogPageSize,
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
func buildAuditLogRows(client *bot.Client, guildID snowflake.ID, entries []model.AuditLogEntry) []partials.AuditLogRow {
	rows := make([]partials.AuditLogRow, len(entries))
	for i, e := range entries {
		label := eventLabel(e.EventType)
		// Legacy rows written before member.update was split: re-derive a
		// specific label from the stored details so old entries don't
		// just say "Member updated".
		if e.EventType == string(audit.EventMemberUpdate) {
			label = refineMemberUpdateLabel(label, e.Details)
		}
		summary, sections := summariseDetail(client, guildID, e.EventType, e.Details)
		rows[i] = partials.AuditLogRow{
			CreatedAt:      e.CreatedAt,
			EventType:      e.EventType,
			EventLabel:     label,
			Actor:          audit.FormatActor(client, guildID, audit.ActorKind(e.ActorKind), e.ActorID, e.Details),
			Target:         audit.FormatTarget(client, guildID, audit.TargetKind(e.TargetKind), e.TargetID, e.Details),
			Reason:         e.Reason,
			DetailSummary:  summary,
			DetailSections: sections,
		}
	}
	return rows
}

// summariseDetail extracts a one-line human summary plus optional
// sections from the stored details JSON for a given event type.
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
// Returns ("", nil) when there's nothing useful to surface.
func summariseDetail(client *bot.Client, guildID snowflake.ID, eventType, detailsJSON string) (summary string, sections []partials.DetailSection) {
	if detailsJSON == "" {
		return "", nil
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
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

	case string(audit.EventWebSettingsUpdate):
		return formatSettingsUpdate(client, guildID, d)

	case string(audit.EventWebPostCreate),
		string(audit.EventWebPostUpdate),
		string(audit.EventWebPostDelete):
		return stringField(d, "post_name"), nil
	}
	return "", nil
}

// formatSettingsUpdate renders a web.settings.update entry's details into
// a humanized summary + a "Changed values" section listing each field as
// "Field name: value". Channel and role IDs are resolved via the cache
// so the disclosure shows "#general" rather than the raw snowflake.
func formatSettingsUpdate(client *bot.Client, guildID snowflake.ID, d map[string]any) (string, []partials.DetailSection) {
	section := stringField(d, "section")
	summary := settingsSectionLabel(section)

	type field struct {
		key, label, value string
	}
	var fields []field
	for k, raw := range d {
		if k == "section" {
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
// Used as the label inside the changed-values disclosure body.
func humanizeFieldName(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "_")
	parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	return strings.Join(parts, " ")
}

// formatSettingsValue renders a setting's stored value for display.
// Booleans become On/Off, channel/role IDs resolve to readable names via
// the cache, empty strings fall back to "(default)" since the per-guild
// settings handlers store cleared overrides as "".
func formatSettingsValue(client *bot.Client, guildID snowflake.ID, key string, raw any) string {
	switch v := raw.(type) {
	case bool:
		if v {
			return "On"
		}
		return "Off"
	case string:
		if v == "" {
			return "(default)"
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
	for _, opt := range auditLogEventOptions() {
		if opt.Value == t {
			return opt.Label
		}
	}
	return t
}

// refineMemberUpdateLabel returns a more specific label for member.update
// based on which keys are present in the stored details JSON. Falls back
// to the generic label when nothing matches — a member.update with no
// trackable change is suppressed at the listener anyway.
func refineMemberUpdateLabel(fallback, detailsJSON string) string {
	if detailsJSON == "" {
		return fallback
	}
	var d struct {
		NickBefore     *string `json:"nick_before"`
		NickAfter      *string `json:"nick_after"`
		RolesAdded     []any   `json:"roles_added"`
		RolesRemoved   []any   `json:"roles_removed"`
		TimeoutUntil   any     `json:"timeout_until"`
		TimeoutCleared *bool   `json:"timeout_cleared"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		return fallback
	}

	switch {
	case d.TimeoutCleared != nil && *d.TimeoutCleared:
		return "Timeout cleared"
	case d.TimeoutUntil != nil:
		return "Member timed out"
	case len(d.RolesAdded) > 0 && len(d.RolesRemoved) > 0:
		return "Roles changed"
	case len(d.RolesAdded) > 0:
		return "Role added"
	case len(d.RolesRemoved) > 0:
		return "Role removed"
	case d.NickAfter != nil || d.NickBefore != nil:
		return "Nickname changed"
	}
	return fallback
}

// parseAuditLogFilters reads the filter values out of the query string into
// the form-state struct used by the template. Empty values mean "no filter".
//
// `from` defaults to today minus auditLogDefaultLookback if missing — but
// only when no other filters narrow the scope, since explicitly filtering
// on actor/target/event already bounds the result set.
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
	if filters.From == "" && filters.Actor == "" && filters.Target == "" && filters.EventType == "" {
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
	query = strings.TrimSpace(strings.TrimPrefix(query, "@"))
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
func resolveTargetQuery(client *bot.Client, guildID snowflake.ID, query string) (ids []snowflake.ID, channelIDs []snowflake.ID, text string) {
	wantUser := !strings.HasPrefix(query, "#")
	wantChannel := !strings.HasPrefix(query, "@")
	query = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(query, "@"), "#"))
	if query == "" {
		return nil, nil, ""
	}
	if id, err := snowflake.Parse(query); err == nil {
		return []snowflake.ID{id}, []snowflake.ID{id}, ""
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

func matchMembersByUsername(client *bot.Client, guildID snowflake.ID, query string) []snowflake.ID {
	q := strings.ToLower(query)
	var ids []snowflake.ID
	for member := range client.Caches.Members(guildID) {
		if strings.Contains(strings.ToLower(member.User.Username), q) {
			ids = append(ids, member.User.ID)
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
		}
	}
	return ids
}

func parsePage(s string) int {
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// auditLogEventOptions returns the dropdown options for the EventType
// filter. Listed centrally here rather than hardcoded in the template so
// adding a new audit event type only requires updating the audit package
// and this slice.
func auditLogEventOptions() []pages.AuditLogEventOption {
	return []pages.AuditLogEventOption{
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
		{Value: string(audit.EventWebSettingsUpdate), Label: "Settings updated", Category: string(audit.CategoryGuild)},
		{Value: string(audit.EventWebPostCreate), Label: "Post created", Category: string(audit.CategoryGuild)},
		{Value: string(audit.EventWebPostUpdate), Label: "Post updated", Category: string(audit.CategoryGuild)},
		{Value: string(audit.EventWebPostDelete), Label: "Post deleted", Category: string(audit.CategoryGuild)},
	}
}
