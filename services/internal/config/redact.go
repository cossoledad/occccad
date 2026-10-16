package config

import (
	"net/url"
	"os"
	"strings"
)

// Redact removes configured credentials from operational error text. Domain
// snapshots never include the environment itself.
func Redact(text string) string {
	dsn := Load().DatabaseURL
	text = strings.ReplaceAll(text, dsn, "<configured database>")
	secrets := []string{}
	for _, key := range []string{"OCCCCAD_POSTGRES_PASSWORD", "OCCCCAD_S3_SECRET_KEY", "OCCCCAD_S3_ACCESS_KEY", "OCCCCAD_ADMIN_PASSWORD"} {
		secrets = append(secrets, os.Getenv(key))
	}
	if parsed, err := url.Parse(dsn); err == nil && parsed.User != nil {
		password, _ := parsed.User.Password()
		secrets = append(secrets, password)
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		for _, form := range []string{url.QueryEscape(secret), url.PathEscape(secret), secret} {
			text = strings.ReplaceAll(text, form, "<redacted>")
		}
	}
	return text
}
