package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKnowledgeConfigurationIsolation(t *testing.T) {
	for _, key := range []string{"APP_ORIGIN", "PREVIEW_ORIGIN", "S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_INSECURE"} {
		t.Setenv(key, "")
	}

	cfg, err := LoadKnowledge()
	require.NoError(t, err)
	require.Empty(t, cfg.Endpoint)
	t.Setenv("APP_ORIGIN", "https://app.test")

	_, err = LoadKnowledge()
	require.Error(t, err)
	t.Setenv("PREVIEW_ORIGIN", "https://preview.test")
	t.Setenv("S3_ENDPOINT", "localhost:9000")
	t.Setenv("S3_BUCKET", "private")
	t.Setenv("S3_ACCESS_KEY", "test-only")
	t.Setenv("S3_SECRET_KEY", "test-only")

	cfg, err = LoadKnowledge()
	require.NoError(t, err)
	require.True(t, cfg.Secure)

	for _, origin := range []string{"https://app.test:8443", "https://preview.test/path", "https://user@preview.test", "http://preview.test", "https://preview.test?query=x"} {
		t.Setenv("PREVIEW_ORIGIN", origin)

		_, err = LoadKnowledge()
		require.Error(t, err, origin)
	}
}
