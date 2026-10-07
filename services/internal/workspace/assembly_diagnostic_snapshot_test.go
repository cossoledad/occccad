package workspace

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func diagnosticFixture(t *testing.T) (AssemblySolveManifest, []AssemblyConstraint) {
	t.Helper()
	definitions := []AssemblyConstraint{}
	poses := geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	primitives := []geometry.AssemblyConstraint{}
	for _, entry := range []struct {
		id, state, mode string
		suppressed      bool
	}{{"accepted", "VERIFIED", "DRIVING", false}, {"pending", "NOT_UPDATED", "DRIVING", false}, {"disabled", "VERIFIED", "DRIVING", true}, {"measurement", "VERIFIED", "MEASURED", false}, {"broken", "BROKEN", "DRIVING", false}} {
		definitions = append(definitions, AssemblyConstraint{ID: entry.id, DefinitionVersion: 2, Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "body", Kind: "BODY"}, EvaluationStatus: modelcore.AssemblyConstraintEvaluationStatus(entry.state), Mode: entry.mode, Suppressed: entry.suppressed, FixedPose: &InstancePose{Rotation: poses.Rotation}})
		if entry.id != "broken" {
			primitives = append(primitives, geometry.AssemblyConstraint{ID: entry.id, Kind: "FIX", FirstBodyID: "body", Mode: entry.mode, FixedPose: &poses})
		}
	}
	manifest, err := newAssemblySolveManifest("document", "revision", "model", []geometry.AssemblyBody{{ID: "body", Pose: poses}}, nil, primitives, nil, nil, nil, definitions)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, definitions
}
func TestAssemblyDiagnosticTablesPreserveDefinitionsAndMissingInputs(t *testing.T) {
	manifest, defs := diagnosticFixture(t)
	failure := AssemblyCompileFailure{ConstraintID: "broken", Endpoint: "FIRST", Phase: "SUPPORT_RESOLUTION", Error: "geometry missing"}
	snapshot, err := buildAssemblyDiagnosticSnapshot(manifest, defs, []AssemblyCompileFailure{failure})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources) != 1 || len(snapshot.Definitions) != 5 || len(snapshot.Primitives) != 4 || len(snapshot.CompileFailures) != 1 {
		t.Fatal("missing constraints or no source dedup", snapshot)
	}
	restored, err := snapshot.RestoreDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	// The manifest sorts its copy. Saved definition order remains the supplied order.
	if !reflect.DeepEqual(restored, defs) {
		t.Fatal("saved states, modes or parameters changed")
	}
	data, err := encodeAssemblyDiagnostic(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	_ = json.Unmarshal(data, &file)
	if len(file.Request) != 0 || strings.Contains(string(data), `"manifest"`) {
		t.Fatal("duplicate numerical authority")
	}
	var decoded AssemblyDiagnosticSnapshot
	_ = json.Unmarshal(file.Snapshot, &decoded)
	if decoded.Digest != decoded.contentDigest() {
		t.Fatal("content digest failed round trip")
	}
	cold, err := diagnosticManifest(decoded)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectDiagnosticConstraints(&cold, AssemblyReplayOptions{Mode: "accepted-pending", ConstraintIDs: []string{"pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if !selected["pending"] || selected["disabled"] || selected["broken"] || !selected["measurement"] {
		t.Fatal("replay participant selection conflated saved states", selected)
	}
	missing, err := ReplayAssemblyDiagnosticWithOptions(context.Background(), nil, data, AssemblyReplayOptions{Mode: "all-driving"})
	if err != nil {
		t.Fatal(err)
	}
	var rejected geometry.AssemblyReplay
	_ = json.Unmarshal(missing, &rejected)
	if rejected.Outcome != "INPUT_ERROR" {
		t.Fatal("missing geometry became successful unconstrained solve", string(missing))
	}
	decoded.Current.Constraints = append(decoded.Current.Constraints, 999)
	if _, err := decoded.ExpandFrame(decoded.Current); err == nil {
		t.Fatal("missing reference silently ignored")
	}
	decoded.Digest = "changed"
	raw, _ := json.Marshal(decoded)
	file.Snapshot = raw
	data, _ = json.Marshal(file)
	if _, err := ReplayAssemblyDiagnosticWithOptions(context.Background(), nil, data, AssemblyReplayOptions{}); err == nil {
		t.Fatal("tampered snapshot accepted")
	}
}
func TestAssemblyOriginalAttemptUnavailableIsExplicit(t *testing.T) {
	manifest, defs := diagnosticFixture(t)
	snapshot, err := buildAssemblyDiagnosticSnapshot(manifest, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	New(nil, nil).appendDiagnosticAttempts(context.Background(), &snapshot, defs)
	if len(snapshot.Attempts) != 1 || len(snapshot.Attempts[0].Missing) == 0 {
		t.Fatal("invented original failure input")
	}
	data, _ := encodeAssemblyDiagnostic(snapshot)
	out, err := ReplayAssemblyDiagnosticWithOptions(context.Background(), nil, data, AssemblyReplayOptions{Mode: "original", TargetConstraintID: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	_ = json.Unmarshal(out, &file)
	if file.Outcome != "INPUT_ERROR" || len(file.Missing) == 0 {
		t.Fatal("current state disguised as original attempt")
	}
}
func TestAssemblyAttemptCollectionIsByteAndFrameBounded(t *testing.T) {
	d := &OperationDiagnostic{}
	data := make([]byte, assemblyAttemptBudget/2)
	d.recordAssemblyAttempt(data)
	d.recordAssemblyAttempt(data)
	d.recordAssemblyAttempt([]byte("overflow"))
	if len(d.AssemblyAttempts) != 2 || d.assemblyBytes > assemblyAttemptBudget || len(d.AssemblyMissing) == 0 {
		t.Fatal("attempt budget not enforced")
	}
}
func TestAssemblyManifestDigestPreservesLargeIntegerBranchIdentity(t *testing.T) {
	m, _ := diagnosticFixture(t)
	m.Constraints[0].AngleBranchState = &geometry.AssemblyAngleBranchState{Winding: math.MaxInt64}
	first := assemblyManifestDigest(m)
	m.Constraints[0].AngleBranchState.Winding--
	if first == assemblyManifestDigest(m) {
		t.Fatal("integer identity rounded through float64")
	}
	zero := []byte(`{"x":-0.0,"y":1e20}`)
	normalized := []byte(`{"y":100000000000000000000,"x":0}`)
	if canonicalAssemblyManifestJSONDigest(zero) != canonicalAssemblyManifestJSONDigest(normalized) {
		t.Fatal("JSONB normalization changed identity")
	}
	// A large double's JSONB decimal expansion fits uint64, but is not an
	// integer identity field. Conversely, winding must remain exact.
	floating := []byte(`{"origin":[1.152921504606847e18],"winding":1152921504606846976}`)
	jsonb := []byte(`{"origin":[1152921504606847000],"winding":1152921504606846976}`)
	if canonicalAssemblyManifestJSONDigest(floating) != canonicalAssemblyManifestJSONDigest(jsonb) {
		t.Fatal("large double JSONB expansion confused with exact branch winding")
	}
}

func TestAssemblyDiagnosticQuantityFailureRetainsUnrelatedComponents(t *testing.T) {
	model := ProductModel{Instances: []ProductInstance{{ID: "a"}, {ID: "b"}}, Constraints: []AssemblyConstraint{
		{ID: "length", Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}},
		{ID: "dependent", Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}},
		{ID: "independent", Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}},
	}}
	for i, text := range []string{"2 mm", "Length + 1 mm", "7 mm"} {
		if err := editAssemblyQuantity(&model, i, &text, []string{"Length", "Dependent", "Independent"}[i], 0); err != nil {
			t.Fatal(err)
		}
	}
	model.Constraints[0].QuantityParameter.Dimension = modelcore.Dimension{}
	model.Constraints[2].Value = -100
	capture := &assemblyDiagnosticCapture{invalid: map[string]bool{}}
	resolveDiagnosticAssemblyQuantities(&model, capture)
	if !capture.invalid["length"] || !capture.invalid["dependent"] || capture.invalid["independent"] || model.Constraints[2].Value != 7 || len(model.Constraints) != 3 {
		t.Fatal("failed component contaminated unrelated constraints", capture, model)
	}
}

func TestAssemblyDiagnosticSubsystemKeepsGroundThirdAxisAndGroupBoundary(t *testing.T) {
	defs := []AssemblyConstraint{
		{ID: "angle", Kind: "ANGLE", Mode: "DRIVING", EvaluationStatus: "NOT_UPDATED", First: AssemblyGeometryRef{InstanceID: "a"}, Second: &AssemblyGeometryRef{InstanceID: "b"}, AngleAxis: &AssemblyGeometryRef{InstanceID: "c"}},
		{ID: "ground", Kind: "FIX", EvaluationStatus: "VERIFIED", First: AssemblyGeometryRef{InstanceID: "c"}},
		{ID: "boundary", Kind: "COINCIDENT", EvaluationStatus: "VERIFIED", First: AssemblyGeometryRef{InstanceID: "b"}, Second: &AssemblyGeometryRef{InstanceID: "d"}},
		{ID: "unrelated", Kind: "FIX", EvaluationStatus: "VERIFIED", First: AssemblyGeometryRef{InstanceID: "e"}},
		groupDefinition("group", AssemblyGroupMember{InstanceID: "c"}, AssemblyGroupMember{InstanceID: "d"}),
	}
	defs[4].EvaluationStatus = "VERIFIED"
	m := AssemblySolveManifest{Definitions: defs, Constraints: []geometry.AssemblyConstraint{{ID: "angle", Kind: "ANGLE", FirstBodyID: "a", SecondBodyID: "b", AngleReferenceBodyID: "c"}, {ID: "ground", Kind: "FIX", FirstBodyID: "c"}, {ID: "boundary", Kind: "COINCIDENT", FirstBodyID: "b", SecondBodyID: "d"}, {ID: "unrelated", Kind: "FIX", FirstBodyID: "e"}}, Intent: &geometry.AssemblySolveIntent{MovingBodyIDs: []string{"a", "e"}, ReferenceBodyIDs: []string{"c", "d"}}}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		m.Bodies = append(m.Bodies, geometry.AssemblyBody{ID: id})
	}
	selected, err := selectDiagnosticConstraints(&m, AssemblyReplayOptions{Mode: "subsystem", TargetConstraintID: "angle"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Bodies) != 4 || len(m.Constraints) != 3 || len(m.GroupStages) != 1 || !selected["ground"] || !selected["boundary"] || !selected["group"] || selected["unrelated"] || !reflect.DeepEqual(m.Intent.MovingBodyIDs, []string{"a"}) || !reflect.DeepEqual(m.Intent.ReferenceBodyIDs, []string{"c", "d"}) {
		t.Fatal("subsystem dropped boundary semantics or added grounding", m, selected)
	}
}

func TestAssemblyDiagnosticRejectsInconsistentParticipantAndNamingReferences(t *testing.T) {
	m, defs := diagnosticFixture(t)
	snapshot, err := buildAssemblyDiagnosticSnapshot(m, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.validateReferences(); err != nil {
		t.Fatal(err)
	}
	snapshot.Current.ParticipantIDs = append(snapshot.Current.ParticipantIDs, "pending")
	if err = snapshot.validateReferences(); err == nil {
		t.Fatal("mismatched participant table accepted")
	}
	snapshot.Current.ParticipantIDs = nil
	snapshot.NamingLinks = []AssemblyDiagnosticNamingLink{{ConstraintID: "missing", Endpoint: "FIRST", Evidence: 999}}
	if err = snapshot.validateReferences(); err == nil {
		t.Fatal("invalid Naming link accepted")
	}
}

func TestAssemblyManifestCompilerScopesLocalGeometryIdentityByBody(t *testing.T) {
	pose := geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	values := []geometry.AssemblyGeometry{{ID: "point", BodyID: "a", Kind: "POINT", LengthUnit: "mm"}, {ID: "point", BodyID: "b", Kind: "POINT", LengthUnit: "mm"}}
	constraints := []geometry.AssemblyConstraint{{ID: "mate", Kind: "COINCIDENT", FirstBodyID: "a", FirstGeometryID: "point", SecondBodyID: "b", SecondGeometryID: "point"}}
	m, err := newAssemblySolveManifest("document", "revision", "hash", []geometry.AssemblyBody{{ID: "a", Pose: pose}, {ID: "b", Pose: pose}}, values, constraints, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Constraints[0].FirstGeometryID == m.Constraints[0].SecondGeometryID || values[0].ID != "point" || constraints[0].FirstGeometryID != "point" {
		t.Fatal("body identity merged or caller input modified")
	}
	if err = validateAssemblySolveManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestAssemblyReferenceCacheIdentityExcludesPresentationAndResolution(t *testing.T) {
	ref := AssemblyGeometryRef{InstanceID: "occurrence", Kind: "PLANE", GeometryID: "datum", SourceVersionID: "accepted", InstancePath: &InstancePath{RootDocumentID: "root", Segments: []InstancePathSegment{{OwnerDocumentID: "root", OwnerVersionID: "root-revision", InstanceID: "occurrence", ReferencedDocumentID: "part", ResolvedVersionID: "accepted"}}}}
	first := assemblyReferenceKey(ref)
	ref.InstancePath.Display = "Renamed"
	ref.InstancePath.Canonical = "display-only"
	ref.InstancePath.Segments[0].InstanceName = "Other name"
	ref.Resolution = &ResolutionSnapshot{}
	if assemblyReferenceKey(ref) != first || len(first) != 64 {
		t.Fatal("presentation/evidence became numerical endpoint identity")
	}
	ref.InstancePath.Segments[0].ResolvedVersionID = "changed"
	if assemblyReferenceKey(ref) == first {
		t.Fatal("different accepted occurrence revision merged")
	}
}
