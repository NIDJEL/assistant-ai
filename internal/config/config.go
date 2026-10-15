package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/joho/godotenv"
)

type Config struct {
	WebPush                                                                    notifications.Config
	ImportUserEmail, ImportUserName, ImportUserPassword                        string
	AuthRateLimit                                                              int
	TrustedProxyCIDRs                                                          []*net.IPNet
	DatabaseURL, SessionSecret, PublicURL, Environment                         string
	YandexClientID, YandexClientSecret, YandexRedirectURI                      string
	Address, Root, DatabasePath, Timezone, NLPProvider, OllamaURL, OllamaModel string
	AutoImport                                                                 bool
	OllamaTimeout                                                              time.Duration
	Location                                                                   *time.Location
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func Load() (Config, error) {
	root, err := filepath.Abs(env("APP_ROOT", "."))
	if err != nil {
		return Config{}, err
	}
	fileEnv, err := godotenv.Read(filepath.Join(root, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf(".env: %w", err)
	}
	value := func(name, fallback string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		if v, ok := fileEnv[name]; ok {
			return v
		}
		return fallback
	}
	c := Config{
		WebPush:         notifications.Config{PublicKey: value("WEB_PUSH_VAPID_PUBLIC_KEY", ""), PrivateKey: value("WEB_PUSH_VAPID_PRIVATE_KEY", ""), Subject: value("WEB_PUSH_SUBJECT", "")},
		ImportUserEmail: value("IMPORT_USER_EMAIL", "dev@rhythm.local"), ImportUserName: value("IMPORT_USER_NAME", "Разработчик"), ImportUserPassword: value("IMPORT_USER_PASSWORD", ""),
		DatabaseURL: value("DATABASE_URL", ""), SessionSecret: value("SESSION_SECRET", ""),
		PublicURL: value("PUBLIC_URL", "http://localhost:8081"), Environment: value("APP_ENV", "development"),
		YandexClientID: value("YANDEX_CLIENT_ID", ""), YandexClientSecret: value("YANDEX_CLIENT_SECRET", ""), YandexRedirectURI: value("YANDEX_REDIRECT_URI", ""),
		Address: value("APP_ADDR", "127.0.0.1:8081"), Root: root,
		DatabasePath: value("DATABASE_PATH", filepath.Join(root, "var/assistant-go.sqlite3")),
		Timezone:     value("APP_TIMEZONE", "Europe/Moscow"), NLPProvider: value("NLP_PROVIDER", "rules"),
		OllamaURL: value("OLLAMA_BASE_URL", "http://127.0.0.1:11434"), OllamaModel: value("OLLAMA_MODEL", "qwen2.5:7b"),
	}
	if err := c.WebPush.Validate(); err != nil {
		return c, err
	}
	if c.AuthRateLimit, err = strconv.Atoi(value("AUTH_RATE_LIMIT", "20")); err != nil || c.AuthRateLimit < 1 || c.AuthRateLimit > 300 {
		return c, errors.New("AUTH_RATE_LIMIT must be between 1 and 300")
	}
	for _, cidr := range strings.Split(value("TRUSTED_PROXY_CIDRS", ""), ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return c, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", err)
		}
		c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs, network)
	}
	public, err := url.Parse(c.PublicURL)
	if err != nil || public.Host == "" || public.User != nil || public.RawQuery != "" || public.Fragment != "" || (public.Path != "" && public.Path != "/") || (public.Scheme != "http" && public.Scheme != "https") {
		return c, errors.New("PUBLIC_URL must be an HTTP(S) origin")
	}
	if c.Environment == "production" && public.Scheme != "https" {
		return c, errors.New("production requires HTTPS PUBLIC_URL")
	}
	for provider, redirect := range map[string]string{"yandex": c.YandexRedirectURI} {
		if redirect == "" {
			continue
		}
		u, err := url.Parse(redirect)
		validPath := err == nil && (u.Path == "/api/auth/"+provider+"/callback" || (provider == "yandex" && u.Path == "//api/auth/yandex/callback"))
		if !validPath || u.Scheme != public.Scheme || u.Host != public.Host || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return c, fmt.Errorf("%s redirect URI must match PUBLIC_URL and callback path", provider)
		}
	}
	if c.Location, err = time.LoadLocation(c.Timezone); err != nil {
		return c, fmt.Errorf("APP_TIMEZONE: %w", err)
	}
	if c.AutoImport, err = strconv.ParseBool(value("AUTO_IMPORT_DEMO", "true")); err != nil {
		return c, fmt.Errorf("AUTO_IMPORT_DEMO: %w", err)
	}
	seconds, err := strconv.Atoi(value("OLLAMA_TIMEOUT", "20"))
	if err != nil || seconds < 1 || seconds > 120 {
		return c, errors.New("OLLAMA_TIMEOUT must be between 1 and 120 seconds")
	}
	c.OllamaTimeout = time.Duration(seconds) * time.Second
	if c.NLPProvider != "rules" && c.NLPProvider != "ollama" && c.NLPProvider != "hybrid" {
		return c, errors.New("NLP_PROVIDER must be rules, ollama or hybrid")
	}
	u, err := url.Parse(c.OllamaURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return c, errors.New("OLLAMA_BASE_URL must be an HTTP(S) URL")
	}
	return c, nil
}
