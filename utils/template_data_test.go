package utils

import (
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBirthdayTemplateAgeIsOptional(t *testing.T) {
	data := MessageTemplateData{}
	assert.Empty(t, data.Age)
	assert.False(t, data.HasAge)

	withoutAge, err := mustache.RenderRaw("{{#HasAge}}turns {{Age}}{{/HasAge}}", true, data)
	require.NoError(t, err)
	assert.Empty(t, withoutAge)

	data.Age, data.HasAge = "36", true
	withAge, err := mustache.RenderRaw("{{#HasAge}}turns {{Age}}{{/HasAge}}", true, data)
	require.NoError(t, err)
	assert.Equal(t, "turns 36", withAge)
}

func TestBirthdayTemplateInfoAddsAgeWithoutChangingGenericInfo(t *testing.T) {
	birthdayInfo := BirthdayMessageTemplateInfo()
	assert.Contains(t, birthdayInfo, "{{Age}}")
	assert.Contains(t, birthdayInfo, "{{#HasAge}}")
	assert.NotContains(t, MessageTemplateInfo(), "{{Age}}")
}
