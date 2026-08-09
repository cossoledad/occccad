package access

import "testing"

func TestRoleLevels(t *testing.T) {
	if RoleViewer.Level() >= RoleEditor.Level() || RoleEditor.Level() >= RoleOwner.Level() {
		t.Fatal("roles must remain ordered VIEWER < EDITOR < OWNER")
	}
	if Role("UNKNOWN").Level() != 0 {
		t.Fatal("unknown roles must not grant access")
	}
}
