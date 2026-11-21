//go:build integration

package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblySTEPDocumentImport(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                             string
		definitions, instances, maxEmpty int
	}{
		{"Bench Vise", 22, 24, 0}, {"Jackhammer", 185, 288, 3}, {"Walking Robot", 18, 191, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExchangeFixture(t)
			name := tc.name + ".step"
			data, err := os.ReadFile(filepath.Join(root, "tests", "test.data", name))
			if err != nil {
				t.Fatal(err)
			}
			job, err := f.importFile(t, name, data)
			if err != nil || job.State != "SUCCEEDED" || job.DocumentID == nil {
				t.Fatalf("import %s: state=%s err=%v", name, job.State, err)
			}
			var payload struct{ Exchange exchange.Report }
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			report := payload.Exchange
			seen := map[string]bool{}
			instances, empty, parts := 0, 0, 0
			byName := map[string]workspace.DocumentView{}
			var visit func(string)
			visit = func(id string) {
				if seen[id] {
					return
				}
				seen[id] = true
				v, err := f.h.workspace.GetDocument(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				byName[v.Document.Name] = v
				if v.Product != nil {
					instances += len(v.Product.Instances)
					for _, inst := range v.Product.Instances {
						if inst.ReferenceMode != "PINNED" || inst.ReferencedVersionID == "" {
							t.Fatal("unversioned occurrence")
						}
						visit(inst.ReferencedDocumentID)
					}
					return
				}
				if v.Part == nil || id == *job.DocumentID {
					t.Fatal("expected assembly root and Part leaves")
				}
				parts++
				if len(v.Part.Features) == 0 {
					empty++
					found := false
					for _, w := range report.Warnings {
						if w.DefinitionID != "" && strings.HasPrefix(w.Detail, v.Document.Name+": ") {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing source warning for %s", v.Document.Name)
					}
				} else {
					feature := v.Part.Features[0]
					key := v.Part.Bodies[0].GeometryKey
					if feature.Result != nil {
						key = feature.Result.GeometryKey
					}
					a := v.Artifacts[key]
					if a.BRepBytes == 0 || a.GLBBytes == 0 || !a.Naming.CanBind {
						t.Fatalf("missing accepted geometry for %s", v.Document.Name)
					}
				}
				if len(v.Part.DatumPlanes) != 3 || len(v.Part.AxisSystems) != 1 {
					t.Fatal("missing built-in references")
				}
				for _, plane := range v.Part.DatumPlanes {
					if plane.Visible == nil || *plane.Visible {
						t.Fatal("visible imported plane", v.Document.Name)
					}
				}
				if a := v.Part.AxisSystems[0]; a.Visible == nil || *a.Visible {
					t.Fatal("visible imported axis system", v.Document.Name)
				}

			}
			visit(*job.DocumentID)
			t.Logf("definitions=%d instances=%d parts=%d empty=%d warnings=%+v", len(seen), instances, parts, empty, report.Warnings)
			if len(seen) != tc.definitions || report.Definitions != tc.definitions || instances != tc.instances || empty != report.EmptyParts || empty > tc.maxEmpty {
				t.Fatal("source structure or report mismatch")
			}
			var editedLabel workspace.DocumentView
			if tc.name == "Bench Vise" {
				verifyImportedIndicatorFillet(t, f, byName["INDICATOR"])
				label := byName["WARNING LABEL - ANSI"]
				verifyImportedBoundaryRepair(t, f, recoverImportedFixture(t, f, *job.InputObjectID, label))
				changed := verifyImportedFaceEdit(t, f, label, 1, "MOVE_PLANAR_FACE", .02, "ADD")
				adoptImportedReference(t, f, byName["MAIN ASSEMBLY"], changed)
				editedLabel = changed
				verifyImportedFaceEdit(t, f, byName["JAW PAD, SLIDING"], 7, "RESIZE_BORE", 5, "REMOVE")
			}
			if tc.name == "Walking Robot" {
				owner := byName["Walking Robot"]
				verifyImportedRobotJoint(t, f, owner, byName["Arm 2"], byName["Arm1-arm2 bolt"])
				owner, err = f.h.workspace.GetDocument(t.Context(), owner.Document.ID)
				if err != nil {
					t.Fatal(err)
				}
				bolt := byName["75 bolt"]
				updated := f.command(t, bolt.Document.ID, workspace.CommandRequest{Type: "RENAME_BODY", BodyID: bolt.Part.ActiveBodyID, Name: "Reviewed bolt"})
				adoptImportedReference(t, f, owner, updated)
			}
			graph, err := f.h.workspace.ExchangeExportGraph(t.Context(), *job.DocumentID, workspace.ExchangeExportOptions{Visibility: "ALL"})
			if err != nil {
				t.Fatal(err)
			}
			omitted := 0
			for _, w := range graph.Warnings {
				if w.Code == "EXCHANGE_EMPTY_OCCURRENCE_OMITTED" {
					omitted++
					if w.DefinitionId == "" || !strings.Contains(w.Message, " / ") {
						t.Fatal("missing omission location", w)
					}
				}
			}
			if omitted != empty {
				t.Fatalf("expected %d single-occurrence empty parts in export report, got %d", empty, omitted)
			}
			output := f.exported(t, graph, "STEP")
			actual, err := f.h.geometry.InspectExchange(t.Context(), uuid.NewString(), "STEP", output)
			if err != nil {
				t.Fatal(err)
			}
			placements := 0
			for _, d := range actual.Definitions {
				placements += len(d.Children)
			}
			if editedLabel.Part != nil {
				props, err := f.h.workspace.GetTopologyElementPropertiesAtVersion(t.Context(), editedLabel.Document.ID, editedLabel.Document.VersionID, editedLabel.Part.Bodies[0].GeometryKey, "BODY", 0)
				if err != nil {
					t.Fatal(err)
				}
				var target string
				for _, def := range actual.Definitions {
					if def.Name == editedLabel.Document.Name {
						target = def.Id
						break
					}
				}
				if target == "" {
					t.Fatal("modified Part missing from exported STEP")
				}
				got := singleSTEPDefinitionVolume(t, f, output, target)
				if math.Abs(got-props.Properties["volume"].(float64)) > 1e-5 {
					t.Fatal("modified assembly exported an incorrect Part volume", got, props.Properties)
				}
			}
			if len(actual.Definitions) != len(graph.Definitions) || placements != instances-omitted {
				t.Fatalf("STEP round trip definitions=%d/%d placements=%d/%d", len(actual.Definitions), len(graph.Definitions), placements, instances-omitted)
			}

		})
	}
}

func verifyImportedIndicatorFillet(t *testing.T, f exchangeFixture, v workspace.DocumentView) {
	t.Helper()
	if v.Part == nil {
		t.Fatal("missing indicator")
	}
	body := v.Part.Bodies[0]
	// Locator 23 belongs to the frozen BREP of this fixture. Use its bound
	// persistent selection through preview and commit.
	s := f.h.workspace
	edge, err := s.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, body.GeometryKey, "EDGE", 23)
	if err != nil || edge.PersistentSelection == nil || edge.GeometryType != "CIRCLE" {
		t.Fatal("rim selection", err)
	}
	req := workspace.CommandRequest{ActorID: f.actor, RequestID: uuid.NewString(), Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: body.ID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: *edge.PersistentSelection, SourceVersionID: v.Document.VersionID}}}}
	if _, err := s.PreviewCommand(t.Context(), v.Document.ID, req); err == nil || !strings.Contains(err.Error(), "MODIFIER_ALGORITHM_FAILED:FILLET") {
		t.Fatal("oversized rim", err)
	}
	req.RequestID = uuid.NewString()
	req.Feature.Length = .25
	preview, err := s.PreviewCommand(t.Context(), v.Document.ID, req)
	if err != nil || preview.PreviewID == "" {
		t.Fatal("preview", err)
	}
	req.PreviewID = preview.PreviewID
	accepted, err := s.ApplyCommand(t.Context(), v.Document.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	last := accepted.Part.Features[len(accepted.Part.Features)-1]
	key := accepted.Part.Bodies[0].GeometryKey
	if last.Type != "FILLET" || last.Length != req.Feature.Length || key == body.GeometryKey || !accepted.Artifacts[key].Naming.CanBind {
		t.Fatal("invalid fillet commit")
	}
}

func boundImportedInput(t *testing.T, f exchangeFixture, v workspace.DocumentView, key, stage, kind string, local uint64) workspace.GeometricInput {
	t.Helper()
	observation, err := f.h.workspace.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, key, kind, local)
	if err != nil || observation.PersistentSelection == nil {
		t.Fatalf("bind %s %d: %v", kind, local, err)
	}
	return workspace.GeometricInput{Kind: "TOPOLOGY", Selection: &workspace.FeatureSelection{Selection: *observation.PersistentSelection, SourceVersionID: v.Document.VersionID, SourceFeatureID: stage}}
}

func verifyImportedFaceEdit(t *testing.T, f exchangeFixture, v workspace.DocumentView, face uint64, kind string, length float64, op string) workspace.DocumentView {
	t.Helper()
	body := v.Part.Bodies[0]
	input := boundImportedInput(t, f, v, body.GeometryKey, v.Part.Features[0].ID, "FACE", face)
	volume := func(view workspace.DocumentView) float64 {
		p, err := f.h.workspace.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, view.Part.Bodies[0].GeometryKey, "BODY", 0)
		if err != nil {
			t.Fatal(err)
		}
		return p.Properties["volume"].(float64)
	}
	original := volume(v)
	req := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: f.actor, RequestID: uuid.NewString(), Feature: &workspace.Feature{Type: kind, BodyID: body.ID, Operation: op, Geometry: &workspace.GeometricFeatureDefinition{Kind: kind, Inputs: []workspace.GeometricInput{input}, Parameters: workspace.GeometryOperationParameters{Distance: length}}}}
	preview, err := f.h.workspace.PreviewCommand(t.Context(), v.Document.ID, req)
	if err != nil {
		t.Fatalf("%s preview: %v", kind, err)
	}
	req.PreviewID = preview.PreviewID
	updated, err := f.h.workspace.ApplyCommand(t.Context(), v.Document.ID, req)
	if err != nil {
		t.Fatalf("%s commit: %v", kind, err)
	}
	actual := volume(updated)
	if (op == "ADD" && actual <= original) || (op == "REMOVE" && actual >= original) {
		t.Fatalf("%s volume %f -> %f", kind, original, actual)
	}
	if kind == "RESIZE_BORE" {
		key := updated.Part.Bodies[0].GeometryKey
		found := false
		for id := uint64(1); ; id++ {
			p, err := f.h.workspace.GetTopologyElementPropertiesAtVersion(t.Context(), updated.Document.ID, updated.Document.VersionID, key, "FACE", id)
			if err != nil {
				break
			}
			if r, ok := p.Properties["radius"].(float64); ok && p.GeometryType == "CYLINDER" && math.Abs(r-length/2) < 1e-7 {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("result is missing the requested cylindrical wall")
		}
	}
	return updated
}

func adoptImportedReference(t *testing.T, f exchangeFixture, owner, updated workspace.DocumentView) {
	t.Helper()
	for _, item := range owner.Product.Instances {
		if item.ReferencedDocumentID != updated.Document.ID {
			continue
		}
		accepted := f.command(t, owner.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES", VersionID: owner.Document.VersionID, ReferencedDocumentID: updated.Document.ID, ReplacementVersionID: updated.Document.VersionID, PinnedInstanceIDs: []string{item.ID}})
		for i, next := range accepted.Product.Instances {
			old := owner.Product.Instances[i]
			if next.ID != old.ID || next.ReferenceMode != "PINNED" || next.ID == item.ID && next.ReferencedVersionID != updated.Document.VersionID || next.ID != item.ID && next.ReferencedVersionID != old.ReferencedVersionID {
				t.Fatal("unexpected reference update", next.ID)
			}
		}
		return
	}
	t.Fatal("missing direct occurrence", updated.Document.Name, owner.Document.Name)
}

// Replay the same per-definition job that a historical partial import enqueues.
// The copy has independent identity, as did the original empty placeholder.
func recoverImportedFixture(t *testing.T, f exchangeFixture, sourceID string, original workspace.DocumentView) workspace.DocumentView {
	t.Helper()
	empty, err := f.h.workspace.CommitImportedEmptyPart(t.Context(), f.actor, f.folder, uuid.NewString(), original.Document.Name, "Bench Vise.step")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	var sourceDefinition string
	if err = f.h.database.QueryRow(t.Context(), `SELECT definition->'source'->>'definitionId' FROM occccad.import_definitions WHERE id=$1`, original.Part.Features[0].ImportDefinitionID).Scan(&sourceDefinition); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"fileName": "Bench Vise.step", "format": "STEP", "requestId": id, "recoveryDefinition": sourceDefinition})
	var jobID string
	err = f.h.database.QueryRow(t.Context(), `INSERT INTO occccad.jobs(job_type,requested_by_user_id,document_id,version_id,input_object_id,payload,idempotency_key,user_visible,state,lease_owner,lease_expires_at,attempt_count) VALUES('EXCHANGE_IMPORT',$1,$2,$3,$4,$5,$6,true,'RUNNING',$7,now()+interval '1 hour',1) RETURNING id::text`, f.actor, empty.Document.ID, empty.Document.VersionID, sourceID, payload, id, f.h.workerID).Scan(&jobID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := f.h.queue.Get(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.h.execute(t.Context(), job); err != nil {
		t.Fatal("recover source definition", err)
	}
	restored, err := f.h.workspace.GetDocument(t.Context(), empty.Document.ID)
	if err != nil || len(restored.Part.Features) != 1 || restored.Part.Features[0].ImportDefinitionID == "" || restored.Part.Features[0].GeometryKey != original.Part.Features[0].GeometryKey || restored.Document.VersionID == empty.Document.VersionID {
		t.Fatal("in-place recovery failed", err)
	}
	return restored
}

func verifyImportedBoundaryRepair(t *testing.T, f exchangeFixture, v workspace.DocumentView) {
	t.Helper()
	body := v.Part.Bodies[0]
	initial, err := f.h.workspace.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, body.GeometryKey, "BODY", 0)
	if err != nil {
		t.Fatal(err)
	}
	var faces []workspace.GeometricInput
	for face := uint64(1); face <= 5; face++ {
		faces = append(faces, boundImportedInput(t, f, v, body.GeometryKey, v.Part.Features[0].ID, "FACE", face))
	}
	surface := func(kind string, inputs ...workspace.GeometricInput) workspace.GeometricInput {
		t.Helper()
		d := &workspace.GeometricFeatureDefinition{Kind: kind, Inputs: inputs, Surface: &workspace.GeometrySurfaceDefinition{Tolerance: 1e-6}}
		v = f.command(t, v.Document.ID, workspace.CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &workspace.Feature{Type: "GEOMETRY", Geometry: d}})
		feature := v.Part.Features[len(v.Part.Features)-1]
		return workspace.GeometricInput{Kind: "RESULT", FeatureID: feature.ID, ResultID: "result"}
	}
	shell := surface("SURFACE_SEW", faces...)
	quality, err := f.h.workspace.AnalyzeShapeQuality(t.Context(), v.Document.ID, workspace.ShapeQualityRequest{RevisionID: v.Document.VersionID, Input: shell, SampleCount: 64})
	if err != nil || !quality.Result.BoundaryComplete || len(quality.Result.BoundaryEdges) != 4 {
		t.Fatalf("open shell report: edges=%d complete=%v err=%v", len(quality.Result.BoundaryEdges), quality.Result.BoundaryComplete, err)
	}
	var edges []workspace.GeometricInput
	degree := map[uint64]int{}
	for _, edge := range quality.Result.BoundaryEdges {
		degree[edge.FirstVertex]++
		degree[edge.LastVertex]++
		edges = append(edges, boundImportedInput(t, f, v, v.Part.Features[len(v.Part.Features)-1].Result.GeometryKey, shell.FeatureID, "EDGE", edge.EdgeID))
	}
	for vertex, count := range degree {
		if vertex == 0 || count != 2 {
			t.Fatal("not a closed loop", vertex, count)
		}
	}
	patch := surface("SURFACE_FILL", edges...)
	closed := surface("SURFACE_SEW", shell, patch)
	v = f.command(t, v.Document.ID, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "CLOSE_SURFACE", Operation: "NEW_BODY", Geometry: &workspace.GeometricFeatureDefinition{Kind: "CLOSE_SURFACE", Inputs: []workspace.GeometricInput{closed}, Surface: &workspace.GeometrySurfaceDefinition{Tolerance: 1e-6}}}})
	body = v.Part.Bodies[len(v.Part.Bodies)-1]
	final, err := f.h.workspace.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, body.GeometryKey, "BODY", 0)
	if err != nil || math.Abs(final.Properties["volume"].(float64)-initial.Properties["volume"].(float64)) > 1e-5 {
		t.Fatal("repaired shape volume", initial.Properties, final.Properties, err)
	}
}

func verifyImportedRobotJoint(t *testing.T, f exchangeFixture, owner, arm, pin workspace.DocumentView) {
	t.Helper()
	// Pick a mating arm and bolt in the source assembly. The source places this
	// pair on the positive-Z half-turn; only typed occurrence IDs leave the test.
	pick := func(part workspace.DocumentView, face uint64) workspace.JointEndpoint {
		t.Helper()
		for _, i := range owner.Product.Instances {
			if i.ReferencedDocumentID != part.Document.ID || math.Abs(i.Rotation[2]) < .99 {
				continue
			}
			support := boundImportedInput(t, f, part, part.Part.Bodies[0].GeometryKey, part.Part.Features[0].ID, "FACE", face).Selection
			return workspace.JointEndpoint{InstanceID: i.ID, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, Axis: &workspace.AssemblyGeometryRef{InstanceID: i.ID, Kind: "FACE", PersistentSelection: &support.Selection, SourceVersionID: support.SourceVersionID, DerivedRole: "cylinder-axis"}}
		}
		t.Fatalf("fixture missing %s", part.Document.Name)
		return workspace.JointEndpoint{}
	}
	first, second := pick(arm, 17), pick(pin, 1)
	const rotation = "turn"
	const slide = "slide"
	joint := workspace.MechanismJoint{ID: "bearing", Name: "Arm and pivot", Kind: "CYLINDRICAL", First: first, Second: &second, Coordinates: []workspace.JointCoordinate{
		{ID: rotation, Role: "AXIAL_ROTATION", Dimension: "ANGLE", Direction: 1, Lower: &workspace.MotionQuantity{Value: -180, Unit: "deg"}, Upper: &workspace.MotionQuantity{Value: 180, Unit: "deg"}, Zero: workspace.MotionQuantity{Unit: "deg"}},
		{ID: slide, Role: "AXIAL_TRANSLATION", Dimension: "LENGTH", Direction: 1, Lower: &workspace.MotionQuantity{Value: -2, Unit: "mm"}, Upper: &workspace.MotionQuantity{Value: 2, Unit: "mm"}, Zero: workspace.MotionQuantity{Unit: "mm"}},
	}}
	ground := workspace.MechanismJoint{ID: "ground", Name: "Arm ground", Kind: "GROUND", First: workspace.JointEndpoint{InstanceID: first.InstanceID, Frame: first.Frame}}
	m := workspace.Mechanism{ID: "pivot-test", Name: "Arm pivot", UnitIDs: []string{first.InstanceID, second.InstanceID}, Joints: []workspace.MechanismJoint{ground, joint}}
	s := f.h.workspace
	preview, err := s.PreviewMechanism(t.Context(), owner.Document.ID, workspace.MechanismPreviewRequest{BaseRevisionID: owner.Document.VersionID, Mechanism: m, DraftJointID: joint.ID})
	if err != nil {
		t.Fatal("preview imported pivot", err)
	}
	defs := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{preview.Mechanism}, Drivers: []workspace.MotionDriver{
		{ID: "angle", Name: "Angle", JointID: joint.ID, MechanismID: m.ID, CoordinateID: rotation}, {ID: "position", Name: "Position", JointID: joint.ID, MechanismID: m.ID, CoordinateID: slide},
	}}
	owner = f.command(t, owner.Document.ID, workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: owner.Document.VersionID, Kinematics: &defs})
	session, err := s.BeginMechanismSession(t.Context(), owner.Document.ID, f.actor, workspace.MechanismSessionBegin{OwnerID: uuid.NewString(), Generation: 1, BaseRevisionID: owner.Document.VersionID, MechanismID: m.ID, ActiveDriverIDs: []string{"angle", "position"}})
	if err != nil {
		t.Fatal("begin imported pivot", err)
	}
	defer s.CancelMechanismSession(owner.Document.ID, f.actor, session.SessionID)
	frame, err := s.UpdateMechanismSession(t.Context(), owner.Document.ID, f.actor, session.SessionID, workspace.MechanismSessionUpdate{Sequence: 1, Values: map[string]workspace.MotionQuantity{"angle": {Value: 10, Unit: "deg"}, "position": {Value: 0, Unit: "mm"}}})
	if err != nil || !frame.Accepted || !frame.Qualified {
		t.Fatal("robot pivot trial", frame.Failure, err)
	}
	if before, after := session.Frame.UnitPoses, frame.Frame.UnitPoses; len(after) != len(owner.Product.Instances) || after[second.InstanceID].Rotation == before[second.InstanceID].Rotation {
		t.Fatal("missing environment or unchanged pivot rotation")
	} else {
		for id, p := range before {
			if id != second.InstanceID && id != first.InstanceID && after[id] != p {
				t.Fatalf("moved a nonmember %s", id)
			}
		}
	}
	current, err := s.GetDocument(t.Context(), owner.Document.ID)
	if err != nil || current.Document.VersionID != owner.Document.VersionID {
		t.Fatal("trial changed Product head", err)
	}
}
