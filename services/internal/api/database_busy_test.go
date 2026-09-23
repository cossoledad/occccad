package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

func TestDatabaseBackpressureRemainsRetryableInfrastructureFailure(t *testing.T) {
	err := fmt.Errorf("read revision: %w", database.ErrBusy)
	for _, write := range []func(http.ResponseWriter){
		func(w http.ResponseWriter) { writeAccessError(w, err) },
		func(w http.ResponseWriter) { writeAuthError(w, err) },
		func(w http.ResponseWriter) { writeWorkspaceResult(w, workspace.DocumentView{}, err) },
	} {
		recorder := httptest.NewRecorder()
		write(recorder)
		if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
			t.Fatalf("overload misclassified: %d %s", recorder.Code, recorder.Body.String())
		}
	}
}
