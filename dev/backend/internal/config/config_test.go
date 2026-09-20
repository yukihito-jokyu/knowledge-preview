package config

import (
	"testing"
	"time"

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

func TestAuthConfigurationRequiresSecureProductionEndpoints(t *testing.T) {
	settings := Auth{
		ClientID:         "client",
		ClientSecret:     "secret",
		RedirectURL:      "https://app.test/api/v1/auth/github/callback",
		AuthorizeURL:     "https://github.com/login/oauth/authorize",
		TokenURL:         "https://github.com/login/oauth/access_token",
		UserURL:          "https://api.github.com/user",
		AllowedGitHubIDs: map[string]struct{}{"42": {}},
		SigningKey:       []byte("12345678901234567890123456789012"),
		Issuer:           "issuer",
		Audience:         "audience",
		AccessTTL:        time.Minute,
		RefreshTTL:       time.Hour,
		StateTTL:         time.Minute,
		AppOrigin:        "https://app.test",
	}
	require.NoError(t, settings.Validate("production"))
	settings.TokenURL = "https://oauth.test/token"
	require.Error(t, settings.Validate("production"))
	require.NoError(t, settings.Validate("test"))
}
