package control

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

type contactRecoveryCase struct{ name, first, firstKind, second, secondKind, relation, side string }

var contactRecoveryCases = []contactRecoveryCase{
	{"plane-plane-face", "cylinder-r6-h12", "PLANE", "cylinder-r6-h12", "PLANE", "FACE", "EXTERNAL"},
	{"plane-cylinder-line", "cylinder-r6-h12", "CYLINDER", "cylinder-r6-h12", "PLANE", "LINE", "EXTERNAL"},
	{"plane-sphere-point", "sphere-r6", "SPHERE", "cylinder-r6-h12", "PLANE", "POINT", "EXTERNAL"},
	{"cylinder-cylinder-line", "cylinder-r6-h12", "CYLINDER", "cylinder-r6-h12", "CYLINDER", "LINE", "EXTERNAL"},
	{"cylinder-cylinder-face", "cylinder-r6-h12", "CYLINDER", "cylinder-r6-h12", "CYLINDER", "FACE", "INTERNAL"},
	{"sphere-sphere-face", "sphere-r6", "SPHERE", "sphere-r6", "SPHERE", "FACE", "INTERNAL"},
	{"sphere-cone-ring", "sphere-r6", "SPHERE", "cone-r9-r3-h12", "CONE", "RING", "INTERNAL"},
	{"sphere-circle-ring", "sphere-r6", "SPHERE", "cylinder-r3-h12", "CIRCLE", "RING", "EXTERNAL"},
	{"cone-cone-line", "cone-r9-r3-h12", "CONE", "cone-r9-r3-h12", "CONE", "LINE", "EXTERNAL"},
	{"cone-cone-face", "cone-r9-r3-h12", "CONE", "cone-r9-r3-h12", "CONE", "FACE", "INTERNAL"},
	{"cone-circle-ring", "cone-r9-r3-h12", "CONE", "cylinder-r3-h12", "CIRCLE", "RING", "EXTERNAL"},
}

// These are transient exact Worker picks used by the formal bind/Publication
// command. Local IDs never become the persistent reference or reconnect key.
func (f *compositionControlFixture) contactPick(t *testing.T, part workspace.DocumentView, bodyID, kind string) workspace.AssemblyGeometryRef {
	t.Helper()
	var body workspace.PartBody
	for _, b := range part.Part.Bodies {
		if b.ID == bodyID {
			body = b
		}
	}
	a := part.Artifacts[body.GeometryKey]
	object, err := f.store.Get(t.Context(), a.Representations["BREP"].ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	topology, _, err := f.client.GetTopologyFromArtifact(t.Context(), a.GeometryID, geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if kind == "CIRCLE" {
		for _, e := range topology.Edges {
			if e.CurveType != 1 {
				continue
			}
			for _, p := range e.Properties {
				if p.Name == "center" && p.GetVectorValue() != nil && math.Abs(p.GetVectorValue().Z) < 1e-9 {
					return workspace.AssemblyGeometryRef{Kind: "EDGE", GeometryKey: body.GeometryKey, TopologyID: e.LocalId}
				}
			}
		}
	} else {
		want := map[string]int32{"PLANE": 0, "CYLINDER": 1, "CONE": 2, "SPHERE": 3}[kind]
		for _, face := range topology.Faces {
			if face.SurfaceType != want {
				continue
			}
			if kind == "PLANE" {
				found := false
				for _, p := range face.Properties {
					if p.Name == "origin" && p.GetVectorValue() != nil && math.Abs(p.GetVectorValue().Z-12) < 1e-9 {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			return workspace.AssemblyGeometryRef{Kind: "FACE", GeometryKey: body.GeometryKey, TopologyID: face.LocalId}
		}
	}
	t.Fatalf("no actual %s support in CAD Body %s", kind, bodyID)
	return workspace.AssemblyGeometryRef{}
}

// Independent Euclidean contact checks from exact OCCT descriptors and accepted
// component poses. This does not read solver residuals or call contact equations.
func assertContactRecoveryGeometry(t *testing.T, f *compositionControlFixture, product workspace.DocumentView, c contactRecoveryCase, first, second workspace.AssemblyGeometryRef, branch int32) {
	t.Helper()
	inspection, err := f.service.InspectAssemblySupports(t.Context(), product.Document.ID, []workspace.AssemblyGeometryRef{first, second})
	if err != nil {
		t.Fatal(err)
	}
	supports := []geometry.AssemblyGeometry{}
	for _, s := range inspection.Supports {
		if s.Status != "RESOLVED" || s.Descriptor == nil {
			t.Fatalf("exact frozen contact support missing: %+v", s)
		}
		g := *s.Descriptor
		found := false
		for _, body := range product.Product.Instances {
			if body.ID == g.BodyID {
				g.Origin = compositionPoint(body, g.Origin)
				g.Direction = compositionRotate(body.Rotation, g.Direction)
				found = true
			}
		}
		if !found {
			t.Fatalf("CAD Body confused with motion body %s", g.BodyID)
		}
		supports = append(supports, g)
	}
	a, b := supports[0], supports[1]
	profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
	near := func(value, want float64) {
		t.Helper()
		if math.Abs(value-want) > profile.LengthTolerance {
			t.Fatalf("independent Contact %s: %.12g want %.12g", c.name, value, want)
		}
	}
	sub := compositionSub
	dot := compositionDot
	norm := compositionNorm
	radial := func(d, n [3]float64) [3]float64 {
		h := dot(d, n)
		return sub(d, [3]float64{h * n[0], h * n[1], h * n[2]})
	}
	if a.Kind == "PLANE" && b.Kind != "PLANE" {
		a, b = b, a
	}
	d := sub(a.Origin, b.Origin)
	switch c.name {
	case "plane-plane-face":
		compositionContactAngle(t, profile, a.Direction, b.Direction, math.Pi, false)
		near(dot(d, b.Direction), 0)
	case "plane-cylinder-line":
		compositionContactAngle(t, profile, a.Direction, b.Direction, math.Pi/2, false)
		near(dot(d, b.Direction), float64(branch)*a.Radius)
	case "plane-sphere-point":
		near(dot(d, b.Direction), float64(branch)*a.Radius)
	case "cylinder-cylinder-line", "cylinder-cylinder-face":
		compositionContactAngle(t, profile, a.Direction, b.Direction, 0, true)
		distance := a.Radius + b.Radius
		if c.relation == "FACE" {
			distance = 0
		}
		near(norm(radial(d, b.Direction)), distance)
	case "sphere-sphere-face":
		near(norm(d), 0)
		near(a.Radius, b.Radius)
	case "sphere-cone-ring":
		near(norm(radial(d, b.Direction)), 0)
		near(dot(d, b.Direction), float64(b.ConeLeaf)*a.Radius/math.Sin(b.HalfAngle))
	case "sphere-circle-ring":
		near(norm(radial(d, b.Direction)), 0)
		near(dot(d, b.Direction), -float64(branch)*math.Sqrt(a.Radius*a.Radius-b.Radius*b.Radius))
	case "cone-cone-face":
		near(norm(d), 0)
		compositionContactAngle(t, profile, a.Direction, b.Direction, 0, false)
		if math.Abs(a.HalfAngle-b.HalfAngle) > profile.AngleTolerance {
			t.Fatal("Contact cone half-angle mismatch")
		}
	case "cone-cone-line":
		compositionContactAngle(t, profile, a.Direction, b.Direction, a.HalfAngle+b.HalfAngle, false)
		g := [3]float64{-a.Direction[0] - b.Direction[0], -a.Direction[1] - b.Direction[1], -a.Direction[2] - b.Direction[2]}
		length := norm(g)
		for i := range g {
			g[i] /= length
		}
		near(norm(radial(d, g)), 0)
		compositionContactAngle(t, profile, g, a.Direction, math.Pi-a.HalfAngle, false)
		compositionContactAngle(t, profile, g, b.Direction, math.Pi-b.HalfAngle, false)
	case "cone-circle-ring":
		compositionContactAngle(t, profile, a.Direction, b.Direction, 0, true)
		d = sub(b.Origin, a.Origin)
		near(norm(radial(d, a.Direction)), 0)
		near(dot(d, a.Direction), float64(a.ConeLeaf)*b.Radius/math.Tan(a.HalfAngle))
	default:
		t.Fatalf("missing independent contact oracle %s", c.name)
	}
}

func TestContactPublicationRecoveryAndEditingThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	for _, c := range contactRecoveryCases {
		t.Run(c.name, func(t *testing.T) {
			part := f.importPart(t, c.first)
			other := f.importPart(t, c.second)
			seq := 0
			partApply := func(request workspace.CommandRequest) {
				t.Helper()
				seq++
				request.ActorID = f.actor
				request.RequestID = fmt.Sprintf("%s-contact-source-%d", part.Document.ID, seq)
				var e error
				part, e = f.service.ApplyCommand(t.Context(), part.Document.ID, request)
				if e != nil {
					t.Fatal(request.Type, e)
				}
			}
			pick := f.contactPick(t, part, part.Part.Bodies[0].ID, c.firstKind)
			publicationType := "SURFACE"
			if pick.Kind == "EDGE" {
				publicationType = "CURVE"
			}
			partApply(workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Analytic mounting support", PublicationType: publicationType, TargetKind: pick.Kind, GeometryKey: pick.GeometryKey, TopologyID: pick.TopologyID, VersionID: part.Document.VersionID})
			publicationID := part.Part.Publications[len(part.Part.Publications)-1].ID
			product, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Contact recovery " + c.name})
			if e != nil {
				t.Fatal(e)
			}
			apply := func(request workspace.CommandRequest) {
				t.Helper()
				seq++
				request.ActorID = f.actor
				request.RequestID = fmt.Sprintf("%s-contact-recovery-%d", product.Document.ID, seq)
				var e error
				product, e = f.service.ApplyCommand(t.Context(), product.Document.ID, request)
				if e != nil {
					t.Fatal(request.Type, e)
				}
			}
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: other.Document.ID})
			moving, reference := product.Product.Instances[0].ID, product.Product.Instances[1].ID
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{19, -7, 24}, Rotation: [4]float64{0, 0, 0, 1}})
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: reference, Kind: "BODY"}})
			first := workspace.AssemblyGeometryRef{InstanceID: moving, Kind: c.firstKind, PublicationRef: &workspace.PublicationRef{PublicationID: publicationID, ExpectedType: publicationType, CompatibilityVersion: "1.0.0"}}
			second := f.support(t, other, c.secondKind)
			second.InstanceID = reference
			if c.secondKind == "CIRCLE" {
				second.DerivedRole = "underlying-circle"
			}
			branch := int32(1)
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONTACT", ContactKind: c.relation, ContactSide: c.side, ContactBranch: &branch, FirstAssemblyRef: &first, SecondAssemblyRef: &second})
			target := product.Product.Constraints[len(product.Product.Constraints)-1].ID
			get := func() workspace.AssemblyConstraint {
				for _, constraint := range product.Product.Constraints {
					if constraint.ID == target {
						return constraint
					}
				}
				t.Fatal("stable Contact definition missing")
				return workspace.AssemblyConstraint{}
			}
			verify := func() {
				t.Helper()
				definition := get()
				if definition.EvaluationStatus != "VERIFIED" || definition.Suppressed {
					t.Fatal("Contact not genuinely accepted", definition)
				}
				assertContactRecoveryGeometry(t, f, product, c, definition.First, *definition.Second, definition.ContactBranch)
			}
			verify()
			sourceBefore := product.Product.Instances[0].ReferencedVersionID
			partApply(workspace.CommandRequest{Type: "EDIT_PUBLICATION", PublicationID: publicationID, Name: "Renamed exact source", SemanticPurpose: "updated contact source"})
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			verify()
			if product.Product.Instances[0].ReferencedVersionID == sourceBefore || product.Product.Instances[0].ReferencedVersionID != part.Document.VersionID || get().First.PublicationResolution.ResolvedVersionID != part.Document.VersionID {
				t.Fatal("source Head update did not refresh accepted Publication evidence")
			}
			// Each branch has a real editor command and a nontrivial legal branch edit
			// where the analytic contract has one. Non-branch relations retain branch 1.
			edit := workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: target, Name: "Edited " + c.name, ContactKind: c.relation, ContactSide: c.side, ContactBranch: &branch}
			if c.name == "sphere-circle-ring" {
				branch = -1
				edit.ContactBranch = &branch
			} else if c.name == "plane-cylinder-line" || c.name == "plane-sphere-point" {
				branch = -1
				edit.ContactBranch = &branch
				edit.ContactSide = "INTERNAL"
			}
			apply(edit)
			verify()
			if get().Name != edit.Name || get().ContactBranch != branch {
				t.Fatal("atomic editor parameter/name edit lost")
			}
			branch = 1
			apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: target, ContactKind: c.relation, ContactSide: c.side, ContactBranch: &branch})
			verify()
			head := product.Document.VersionID
			poses := append([]workspace.ProductInstance(nil), product.Product.Instances...)
			invalid := int32(0)
			if _, e = f.service.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", ActorID: f.actor, TargetID: target, ContactBranch: &invalid}); e == nil {
				t.Fatal("invalid branch accepted")
			}
			unchanged, e := f.service.GetDocument(t.Context(), product.Document.ID)
			if e != nil || unchanged.Document.VersionID != head || !reflect.DeepEqual(unchanged.Product.Instances, poses) {
				t.Fatal("invalid branch changed Head/accepted pose", e)
			}
			partApply(workspace.CommandRequest{Type: "DELETE_PUBLICATION", PublicationID: publicationID})
			acceptedBeforeBreak := append([]workspace.ProductInstance(nil), product.Product.Instances...)
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			if get().EvaluationStatus != "BROKEN" || get().Suppressed || product.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
				t.Fatal("broken source was suppressed, certified or blocked unrelated Fix", get())
			}
			for i, pose := range product.Product.Instances {
				if pose.Translation != acceptedBeforeBreak[i].Translation || pose.Rotation != acceptedBeforeBreak[i].Rotation {
					t.Fatal("broken source adopted failed candidate pose")
				}
			}
			partApply(workspace.CommandRequest{Type: "UNDO"})
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			verify()
			partApply(workspace.CommandRequest{Type: "DELETE_PUBLICATION", PublicationID: publicationID})
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			// Explicit replacement uses a fresh formal bind on the accepted source
			// revision, never localId guessing or the deleted Publication's stale value.
			replacement := f.support(t, part, c.firstKind)
			replacement.InstanceID = moving
			if c.firstKind == "CIRCLE" {
				replacement.DerivedRole = "underlying-circle"
			}
			apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: target, FirstAssemblyRef: &replacement, SecondAssemblyRef: &second, ContactKind: c.relation, ContactSide: c.side, ContactBranch: &branch, Name: "Reconnected " + c.name})
			verify()
			if get().First.PublicationRef != nil || get().First.PersistentSelection == nil {
				t.Fatal("explicit Reconnect did not replace stable support source")
			}
			apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: target})
			for _, definition := range product.Product.Constraints {
				if definition.ID == target {
					t.Fatal("delete retained Contact")
				}
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			verify()
			apply(workspace.CommandRequest{Type: "REDO"})
			for _, definition := range product.Product.Constraints {
				if definition.ID == target {
					t.Fatal("Redo retained Contact")
				}
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			verify()
			cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
			read, e := cold.GetDocument(t.Context(), product.Document.ID)
			if e != nil || !reflect.DeepEqual(read.Product.Constraints, product.Product.Constraints) {
				t.Fatal("cold Contact recovery lost public definitions", e)
			}
			var digest string
			if e = f.db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&digest); e != nil {
				t.Fatal(e)
			}
			replay, e := cold.ReplayAssemblySolveManifest(t.Context(), product.Document.ID, digest, product.Document.ID+"-source-recovery-replay")
			if e != nil || replay.Status != "CONVERGED" {
				t.Fatal("frozen source recovery replay", e, replay)
			}
		})
	}
}

// Unlike the revision/metadata update above, these source edits change the
// actual exact plane and require a new accepted component pose.
func TestContactDatumPublicationGeometryUpdateThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	for _, c := range []contactRecoveryCase{
		{name: "plane-plane-face", second: "cylinder-r6-h12", secondKind: "PLANE", relation: "FACE", side: "EXTERNAL"},
		{name: "plane-cylinder-line", second: "cylinder-r6-h12", secondKind: "CYLINDER", relation: "LINE", side: "EXTERNAL"},
		{name: "plane-sphere-point", second: "sphere-r6", secondKind: "SPHERE", relation: "POINT", side: "EXTERNAL"},
	} {
		t.Run(c.name, func(t *testing.T) {
			part := f.importPart(t, "cylinder-r6-h12")
			other := f.importPart(t, c.second)
			seq := 0
			partApply := func(request workspace.CommandRequest) {
				t.Helper()
				seq++
				request.ActorID, request.RequestID = f.actor, fmt.Sprintf("%s-datum-contact-%d", part.Document.ID, seq)
				var err error
				part, err = f.service.ApplyCommand(t.Context(), part.Document.ID, request)
				if err != nil {
					t.Fatal(request.Type, err)
				}
			}
			partApply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Original contact plane", Origin: [3]float64{0, 0, 12}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			partApply(workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Contact datum", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: part.DatumPlanes[len(part.DatumPlanes)-1].ID})
			publication := part.Part.Publications[len(part.Part.Publications)-1].ID
			product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Contact geometric source " + c.name})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(request workspace.CommandRequest) {
				t.Helper()
				seq++
				request.ActorID, request.RequestID = f.actor, fmt.Sprintf("%s-datum-contact-%d", product.Document.ID, seq)
				var err error
				product, err = f.service.ApplyCommand(t.Context(), product.Document.ID, request)
				if err != nil {
					t.Fatal(request.Type, err)
				}
			}
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: other.Document.ID})
			moving, fixed := product.Product.Instances[0].ID, product.Product.Instances[1].ID
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{19, -7, 24}, Rotation: [4]float64{0, 0, 0, 1}})
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: fixed, Kind: "BODY"}})
			first := workspace.AssemblyGeometryRef{InstanceID: moving, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: publication, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}}
			second := f.support(t, other, c.secondKind)
			second.InstanceID = fixed
			branch := int32(1)
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONTACT", ContactKind: c.relation, ContactSide: c.side, ContactBranch: &branch, FirstAssemblyRef: &first, SecondAssemblyRef: &second})
			verify := func(z float64) {
				t.Helper()
				definition := product.Product.Constraints[len(product.Product.Constraints)-1]
				if definition.EvaluationStatus != "VERIFIED" || definition.Suppressed {
					t.Fatal("changed Datum Contact not accepted", definition)
				}
				assertContactRecoveryGeometry(t, f, product, c, definition.First, *definition.Second, branch)
				inspection, err := f.service.InspectAssemblySupports(t.Context(), product.Document.ID, []workspace.AssemblyGeometryRef{definition.First})
				if err != nil || len(inspection.Supports) != 1 || inspection.Supports[0].Descriptor == nil {
					t.Fatal("missing redirected descriptor", err)
				}
				if math.Abs(inspection.Supports[0].Descriptor.Origin[2]-z) > 1e-9 || definition.First.PublicationResolution == nil || definition.First.PublicationResolution.ResolvedVersionID != part.Document.VersionID || product.Product.Instances[0].ReferencedVersionID != part.Document.VersionID {
					t.Fatal("source geometry/revision not actually replaced", inspection, definition.First)
				}
			}
			verify(12)
			before := product.Product.Instances[0]
			partApply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Shifted contact plane", Origin: [3]float64{0, 0, 15}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			partApply(workspace.CommandRequest{Type: "REDIRECT_PUBLICATION", PublicationID: publication, PublicationType: "PLANE", TargetKind: "PLANE", TargetID: part.DatumPlanes[len(part.DatumPlanes)-1].ID})
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			verify(15)
			if compositionNorm(compositionSub(before.Translation, product.Product.Instances[0].Translation)) < compositionAcceptanceProfile(t, f, product.Document.VersionID).LengthTolerance {
				t.Fatal("geometry source changed but accepted component never moved")
			}
			partApply(workspace.CommandRequest{Type: "UNDO"})
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			verify(12)
			partApply(workspace.CommandRequest{Type: "REDO"})
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			verify(15)
		})
	}
}

func TestContactMultiBodySharedNestedPublicationThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "sphere-r6")
	seq := 0
	partApply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-contact-multibody-part-%d", part.Document.ID, seq)
		var err error
		part, err = f.service.ApplyCommand(t.Context(), part.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	sphereBody := part.Part.Bodies[0].ID
	partApply(workspace.CommandRequest{Type: "CREATE_BODY", Name: "Plane support CAD Body"})
	planeBody := part.Part.ActiveBodyID
	partApply(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: planeBody, Plane: "XY"})
	sketch := part.Part.Features[len(part.Part.Features)-1].ID
	partApply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 30, Y: 0}, Second: &workspace.SketchPoint2{X: 40, Y: 10}}}})
	partApply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: planeBody, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 12})
	if len(part.Part.Bodies) != 2 {
		t.Fatal("expected two real CAD Bodies")
	}
	var spherePublication, planePublication string
	for _, support := range []struct {
		body, kind string
		target     *string
	}{{sphereBody, "SPHERE", &spherePublication}, {planeBody, "PLANE", &planePublication}} {
		pick := f.contactPick(t, part, support.body, support.kind)
		partApply(workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: support.kind + " support", PublicationType: "SURFACE", TargetKind: pick.Kind, GeometryKey: pick.GeometryKey, TopologyID: pick.TopologyID, VersionID: part.Document.VersionID})
		*support.target = part.Part.Publications[len(part.Part.Publications)-1].ID
	}
	child, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Nested rigid Contact source"})
	if err != nil {
		t.Fatal(err)
	}
	childApply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-contact-child-%d", child.Document.ID, seq)
		child, err = f.service.ApplyCommand(t.Context(), child.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	childApply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	leaf := child.Product.Instances[0].ID
	childApply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{5, 2, -3}, Rotation: [4]float64{0, 0, math.Sin(.2), math.Cos(.2)}})
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Shared multiBody nested Contact"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-contact-nested-%d", product.Document.ID, seq)
		product, err = f.service.ApplyCommand(t.Context(), product.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	outer, direct := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: outer, Translation: [3]float64{27, -9, 31}, Rotation: [4]float64{math.Sin(.13), 0, 0, math.Cos(.13)}})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: direct, Kind: "BODY"}})
	refs := func() (workspace.AssemblyGeometryRef, workspace.AssemblyGeometryRef) {
		path := workspace.InstancePath{RootDocumentID: product.Document.ID, Segments: []workspace.InstancePathSegment{{OwnerDocumentID: product.Document.ID, OwnerVersionID: product.Document.VersionID, InstanceID: outer, ReferencedDocumentID: child.Document.ID, ResolvedVersionID: child.Document.VersionID}, {OwnerDocumentID: child.Document.ID, OwnerVersionID: child.Document.VersionID, InstanceID: leaf, ReferencedDocumentID: part.Document.ID, ResolvedVersionID: part.Document.VersionID}}}
		return workspace.AssemblyGeometryRef{InstanceID: outer, InstancePath: &path, Kind: "SPHERE", PublicationRef: &workspace.PublicationRef{PublicationID: spherePublication, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}}, workspace.AssemblyGeometryRef{InstanceID: direct, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: planePublication, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}}
	}
	first, second := refs()
	inspection, err := f.service.InspectAssemblySupports(t.Context(), product.Document.ID, []workspace.AssemblyGeometryRef{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Supports[0].Status != "RESOLVED" || inspection.Supports[1].Status != "RESOLVED" || inspection.Supports[0].Descriptor.Origin != [3]float64{5, 2, -3} || inspection.Supports[0].Descriptor.BodyID != outer || inspection.Supports[1].Descriptor.BodyID != direct || math.Abs(inspection.Supports[1].Descriptor.Origin[2]-12) > 1e-9 {
		t.Fatalf("shared multiBody nested supports mixed or transformed twice: %+v", inspection)
	}
	branch := int32(1)
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONTACT", ContactKind: "POINT", ContactSide: "EXTERNAL", ContactBranch: &branch, FirstAssemblyRef: &first, SecondAssemblyRef: &second})
	target := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	c := contactRecoveryCase{name: "plane-sphere-point", relation: "POINT"}
	get := func() workspace.AssemblyConstraint {
		for _, def := range product.Product.Constraints {
			if def.ID == target {
				return def
			}
		}
		t.Fatal("nested Contact missing")
		return workspace.AssemblyConstraint{}
	}
	verify := func() {
		t.Helper()
		def := get()
		if def.EvaluationStatus != "VERIFIED" || def.First.InstancePath == nil || len(def.First.InstancePath.Segments) != 2 {
			t.Fatal("nested Contact not accepted/full occurrence lost", def)
		}
		if def.First.PublicationResolution.GeometryKey == def.Second.PublicationResolution.GeometryKey {
			t.Fatal("distinct CAD Body publications share wrong artifact")
		}
		assertContactRecoveryGeometry(t, f, product, c, def.First, *def.Second, 1)
	}
	verify()
	var raw []byte
	if err = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var manifest workspace.AssemblySolveManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Bodies) != 2 {
		t.Fatalf("CAD Bodies incorrectly became independent solver bodies: %d", len(manifest.Bodies))
	}
	for _, body := range manifest.Bodies {
		if body.ID != outer && body.ID != direct {
			t.Fatal("solver body identity is not direct occurrence", body.ID)
		}
	}
	update := func() {
		childApply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
		apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	}
	partApply(workspace.CommandRequest{Type: "EDIT_PUBLICATION", PublicationID: spherePublication, Name: "Renamed nested sphere"})
	update()
	verify()
	partApply(workspace.CommandRequest{Type: "DELETE_PUBLICATION", PublicationID: spherePublication})
	update()
	if get().EvaluationStatus != "BROKEN" || get().Suppressed {
		t.Fatal("deleted nested publication did not break independently")
	}
	partApply(workspace.CommandRequest{Type: "UNDO"})
	update()
	verify()
	first, second = refs()
	first.DerivedRole = "sphere-center"
	pointInspection, err := f.service.InspectAssemblySupports(t.Context(), product.Document.ID, []workspace.AssemblyGeometryRef{first})
	if err != nil || pointInspection.Supports[0].ExactType != "POINT" || pointInspection.Supports[0].Descriptor.Origin != [3]float64{5, 2, -3} {
		t.Fatal("stable derived sphere centre did not preserve exact source/owning transform", err, pointInspection)
	}
}
