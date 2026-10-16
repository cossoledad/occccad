package config

import (
	"strings"
	"testing"
)

func TestOperationalErrorsRedactConfiguredCredentials(t *testing.T) {
	t.Setenv("OCCCCAD_DATABASE_URL", "postgres://actor:private%40password@localhost/occccad")
	t.Setenv("OCCCCAD_S3_SECRET_KEY", "storage-private-key")
	t.Setenv("OCCCCAD_ADMIN_PASSWORD", "admin-private-key")
	message := Redact("postgres://actor:private%40password@localhost/occccad private@password storage-private-key admin-private-key")
	for _, secret := range []string{"private%40password", "private@password", "storage-private-key", "admin-private-key"} {
		if strings.Contains(message, secret) {
			t.Fatal("operational diagnostic leaked a credential")
		}
	}
}
