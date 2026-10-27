package workspace

import "testing"

func TestHistoryCapabilityBatchEmptyDoesNotQuery(t *testing.T) {
	service := &Service{}
	if err := service.populateHistoryCapabilities(t.Context(), nil, "actor"); err != nil {
		t.Fatal(err)
	}
	if err := service.populateHistoryCapabilities(t.Context(), []DocumentSummary{{ID: "document"}}, ""); err != nil {
		t.Fatal(err)
	}
}
