package access

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
)

func TestLocalModeInheritedAndTeamPermissions(t *testing.T) {
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "access.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	ctx := t.Context()
	member, outsider, root, child, doc, team := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{member, outsider} {
		if _, e = db.Exec(ctx, `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Member')`, id, id+"@test.invalid"); e != nil {
			t.Fatal(e)
		}
	}
	statements := []struct {
		q string
		a []any
	}{
		{`INSERT INTO occccad.folders(id,name,owner_user_id) VALUES($1,'root',$2)`, []any{root, DefaultUserID}},
		{`INSERT INTO occccad.folders(id,name,parent_id,owner_user_id) VALUES($1,'child',$2,$3)`, []any{child, root, DefaultUserID}},
		{`INSERT INTO occccad.documents(id,name,document_type,folder_id,owner_user_id) VALUES($1,'document','PRODUCT',$2,$3)`, []any{doc, child, DefaultUserID}},
		{`INSERT INTO occccad.teams(id,name,owner_user_id) VALUES($1,'team',$2)`, []any{team, DefaultUserID}},
		{`INSERT INTO occccad.team_members(team_id,user_id,role) VALUES($1,$2,'MEMBER')`, []any{team, member}},
		{`INSERT INTO occccad.resource_grants(resource_type,resource_id,team_id,role,granted_by_user_id) VALUES('FOLDER',$1,$2,'EDITOR',$3)`, []any{root, team, DefaultUserID}},
	}
	for _, s := range statements {
		if _, e = db.Exec(ctx, s.q, s.a...); e != nil {
			t.Fatal(e)
		}
	}
	svc := New(db)
	for _, item := range []struct {
		id   string
		role Role
	}{{DefaultUserID, RoleOwner}, {member, RoleEditor}, {outsider, "NONE"}} {
		got, e := svc.EffectiveDocumentRole(ctx, doc, item.id)
		if e != nil || got != item.role {
			t.Fatalf("document permission %q want %q: %v", got, item.role, e)
		}
		got, e = svc.EffectiveFolderRole(ctx, child, item.id)
		if e != nil || got != item.role {
			t.Fatalf("folder permission %q want %q: %v", got, item.role, e)
		}
	}
	// Revocation must be visible immediately, including through an ancestor.
	if _, e = db.Exec(ctx, `DELETE FROM occccad.team_members WHERE team_id=$1 AND user_id=$2`, team, member); e != nil {
		t.Fatal(e)
	}
	got, e := svc.EffectiveDocumentRole(ctx, doc, member)
	if e != nil || got != "NONE" {
		t.Fatalf("stale permission %q %v", got, e)
	}
}
