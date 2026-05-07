package model

import (
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type PostTestSuite struct {
	ModelTestSuite
}

func TestPostSuite(t *testing.T) {
	suite.Run(t, new(PostTestSuite))
}

const postTestGuild = snowflake.ID(1234567890)

func (s *PostTestSuite) TestCreatePostAssignsVersion1() {
	p, err := CreatePost(postTestGuild, "Hello", "[]", 0, 99)
	require.NoError(s.T(), err)
	assert.EqualValues(s.T(), 1, p.Version)
	assert.Equal(s.T(), "Hello", p.Name)
	assert.NotZero(s.T(), p.ID)
}

func (s *PostTestSuite) TestUpdatePostFieldsBumpsVersionOnSuccess() {
	p, _ := CreatePost(postTestGuild, "n1", "[]", 0, 99)
	updated, err := UpdatePostFields(postTestGuild, p.ID, p.Version, "n2", `[{"type":10,"content":"x"}]`, 0, 99)
	require.NoError(s.T(), err)
	assert.EqualValues(s.T(), 2, updated.Version)
	assert.Equal(s.T(), "n2", updated.Name)
}

func (s *PostTestSuite) TestUpdatePostFieldsReturnsStaleVersionOnConflict() {
	p, _ := CreatePost(postTestGuild, "n1", "[]", 0, 99)
	_, err := UpdatePostFields(postTestGuild, p.ID, p.Version, "n2", "[]", 0, 99)
	require.NoError(s.T(), err)
	_, err = UpdatePostFields(postTestGuild, p.ID, p.Version, "n3", "[]", 0, 99)
	assert.ErrorIs(s.T(), err, ErrPostStaleVersion)
}

func (s *PostTestSuite) TestUpdatePostFieldsReturnsNotFoundForMissingPost() {
	_, err := UpdatePostFields(postTestGuild, 99999, 1, "x", "[]", 0, 99)
	assert.ErrorIs(s.T(), err, gorm.ErrRecordNotFound)
}

func (s *PostTestSuite) TestUpdatePostFieldsReturnsNotFoundForWrongGuild() {
	p, _ := CreatePost(postTestGuild, "n1", "[]", 0, 99)
	const otherGuild = snowflake.ID(9999999999)
	_, err := UpdatePostFields(otherGuild, p.ID, p.Version, "n2", "[]", 0, 99)
	assert.ErrorIs(s.T(), err, gorm.ErrRecordNotFound)
}

func (s *PostTestSuite) TestReplacePostMessagesDensifiesPositions() {
	p, _ := CreatePost(postTestGuild, "n1", "[]", 0, 99)
	err := ReplacePostMessages(p.ID, []PostMessage{
		{ChannelID: 100, MessageID: 1001},
		{ChannelID: 100, MessageID: 1002},
		{ChannelID: 100, MessageID: 1003},
	})
	require.NoError(s.T(), err)
	rows, err := ListPostMessages(postTestGuild, p.ID)
	require.NoError(s.T(), err)
	require.Len(s.T(), rows, 3)
	assert.Equal(s.T(), 0, rows[0].Position)
	assert.Equal(s.T(), 1, rows[1].Position)
	assert.Equal(s.T(), 2, rows[2].Position)
}

func (s *PostTestSuite) TestReplacePostMessagesNilClearsAll() {
	p, _ := CreatePost(postTestGuild, "n1", "[]", 0, 99)
	_ = ReplacePostMessages(p.ID, []PostMessage{{ChannelID: 100, MessageID: 1}})
	require.NoError(s.T(), ReplacePostMessages(p.ID, nil))
	rows, _ := ListPostMessages(postTestGuild, p.ID)
	assert.Empty(s.T(), rows)
}

func (s *PostTestSuite) TestDeletePostDropsCascadingMessages() {
	p, _ := CreatePost(postTestGuild, "n1", "[]", 0, 99)
	_ = ReplacePostMessages(p.ID, []PostMessage{{ChannelID: 100, MessageID: 1}})
	require.NoError(s.T(), DeletePost(postTestGuild, p.ID))
	rows, _ := ListPostMessages(postTestGuild, p.ID)
	assert.Empty(s.T(), rows)
	_, err := GetPost(postTestGuild, p.ID)
	assert.ErrorIs(s.T(), err, gorm.ErrRecordNotFound)
}

func (s *PostTestSuite) TestListPostsWithCounts_RoundTrip() {
	// Unpublished post — no PostMessage rows.
	p1, _ := CreatePost(postTestGuild, "draft", "[]", 0, 99)

	// Published post — two PostMessage rows.
	p2, _ := CreatePost(postTestGuild, "published", "[]", 0, 99)
	_ = ReplacePostMessages(p2.ID, []PostMessage{
		{ChannelID: 100, MessageID: 2001},
		{ChannelID: 100, MessageID: 2002},
	})

	entries, err := ListPostsWithCounts(postTestGuild)
	require.NoError(s.T(), err)
	require.Len(s.T(), entries, 2)

	byID := make(map[uint]PostListEntry, len(entries))
	for _, e := range entries {
		byID[e.Post.ID] = e
	}

	assert.Equal(s.T(), 0, byID[p1.ID].MessageCount, "draft post should have zero messages")
	assert.Equal(s.T(), 2, byID[p2.ID].MessageCount, "published post should have two messages")
}
