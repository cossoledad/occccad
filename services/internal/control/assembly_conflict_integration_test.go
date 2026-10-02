package control

import (
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"testing"
)

// Real Router/Worker/PostgreSQL evidence; the fixture refuses application DBs.
// No numerical result or admission mock is used for deletion SAT witnesses.
func TestAssemblyConflictReadOnlyRepairThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	plane := f.support(t, part, "PLANE")
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "M5 actual conflict and repair"})
	if err != nil {
		t.Fatal(err)
	}
	serial := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		serial++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-m5-%d", p.Document.ID, serial)
		p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	a, b := plane, plane
	a.InstanceID = p.Product.Instances[0].ID
	b.InstanceID = p.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a.InstanceID, Kind: "BODY"}})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", ConstraintFamily: "Offset", FirstAssemblyRef: &a, SecondAssemblyRef: &b, Value: 4, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	accepted := p.Product.Constraints[len(p.Product.Constraints)-1].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", ConstraintFamily: "Offset", FirstAssemblyRef: &a, SecondAssemblyRef: &b, Value: 8, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	failed := p.Product.Constraints[len(p.Product.Constraints)-1].ID
	if p.Product.Constraints[len(p.Product.Constraints)-1].EvaluationStatus == "VERIFIED" {
		t.Fatal("contradictory definition falsely Verified")
	}
	before, _ := json.Marshal(p.Product)
	head := p.Document.VersionID
	var manifestsBefore, manifestsAfter int
	if err = f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_document_id=$1`, p.Document.ID).Scan(&manifestsBefore); err != nil {
		t.Fatal(err)
	}
	r, err := f.service.AnalyzeAssemblyConflicts(t.Context(), p.Document.ID, f.actor, workspace.AssemblyConflictRequest{BaseRevisionID: head, TargetConstraintIDs: []string{failed}, MaxProbes: 16, TimeBudgetMS: 5000})
	if err != nil || r.Status != "UNSAT" || !r.Complete {
		t.Fatal("real three-valued analysis", r, err)
	}
	var certificate *workspace.AssemblyConflictItem
	for i := range r.Items {
		if r.Items[i].Evidence == "VERIFIED_IRREDUCIBLE" {
			certificate = &r.Items[i]
		}
	}
	if certificate == nil || len(certificate.ConstraintIDs) != 2 || !certificate.Irreducible {
		t.Fatal("missing verified deletion evidence", r)
	}
	contains := func(ids []string, id string) bool {
		for _, value := range ids {
			if value == id {
				return true
			}
		}
		return false
	}
	if !contains(certificate.ConstraintIDs, failed) || !contains(certificate.ConstraintIDs, accepted) {
		t.Fatal(certificate)
	}
	for _, probe := range r.Probes {
		if probe.Oracle == "SAT" && probe.SolverStatus == "" && len(probe.ConstraintIDs) > 0 {
			t.Fatal("no actual Worker SAT evidence", probe)
		}
	}
	read, err := f.service.GetDocument(t.Context(), p.Document.ID)
	after, _ := json.Marshal(read.Product)
	if err != nil || read.Document.VersionID != head || string(before) != string(after) {
		t.Fatal("diagnostics changed Head/poses/definitions", err)
	}
	if err = f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_document_id=$1`, p.Document.ID).Scan(&manifestsAfter); err != nil || manifestsBefore != manifestsAfter {
		t.Fatal("read-only diagnosis persisted solve history", manifestsBefore, manifestsAfter, err)
	}
	// Existing explicit user repair, not an automatic diagnostic mutation.
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{failed}, Suppressed: boolPointer(true)})
	if _, err = f.service.AnalyzeAssemblyConflicts(t.Context(), p.Document.ID, f.actor, workspace.AssemblyConflictRequest{BaseRevisionID: head, TargetConstraintIDs: []string{accepted}}); err == nil {
		t.Fatal("old diagnosis allowed against newer Head")
	}
	repaired, err := f.service.AnalyzeAssemblyConflicts(t.Context(), p.Document.ID, f.actor, workspace.AssemblyConflictRequest{BaseRevisionID: p.Document.VersionID, TargetConstraintIDs: []string{accepted}, TimeBudgetMS: 5000})
	if err != nil || repaired.Status != "SAT" || !repaired.Complete || contains(repaired.ScopeConstraintIDs, failed) {
		t.Fatal("repair failed/not excluded Suppressed", repaired, err)
	}
	t.Logf("M5 real conflict probes=%d evidence=%s; repaired SAT probes=%d; no diagnostic Revision/manifest", r.ProbeCount, certificate.Evidence, repaired.ProbeCount)
}
