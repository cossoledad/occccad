package api

import (
	"net/http/httptest"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestOpenDocumentRegistryPreservesOrderAndWorkspaceScope(t *testing.T) {
	registry := newOpenDocumentRegistry()
	part := workspace.DocumentSummary{ID: "part", Name: "Part", Type: "PART", VersionID: "p1"}
	product := workspace.DocumentSummary{ID: "product", Name: "Product", Type: "PRODUCT", VersionID: "a1"}

	registry.Open("user", "window-a", product)
	registry.Open("user", "window-a", part)
	part.VersionID = "p2"
	registry.Open("user", "window-a", part)
	registry.Open("user", "window-b", part)

	windowA := registry.List("user", "window-a")
	if len(windowA) != 2 || windowA[0].ID != "product" || windowA[1].ID != "part" || windowA[1].VersionID != "p2" {
		t.Fatalf("repeat open must update in place without reordering: %#v", windowA)
	}
	windowB := registry.List("user", "window-b")
	if len(windowB) != 1 || windowB[0].ID != "part" {
		t.Fatalf("browser workspaces must be isolated: %#v", windowB)
	}

	product.VersionID = "a2"
	registry.Update("user", product)
	if got := registry.List("user", "window-b"); len(got) != 1 {
		t.Fatalf("summary updates must not open a missing tab: %#v", got)
	}
	if got := registry.List("user", "window-a"); got[0].VersionID != "a2" || got[1].ID != "part" {
		t.Fatalf("summary update must retain the existing position: %#v", got)
	}

	registry.Close("user", "window-a", "part")
	if got := registry.List("user", "window-a"); len(got) != 1 || got[0].ID != "product" {
		t.Fatalf("close must affect only the requesting workspace: %#v", got)
	}
	if got := registry.List("user", "window-b"); len(got) != 1 || got[0].ID != "part" {
		t.Fatalf("closing another workspace changed this workspace: %#v", got)
	}

	registry.CloseDocument("user", "part")
	if got := registry.List("user", "window-b"); len(got) != 0 {
		t.Fatalf("deleting a document must remove it from all user workspaces: %#v", got)
	}
}

func TestOpenDocumentWorkspaceID(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/open-documents", nil)
	if got := openDocumentWorkspaceID(request); got != "default" {
		t.Fatalf("missing workspace id = %q", got)
	}
	request.Header.Set("X-OCCCCAD-Workspace-ID", "window-a")
	if got := openDocumentWorkspaceID(request); got != "window-a" {
		t.Fatalf("workspace id = %q", got)
	}
}
