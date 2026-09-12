package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()

	require.Error(t, err)
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_ADDR", "")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, defaultEnvironment, cfg.Environment)
	assert.Equal(t, defaultHTTPAddr, cfg.HTTPAddr)
}
