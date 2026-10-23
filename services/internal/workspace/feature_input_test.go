package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestFeatureInputResolutionPreservesFailuresAndPlaneType(t *testing.T) {
	for _, status := range []modelcore.SelectionResolutionStatus{modelcore.SelectionMissing, modelcore.SelectionAmbiguous, modelcore.SelectionTypeMismatch, modelcore.SelectionOutsideCurrentTip, modelcore.SelectionSourceUnavailable, modelcore.SelectionContractMismatch} {
		result, err := featureInputResolution(modelcore.SelectionResolution{Status: status, DiagnosticCode: "retained-code", Diagnostic: "retained evidence"}, "PLANE")
		if err != nil || result.Status != status || result.DiagnosticCode != "retained-code" || result.Diagnostic != "retained evidence" {
			t.Fatalf("lost failure evidence: %+v %v", result, err)
		}
	}
	// A topological FACE can resolve uniquely while no longer being planar.
	result, err := featureInputResolution(modelcore.SelectionResolution{Status: modelcore.SelectionResolved, Candidates: []modelcore.ResolvedTopologyElement{{Type: modelcore.PersistentTopologyFace, Evidence: modelcore.TopologySelectionEvidence{GeometryType: "CYLINDER"}}}}, "PLANE")
	if err != nil || result.Status != modelcore.SelectionTypeMismatch {
		t.Fatal("accepted a non-planar neutral face", result, err)
	}
	if _, err = featureInputResolution(modelcore.SelectionResolution{Status: modelcore.SelectionResolved}, ""); err == nil {
		t.Fatal("malformed unique resolver result accepted")
	}
}

func TestFeatureInputUpstreamErrorKeepsCauseAndActualNode(t *testing.T) {
	model := PartModel{Features: []Feature{{ID: "upstream", Name: "Broken chamfer"}, {ID: "tip", Name: "Requested tip"}}}
	cause := errors.New("worker transport unavailable")
	failure := featureInputStageError(model, "tip", fmt.Errorf("FEATURE_FAILED[upstream]: %w", cause))
	if !errors.Is(failure, cause) || !strings.Contains(failure.Error(), "Broken chamfer") || !strings.Contains(failure.Error(), "[upstream]") {
		t.Fatal("lost upstream cause or location", failure)
	}
	if !errors.Is(featureInputStageError(model, "tip", context.Canceled), context.Canceled) {
		t.Fatal("cancellation was converted to reference failure")
	}
}
