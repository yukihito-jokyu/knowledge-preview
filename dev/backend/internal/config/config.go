package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultEnvironment = "development"
)

// Config は環境変数から読み込む設定。
type Config struct {
	Environment string
	HTTPAddr    string
	DatabaseURL string
	Auth        Auth
}

type Auth struct {
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	AuthorizeURL     string
	TokenURL         string
	UserURL          string
	AllowedGitHubIDs map[string]struct{}
	SigningKey       []byte
	Issuer           string
	Audience         string
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	StateTTL         time.Duration
	AppOrigin        string
}

func Load() (Config, error) {
	cfg := Config{
		Environment: valueOrDefault("APP_ENV", defaultEnvironment),
		HTTPAddr:    valueOrDefault("HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Auth:        loadAuth(),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func (a Auth) Validate(environment string) error {
	if a.ClientID == "" || a.ClientSecret == "" || a.RedirectURL == "" || len(a.SigningKey) < 32 || a.Issuer == "" ||
		a.Audience == "" ||
		a.AppOrigin == "" ||
		len(a.AllowedGitHubIDs) == 0 {
		return fmt.Errorf("authentication settings are incomplete")
	}

	redirect, err := url.Parse(a.RedirectURL)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil ||
		redirect.Fragment != "" ||
		redirect.RawQuery != "" {
		return fmt.Errorf("GITHUB_REDIRECT_URL must be an HTTPS URL without query or fragment")
	}

	app, err := exactOrigin(a.AppOrigin)
	if err != nil || redirect.Host != app.Host {
		return fmt.Errorf("GITHUB_REDIRECT_URL must use APP_ORIGIN")
	}

	if environment != "test" && environment != "local" {
		if a.AuthorizeURL != "https://github.com/login/oauth/authorize" ||
			a.TokenURL != "https://github.com/login/oauth/access_token" ||
			a.UserURL != "https://api.github.com/user" {
			return fmt.Errorf("production OAuth endpoints must use GitHub")
		}
	}

	return nil
}

func loadAuth() Auth {
	allowed := make(map[string]struct{})

	for _, id := range strings.Split(os.Getenv("AUTH_ALLOWED_GITHUB_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = struct{}{}
		}
	}

	appOrigin := os.Getenv("APP_ORIGIN")

	redirectURL := os.Getenv("GITHUB_REDIRECT_URL")
	if redirectURL == "" && appOrigin != "" {
		redirectURL = strings.TrimRight(appOrigin, "/") + "/api/v1/auth/github/callback"
	}

	return Auth{
		ClientID:         os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret:     os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:      redirectURL,
		AuthorizeURL:     valueOrDefault("GITHUB_AUTHORIZE_URL", "https://github.com/login/oauth/authorize"),
		TokenURL:         valueOrDefault("GITHUB_TOKEN_URL", "https://github.com/login/oauth/access_token"),
		UserURL:          valueOrDefault("GITHUB_USER_URL", "https://api.github.com/user"),
		AllowedGitHubIDs: allowed,
		SigningKey:       []byte(os.Getenv("JWT_SIGNING_KEY")),
		Issuer:           os.Getenv("JWT_ISSUER"),
		Audience:         os.Getenv("JWT_AUDIENCE"),
		AccessTTL:        durationOrDefault("AUTH_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:       durationOrDefault("AUTH_REFRESH_TTL", 30*24*time.Hour),
		StateTTL:         durationOrDefault("AUTH_STATE_TTL", 10*time.Minute),
		AppOrigin:        appOrigin,
	}
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
