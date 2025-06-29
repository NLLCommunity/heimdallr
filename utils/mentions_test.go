package utils

import (
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
)

func TestMentionRoleOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		id       *snowflake.ID
		def      string
		expected string
	}{
		{
			name:     "valid role ID",
			id:       Ref(snowflake.ID(123456789)),
			def:      "everyone",
			expected: "<@&123456789>",
		},
		{
			name:     "nil role ID",
			id:       nil,
			def:      "everyone",
			expected: "everyone",
		},
		{
			name:     "zero role ID",
			id:       Ref(snowflake.ID(0)),
			def:      "default role",
			expected: "default role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MentionRoleOrDefault(tt.id, tt.def)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMentionChannelOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		id       *snowflake.ID
		def      string
		expected string
	}{
		{
			name:     "valid channel ID",
			id:       Ref(snowflake.ID(987654321)),
			def:      "#general",
			expected: "<#987654321>",
		},
		{
			name:     "nil channel ID",
			id:       nil,
			def:      "#general",
			expected: "#general",
		},
		{
			name:     "zero channel ID",
			id:       Ref(snowflake.ID(0)),
			def:      "#default-channel",
			expected: "#default-channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MentionChannelOrDefault(tt.id, tt.def)
			assert.Equal(t, tt.expected, result)
		})
	}
}
