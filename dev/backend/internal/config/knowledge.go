package config

import (
	"fmt"
	"net/url"
	"os"
)

// Knowledgeの保存機能は、すべての設定が揃うまで無効にする。
// 認証はcmd/apiで接続し、この設定で仮の認証情報を有効にすることはない。
type Knowledge struct {
	AppOrigin     string
	PreviewOrigin string
	Endpoint      string
	Bucket        string
	AccessKey     string
	SecretKey     string
	Secure        bool
}

func LoadKnowledge() (Knowledge, error) {
	cfg := Knowledge{
		AppOrigin:     os.Getenv("APP_ORIGIN"),
		PreviewOrigin: os.Getenv("PREVIEW_ORIGIN"),
		Endpoint:      os.Getenv("S3_ENDPOINT"),
		Bucket:        os.Getenv("S3_BUCKET"),
		AccessKey:     os.Getenv("S3_ACCESS_KEY"),
		SecretKey:     os.Getenv("S3_SECRET_KEY"),
		Secure:        os.Getenv("S3_INSECURE") != "true",
	}
	if cfg.AppOrigin == "" && cfg.PreviewOrigin == "" && cfg.Endpoint == "" && cfg.Bucket == "" &&
		cfg.AccessKey == "" &&
		cfg.SecretKey == "" {
		return cfg, nil
	}

	if cfg.AppOrigin == "" || cfg.PreviewOrigin == "" || cfg.Endpoint == "" || cfg.Bucket == "" ||
		cfg.AccessKey == "" ||
		cfg.SecretKey == "" {
		return cfg, fmt.Errorf("knowledge requires APP_ORIGIN, PREVIEW_ORIGIN and S3 settings")
	}

	app, err := exactOrigin(cfg.AppOrigin)
	if err != nil {
		return cfg, err
	}

	preview, err := exactOrigin(cfg.PreviewOrigin)
	if err != nil {
		return cfg, err
	}

	if app.Hostname() == preview.Hostname() {
		return cfg, fmt.Errorf("preview requires a separate hostname to isolate host-only cookies")
	}

	return cfg, nil
}

func exactOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" ||
		u.Fragment != "" {
		return nil, fmt.Errorf("origins must be exact HTTPS origins without a path")
	}

	return u, nil
}
