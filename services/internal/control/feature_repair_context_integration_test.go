//go:build integration

package control

import (
	"math"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestHistoricalComparisonKeepsAcceptedOccurrenceAndCurrentHeads(t *testing.T) {
	c := newContextInsertionScene(t, 35)
	scope := c.scope()
	partHead, rootHead := c.v.Document.VersionID, c.root.Document.VersionID
	r := workspace.FeatureRepairRequest{PartDependencyRequest: workspace.PartDependencyRequest{ContextReadScope: scope, VersionID: partHead}, FeatureID: c.base.ID, ActorID: p6Actor}
	comparison, err := c.s.GetFeatureRepairContext(t.Context(), c.v.Document.ID, r)
	if err != nil || comparison.Snapshot == nil || len(comparison.Snapshot.Artifacts) != 1 {
		t.Fatal("accepted occurrence history", err, comparison)
	}
	snapshot := comparison.Snapshot
	if snapshot.RootProductRevisionID == "" || snapshot.RootProductRevisionID == rootHead || snapshot.ContextVariantID == "" || math.Abs(snapshot.Artifacts[0].Volume-8000) > 1e-5 {
		t.Fatal("history used defaults, another occurrence or unaccepted source Head", snapshot)
	}
	c.s.DiscardPreview(c.v.Document.ID, p6Actor, comparison.DisplayID)
	r.CompareVersionID = snapshot.RootProductRevisionID
	explicit, err := c.s.GetFeatureRepairContext(t.Context(), c.v.Document.ID, r)
	if err != nil || explicit.Snapshot == nil || explicit.Snapshot.Artifacts[0].Representations["VISUAL"].Digest != snapshot.Artifacts[0].Representations["VISUAL"].Digest {
		t.Fatal("explicit frozen comparison differs", err)
	}
	c.s.DiscardPreview(c.v.Document.ID, p6Actor, explicit.DisplayID)
	r.CompareVersionID = c.source.Document.VersionID
	if _, err = c.s.GetFeatureRepairContext(t.Context(), c.v.Document.ID, r); err == nil {
		t.Fatal("foreign revision accepted")
	}
	r.CompareVersionID = ""
	r.MaxRevisions = 257
	if _, err = c.s.GetFeatureRepairContext(t.Context(), c.v.Document.ID, r); err == nil {
		t.Fatal("unbounded history search accepted")
	}
	r.MaxRevisions = 64
	r.RootProductRevisionID = c.source.Document.VersionID
	if _, err = c.s.GetFeatureRepairContext(t.Context(), c.v.Document.ID, r); err == nil {
		t.Fatal("stale root comparison accepted")
	}
	c.reload()
	if c.v.Document.VersionID != partHead || c.root.Document.VersionID != rootHead {
		t.Fatal("comparison changed a Head")
	}
}
