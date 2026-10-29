package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

const minAdminTokenLen = 16

type Config struct {
	Addr                string
	ResendAPIKey        string
	ResendWebhookSecret string
	AdminToken          string
	Domain              string
	DBPath              string
}

// Load 先加载项目根目录的 .env（已存在的环境变量优先），再读取配置。
func Load() (*Config, error) {
	loadDotEnv(".env")

	cfg := &Config{
		Addr:                getenv("XEMAIL_ADDR", ":1980"),
		ResendAPIKey:        firstEnv("RESEND_API_KEY", "resend_key"),
		ResendWebhookSecret: os.Getenv("RESEND_WEBHOOK_SECRET"),
		AdminToken:          os.Getenv("ADMIN_TOKEN"),
		Domain:              strings.ToLower(strings.TrimPrefix(firstEnv("XEMAIL_DOMAIN", "domain"), "@")),
		DBPath:              getenv("XEMAIL_DB_PATH", "data/xemail.db"),
	}
	if cfg.ResendAPIKey == "" {
		return nil, errors.New("Resend API key 未设置：在 .env 中配置 RESEND_API_KEY（或 resend_key）")
	}
	if len(cfg.AdminToken) < minAdminTokenLen {
		return nil, errors.New("ADMIN_TOKEN 未设置或少于 16 位：在 .env 中添加，例如 ADMIN_TOKEN=$(openssl rand -hex 24)")
	}
	return cfg, nil
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
