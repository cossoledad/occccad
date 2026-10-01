package control

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

type fourFamilyCase struct{ capability, family, first, second, relation string }

// Each subtest names a concrete capability, not a shared foundation that claims
// every geometrical combination. All source supports are actual imported BREP
// or persisted Part frame/derived supports, resolved by the authoritative path.
func TestFourFamiliesCapabilityLifecycleThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	parts := map[string]workspace.DocumentView{}
	for _, fixture := range []string{"cylinder-r6-h12", "sphere-r6", "cone-r9-r3-h12"} {
		parts[fixture] = f.importPart(t, fixture)
	}
	cases := []fourFamilyCase{
		{"coincidence.point-point", "Coincidence", "POINT", "POINT", ""},
		{"coincidence.point-axis", "Coincidence", "POINT", "AXIS", ""},
		{"coincidence.point-plane", "Coincidence", "POINT", "PLANE", ""},
		{"coincidence.axis-axis", "Coincidence", "AXIS", "AXIS", ""},
		{"coincidence.axis-plane", "Coincidence", "AXIS", "PLANE", ""},
		{"coincidence.plane-plane", "Coincidence", "PLANE", "PLANE", ""},
		{"coincidence.cylinder-axis", "Coincidence", "CYLINDER", "AXIS", ""},
		{"coincidence.cylinder-cylinder-axis", "Coincidence", "CYLINDER", "CYLINDER", ""},
		{"coincidence.frame-frame", "Coincidence", "FRAME", "FRAME", ""},
		{"coincidence.point-curve/circle", "Coincidence", "POINT", "CIRCLE", ""},
		{"coincidence.point-curve/line", "Coincidence", "POINT", "AXIS", ""},
		{"coincidence.point-surface/plane", "Coincidence", "POINT", "PLANE", ""},
		{"coincidence.point-surface/sphere", "Coincidence", "POINT", "SPHERE", ""},
		{"coincidence.point-surface/cylinder", "Coincidence", "POINT", "CYLINDER", ""},
		{"coincidence.point-surface/cone", "Coincidence", "POINT", "CONE", ""},
	}
	for _, pair := range [][2]string{{"POINT", "POINT"}, {"POINT", "AXIS"}, {"POINT", "PLANE"}, {"AXIS", "AXIS"}, {"AXIS", "PLANE"}, {"PLANE", "PLANE"}} {
		cases = append(cases, fourFamilyCase{"offset." + strings.ToLower(pair[0]) + "-" + strings.ToLower(pair[1]), "Offset", pair[0], pair[1], ""})
	}
	for _, relation := range []string{"FREE", "DIRECTED", "PARALLEL", "PERPENDICULAR"} {
		for _, pair := range [][2]string{{"AXIS", "AXIS"}, {"AXIS", "PLANE"}, {"AXIS", "CYLINDER"}, {"PLANE", "PLANE"}, {"PLANE", "CYLINDER"}, {"CYLINDER", "CYLINDER"}} {
			cases = append(cases, fourFamilyCase{"angle." + strings.ToLower(relation) + "." + strings.ToLower(pair[0]) + "-" + strings.ToLower(pair[1]), "Angle", pair[0], pair[1], relation})
		}
	}
	for _, mode := range []string{"SPACE", "RELATIVE"} {
		cases = append(cases, fourFamilyCase{"fix." + strings.ToLower(mode), "Fix", "BODY", "", mode})
	}
	for _, test := range cases {
		t.Run(test.capability, func(t *testing.T) { runFourFamilyCase(t, f, parts, test) })
	}
}

func fourFamilySource(t *testing.T, f *compositionControlFixture, parts map[string]workspace.DocumentView, kind string) (workspace.DocumentView, workspace.AssemblyGeometryRef) {
	t.Helper()
	part := parts["cylinder-r6-h12"]
	switch kind {
	case "POINT":
		part = parts["sphere-r6"]
		ref := f.support(t, part, "SPHERE")
		ref.DerivedRole = "sphere-center"
		return part, ref
	case "AXIS":
		ref := f.support(t, part, "CYLINDER")
		ref.DerivedRole = "cylinder-axis"
		return part, ref
	case "SPHERE":
		part = parts["sphere-r6"]
	case "CONE":
		part = parts["cone-r9-r3-h12"]
	case "FRAME":
		return part, workspace.AssemblyGeometryRef{Kind: "FRAME", GeometryID: "axis-system-default"}
	case "BODY":
		return part, workspace.AssemblyGeometryRef{Kind: "BODY"}
	}
	return part, f.support(t, part, kind)
}

func runFourFamilyCase(t *testing.T, f *compositionControlFixture, parts map[string]workspace.DocumentView, test fourFamilyCase) {
	t.Helper()
	firstPart, first := fourFamilySource(t, f, parts, test.first)
	secondPart := firstPart
	var second workspace.AssemblyGeometryRef
	if test.second != "" {
		secondPart, second = fourFamilySource(t, f, parts, test.second)
	}
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: test.capability})
	if err != nil {
		t.Fatal(err)
	}
	id, sequence := product.Document.ID, 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		sequence++
		r.ActorID = f.actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-four-%d", id, sequence)
		}
		product, err = f.service.ApplyCommand(t.Context(), id, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", Name: "Moving", ReferencedDocumentID: firstPart.Document.ID})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", Name: "Reference", ReferencedDocumentID: secondPart.Document.ID})
	moving, reference := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	refRotation := [4]float64{0, math.Sin(.17), 0, math.Cos(.17)}
	q := [4]float64{math.Cos(.17) * math.Sin(.2), math.Sin(.17) * math.Cos(.2), -math.Sin(.17) * math.Sin(.2), math.Cos(.17) * math.Cos(.2)}
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: reference, Translation: [3]float64{7, -11, 5}, Rotation: refRotation})
	displacement := compositionRotate(refRotation, [3]float64{13, -2, 7})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{7 + displacement[0], -11 + displacement[1], 5 + displacement[2]}, Rotation: q})
	first.InstanceID, second.InstanceID = moving, reference
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: reference, Kind: "BODY"}})
	request := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-create", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintFamily: test.family, FirstAssemblyRef: &first, DirectionRelation: "UNDEFINED"}
	if test.second != "" {
		request.SecondAssemblyRef = &second
	}
	switch test.family {
	case "Coincidence":
		request.ConstraintKind = "COINCIDENT"
		if strings.Contains(test.capability, "/") {
			request.ConstraintSubtype = strings.TrimPrefix(strings.Split(test.capability, "/")[0], "coincidence.")
		}
	case "Offset":
		request.ConstraintKind = "DISTANCE"
		request.Value = 8
		if test.first == "PLANE" || test.second == "PLANE" {
			request.DistanceRelation = "SELECTED_PLANE_NORMAL_V1"
		}
	case "Angle":
		request.ConstraintKind = "ANGLE"
		request.AngleRelation = test.relation
		request.Value = .7
		if test.relation == "DIRECTED" {
			axis := workspace.AssemblyGeometryRef{InstanceID: reference, Kind: "FRAME", GeometryID: "axis-system-default", DerivedRole: "frame-axis-x"}
			request.AngleAxis = &axis
		}
	case "Fix":
		request.ConstraintKind = "FIX"
		request.FixMode = test.relation
		request.DirectionRelation = ""
	}
	initial := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	head := product.Document.VersionID
	preview, e := f.service.PreviewCommand(t.Context(), id, request)
	if e != nil || preview.ConstraintEvaluation == nil || preview.ConstraintEvaluation.Status != "VERIFIED" {
		t.Fatalf("concrete capability preview: error=%v evaluation=%+v build=%s", e, preview.ConstraintEvaluation, preview.AssemblySolverBuild)
	}
	read, e := f.service.GetDocument(t.Context(), id)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("Preview advanced Head", e)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	target := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	definition := func() workspace.AssemblyConstraint {
		t.Helper()
		for _, c := range product.Product.Constraints {
			if c.ID == target {
				return c
			}
		}
		t.Fatal("definition identity lost")
		return workspace.AssemblyConstraint{}
	}
	if definition().Family != test.family || definition().DefinitionVersion != 2 || definition().Subtype == "" || definition().EvaluationStatus != "VERIFIED" {
		t.Fatal("public definition not authoritative", definition())
	}
	assertFourFamilyGeometry(t, f, id, product, test, first, second, definition().Value)
	if test.family != "Fix" && reflect.DeepEqual(initial, product.Product.Instances) {
		t.Fatal("initially infeasible case never adopted movement")
	}
	edit := workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: target, DirectionRelation: "UNDEFINED", Name: "Edited " + test.capability}
	if test.family == "Offset" || (test.family == "Angle" && (test.relation == "FREE" || test.relation == "DIRECTED")) {
		expression := "10 mm"
		if test.family == "Angle" {
			expression = "50 deg"
		}
		edit.QuantityExpression = &expression
	} else if test.family == "Fix" {
		pose := workspace.InstancePose{Translation: [3]float64{13, 4, -2}, Rotation: [4]float64{0, math.Sin(.13), 0, math.Cos(.13)}}
		edit.FixedPose = &pose
		edit.FixMode = test.relation
		edit.DirectionRelation = ""
	} else {
		edit.Value = request.Value
		edit.AngleRelation = test.relation
	}
	apply(edit)
	if definition().EvaluationStatus != "VERIFIED" {
		t.Fatal("edit not satisfied", definition())
	}
	if definition().Name != edit.Name {
		t.Fatal("atomic rename was not persisted", definition())
	}
	assertFourFamilyGeometry(t, f, id, product, test, first, second, definition().Value)
	measurable := test.family == "Offset" || (test.family == "Angle" && (test.relation == "FREE" || test.relation == "DIRECTED"))
	if measurable {
		before := append([]workspace.ProductInstance(nil), product.Product.Instances...)
		original := definition()
		mode := "MEASURED"
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{target}, ConstraintMode: &mode})
		if definition().Mode != "MEASURED" || definition().MeasuredValue == nil || !reflect.DeepEqual(before, product.Product.Instances) || definition().Value != original.Value || !reflect.DeepEqual(definition().QuantityParameter, original.QuantityParameter) {
			t.Fatal("Measured drove/overwrote definition or lacks value", definition())
		}
		measurementTolerance := compositionAcceptanceProfile(t, f, product.Document.VersionID).LengthTolerance
		if test.family == "Angle" {
			measurementTolerance = compositionAcceptanceProfile(t, f, product.Document.VersionID).AngleTolerance
		}
		if math.Abs(*definition().MeasuredValue-original.Value) > measurementTolerance {
			t.Fatal("measurement disagrees with independently verified adopted geometry", definition())
		}
		if test.family == "Offset" && (test.first == "AXIS" || test.first == "PLANE") && test.second == "PLANE" {
			invalidRotation := refRotation
			if test.first == "PLANE" {
				invalidRotation = q
			}
			position := product.Product.Instances[0].Translation
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: position, Rotation: invalidRotation})
			if definition().Mode != "MEASURED" || definition().MeasuredValue != nil || definition().Value != original.Value || !strings.Contains(definition().EvaluationSummary, "measurement undefined") {
				t.Fatal("invalid constant-offset measurement retained stale value, drove, or destroyed target", definition())
			}
			if product.Product.Instances[0].Translation != position {
				t.Fatal("measurement moved component during explicit movement")
			}
		}
		for _, suppressed := range []bool{true, false} {
			apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{target}, Suppressed: &suppressed})
			if definition().Mode != "MEASURED" {
				t.Fatal("activation lost measured mode")
			}
		}
		mode = "DRIVING"
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{target}, ConstraintMode: &mode})
		if definition().Value != original.Value || !reflect.DeepEqual(definition().QuantityParameter, original.QuantityParameter) {
			t.Fatal("Driving failed to restore retained expression")
		}
		if definition().EvaluationStatus != "VERIFIED" {
			t.Fatal("Driving restoration did not re-solve retained definition", definition())
		}
	} else {
		mode := "MEASURED"
		head = product.Document.VersionID
		if _, e = f.service.ApplyCommand(t.Context(), id, workspace.CommandRequest{ActorID: f.actor, Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{target}, ConstraintMode: &mode}); e == nil {
			t.Fatal("unsupported Measure accepted")
		}
		read, e = f.service.GetDocument(t.Context(), id)
		if e != nil || read.Document.VersionID != head {
			t.Fatal("invalid mode advanced Head", e)
		}
	}
	for _, suppressed := range []bool{true, false} {
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{target}, Suppressed: &suppressed})
		if definition().Suppressed != suppressed {
			t.Fatal("activation state lost")
		}
	}
	if definition().EvaluationStatus != "VERIFIED" {
		t.Fatal("reactivation did not verify concrete capability", definition())
	}
	assertFourFamilyGeometry(t, f, id, product, test, first, second, definition().Value)
	cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
	read, e = cold.GetDocument(t.Context(), id)
	if e != nil || !reflect.DeepEqual(read.Product.Constraints, product.Product.Constraints) {
		t.Fatal("cold read changed definition", e)
	}
	var digest string
	if e = f.db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&digest); e != nil {
		t.Fatal(e)
	}
	replayed, e := cold.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-cold-replay")
	if e != nil || replayed.Status != "CONVERGED" {
		t.Fatal("concrete frozen replay", e, replayed)
	}
	expectedRank := fourFamilyExpectedRank(t, test.capability)
	foundComponent := false
	for _, component := range replayed.Result.Components {
		for _, body := range component.BodyIDs {
			if body == moving {
				foundComponent = true
				if test.family == "Fix" {
					if component.TangentVariableCount != 0 || component.RelativeDof != 0 || component.GaugeDof != 0 {
						t.Fatal("Fix did not eliminate moving unit", component)
					}
				} else if component.JacobianRank != uint64(expectedRank) || component.RelativeDof != uint64(6-expectedRank) || component.GaugeDof != 0 {
					t.Fatal("independent rank/relative DOF do not match frozen contract", expectedRank, component)
				}
			}
		}
	}
	if !foundComponent {
		t.Fatal("frozen solve omitted moving motion evidence")
	}
	release, e := cold.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{ActorID: f.actor, RequestID: id + "-release", Name: test.capability})
	if e != nil {
		t.Fatal("release", e)
	}
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{target}, Suppressed: &suppressed})
	apply(workspace.CommandRequest{Type: "UNDO"})
	if definition().Suppressed {
		t.Fatal("Undo lost activation")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if !definition().Suppressed {
		t.Fatal("Redo lost activation")
	}
	frozen, e := cold.ReplayProductRelease(t.Context(), id, release.ID, id+"-release-replay")
	if e != nil || frozen.Assembly == nil {
		t.Fatal("frozen Release", e)
	}
	var raw []byte
	if e = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, frozen.Assembly.ManifestDigest).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var manifest workspace.AssemblySolveManifest
	if e = json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range manifest.Definitions {
		if c.ID == target {
			found = true
			if c.Suppressed || c.Family != test.family || c.Value != definition().Value {
				t.Fatal("Head overwrote frozen Release definition", c)
			}
		}
	}
	if !found {
		t.Fatal("Release lost concrete definition")
	}
}

func fourFamilyExpectedRank(t *testing.T, capability string) int {
	t.Helper()
	// Read the logical single catalog, rather than maintaining another expected
	// rank matrix in this Go adapter or deriving it from solver observations.
	raw, e := os.ReadFile(filepath.Join("..", "assemblycontract", "catalog.json"))
	if e != nil {
		t.Fatal(e)
	}
	var catalog struct {
		Capabilities []struct {
			ID   string `json:"capabilityId"`
			Rank struct {
				Normal *int `json:"normal"`
			} `json:"rank"`
		} `json:"capabilities"`
	}
	if e = json.Unmarshal(raw, &catalog); e != nil {
		t.Fatal(e)
	}
	id := strings.Split(capability, "/")[0]
	for _, c := range catalog.Capabilities {
		if c.ID == id {
			if c.Rank.Normal == nil {
				t.Fatal("normal rank has not been frozen", id)
			}
			return *c.Rank.Normal
		}
	}
	t.Fatal("concrete test capability absent from single catalog", id)
	return 0
}

func assertFourFamilyGeometry(t *testing.T, f *compositionControlFixture, id string, p workspace.DocumentView, c fourFamilyCase, first, second workspace.AssemblyGeometryRef, target float64) {
	t.Helper()
	if first.PublicationRef != nil || second.PublicationRef != nil {
		for _, definition := range p.Product.Constraints {
			if definition.Family == c.family && definition.First.InstanceID == first.InstanceID && definition.Second != nil && definition.Second.InstanceID == second.InstanceID {
				first, second = definition.First, *definition.Second
				break
			}
		}
	}
	poses := map[string]workspace.ProductInstance{}
	for _, body := range p.Product.Instances {
		poses[body.ID] = body
	}
	if c.family == "Fix" {
		definition := p.Product.Constraints[len(p.Product.Constraints)-1]
		profile := compositionAcceptanceProfile(t, f, p.Document.VersionID)
		if definition.FixedPose == nil || compositionNorm(compositionSub(poses[first.InstanceID].Translation, definition.FixedPose.Translation)) > profile.LengthTolerance {
			t.Fatal("Fix capture not adopted", definition)
		}
		positive, negative := 0.0, 0.0
		for i, v := range poses[first.InstanceID].Rotation {
			positive += math.Pow(v-definition.FixedPose.Rotation[i], 2)
			negative += math.Pow(v+definition.FixedPose.Rotation[i], 2)
		}
		// A unit quaternion chord of 2*sin(theta/4) corresponds to theta
		// radians of physical rotation; q and -q denote the same orientation.
		if math.Sqrt(math.Min(positive, negative)) > 2*math.Sin(profile.AngleTolerance/4) {
			t.Fatal("Fix captured orientation not adopted", definition)
		}
		return
	}
	inspected, e := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{first, second})
	if e != nil || len(inspected.Supports) != 2 || inspected.Supports[0].Descriptor == nil || inspected.Supports[1].Descriptor == nil {
		t.Fatal("exact oracle supports", e, inspected)
	}
	a, b := *inspected.Supports[0].Descriptor, *inspected.Supports[1].Descriptor
	pa, pb := poses[first.InstanceID], poses[second.InstanceID]
	x, y := compositionPoint(pa, a.Origin), compositionPoint(pb, b.Origin)
	u, v := compositionRotate(pa.Rotation, a.Direction), compositionRotate(pb.Rotation, b.Direction)
	delta := compositionSub(x, y)
	dot := func(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	cross := func(a, b [3]float64) [3]float64 {
		return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
	}
	profile := compositionAcceptanceProfile(t, f, p.Document.VersionID)
	checkTolerance := func(value float64, label string, tolerance float64) {
		t.Helper()
		if math.Abs(value) > tolerance {
			t.Fatal(label, value, "geometry", a, b, "poses", pa, pb)
		}
	}
	check := func(value float64, label string) { checkTolerance(value, label, profile.LengthTolerance) }
	checkAngle := func(value float64, label string) { checkTolerance(value, label, profile.AngleTolerance) }
	if c.family == "Angle" {
		switch c.relation {
		case "PARALLEL":
			checkAngle(math.Asin(math.Min(1, compositionNorm(cross(u, v)))), "parallel directions")
		case "PERPENDICULAR":
			checkAngle(math.Asin(math.Min(1, math.Abs(dot(u, v)))), "perpendicular directions")
		case "FREE":
			checkAngle(math.Atan2(compositionNorm(cross(u, v)), dot(u, v))-math.Min(target, 2*math.Pi-target), "free spatial angle")
		case "DIRECTED":
			k := compositionRotate(pb.Rotation, [3]float64{1, 0, 0})
			for i := range u {
				u[i] -= dot(compositionRotate(pa.Rotation, a.Direction), k) * k[i]
				v[i] -= dot(compositionRotate(pb.Rotation, b.Direction), k) * k[i]
			}
			if compositionNorm(u) < 1e-8 || compositionNorm(v) < 1e-8 {
				t.Fatal("projected angle undefined")
			}
			angle := math.Atan2(dot(k, cross(u, v)), dot(u, v))
			difference := math.Atan2(math.Sin(angle-target), math.Cos(angle-target))
			checkAngle(difference, "directed projected angle")
		}
		return
	}
	if c.family == "Offset" {
		value := 0.0
		switch {
		case a.Kind == "PLANE" || b.Kind == "PLANE":
			n := u
			if a.Kind != "PLANE" {
				n = v
			}
			value = dot(n, delta)
			if a.Kind == "AXIS" {
				checkAngle(math.Asin(math.Min(1, math.Abs(dot(u, n)))), "line must be parallel plane")
			}
			if b.Kind == "AXIS" {
				checkAngle(math.Asin(math.Min(1, math.Abs(dot(v, n)))), "line must be parallel plane")
			}
			if a.Kind == "PLANE" && b.Kind == "PLANE" {
				checkAngle(math.Asin(math.Min(1, compositionNorm(cross(u, v)))), "offset planes parallel")
			}
		case a.Kind == "POINT" && b.Kind == "POINT":
			value = compositionNorm(delta)
		case a.Kind == "POINT" && b.Kind == "AXIS":
			value = compositionNorm(cross(delta, v))
		case a.Kind == "AXIS" && b.Kind == "AXIS":
			n := cross(u, v)
			if compositionNorm(n) > 1e-9 {
				value = math.Abs(dot(delta, n)) / compositionNorm(n)
			} else {
				value = compositionNorm(cross(delta, u))
			}
		default:
			t.Fatal("oracle lacks real Offset pair", a.Kind, b.Kind)
		}
		check(value-target, "actual Offset distance")
		return
	}
	switch {
	case a.Kind == "POINT" && b.Kind == "POINT":
		check(compositionNorm(delta), "point coincidence")
	case a.Kind == "POINT" && b.Kind == "AXIS":
		check(compositionNorm(cross(delta, v)), "point on infinite line")
	case a.Kind == "POINT" && b.Kind == "PLANE":
		check(dot(delta, v), "point on plane")
	case a.Kind == "POINT" && b.Kind == "CIRCLE":
		check(dot(delta, v), "point in circle plane")
		check(compositionNorm(delta)-b.Radius, "point on exact circle")
	case a.Kind == "POINT" && b.Kind == "SPHERE":
		check(compositionNorm(delta)-b.Radius, "point on sphere")
	case a.Kind == "POINT" && b.Kind == "CYLINDER":
		check(compositionNorm(cross(delta, v))-b.Radius, "point on cylinder")
	case a.Kind == "POINT" && b.Kind == "CONE":
		check(compositionNorm(cross(delta, v))-dot(delta, v)*float64(b.ConeLeaf)*math.Tan(b.HalfAngle), "point on selected cone leaf")
	case a.Kind == "FRAME" && b.Kind == "FRAME":
		check(compositionNorm(delta), "frame origin")
		checkAngle(2*math.Asin(math.Min(1, compositionNorm(compositionSub(u, v))/2)), "frame Z")
		checkAngle(2*math.Asin(math.Min(1, compositionNorm(compositionSub(compositionRotate(pa.Rotation, a.XDirection), compositionRotate(pb.Rotation, b.XDirection)))/2)), "frame X")
	case a.Kind == "PLANE" && b.Kind == "PLANE":
		check(dot(delta, v), "coplanar origin")
		checkAngle(math.Asin(math.Min(1, compositionNorm(cross(u, v)))), "coplanar normal")
	case a.Kind == "AXIS" && b.Kind == "PLANE":
		check(dot(delta, v), "line origin in plane")
		checkAngle(math.Asin(math.Min(1, math.Abs(dot(u, v)))), "line direction in plane")
	case (a.Kind == "AXIS" || a.Kind == "CYLINDER") && (b.Kind == "AXIS" || b.Kind == "CYLINDER"):
		check(compositionNorm(cross(delta, v)), "coaxial origin")
		checkAngle(math.Asin(math.Min(1, compositionNorm(cross(u, v)))), "coaxial direction")
	default:
		t.Fatal("oracle lacks real Coincidence pair", a.Kind, b.Kind)
	}
}

// Representative reference recovery is deliberately separate from the concrete
// pair matrix: it certifies these named Publication/Datum/nested/multi-Body
// scenarios, not every source provenance of every capability.
func TestFourFamiliesPublicationNestedMultiBodyRecoveryThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	for _, family := range []string{"Coincidence", "Offset", "Angle"} {
		t.Run(family, func(t *testing.T) {
			part := f.importPart(t, "cylinder-r6-h12")
			seq := 0
			partApply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = f.actor
				r.RequestID = fmt.Sprintf("%s-source-%d", part.Document.ID, seq)
				next, e := f.service.ApplyCommand(t.Context(), part.Document.ID, r)
				if e != nil {
					t.Fatal(r.Type, e)
				}
				part = next
			}
			partApply(workspace.CommandRequest{Type: "CREATE_BODY"})
			body := part.Part.ActiveBodyID
			partApply(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
			sketch := part.Part.Features[len(part.Part.Features)-1].ID
			partApply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 40, Y: 0}, Second: &workspace.SketchPoint2{X: 50, Y: 10}}}})
			partApply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
			if len(part.Part.Bodies) != 2 {
				t.Fatal("multi CAD Body fixture missing")
			}
			partApply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Mount datum", Origin: [3]float64{3, 2, 7}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			datum := part.DatumPlanes[len(part.DatumPlanes)-1].ID
			partApply(workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mounting plane", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: datum})
			publication := part.Part.Publications[len(part.Part.Publications)-1].ID
			child, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Rigid nested source"})
			if e != nil {
				t.Fatal(e)
			}
			child, e = f.service.ApplyCommand(t.Context(), child.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			if e != nil {
				t.Fatal(e)
			}
			leaf := child.Product.Instances[0].ID
			child, e = f.service.ApplyCommand(t.Context(), child.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{4, 2, 3}, Rotation: [4]float64{0, 0, math.Sin(.11), math.Cos(.11)}})
			if e != nil {
				t.Fatal(e)
			}
			product, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: family + " recovery"})
			if e != nil {
				t.Fatal(e)
			}
			id := product.Document.ID
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = f.actor
				r.RequestID = fmt.Sprintf("%s-recovery-%d", id, seq)
				var err error
				product, err = f.service.ApplyCommand(t.Context(), id, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			outer, direct := product.Product.Instances[0].ID, product.Product.Instances[1].ID
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: outer, Translation: [3]float64{31, -4, 7}, Rotation: [4]float64{math.Sin(.2), 0, 0, math.Cos(.2)}})
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: direct, Kind: "BODY"}})
			refs := func(pub string, kind string) (workspace.AssemblyGeometryRef, workspace.AssemblyGeometryRef) {
				path := workspace.InstancePath{RootDocumentID: id, Segments: []workspace.InstancePathSegment{{OwnerDocumentID: id, OwnerVersionID: product.Document.VersionID, InstanceID: outer, ReferencedDocumentID: child.Document.ID, ResolvedVersionID: child.Document.VersionID}, {OwnerDocumentID: child.Document.ID, OwnerVersionID: child.Document.VersionID, InstanceID: leaf, ReferencedDocumentID: part.Document.ID, ResolvedVersionID: part.Document.VersionID}}}
				a := workspace.AssemblyGeometryRef{InstanceID: outer, InstancePath: &path, Kind: kind}
				b := workspace.AssemblyGeometryRef{InstanceID: direct, Kind: kind}
				if kind == "PUBLICATION" {
					a.Kind, b.Kind = "PLANE", "PLANE"
					a.PublicationRef = &workspace.PublicationRef{PublicationID: pub, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}
					b.PublicationRef = &workspace.PublicationRef{PublicationID: pub, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}
				} else {
					a.GeometryID, b.GeometryID = pub, pub
				}
				return a, b
			}
			a, b := refs(publication, "PUBLICATION")
			r := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintFamily: family, FirstAssemblyRef: &a, SecondAssemblyRef: &b, DirectionRelation: "UNDEFINED"}
			c := fourFamilyCase{family: family, first: "PLANE", second: "PLANE"}
			switch family {
			case "Coincidence":
				r.ConstraintKind = "COINCIDENT"
			case "Offset":
				r.ConstraintKind = "DISTANCE"
				r.Value = 8
				r.DistanceRelation = "SELECTED_PLANE_NORMAL_V1"
			case "Angle":
				r.ConstraintKind = "ANGLE"
				r.AngleRelation = "FREE"
				r.Value = .7
				c.relation = "FREE"
			}
			apply(r)
			target := product.Product.Constraints[len(product.Product.Constraints)-1].ID
			get := func() workspace.AssemblyConstraint {
				for _, constraint := range product.Product.Constraints {
					if constraint.ID == target {
						return constraint
					}
				}
				t.Fatal("lost recovery definition")
				return workspace.AssemblyConstraint{}
			}
			if get().EvaluationStatus != "VERIFIED" {
				t.Fatal("nested multi-Body public support not satisfied", get())
			}
			assertFourFamilyGeometry(t, f, id, product, c, a, b, get().Value)
			partApply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Redirected datum", Origin: [3]float64{6, -3, 9}, Normal: [3]float64{0, 1, 0}, UDirection: [3]float64{1, 0, 0}})
			replacement := part.DatumPlanes[len(part.DatumPlanes)-1].ID
			partApply(workspace.CommandRequest{Type: "REDIRECT_PUBLICATION", PublicationID: publication, PublicationType: "PLANE", TargetKind: "PLANE", TargetID: replacement})
			update := func() {
				var err error
				child, err = f.service.ApplyCommand(t.Context(), child.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "UPDATE_REFERENCES"})
				if err != nil {
					t.Fatal("nested source update", err)
				}
				apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			}
			update()
			a, b = refs(publication, "PUBLICATION")
			if get().EvaluationStatus != "VERIFIED" {
				t.Fatal("redirected source not re-resolved", get())
			}
			assertFourFamilyGeometry(t, f, id, product, c, a, b, get().Value)
			partApply(workspace.CommandRequest{Type: "DELETE_PUBLICATION", PublicationID: publication})
			update()
			if get().EvaluationStatus != "BROKEN" || get().Suppressed {
				t.Fatal("deleted publication did not remain orthogonal Broken definition", get())
			}
			if product.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
				t.Fatal("Broken definition blocked unrelated Fix")
			}
			partApply(workspace.CommandRequest{Type: "UNDO"})
			update()
			a, b = refs(publication, "PUBLICATION")
			if get().EvaluationStatus != "VERIFIED" {
				t.Fatal("stable Publication Undo failed to reconnect", get())
			}
			assertFourFamilyGeometry(t, f, id, product, c, a, b, get().Value)
			partApply(workspace.CommandRequest{Type: "DELETE_PUBLICATION", PublicationID: publication})
			update()
			a, b = refs(replacement, "PLANE")
			apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: target, FirstAssemblyRef: &a, SecondAssemblyRef: &b, Value: get().Value, AngleRelation: c.relation, DistanceRelation: r.DistanceRelation, DirectionRelation: "UNDEFINED"})
			if get().EvaluationStatus != "VERIFIED" {
				t.Fatal("explicit support replacement did not reconnect", get())
			}
			assertFourFamilyGeometry(t, f, id, product, c, a, b, get().Value)
			var raw []byte
			if e = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			var manifest workspace.AssemblySolveManifest
			if e = json.Unmarshal(raw, &manifest); e != nil {
				t.Fatal(e)
			}
			if len(manifest.Bodies) != 2 {
				t.Fatal("CAD Bodies/nested leaf became independent motion components", manifest.Bodies)
			}
			for _, g := range manifest.Geometry {
				if g.BodyID != outer && g.BodyID != direct {
					t.Fatal("descriptor escaped owning motion unit", g)
				}
			}
		})
	}
}

func TestSixFamiliesCoupledAcceptanceAndFrozenHistoryThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	product, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Six public families coupled"})
	if e != nil {
		t.Fatal(e)
	}
	id, seq := product.Document.ID, 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-six-%d", id, seq)
		var err error
		product, err = f.service.ApplyCommand(t.Context(), id, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	for _, name := range []string{"GroupA", "GroupB", "Ground"} {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name})
	}
	a, b, c := product.Product.Instances[0].ID, product.Product.Instances[1].ID, product.Product.Instances[2].ID
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{0, 0, 20}, Rotation: [4]float64{0, 0, 0, 1}})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: c, Kind: "BODY"}})
	pa, pb := f.support(t, part, "PLANE"), f.support(t, part, "PLANE")
	pa.InstanceID, pb.InstanceID = a, b
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: &pb, SecondAssemblyRef: &pa, Value: 8, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	axa, axb := f.support(t, part, "CYLINDER"), f.support(t, part, "CYLINDER")
	axa.InstanceID, axb.InstanceID = a, b
	axa.DerivedRole, axb.DerivedRole = "cylinder-axis", "cylinder-axis"
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &axa, SecondAssemblyRef: &axb, DirectionRelation: "SAME"})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "ANGLE", FirstAssemblyRef: &axa, SecondAssemblyRef: &axb, AngleRelation: "FREE", Value: 0})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupName: "Inner solved group", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}}})
	groupID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	pc := f.support(t, part, "PLANE")
	pc.InstanceID = c
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONTACT", FirstAssemblyRef: &pa, SecondAssemblyRef: &pc, ContactKind: "FACE", ContactSide: "EXTERNAL"})
	families := map[string]bool{}
	for _, definition := range product.Product.Constraints {
		families[definition.Family] = true
		if definition.EvaluationStatus != "VERIFIED" {
			t.Fatal("six-family coupled definition not accepted", definition)
		}
	}
	if len(families) != 6 {
		t.Fatal("not six concrete public families", families)
	}
	// Assert final mathematical relations independently, including the internal
	// definitions after the external Contact has moved/turned the whole group.
	assertFourFamilyGeometry(t, f, id, product, fourFamilyCase{family: "Offset"}, pb, pa, 8)
	assertFourFamilyGeometry(t, f, id, product, fourFamilyCase{family: "Coincidence"}, axa, axb, 0)
	assertFourFamilyGeometry(t, f, id, product, fourFamilyCase{family: "Angle", relation: "FREE"}, axa, axb, 0)
	inspected, e := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{pa, pc})
	if e != nil || inspected.Supports[0].Descriptor == nil || inspected.Supports[1].Descriptor == nil {
		t.Fatal(e)
	}
	poses := map[string]workspace.ProductInstance{}
	for _, p := range product.Product.Instances {
		poses[p.ID] = p
	}
	ga, gc := inspected.Supports[0].Descriptor, inspected.Supports[1].Descriptor
	x, y := compositionPoint(poses[a], ga.Origin), compositionPoint(poses[c], gc.Origin)
	n, m := compositionRotate(poses[a].Rotation, ga.Direction), compositionRotate(poses[c].Rotation, gc.Direction)
	profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
	if math.Abs(compositionDot(compositionSub(x, y), m)) > profile.LengthTolerance || 2*math.Asin(math.Min(1, compositionNorm([3]float64{n[0] + m[0], n[1] + m[1], n[2] + m[2]})/2)) > profile.AngleTolerance {
		t.Fatal("external plane face contact is not coplanar opposite materials", x, y, n, m)
	}
	cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
	var digest string
	if e = f.db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&digest); e != nil {
		t.Fatal(e)
	}
	replay, e := cold.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-six-replay")
	if e != nil || replay.Status != "CONVERGED" || len(replay.Result.GroupEvidence) != 1 {
		t.Fatal("six-family staged cold replay", e, replay)
	}
	if len(replay.Result.GroupEvidence[0].InternalConstraintIDs) != 3 {
		t.Fatal("group did not freeze Offset/Coincidence/Angle inner boundary", replay.Result.GroupEvidence)
	}
	foundGroupComponent := false
	for _, component := range replay.Result.Components {
		for _, body := range component.BodyIDs {
			if body == a {
				foundGroupComponent = true
				// One grounded outer reference, one rigid group. Face contact removes
				// three of the group's six motions; internal/redundant equations must
				// not repeatedly subtract their already eliminated relative freedoms.
				if component.TangentVariableCount != 6 || component.JacobianRank != 3 || component.RelativeDof != 3 || component.GaugeDof != 0 {
					t.Fatal("staged rigid-group Contact rank/DOF double counted internal definitions", component)
				}
			}
		}
	}
	if !foundGroupComponent {
		t.Fatal("outer group motion evidence absent")
	}
	release, e := cold.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{ActorID: f.actor, RequestID: id + "-six-release", Name: "Six accepted"})
	if e != nil {
		t.Fatal(e)
	}
	var saved workspace.AssemblyConstraint
	for _, g := range product.Product.Constraints {
		if g.ID == groupID {
			saved = g
		}
	}
	acceptedPoses := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: &pb, SecondAssemblyRef: &pa, Value: 50, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	failed := product.Product.Constraints[len(product.Product.Constraints)-1]
	if failed.EvaluationStatus == "VERIFIED" || failed.Suppressed {
		t.Fatal("coupled conflicting definition hidden or fake Verified", failed)
	}
	for _, g := range product.Product.Constraints {
		if g.ID == groupID && (!reflect.DeepEqual(g.GroupRelations, saved.GroupRelations) || g.GroupCaptureDigest != saved.GroupCaptureDigest) {
			t.Fatal("failed internal combination recaptured group", g)
		}
	}
	for i, p := range product.Product.Instances {
		if compositionNorm(compositionSub(p.Translation, acceptedPoses[i].Translation)) > compositionAcceptanceProfile(t, f, product.Document.VersionID).LengthTolerance {
			t.Fatal("failed candidate pose entered accepted Revision")
		}
	}
	if _, e = cold.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{ActorID: f.actor, RequestID: id + "-bad-release", Name: "Must reject active failed definition"}); e == nil {
		t.Fatal("Release gate only checked accepted subset")
	}
	frozen, e := cold.ReplayProductRelease(t.Context(), id, release.ID, id+"-six-frozen-release")
	if e != nil || frozen.Assembly == nil {
		t.Fatal("failed Head polluted accepted six-family Release", e)
	}
}
