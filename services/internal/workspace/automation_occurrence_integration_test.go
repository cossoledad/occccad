package workspace

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
)

type automationOccurrenceFixture struct {
	t     *testing.T
	db    database.DB
	s     *Service
	acl   *access.Service
	actor string
}

func newAutomationOccurrenceFixture(t *testing.T) *automationOccurrenceFixture {
	t.Helper()
	db := testsupport.OpenDevelopmentDatabase(t)
	actor := uuid.NewString()
	if _, err := db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Occurrence acceptance')`, actor, actor+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeWorker)
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	return &automationOccurrenceFixture{t: t, db: db, s: NewWithArtifacts(db, worker, artifact.NewService(db, store, staging)), acl: access.New(db), actor: actor}
}
func (f *automationOccurrenceFixture) create(kind string) DocumentView {
	f.t.Helper()
	view, err := f.s.CreateDocument(f.t.Context(), CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: kind, Name: "Occurrence " + uuid.NewString()})
	if err != nil {
		f.t.Fatal(err)
	}
	return view
}
func (f *automationOccurrenceFixture) apply(document string, request CommandRequest) DocumentView {
	f.t.Helper()
	snapshot, err := f.s.ReadModelSnapshot(f.t.Context(), document, "")
	if err != nil {
		f.t.Fatal(err)
	}
	request.ActorID = f.actor
	if request.RequestID == "" {
		request.RequestID = uuid.NewString()
	}
	request.ExpectedHead = &snapshot.CommandBaseline
	preview, err := f.s.PreviewCommand(f.t.Context(), document, request)
	if err != nil || preview.PreviewID == "" || preview.EvaluationOutcome == "DEFINITION_ONLY" {
		f.t.Fatalf("%s preview: %+v %v", request.Type, preview, err)
	}
	request.PreviewID = preview.PreviewID
	view, err := f.s.ApplyCommand(f.t.Context(), document, request)
	if err != nil {
		f.t.Fatalf("%s commit: %v", request.Type, err)
	}
	return view
}
func (f *automationOccurrenceFixture) cylinder(outer, inner, length float64) DocumentView {
	f.t.Helper()
	view := f.create("PART")
	view = f.apply(view.Document.ID, CommandRequest{Type: "CREATE_SKETCH", BodyID: view.Part.ActiveBodyID, Plane: "XY"})
	sketch := ""
	for _, feature := range view.Part.Features {
		if feature.Type == "SKETCH" {
			sketch = feature.ID
		}
	}
	ops := []SketchOperation{{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: outer}}}
	if inner > 0 {
		ops = append(ops, SketchOperation{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: inner}})
	}
	view = f.apply(view.Document.ID, CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
	view = f.apply(view.Document.ID, CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: view.Part.ActiveBodyID, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: length})
	volume := 0.0
	for _, a := range view.Artifacts {
		volume += a.Volume
	}
	if math.Abs(volume-math.Pi*(outer*outer-inner*inner)*length) > 1e-5 {
		f.t.Fatal("native parameterized extrusion volume mismatch", volume)
	}
	return view
}
func (f *automationOccurrenceFixture) insert(product DocumentView, part DocumentView, mode string) (DocumentView, string) {
	f.t.Helper()
	old := map[string]bool{}
	for _, i := range product.Product.Instances {
		old[i.ID] = true
	}
	product = f.apply(product.Document.ID, CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, ReferenceMode: mode})
	for _, i := range product.Product.Instances {
		if !old[i.ID] {
			return product, i.ID
		}
	}
	f.t.Fatal("new stable instance not reported")
	return product, ""
}
func occurrenceIDs(root string, ids ...string) InstancePath {
	p := InstancePath{RootDocumentID: root}
	for _, id := range ids {
		p.Segments = append(p.Segments, InstancePathSegment{InstanceID: id})
	}
	return p
}
func (f *automationOccurrenceFixture) read(root, revision string, ids ...string) AutomationOccurrence {
	f.t.Helper()
	occ, err := f.s.ReadAutomationOccurrence(f.t.Context(), f.acl, f.actor, root, revision, occurrenceIDs(root, ids...))
	if err != nil {
		f.t.Fatal(err)
	}
	return occ
}

func TestAutomationOccurrenceProductionShaftHoleVerification(t *testing.T) {
	f := newAutomationOccurrenceFixture(t)
	shaft := f.cylinder(5, 0, 35)
	sleeve := f.cylinder(12, 5.1, 20)
	product := f.create("PRODUCT")
	product, moving := f.insert(product, shaft, "FOLLOW_HEAD")
	product, reference := f.insert(product, sleeve, "PINNED")
	angle := 37 * math.Pi / 180
	product = f.apply(product.Document.ID, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{47, -31, 19}, Rotation: [4]float64{0, math.Sin(angle / 2), 0, math.Cos(angle / 2)}})
	product = f.apply(product.Document.ID, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: reference, Translation: [3]float64{10, 20, 30}, Rotation: [4]float64{0, 0, 0, 1}})
	root := product.Document.ID
	shaftOcc := f.read(root, product.Document.VersionID, moving)
	sleeveOcc := f.read(root, product.Document.VersionID, reference)
	shaftGeometry, err := f.s.ReadAutomationOccurrenceGeometry(t.Context(), shaftOcc, shaft.Part.ActiveBodyID, "FACE", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	sleeveGeometry, err := f.s.ReadAutomationOccurrenceGeometry(t.Context(), sleeveOcc, sleeve.Part.ActiveBodyID, "FACE", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	choose := func(items []AutomationGeometry, kind string, side int32, normalSign float64) AutomationGeometry {
		t.Helper()
		var matches []AutomationGeometry
		for _, i := range items {
			if i.Kind != kind {
				continue
			}
			if kind == "CYLINDER" && i.Definition.MaterialSide != side {
				continue
			}
			if kind == "PLANE" && i.Definition.Direction[2]*normalSign < .999999 {
				continue
			}
			matches = append(matches, i)
		}
		if len(matches) != 1 {
			t.Fatalf("geometry choice must be unique by actual analytic facts: %s %d %+v", kind, len(matches), matches)
		}
		return matches[0]
	}
	a := choose(shaftGeometry.Items, "CYLINDER", 1, 0)
	b := choose(sleeveGeometry.Items, "CYLINDER", -1, 0)
	p := choose(shaftGeometry.Items, "PLANE", 0, -1)
	q := choose(sleeveGeometry.Items, "PLANE", 0, -1)
	if a.AxialRangeMM == nil || b.AxialRangeMM == nil || math.Abs(a.Definition.Radius-5) > 1e-9 || math.Abs(b.Definition.Radius-5.1) > 1e-9 {
		t.Fatal("exact cylinder radius/range missing", a, b)
	}
	zero := 0.
	target := uint64(1)
	input := AutomationAssemblyVerificationInput{RevisionID: product.Document.VersionID, MovingInstanceID: moving, ReferenceInstanceID: reference, FirstAxis: &a.Reference, SecondAxis: &b.Reference, FirstPlane: &p.Reference, SecondPlane: &q.Reference, DirectionRelation: "SAME", OffsetMM: &zero, TargetDOF: &target}
	before, err := f.s.VerifyAutomationAssembly(t.Context(), f.acl, f.actor, root, input)
	if err != nil {
		t.Fatal(err)
	}
	if before.Passed || len(before.Failures) == 0 {
		t.Fatal("unassembled persisted poses passed read-only solve", before)
	}
	product = f.apply(root, CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &AssemblyGeometryRef{InstanceID: reference, Kind: "BODY"}})
	product = f.apply(root, CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONCENTRIC", DirectionRelation: "SAME", FirstAssemblyRef: &a.Reference, SecondAssemblyRef: &b.Reference})
	product = f.apply(root, CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", DirectionRelation: "SAME", FirstAssemblyRef: &p.Reference, SecondAssemblyRef: &q.Reference})
	input.RevisionID = product.Document.VersionID
	result, err := f.s.VerifyAutomationAssembly(t.Context(), f.acl, f.actor, root, input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.SolverStatus != "CONVERGED" || result.RelativeDOF != 1 || result.TranslationDOF != 0 || result.RotationDOF != 1 || result.RotationAxis == nil {
		raw, _ := json.MarshalIndent(result, "", "  ")
		t.Fatalf("native shaft/hole acceptance failed: %s", raw)
	}
	inferred, err := f.s.VerifyAutomationAssembly(t.Context(), f.acl, f.actor, root, AutomationAssemblyVerificationInput{RevisionID: product.Document.VersionID, MovingInstanceID: moving, ReferenceInstanceID: reference, DirectionRelation: "SAME", OffsetMM: &zero, TargetDOF: &target})
	if err != nil || !inferred.Passed {
		t.Fatal("actual constraints did not provide unique formal goal refs", inferred, err)
	}
	wrong := input
	wrong.DirectionRelation = "OPPOSITE"
	invalid, err := f.s.VerifyAutomationAssembly(t.Context(), f.acl, f.actor, root, wrong)
	if err != nil || invalid.Passed {
		t.Fatal("wrong end direction accepted", invalid, err)
	}
	t.Logf("production Worker: status=%s, axis distance=%.3g mm angle=%.3g rad plane offset=%.3g mm, DOF=%d (translation=%d rotation=%d), solver=%s", result.SolverStatus, result.AxisDistanceMM, result.AxisAngleRad, result.PlaneOffsetMM, result.RelativeDOF, result.TranslationDOF, result.RotationDOF, result.SolverBuild)
}

func TestAutomationOccurrenceRepeatedNestedPinnedAndACL(t *testing.T) {
	f := newAutomationOccurrenceFixture(t)
	part := f.cylinder(4, 0, 15)
	part = f.apply(part.Document.ID, CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy", Name: "Published plane"})
	if len(part.Part.Publications) != 1 {
		t.Fatal("fixture Publication was not persisted")
	}
	oldPartRevision := part.Document.VersionID
	child := f.create("PRODUCT")
	child, leaf := f.insert(child, part, "FOLLOW_HEAD")
	child = f.apply(child.Document.ID, CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: leaf, PublicationID: part.Part.Publications[0].ID, Name: "Forwarded plane"})
	child = f.apply(child.Document.ID, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{3, 4, 5}, Rotation: [4]float64{0, 0, 0, 1}})
	root := f.create("PRODUCT")
	root, a := f.insert(root, child, "PINNED")
	root, b := f.insert(root, child, "FOLLOW_HEAD")
	root = f.apply(root.Document.ID, CommandRequest{Type: "RENAME_INSTANCE", InstanceID: a, Name: "Same name"})
	root = f.apply(root.Document.ID, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{10, 20, 30}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}})
	oldRoot := root.Document.VersionID
	occA := f.read(root.Document.ID, oldRoot, a, leaf)
	occB := f.read(root.Document.ID, oldRoot, b, leaf)
	if occA.SourceDocumentID != part.Document.ID || occB.SourceRevisionID != oldPartRevision || occA.Path.Canonical == occB.Path.Canonical || occA.Definition.ModelHash != occB.Definition.ModelHash || occA.WorldPose.Translation == occB.WorldPose.Translation {
		t.Fatal("definition reuse/occurrence identity or pose mixed", occA, occB)
	}
	want := [3]float64{6, 23, 35}
	for k, v := range want {
		if math.Abs(occA.WorldPose.Translation[k]-v) > 1e-9 {
			t.Fatal("nested rigid transform composed incorrectly", occA.WorldPose)
		}
	}
	part = f.apply(part.Document.ID, CommandRequest{Type: "RENAME_BODY", BodyID: part.Part.ActiveBodyID, Name: "New head"})
	child = f.apply(child.Document.ID, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{100, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	old := f.read(root.Document.ID, oldRoot, a, leaf)
	follow := f.read(root.Document.ID, oldRoot, b, leaf)
	if old.SourceRevisionID != oldPartRevision || old.WorldPose != occA.WorldPose || follow.SourceRevisionID != oldPartRevision || follow.WorldPose != occB.WorldPose {
		t.Fatal("historical/PINNED/FOLLOW_HEAD snapshot silently followed a new Head", old, follow)
	}
	bad := occA.Path
	bad.Segments = append([]InstancePathSegment(nil), bad.Segments...)
	bad.Segments[1].ResolvedVersionID = part.Document.VersionID
	if _, err := f.s.ReadAutomationOccurrence(t.Context(), f.acl, f.actor, root.Document.ID, oldRoot, bad); !errors.Is(err, ErrValidation) {
		t.Fatal("forged resolved source version accepted", err)
	}
	observer := uuid.NewString()
	if _, err := f.db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Occurrence observer')`, observer, observer+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	grant := func(document string) {
		t.Helper()
		if _, err := f.acl.UpsertGrant(t.Context(), "DOCUMENT", document, f.actor, access.ShareRequest{SubjectType: "USER", SubjectID: observer, Role: access.RoleViewer}); err != nil {
			t.Fatal(err)
		}
	}
	grant(root.Document.ID)
	if _, err := f.s.ReadAutomationOccurrence(t.Context(), f.acl, observer, root.Document.ID, oldRoot, occurrenceIDs(root.Document.ID, a, leaf)); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("intermediate Product ACL bypass", err)
	}
	grant(child.Document.ID)
	if _, err := f.s.ReadAutomationOccurrence(t.Context(), f.acl, observer, root.Document.ID, oldRoot, occurrenceIDs(root.Document.ID, a, leaf)); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("source Part ACL bypass", err)
	}
	forwarded, err := f.s.ReadAutomationOccurrence(t.Context(), f.acl, observer, root.Document.ID, oldRoot, occurrenceIDs(root.Document.ID, a))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReadAutomationOccurrenceGeometry(t.Context(), forwarded, "", "PUBLICATION", "", "", 20); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("forwarding Publication disclosed an unauthorized Part descriptor", err)
	}
	grant(part.Document.ID)
	if _, err := f.s.ReadAutomationOccurrence(t.Context(), f.acl, observer, root.Document.ID, oldRoot, occurrenceIDs(root.Document.ID, a, leaf)); err != nil {
		t.Fatal("all source grants did not permit exact historical observation", err)
	}
	publicationPage, err := f.s.ReadAutomationOccurrenceGeometry(t.Context(), forwarded, "", "PUBLICATION", "", "", 20)
	if err != nil || len(publicationPage.Items) != 1 || publicationPage.Items[0].Reference.Kind != "PLANE" || publicationPage.Items[0].Reference.PublicationRef == nil {
		t.Fatal("formal forwarding Publication observation failed", publicationPage, err)
	}
	for k, v := range want {
		if math.Abs(publicationPage.Items[0].World.Origin[k]-v) > 1e-9 {
			t.Fatal("forwarded Publication definition/world transform folded incorrectly", publicationPage.Items[0])
		}
	}
}
