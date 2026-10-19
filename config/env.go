package config

import (
	applog "LoanService/pkg/log"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/subosito/gotenv"
)

// LoadLocalEnv loads the nearest .env from the working directory up to the
// project root. Explicit process environment variables always take precedence.
// A missing .env is allowed for container deployments.
func LoadLocalEnv() error {
	dir, err := os.Getwd()
	if err != nil {
		applog.Error(context.Background(), "env.LoadLocalEnv failed", err)
		return fmt.Errorf("find local configuration: %w", err)
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			if err := gotenv.Load(path); err != nil {
				applog.Error(context.Background(), "env.LoadLocalEnv failed", err)
				return fmt.Errorf("load local configuration: %w", err)
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read local configuration: %w", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}
