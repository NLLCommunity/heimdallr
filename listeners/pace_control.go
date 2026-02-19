package listeners

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// maxRetention is the maximum time entries are kept in the buffer.
// This must be at least as large as the largest configured window.
// Entries older than this are pruned on every write.
const maxRetention = 5 * time.Minute

type paceEntry struct {
	Timestamp time.Time
	WordCount int
	AuthorID  snowflake.ID
}

type paceBuffer struct {
	mu      sync.Mutex
	entries []paceEntry
}

// paceBuffers stores rolling message data keyed by "guildID:channelID".
var paceBuffers sync.Map

func OnPaceControlMessageCreate(e *events.GuildMessageCreate) {
	content := e.Message.Content
	if content == "" {
		return
	}

	words := len(strings.Fields(content))
	if words == 0 {
		return
	}

	key := e.GuildID.String() + ":" + e.Message.ChannelID.String()

	val, loaded := paceBuffers.LoadOrStore(key, &paceBuffer{})
	buf := val.(*paceBuffer)

	if !loaded {
		slog.Debug("pace-control: tracking new channel", "key", key)
	}

	now := time.Now()
	buf.mu.Lock()
	buf.entries = append(buf.entries, paceEntry{
		Timestamp: now,
		WordCount: words,
		AuthorID:  e.Message.Author.ID,
	})
	// Prune entries older than maxRetention
	cutoff := now.Add(-maxRetention)
	start := 0
	for start < len(buf.entries) && buf.entries[start].Timestamp.Before(cutoff) {
		start++
	}
	if start > 0 {
		buf.entries = buf.entries[start:]
	}
	entryCount := len(buf.entries)
	buf.mu.Unlock()

	slog.Debug("pace-control: message tracked",
		"guild", e.GuildID,
		"channel", e.Message.ChannelID,
		"words", words,
		"author", e.Message.Author.ID,
		"buffer_size", entryCount,
	)
}

// GetPaceStats returns the total words within wpmWindow and unique author
// count within userWindow for a given channel key.
func GetPaceStats(key string, wpmWindow, userWindow time.Duration) (totalWords int, activeUsers int) {
	val, ok := paceBuffers.Load(key)
	if !ok {
		return 0, 0
	}
	buf := val.(*paceBuffer)

	now := time.Now()
	wpmCutoff := now.Add(-wpmWindow)
	userCutoff := now.Add(-userWindow)

	// Use the older cutoff for pruning
	pruneCutoff := now.Add(-maxRetention)

	authors := make(map[snowflake.ID]struct{})

	buf.mu.Lock()
	defer buf.mu.Unlock()

	// Prune entries beyond max retention
	start := 0
	for start < len(buf.entries) && buf.entries[start].Timestamp.Before(pruneCutoff) {
		start++
	}
	if start > 0 {
		buf.entries = buf.entries[start:]
	}

	for _, e := range buf.entries {
		if !e.Timestamp.Before(wpmCutoff) {
			totalWords += e.WordCount
		}
		if !e.Timestamp.Before(userCutoff) {
			authors[e.AuthorID] = struct{}{}
		}
	}

	return totalWords, len(authors)
}

// ActiveChannelKeys returns all channel keys that currently have pace data.
func ActiveChannelKeys() []string {
	var keys []string
	paceBuffers.Range(func(key, _ any) bool {
		keys = append(keys, key.(string))
		return true
	})
	return keys
}
