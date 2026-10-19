package util

import (
	applog "LoanService/pkg/log"
	"context"
	"net/url"
	"strings"
)

// ValidURL reports whether raw is an HTTPS URL without credentials and with
// one of the allowed path suffixes. Query parameters do not affect the suffix.
func ValidURL(raw string, suffixes ...string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		if err != nil {
			applog.Error(context.Background(), "url.ValidURL failed", err)
		}

		return false
	}
	for _, suffix := range suffixes {
		if suffix != "" && strings.HasSuffix(strings.ToLower(parsed.Path), strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}
