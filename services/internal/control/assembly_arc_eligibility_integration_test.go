package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

// A real imported sphere seam is a trimmed circle, not a browser polyline.
// Inspection must preserve its exact geometry yet require explicit underlying
// support before a whole-circle relationship is offered or submitted.
func TestTrimmedArcEligibilityAndUnderlyingCircleThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "sphere-r6")
	point, arc := f.support(t, part, "SPHERE"), f.support(t, part, "CIRCLE")
	point.DerivedRole = "sphere-center"
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Explicit trimmed support"})
	if err != nil {
		t.Fatal(err)
	}
	id, seq := product.Document.ID, 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		seq++
		request.ActorID = f.actor
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("%s-arc-%d", id, seq)
		}
		product, err = f.service.ApplyCommand(t.Context(), id, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	moving, reference := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	point.InstanceID, arc.InstanceID = moving, reference
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: reference, Translation: [3]float64{19, -7, 11}, Rotation: [4]float64{0, math.Sin(.21), 0, math.Cos(.21)}})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{42, 13, -9}, Rotation: [4]float64{0, 0, math.Sin(.17), math.Cos(.17)}})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: reference, Kind: "BODY"}})
	inspect, err := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{point, arc})
	if err != nil || len(inspect.Supports) != 2 {
		t.Fatal("real seam inspection", err, inspect)
	}
	point, arc = inspect.Supports[0].Reference, inspect.Supports[1].Reference
	seam := inspect.Supports[1]
	if seam.Status != "RESOLVED" || seam.ExactType != "CIRCLE" || seam.Descriptor == nil || seam.Descriptor.ParameterStart == nil || seam.Descriptor.ParameterEnd == nil || *seam.Descriptor.ParameterEnd-*seam.Descriptor.ParameterStart >= 2*math.Pi-1e-8 || seam.ConstraintEligible || seam.ConstraintDiagnosticCode == "" || seam.ConstraintDiagnostic == "" {
		t.Fatal("trimmed support was confused with complete circle or Broken", seam)
	}
	if !inspect.Supports[0].ConstraintEligible {
		t.Fatal("sphere-centre point incorrectly rejected", inspect.Supports[0])
	}
	head := product.Document.VersionID
	var before int
	if err = f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1`, head).Scan(&before); err != nil {
		t.Fatal(err)
	}
	request := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-arc-invalid", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", ConstraintFamily: "Coincidence", ConstraintSubtype: "point-curve", FirstAssemblyRef: &point, SecondAssemblyRef: &arc}
	if _, err = f.service.PreviewCommand(t.Context(), id, request); !errors.Is(err, workspace.ErrValidation) {
		t.Fatal("implicit trimmed circle Preview was accepted", err)
	}
	if _, err = f.service.ApplyCommand(t.Context(), id, request); !errors.Is(err, workspace.ErrValidation) {
		t.Fatal("implicit trimmed circle command was accepted", err)
	}
	read, err := f.service.GetDocument(t.Context(), id)
	var after int
	if err != nil || read.Document.VersionID != head || !reflect.DeepEqual(read.Product, product.Product) || f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1`, head).Scan(&after) != nil || after != before {
		t.Fatal("illegal support produced a Revision/solve or changed poses", err, before, after)
	}
	arc.DerivedRole = "underlying-circle"
	derived, err := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{arc})
	if err != nil || !derived.Supports[0].ConstraintEligible || derived.Supports[0].ConstraintDiagnosticCode != "" || derived.Supports[0].Descriptor.ParameterStart == nil || derived.Supports[0].Descriptor.ParameterEnd == nil || *derived.Supports[0].Descriptor.ParameterStart != *seam.Descriptor.ParameterStart || *derived.Supports[0].Descriptor.ParameterEnd != *seam.Descriptor.ParameterEnd {
		t.Fatal("explicit underlying circle lost trimmed provenance", err, derived)
	}
	arc = derived.Supports[0].Reference
	initial := product.Product.Instances[0].Translation
	request.RequestID, request.SecondAssemblyRef = id+"-arc-explicit", &arc
	preview, err := f.service.PreviewCommand(t.Context(), id, request)
	if err != nil || preview.PreviewID == "" {
		t.Fatal("explicit underlying circle Preview", err, preview)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	definition := product.Product.Constraints[len(product.Product.Constraints)-1]
	if definition.Family != "Coincidence" || definition.DefinitionVersion != 2 || definition.Subtype != "point-curve" || definition.EvaluationStatus != "VERIFIED" || definition.Second.DerivedRole != "underlying-circle" {
		t.Fatal("explicit support intent not persisted", definition)
	}
	profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
	final, err := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{definition.First, *definition.Second})
	if err != nil {
		t.Fatal(err)
	}
	poses := map[string]workspace.ProductInstance{}
	for _, p := range product.Product.Instances {
		poses[p.ID] = p
	}
	p := compositionPoint(poses[moving], final.Supports[0].Descriptor.Origin)
	circle := final.Supports[1].Descriptor
	delta := compositionSub(p, compositionPoint(poses[reference], circle.Origin))
	normal := compositionRotate(poses[reference].Rotation, circle.Direction)
	if math.Abs(compositionDot(delta, normal)) > profile.LengthTolerance || math.Abs(compositionNorm(delta)-circle.Radius) > profile.LengthTolerance || compositionNorm(compositionSub(poses[moving].Translation, initial)) <= profile.LengthTolerance {
		t.Fatal("independent whole-circle geometry/motion failed", p, delta, circle.Radius)
	}
	cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
	read, err = cold.GetDocument(t.Context(), id)
	if err != nil || !reflect.DeepEqual(read.Product.Constraints, product.Product.Constraints) {
		t.Fatal("cold read changed explicit Arc role", err)
	}
	var digest string
	var raw []byte
	if err = f.db.QueryRow(t.Context(), `SELECT digest,manifest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&digest, &raw); err != nil {
		t.Fatal(err)
	}
	var manifest workspace.AssemblySolveManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	frozenCircle := false
	for _, g := range manifest.Geometry {
		if g.Kind == "CIRCLE" && g.ParameterStart != nil && g.ParameterEnd != nil {
			frozenCircle = true
		}
	}
	if !frozenCircle {
		t.Fatal("manifest omitted trimmed source domain")
	}
	replayed, err := cold.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-arc-replay")
	if err != nil || replayed.Status != "CONVERGED" {
		t.Fatal("explicit whole-circle cold replay", err, replayed)
	}
	release, err := cold.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{ActorID: f.actor, RequestID: id + "-arc-release", Name: "Explicit underlying circle"})
	if err != nil {
		t.Fatal(err)
	}
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{definition.ID}, Suppressed: &suppressed})
	apply(workspace.CommandRequest{Type: "UNDO"})
	if restored := product.Product.Constraints[len(product.Product.Constraints)-1]; restored.Suppressed || restored.Second.DerivedRole != "underlying-circle" || restored.EvaluationStatus != "VERIFIED" {
		t.Fatal("Undo lost explicit source or activation", restored)
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if restored := product.Product.Constraints[len(product.Product.Constraints)-1]; !restored.Suppressed || restored.Second.DerivedRole != "underlying-circle" {
		t.Fatal("Redo lost explicit source or activation", restored)
	}
	frozen, err := cold.ReplayProductRelease(t.Context(), id, release.ID, id+"-arc-release-replay")
	if err != nil || frozen.Assembly == nil {
		t.Fatal("frozen underlying circle Release", err, frozen)
	}
	if err = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, frozen.Assembly.ManifestDigest).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	frozenDefinition := false
	for _, def := range manifest.Definitions {
		if def.ID == definition.ID {
			frozenDefinition = true
			if def.Suppressed || def.Second == nil || def.Second.DerivedRole != "underlying-circle" {
				t.Fatal("later Head activation leaked into frozen Release", def)
			}
		}
	}
	if !frozenDefinition {
		t.Fatal("Release lost explicit underlying-circle definition")
	}
}
