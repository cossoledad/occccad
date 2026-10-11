package workspace

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/debugartifact"
)

func TestAssemblyInteractionInvalidSessionRetainsOriginalUpdate(t *testing.T) {
	service := &Service{}
	store, err := debugartifact.NewStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.SetDiagnosticArtifactStore(store)
	input := AssemblyInteractionUpdate{SessionID: "expired", Sequence: 12, GoalSequence: 10, Final: true}
	input.Target.TargetSequence = 12
	input.Target.TargetPose.Translation = [3]float64{7, 8, 9}
	_, err = service.UpdateAssemblyInteraction(t.Context(), "document", "actor", input)
	var retained interface{ DiagnosticID() string }
	if !errors.Is(err, ErrValidation) || !errors.As(err, &retained) {
		t.Fatal("session failure lost its diagnostic", err)
	}
	ids := strings.Split(retained.DiagnosticID(), "/")
	raw, err := service.ReadOperationDiagnostic(t.Context(), ids[0], ids[1])
	if err != nil {
		t.Fatal(err)
	}
	var record OperationDiagnostic
	if err = json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record.InteractionUpdate == nil || *record.InteractionUpdate != input || record.Mode != "ASSEMBLY" || len(record.AssemblyAttempts) != 0 {
		t.Fatal("incorrect frozen update", record)
	}
}
