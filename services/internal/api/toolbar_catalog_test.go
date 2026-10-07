package api

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/workbenchconfig"
	"net/http/httptest"
	"testing"
)

func TestToolbarCatalogNeedsNoDatabaseOrGeometry(t *testing.T) {
	response := httptest.NewRecorder()
	(&Server{}).toolbarCatalog(response, httptest.NewRequest("GET", "/api/ui/toolbar-catalog", nil))
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var result struct {
		workbenchconfig.Catalog
		Toolbars []workbenchconfig.Toolbar `json:"toolbars"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(result.Toolbars) != len(result.Placements) {
		t.Fatal("partial projection")
	}
}
