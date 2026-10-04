package authn

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/database"
)

func TestLocalModeLoginExpiryAndAtomicSession(t *testing.T) {
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "auth.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	s := New(db, time.Hour)
	ctx := t.Context()
	if e = s.BootstrapAdmin(ctx, "local@example.test", "Local", "local-password"); e != nil {
		t.Fatal(e)
	}
	session, e := s.Login(ctx, "local@example.test", "local-password", "test", "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	user, e := s.Authenticate(ctx, session.Token)
	if e != nil || user.ID != session.User.ID {
		t.Fatal(e)
	}
	if e = s.Logout(ctx, session.Token); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, session.Token); !errors.Is(e, ErrUnauthorized) {
		t.Fatalf("revoked session: %v", e)
	}
	// PostgreSQL's previous writable CTE is now an explicit transaction. Inject a
	// failure into its second write and ensure the session insert rolls back.
	if _, e = db.Exec(ctx, `CREATE TRIGGER reject_login BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT,'injected login failure'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Login(ctx, "local@example.test", "local-password", "test", "127.0.0.1"); e == nil {
		t.Fatal("expected failure")
	}
	var n int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM occccad.user_sessions`).Scan(&n); e != nil || n != 1 {
		t.Fatalf("partial session: %d %v", n, e)
	}
}
