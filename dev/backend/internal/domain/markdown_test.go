package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownMetadataAndBoundaries(t *testing.T) {
	original := Knowledge{Format: "markdown", Title: "old", Tags: []string{"keep"}, LearningStatus: "unlearned"}
	updated, body, err := ApplySource(
		original,
		"---\ntitle: 日本語の題名\ntags: [ Docker , Docker, '']\nlearningStatus: learning\n---\n# body",
	)
	require.NoError(t, err)
	require.Equal(t, "日本語の題名", updated.Title)
	require.Equal(t, []string{"Docker"}, updated.Tags)
	require.Equal(t, "learning", updated.LearningStatus)
	require.Equal(t, "# body", body)

	preserved, _, err := ApplySource(original, "new body")
	require.NoError(t, err)
	require.Equal(t, original, preserved)

	for _, source := range []string{" ", "---\ntags: no\n---\nbody", "---\nownerId: other\n---\nbody", "---\ntitle: a\ntitle: b\n---\nbody", "---\ntitle: &a hello\n---\nbody", "---\ntitle: ok\n---\n ", "---\ntags: [1]\n---\nbody", "---\nlearningStatus: invalid\n---\nbody", "---\ntitle: [\n---\nbody", string([]byte{0xff})} {
		_, _, err := ApplySource(original, source)
		require.ErrorIs(t, err, ErrValidation, source)

		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
		require.Positive(t, validation.Detail.Line)
		require.Positive(t, validation.Detail.Column)
	}

	for _, size := range []int{MaxSourceBytes - 1, MaxSourceBytes} {
		_, _, err := ApplySource(original, strings.Repeat("x", size))
		require.NoError(t, err)
	}

	_, _, err = ApplySource(original, strings.Repeat("x", MaxSourceBytes+1))
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestMarkdownConsumesEntireFrontMatter(t *testing.T) {
	original := Knowledge{Format: "markdown", Title: "old"}
	for _, front := range []string{"title: first\n...\nowner: other", "title: first\n...\ngarbage: [", "title: first\n...\n--- # second document\ntitle: second"} {
		_, _, err := ApplySource(original, "---\n"+front+"\n---\nbody")
		require.ErrorIs(t, err, ErrValidation, front)

		var detail *ValidationError
		require.ErrorAs(t, err, &detail)
		require.GreaterOrEqual(t, detail.Detail.Line, 3)
		require.Positive(t, detail.Detail.Column)
	}

	for _, front := range []string{"title: first", "title: first\n...", "title: first\n...\n# comment\n"} {
		k, body, err := ApplySource(original, "---\n"+front+"\n---\nbody")
		require.NoError(t, err, front)
		require.Equal(t, "first", k.Title)
		require.Equal(t, "body", body)
	}
}
