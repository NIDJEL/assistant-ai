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

	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/joho/godotenv"
)

type Config struct {
	NLUMode                                                                    string
	SemanticProvider                                                           string
	GigaChat                                                                   llm.GigaChatConfig
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

func (c Config) EffectiveNLPProvider() string {
	if c.SemanticProvider != "" {
		return c.SemanticProvider
	}
	if c.NLUMode == "simple_llm" {
		return "gigachat"
	}
	return c.NLPProvider
}

func Load() (Config, error) {
	root, err := filepath.Abs(env("APP_ROOT", "."))
	if err != nil {
		return Config{}, err
	}
	fileEnv, err := godotenv.Read(filepath.Join(root, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return Config{}, errors.New(".env: invalid configuration format")
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
		NLUMode:          value("GENA_NLU_MODE", ""),
		SemanticProvider: value("GENA_SEMANTIC_PROVIDER", ""),
		GigaChat:         llm.GigaChatConfig{AuthorizationKey: value("GIGACHAT_AUTHORIZATION_KEY", ""), Scope: value("GIGACHAT_SCOPE", "GIGACHAT_API_PERS"), BaseURL: value("GIGACHAT_BASE_URL", "https://api.giga.chat"), Model: value("GIGACHAT_MODEL", "GigaChat-3-Ultra"), CAFile: value("GIGACHAT_CA_FILE", "")},
		WebPush:          notifications.Config{PublicKey: value("WEB_PUSH_VAPID_PUBLIC_KEY", ""), PrivateKey: value("WEB_PUSH_VAPID_PRIVATE_KEY", ""), Subject: value("WEB_PUSH_SUBJECT", "")},
		ImportUserEmail:  value("IMPORT_USER_EMAIL", "dev@rhythm.local"), ImportUserName: value("IMPORT_USER_NAME", "Разработчик"), ImportUserPassword: value("IMPORT_USER_PASSWORD", ""),
		DatabaseURL: value("DATABASE_URL", ""), SessionSecret: value("SESSION_SECRET", ""),
		PublicURL: value("PUBLIC_URL", "http://localhost:8081"), Environment: value("APP_ENV", "development"),
		YandexClientID: value("YANDEX_CLIENT_ID", ""), YandexClientSecret: value("YANDEX_CLIENT_SECRET", ""), YandexRedirectURI: value("YANDEX_REDIRECT_URI", ""),
		Address: value("APP_ADDR", "127.0.0.1:8081"), Root: root,
		DatabasePath: value("DATABASE_PATH", filepath.Join(root, "var/assistant-go.sqlite3")),
		Timezone:     value("APP_TIMEZONE", "Europe/Moscow"), NLPProvider: value("NLP_PROVIDER", "rules"),
		OllamaURL: value("OLLAMA_BASE_URL", "http://127.0.0.1:11434"), OllamaModel: value("OLLAMA_MODEL", "qwen2.5:7b"),
	}
	if c.NLUMode == "" {
		c.NLUMode = "simple_llm"
		if c.Environment == "production" {
			c.NLUMode = "semantic_plan_legacy"
		}
	}
	if c.NLUMode != "simple_llm" && c.NLUMode != "semantic_plan_legacy" {
		return c, errors.New("GENA_NLU_MODE must be simple_llm or semantic_plan_legacy")
	}
	if err := c.WebPush.Validate(); err != nil {
		return c, err
	}
	if c.SemanticProvider != "" && c.SemanticProvider != "rules" && c.SemanticProvider != "ollama" && c.SemanticProvider != "gigachat" {
		return c, errors.New("GENA_SEMANTIC_PROVIDER must be rules, ollama or gigachat")
	}
	if c.GigaChat.Timeout, err = time.ParseDuration(value("GIGACHAT_TIMEOUT", "30s")); err != nil || c.GigaChat.Timeout <= 0 || c.GigaChat.Timeout > 2*time.Minute {
		return c, errors.New("GIGACHAT_TIMEOUT must be a duration between 0 and 2m")
	}
	if c.GigaChat.MaxConcurrent, err = strconv.Atoi(value("GIGACHAT_MAX_CONCURRENT", "1")); err != nil || c.GigaChat.MaxConcurrent < 1 || c.GigaChat.MaxConcurrent > 16 {
		return c, errors.New("GIGACHAT_MAX_CONCURRENT must be between 1 and 16")
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
