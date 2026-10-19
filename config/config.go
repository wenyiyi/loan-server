package config

import (
	applog "LoanService/pkg/log"
	"context"
	"fmt"
	"net"
	"net/mail"
	"os"
	"strings"

	"LoanService/internal/model"

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
	Address        string `yaml:"address"`
	From           string `yaml:"from"`
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	AllowPlaintext bool   `yaml:"allow_plaintext"`
}

type TraceConfig struct {
	Endpoint string `yaml:"endpoint"`
	Name     string `yaml:"name"`
}

const DefaultPath = "config/local.yml"

// NewConfig reads one YAML file. Environment variables never override its values.
func NewConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		applog.Error(context.Background(), "read configuration failed", err)
		return nil, fmt.Errorf("read configuration %s: %w", path, err)
	}
	var raw struct {
		Database struct {
			URL string `yaml:"url"`
		} `yaml:"database"`
		HTTP struct {
			Addr string `yaml:"addr"`
		} `yaml:"http"`
		Auth struct {
			Tokens map[string]model.Identity `yaml:"tokens"`
		} `yaml:"auth"`
		SMTP  SMTPConfig  `yaml:"smtp"`
		Trace TraceConfig `yaml:"trace"`
		Log   struct {
			Level string `yaml:"level"`
		} `yaml:"log"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		applog.Error(context.Background(), "decode configuration failed", err)
		return nil, fmt.Errorf("decode configuration %s: %w", path, err)
	}
	cfg := &Config{
		DatabaseURL: raw.Database.URL, HTTPAddr: raw.HTTP.Addr, Auth: raw.Auth.Tokens,
		SMTP: raw.SMTP, Trace: raw.Trace, LogLevel: raw.Log.Level,
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.Trace.Name == "" {
		cfg.Trace.Name = "amartha-loan-service"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database.url required in %s", path)
	}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		applog.Error(context.Background(), "parse HTTP address failed", err)
		return nil, fmt.Errorf("invalid http.addr in %s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) ValidateAuth() error {
	if len(c.Auth) == 0 {
		return fmt.Errorf("auth.tokens must be non-empty")
	}
	for token, identity := range c.Auth {
		if strings.TrimSpace(identity.ID) == "" {
			return fmt.Errorf("auth identity id must be non-empty")
		}
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
		return fmt.Errorf("valid smtp.address required")
	}
	address, err := mail.ParseAddress(c.SMTP.From)
	if err != nil || address.Address != c.SMTP.From {
		if err != nil {
			applog.Error(context.Background(), "config.ValidateSMTP failed", err)
		}

		return fmt.Errorf("valid smtp.from required")
	}
	return nil
}
