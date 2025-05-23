package utils

import (
	"fmt"

	"github.com/disgoorg/snowflake/v2"
)

// MentionRoleOrDefault formats a mention for a role by its ID. If the ID
// pointer is nil or zero, use a provided fallback value instead.
func MentionRoleOrDefault(id *snowflake.ID, def string) string {
	if id == nil || *id == 0 {
		return def
	}

	return fmt.Sprintf("<@&%d>", *id)
}

// MentionChannelOrDefault formats a mention for a channel by its ID. If the ID
// pointer is nil or zero, use a provided fallback value instead.
func MentionChannelOrDefault(id *snowflake.ID, def string) string {
	if id == nil || *id == 0 {
		return def
	}

	return fmt.Sprintf("<#%d>", *id)
}
