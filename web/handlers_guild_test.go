package web

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/disgoorg/disgo/rest"
	"github.com/stretchr/testify/assert"
)

// A Discord 401 must be recognized even when wrapped, while other REST
// failures (or plain errors) must not be - they take the 502 branch in
// handleGuilds instead of bouncing the user through consent.
func TestIsUnauthorizedRest(t *testing.T) {
	unauthorized := &rest.Error{Response: &http.Response{StatusCode: http.StatusUnauthorized}}
	assert.True(t, isUnauthorizedRest(unauthorized))
	assert.True(t, isUnauthorizedRest(fmt.Errorf("wrapped: %w", unauthorized)))

	assert.False(t, isUnauthorizedRest(&rest.Error{Response: &http.Response{StatusCode: http.StatusInternalServerError}}))
	assert.False(t, isUnauthorizedRest(&rest.Error{}))
	assert.False(t, isUnauthorizedRest(errors.New("connection refused")))
	assert.False(t, isUnauthorizedRest(nil))
}
