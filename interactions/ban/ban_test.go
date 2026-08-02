package ban

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
)

func TestBanDMFailureLogFieldsOnlyIncludeSafeUserID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	user := discord.User{
		ID:       snowflake.ID(123456789012345678),
		Username: "secret-username",
	}

	logger.Info("Could not DM user with ban information", banDMFailureLogFields(user, errors.New("send failed"))...)

	output := buf.String()
	assert.Contains(t, output, "user_id=123456789012345678")
	assert.Contains(t, output, "err=\"send failed\"")
	assert.NotContains(t, output, "secret-username")
	assert.NotContains(t, output, "user=")
}
