package config

import (
	applog "LoanService/pkg/log"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"os"
	"strings"

	"LoanService/internal/model"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DatabaseURL string
	Auth        map[string]model.Identity
	HTTPAddr    string
	SMTP        SMTPConfig
	Trace       TraceConfig
	LogLevel    string
}

type SMTPConfig struct {
	Address, From, Username, Password string
	AllowPlaintext                    bool
}

type TraceConfig struct {
	Endpoint string
	Name     string
}

// NewConfig loads YAML configuration and environment overrides. APP_CONF overrides path.
// An empty path allows an environment-only deployment.
func NewConfig(path string) (*Config, error) {
	if envPath := os.Getenv("APP_CONF"); envPath != "" {
		path = envPath
	}
	conf := viper.New()
	conf.SetDefault("http.addr", ":8080")
	conf.SetDefault("trace.name", "amartha-loan-service")
	conf.SetDefault("log.level", "info")
	bindings := map[string]string{
		"database.url": "DATABASE_URL", "auth.tokens": "AUTH_TOKENS", "http.addr": "HTTP_ADDR",
		"smtp.address": "SMTP_ADDR", "smtp.from": "SMTP_FROM", "smtp.username": "SMTP_USERNAME",
		"smtp.password": "SMTP_PASSWORD", "smtp.allow_plaintext": "SMTP_ALLOW_PLAINTEXT",
		"trace.endpoint": "OTEL_EXPORTER_OTLP_ENDPOINT", "trace.name": "OTEL_SERVICE_NAME", "log.level": "LOG_LEVEL",
	}
	for key, env := range bindings {
		if err := conf.BindEnv(key, env); err != nil {
			applog.Error(context.Background(), "config.NewConfig failed", err)
			return nil, fmt.Errorf("bind configuration %s: %w", key, err)
		}
	}
	if path != "" {
		conf.SetConfigFile(path)
		if err := conf.ReadInConfig(); err != nil {
			applog.Error(context.Background(), "config.NewConfig failed", err)
			return nil, fmt.Errorf("read configuration: %w", err)
		}
	}
	cfg := &Config{
		DatabaseURL: conf.GetString("database.url"), HTTPAddr: conf.GetString("http.addr"), LogLevel: conf.GetString("log.level"),
		SMTP:  SMTPConfig{Address: conf.GetString("smtp.address"), From: conf.GetString("smtp.from"), Username: conf.GetString("smtp.username"), Password: conf.GetString("smtp.password"), AllowPlaintext: conf.GetBool("smtp.allow_plaintext")},
		Trace: TraceConfig{Endpoint: conf.GetString("trace.endpoint"), Name: conf.GetString("trace.name")},
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database.url or DATABASE_URL required")
	}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		applog.Error(context.Background(), "config.NewConfig failed", err)
		return nil, fmt.Errorf("invalid HTTP address")
	}
	// Read token keys directly: Viper normalizes map keys, but bearer tokens are case-sensitive.
	if raw := os.Getenv("AUTH_TOKENS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.Auth); err != nil {
			applog.Error(context.Background(), "config.NewConfig failed", err)
			return nil, fmt.Errorf("AUTH_TOKENS must be a JSON token to identity map")
		}
	} else if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			applog.Error(context.Background(), "config.NewConfig failed", err)
			return nil, fmt.Errorf("read authentication configuration: %w", err)
		}
		var raw struct {
			Auth struct {
				Tokens map[string]model.Identity `yaml:"tokens"`
			} `yaml:"auth"`
		}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			applog.Error(context.Background(), "config.NewConfig failed", err)
			return nil, fmt.Errorf("decode authentication configuration: %w", err)
		}
		cfg.Auth = raw.Auth.Tokens
	}
	return cfg, nil
}

func (c *Config) ValidateAuth() error {
	if len(c.Auth) == 0 {
		return fmt.Errorf("auth.tokens or AUTH_TOKENS must be non-empty")
	}
	for token, identity := range c.Auth {
		if len(token) < 24 || (identity.Role != "staff" && identity.Role != "investor") {
			return fmt.Errorf("invalid auth identity or token shorter than 24 characters")
		}
		if identity.Role == "investor" {
			address, err := mail.ParseAddress(identity.Email)
			if err != nil || address.Address != identity.Email || strings.ContainsAny(identity.Email, "\r\n") {
				if err != nil {
					applog.Error(context.Background(), "config.ValidateAuth failed", err)
				}

				return fmt.Errorf("investor email required")
			}
		}
	}
	return nil
}

func (c *Config) ValidateSMTP() error {
	if _, _, err := net.SplitHostPort(c.SMTP.Address); err != nil {
		applog.Error(context.Background(), "config.ValidateSMTP failed", err)
		return fmt.Errorf("valid SMTP_ADDR required")
	}
	address, err := mail.ParseAddress(c.SMTP.From)
	if err != nil || address.Address != c.SMTP.From {
		if err != nil {
			applog.Error(context.Background(), "config.ValidateSMTP failed", err)
		}

		return fmt.Errorf("valid SMTP_FROM required")
	}
	return nil
}
