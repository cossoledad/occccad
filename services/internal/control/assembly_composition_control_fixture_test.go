package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type compositionControlFixture struct {
	db                *database.Pool
	client            *geometry.Client
	store             *artifact.Service
	service           *workspace.Service
	actor, fixtureDir string
}

func newCompositionControlFixture(t *testing.T, options ...grpc.DialOption) *compositionControlFixture {
	t.Helper()
	binary, dsn := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || dsn == "" {
		t.Skip("ENVIRONMENT_BLOCKED: requires matching real Worker and dedicated test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/occccad_offset_contract_test" {
		t.Fatal("unsafe test database: only occccad_offset_contract_test is allowed")
	}
	directory := os.Getenv("OCCCCAD_ASSEMBLY_FIXTURE_DIR")
	if directory == "" {
		directory = filepath.Join("..", "..", "..", "build", "constraint-composition", "analytic-fixtures")
	}
	if _, err = os.Stat(filepath.Join(directory, "sphere-r6.brep")); err != nil {
		t.Skip("ENVIRONMENT_BLOCKED: generate analytic fixtures with AssemblyExactSupport.ExportAnalyticRouterFixtures and OCCCCAD_ASSEMBLY_FIXTURE_DIR")
	}
	db, err := database.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var actual string
	if err = db.QueryRow(t.Context(), "SELECT current_database()").Scan(&actual); err != nil || actual != "occccad_offset_contract_test" {
		t.Fatal("test connection is not dedicated database", err)
	}
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	dir := t.TempDir()
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: dir, LogDirectory: t.TempDir()})
	t.Cleanup(pool.Close)
	if err = pool.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(serveGeometry(t, pool), options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	local, err := artifact.NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.NewService(db, local)
	return &compositionControlFixture{db: db, client: client, store: store, service: workspace.NewWithArtifacts(db, client, store), actor: "00000000-0000-7000-8000-000000000001", fixtureDir: directory}
}
func (f *compositionControlFixture) importPart(t *testing.T, name string) workspace.DocumentView {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.fixtureDir, name+".brep"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.Put(t.Context(), artifact.KindExchangeSource, "application/vnd.opencascade.brep", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	// A fresh domain request identity, never identity inferred from topology order.
	part, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PART", Name: name + " fixture seed"})
	if err != nil {
		t.Fatal(err)
	}
	requestID := "analytic-import-" + part.Document.ID
	key := "analytic-" + part.Document.ID
	ref := geometry.ArtifactReference{Backend: source.Backend, ObjectKey: source.Key, SHA256: source.SHA256, Size: source.Size, ContentType: source.ContentType}
	evaluation, err := f.client.ImportExchange(t.Context(), requestID, key, "BREP", ref, "", artifact.StagingKey(requestID, "shape.brep"), artifact.StagingKey(requestID, "mesh.glb"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.CommitImportedPart(t.Context(), f.actor, "", requestID, name, name+".brep", "BREP", key, evaluation, &workspace.ImportSource{ObjectID: source.ID, SHA256: source.SHA256, Format: "BREP"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func (f *compositionControlFixture) support(t *testing.T, part workspace.DocumentView, kind string) workspace.AssemblyGeometryRef {
	t.Helper()
	body := part.Part.Bodies[0]
	a := part.Artifacts[body.GeometryKey]
	object, err := f.store.Get(t.Context(), a.Representations["BREP"].ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	response, _, err := f.client.GetTopologyFromArtifact(t.Context(), a.GeometryID, geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	bind := func(topology string, id uint64) workspace.AssemblyGeometryRef {
		selection, e := f.service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{BodyID: body.ID, SourceVersionID: part.Document.VersionID, GeometryKey: body.GeometryKey, Kind: topology, LocalID: id})
		if e != nil {
			t.Fatal(e)
		}
		if selection.SourceBodyID != body.ID || selection.SourceDocumentID != part.Document.ID {
			t.Fatal("support escaped its CAD Body/source document")
		}
		return workspace.AssemblyGeometryRef{Kind: topology, SourceVersionID: part.Document.VersionID, PersistentSelection: &selection}
	}
	for _, face := range response.Faces {
		expected := map[string]int32{"PLANE": 0, "CYLINDER": 1, "CONE": 2, "SPHERE": 3}[kind]
		if kind != "CIRCLE" && face.SurfaceType == expected {
			if kind == "PLANE" {
				// Deliberately select the known top cap (z=12, outward +Z),
				// rather than assuming an OCCT face enumeration order.
				matched := false
				for _, property := range face.Properties {
					if property.Name == "origin" && property.GetVectorValue() != nil && math.Abs(property.GetVectorValue().Z-12) < 1e-9 {
						matched = true
					}
				}
				if !matched {
					continue
				}
			}
			return bind("FACE", face.LocalId)
		}
	}
	if kind == "CIRCLE" {
		for _, edge := range response.Edges {
			if edge.CurveType != 1 {
				continue
			}
			for _, p := range edge.Properties {
				if p.Name == "center" && p.GetVectorValue() != nil && math.Abs(p.GetVectorValue().Z) < 1e-9 {
					return bind("EDGE", edge.LocalId)
				}
			}
		}
	}
	t.Fatalf("no exact %s support in %s", kind, part.Document.ID)
	return workspace.AssemblyGeometryRef{}
}

func compositionRotate(q [4]float64, p [3]float64) [3]float64 {
	u := [3]float64{q[0], q[1], q[2]}
	dot := u[0]*p[0] + u[1]*p[1] + u[2]*p[2]
	cross := [3]float64{u[1]*p[2] - u[2]*p[1], u[2]*p[0] - u[0]*p[2], u[0]*p[1] - u[1]*p[0]}
	out := [3]float64{}
	for i := range out {
		out[i] = 2*dot*u[i] + (q[3]*q[3]-u[0]*u[0]-u[1]*u[1]-u[2]*u[2])*p[i] + 2*q[3]*cross[i]
	}
	return out
}
func compositionPoint(p workspace.ProductInstance, local [3]float64) [3]float64 {
	out := compositionRotate(p.Rotation, local)
	for i := range out {
		out[i] += p.Translation[i]
	}
	return out
}
func compositionNorm(p [3]float64) float64 { return math.Sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2]) }
func compositionSub(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}
func compositionDot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func compositionDirectionAngle(a, b [3]float64) float64 {
	cross := [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
	return math.Atan2(compositionNorm(cross), compositionDot(a, b))
}

func compositionContactAngle(t *testing.T, profile geometry.AssemblySolverProfile, a, b [3]float64, want float64, unoriented bool) {
	t.Helper()
	angle := compositionDirectionAngle(a, b)
	if unoriented {
		angle = math.Min(angle, math.Pi-angle)
	}
	if math.Abs(angle-want) > profile.AngleTolerance {
		t.Fatalf("independent Contact angle %.12g rad want %.12g (frozen tolerance %.12g)", angle, want, profile.AngleTolerance)
	}
}

func TestContactAnalyticSupportsAndHistoryThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	fixtures := map[string]workspace.DocumentView{}
	for _, name := range []string{"sphere-r6", "cylinder-r6-h12", "cylinder-r3-h12", "cone-r9-r3-h12"} {
		fixtures[name] = f.importPart(t, name)
	}
	cases := []struct{ name, first, firstKind, second, secondKind, relation string }{
		{"plane-sphere", "sphere-r6", "SPHERE", "cylinder-r6-h12", "PLANE", "POINT"},
		{"plane-cylinder", "cylinder-r6-h12", "CYLINDER", "cylinder-r6-h12", "PLANE", "LINE"},
		{"plane-plane", "cylinder-r6-h12", "PLANE", "cylinder-r6-h12", "PLANE", "FACE"},
		{"cylinder-cylinder-line", "cylinder-r6-h12", "CYLINDER", "cylinder-r6-h12", "CYLINDER", "LINE"},
		{"cylinder-cylinder-face", "cylinder-r6-h12", "CYLINDER", "cylinder-r6-h12", "CYLINDER", "FACE"},
		{"sphere-sphere-face", "sphere-r6", "SPHERE", "sphere-r6", "SPHERE", "FACE"},
		{"sphere-cone-ring", "sphere-r6", "SPHERE", "cone-r9-r3-h12", "CONE", "RING"},
		{"sphere-circle-ring", "sphere-r6", "SPHERE", "cylinder-r3-h12", "CIRCLE", "RING"},
		{"cone-cone-line", "cone-r9-r3-h12", "CONE", "cone-r9-r3-h12", "CONE", "LINE"},
		{"cone-cone-face", "cone-r9-r3-h12", "CONE", "cone-r9-r3-h12", "CONE", "FACE"},
		{"cone-circle-ring", "cone-r9-r3-h12", "CONE", "cylinder-r3-h12", "CIRCLE", "RING"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Contact " + c.name})
			if err != nil {
				t.Fatal(err)
			}
			id := product.Document.ID
			seq := 0
			apply := func(req workspace.CommandRequest) {
				t.Helper()
				seq++
				req.ActorID = f.actor
				if req.RequestID == "" {
					req.RequestID = fmt.Sprintf("%s-contact-%d", id, seq)
				}
				product, err = f.service.ApplyCommand(t.Context(), id, req)
				if err != nil {
					t.Fatal(req.Type, err)
				}
			}
			firstPart, secondPart := fixtures[c.first], fixtures[c.second]
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: firstPart.Document.ID, Name: "Moving"})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: secondPart.Document.ID, Name: "Reference"})
			moving, reference := product.Product.Instances[0].ID, product.Product.Instances[1].ID
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{19, -7, 24}, Rotation: [4]float64{0, 0, 0, 1}})
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: reference, Kind: "BODY"}})
			first, second := f.support(t, firstPart, c.firstKind), f.support(t, secondPart, c.secondKind)
			first.InstanceID = moving
			second.InstanceID = reference
			headBeforeInspection := product.Document.VersionID
			var evidenceBefore, evidenceAfter int
			if e := f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1`, headBeforeInspection).Scan(&evidenceBefore); e != nil {
				t.Fatal(e)
			}
			inspected, e := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{first, second})
			if e != nil || len(inspected.Supports) != 2 {
				t.Fatal("precise support inspection", e)
			}
			for i, want := range []string{c.firstKind, c.secondKind} {
				support := inspected.Supports[i]
				if support.Status != "RESOLVED" || support.ExactType != want || support.Descriptor == nil {
					t.Fatalf("pick category was confused with exact geometry: %+v", support)
				}
			}
			first, second = inspected.Supports[0].Reference, inspected.Supports[1].Reference
			if e = f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1`, headBeforeInspection).Scan(&evidenceAfter); e != nil || evidenceAfter != evidenceBefore || inspected.VersionID != headBeforeInspection {
				t.Fatal("inspection produced solve evidence/Revision", e)
			}
			initial := product.Product.Instances[0]
			request := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ActorID: f.actor, RequestID: id + "-contact-create", ConstraintKind: "CONTACT", ContactKind: c.relation, ContactSide: "EXTERNAL", FirstAssemblyRef: &first, SecondAssemblyRef: &second}
			if c.name == "cylinder-cylinder-face" || c.name == "sphere-sphere-face" || c.name == "cone-cone-face" || c.name == "sphere-cone-ring" {
				invalid := request
				invalid.RequestID += "-incompatible-material-side"
				invalidPreview, invalidError := f.service.PreviewCommand(t.Context(), id, invalid)
				if invalidError != nil || invalidPreview.ConstraintEvaluation == nil || invalidPreview.ConstraintEvaluation.Status != "IMPOSSIBLE" {
					t.Fatalf("same-outward sources must reject external material side, not certify contact: %+v", invalidPreview)
				}
				unchanged, readError := f.service.GetDocument(t.Context(), id)
				if readError != nil || unchanged.Document.VersionID != product.Document.VersionID || !reflect.DeepEqual(unchanged.Product.Instances, product.Product.Instances) {
					t.Fatal("incompatible material Preview changed Head/accepted poses", readError)
				}
				request.ContactSide = "INTERNAL"
			}
			preview, e := f.service.PreviewCommand(t.Context(), id, request)
			if e != nil || preview.PreviewID == "" || preview.ConstraintEvaluation == nil || preview.ConstraintEvaluation.Status != "VERIFIED" {
				t.Fatalf("Contact Preview not accepted: %v %+v", e, preview)
			}
			request.PreviewID = preview.PreviewID
			apply(request)
			definition := product.Product.Constraints[len(product.Product.Constraints)-1]
			targetID := definition.ID
			if definition.EvaluationStatus != "VERIFIED" {
				t.Fatalf("legal target Contact excluded: %s: %s", definition.EvaluationStatus, definition.EvaluationSummary)
			}
			if definition.Family != "Contact" || definition.DefinitionVersion < 2 {
				t.Fatal("Contact lacks versioned public definition")
			}
			var mp, rp workspace.ProductInstance
			for _, p := range product.Product.Instances {
				if p.ID == moving {
					mp = p
				} else if p.ID == reference {
					rp = p
				}
			}
			if reflect.DeepEqual(initial, mp) {
				t.Fatal("initial non-feasible Contact did not actually move")
			}
			// Independent equations derived from known OCCT fixture geometry, not the
			// solver's own reported residual or frozen numerical descriptor.
			mcenter := compositionPoint(mp, [3]float64{})
			rcenter := compositionPoint(rp, [3]float64{})
			ma := compositionRotate(mp.Rotation, [3]float64{0, 0, 1})
			ra := compositionRotate(rp.Rotation, [3]float64{0, 0, 1})
			delta := compositionSub(mcenter, rcenter)
			profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
			near := func(value, target float64) {
				t.Helper()
				if math.Abs(value-target) > profile.LengthTolerance {
					t.Fatalf("independent geometry: %.12g want %.12g", value, target)
				}
			}
			switch c.name {
			case "plane-sphere":
				near(compositionDot(compositionSub(mcenter, compositionPoint(rp, [3]float64{0, 0, 12})), ra), 6)
			case "plane-cylinder":
				compositionContactAngle(t, profile, ma, ra, math.Pi/2, false)
				near(compositionDot(compositionSub(mcenter, compositionPoint(rp, [3]float64{0, 0, 12})), ra), 6)
			case "plane-plane":
				compositionContactAngle(t, profile, ma, ra, math.Pi, false)
				near(compositionDot(compositionSub(compositionPoint(mp, [3]float64{0, 0, 12}), compositionPoint(rp, [3]float64{0, 0, 12})), ra), 0)
			case "cylinder-cylinder-line", "cylinder-cylinder-face":
				compositionContactAngle(t, profile, ma, ra, 0, true)
				radial := compositionSub(delta, [3]float64{ra[0] * compositionDot(delta, ra), ra[1] * compositionDot(delta, ra), ra[2] * compositionDot(delta, ra)})
				target := 12.
				if c.relation == "FACE" {
					target = 0
				}
				near(compositionNorm(radial), target)
			case "sphere-sphere-face":
				near(compositionNorm(delta), 0)
			case "sphere-cone-ring":
				apex := compositionPoint(rp, [3]float64{0, 0, 18})
				d := compositionSub(mcenter, apex)
				height := compositionDot(d, ra)
				near(compositionNorm(compositionSub(d, [3]float64{ra[0] * height, ra[1] * height, ra[2] * height})), 0)
				near(-height*math.Sin(math.Atan(.5)), 6) // selected cone leaf is -Z
			case "sphere-circle-ring":
				height := compositionDot(delta, ra)
				near(compositionNorm(compositionSub(delta, [3]float64{ra[0] * height, ra[1] * height, ra[2] * height})), 0)
				near(height, -math.Sqrt(36-9)) // branch +1 puts circle above sphere centre
			case "cone-cone-line", "cone-cone-face":
				apexDelta := compositionSub(compositionPoint(mp, [3]float64{0, 0, 18}), compositionPoint(rp, [3]float64{0, 0, 18}))
				if c.relation == "FACE" {
					near(compositionNorm(apexDelta), 0)
				} else {
					// Equal-angle external cones share a generator along the
					// bisector of their leaf axes (-Z), not necessarily an apex.
					g := [3]float64{-ma[0] - ra[0], -ma[1] - ra[1], -ma[2] - ra[2]}
					length := compositionNorm(g)
					for i := range g {
						g[i] /= length
					}
					compositionContactAngle(t, profile, g, ma, math.Pi-math.Atan(.5), false)
					compositionContactAngle(t, profile, g, ra, math.Pi-math.Atan(.5), false)
					near(compositionNorm([3]float64{apexDelta[1]*g[2] - apexDelta[2]*g[1], apexDelta[2]*g[0] - apexDelta[0]*g[2], apexDelta[0]*g[1] - apexDelta[1]*g[0]}), 0)
				}
				angle := 2 * math.Atan(.5)
				if c.relation == "FACE" {
					angle = 0
				}
				compositionContactAngle(t, profile, ma, ra, angle, false)
			case "cone-circle-ring":
				compositionContactAngle(t, profile, ma, ra, 0, true)
				d := compositionSub(rcenter, compositionPoint(mp, [3]float64{0, 0, 18}))
				height := compositionDot(d, ma)
				near(compositionNorm(compositionSub(d, [3]float64{ma[0] * height, ma[1] * height, ma[2] * height})), 0)
				near(height, -6)
			}
			acceptedPoses := append([]workspace.ProductInstance(nil), product.Product.Instances...)
			for _, s := range []bool{true, false} {
				apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{targetID}, Suppressed: &s})
			}
			if product.Product.Constraints[len(product.Product.Constraints)-1].EvaluationStatus != "VERIFIED" {
				t.Fatal("Contact failed activation restoration")
			}
			mode := "MEASURED"
			head := product.Document.VersionID
			if _, e := f.service.ApplyCommand(t.Context(), id, workspace.CommandRequest{ActorID: f.actor, Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{targetID}, ConstraintMode: &mode}); e == nil {
				t.Fatal("Contact incorrectly accepts Measured")
			}
			cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
			read, e := cold.GetDocument(t.Context(), id)
			if e != nil || read.Document.VersionID != head || read.Product.Constraints[len(read.Product.Constraints)-1].ContactKind != c.relation {
				t.Fatal("invalid mode changed Head or cold definition", e)
			}
			var digest string
			if e = f.db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, head).Scan(&digest); e != nil {
				t.Fatal(e)
			}
			replay, e := cold.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-contact-replay")
			if e != nil || replay.Status != "CONVERGED" {
				t.Fatal("Contact cold replay", e, replay)
			}
			for _, body := range replay.Result.Bodies {
				for _, pose := range acceptedPoses {
					if body.ID == pose.ID && compositionNorm(compositionSub(body.Pose.Translation, pose.Translation)) > profile.LengthTolerance {
						t.Fatal("replay changed accepted translation")
					}
				}
			}
			var raw []byte
			if e = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, digest).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			var manifest workspace.AssemblySolveManifest
			if e = json.Unmarshal(raw, &manifest); e != nil {
				t.Fatal(e)
			}
			if len(manifest.Bodies) != 2 {
				t.Fatal("CAD Body support was confused with solver motion units")
			}
			release, e := cold.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{RequestID: id + "-contact-release", Name: "Frozen " + c.name, ActorID: f.actor})
			if e != nil {
				t.Fatal(e)
			}
			suppressed := true
			apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{targetID}, Suppressed: &suppressed})
			apply(workspace.CommandRequest{Type: "UNDO"})
			if product.Product.Constraints[len(product.Product.Constraints)-1].Suppressed {
				t.Fatal("Undo did not restore active Contact")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			if !product.Product.Constraints[len(product.Product.Constraints)-1].Suppressed {
				t.Fatal("Redo did not restore suppressed Contact")
			}
			frozen, e := cold.ReplayProductRelease(t.Context(), id, release.ID, id+"-contact-release-replay")
			if e != nil || frozen.Assembly == nil {
				t.Fatal("Contact Release lost frozen evidence", e)
			}
			if e = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, frozen.Assembly.ManifestDigest).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(raw, &manifest); e != nil {
				t.Fatal(e)
			}
			foundFrozen := false
			for _, d := range manifest.Definitions {
				if d.ID == targetID {
					foundFrozen = true
					if d.Suppressed || d.ContactKind != c.relation {
						t.Fatal("Head suppression changed frozen Release definition")
					}
				}
			}
			if !foundFrozen {
				t.Fatal("Release lost Contact definition")
			}
		})
	}
}

func TestPreciseAssemblySupportInspectionThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "sphere-r6")
	partApply := func(req workspace.CommandRequest) {
		t.Helper()
		req.ActorID = f.actor
		next, e := f.service.ApplyCommand(t.Context(), part.Document.ID, req)
		if e != nil {
			t.Fatal(req.Type, e)
		}
		part = next
	}
	// One imported sphere CAD Body plus a separate native box CAD Body; the Part
	// occurrence must remain exactly one assembly motion unit.
	partApply(workspace.CommandRequest{Type: "CREATE_BODY"})
	body := part.Part.ActiveBodyID
	partApply(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
	sketch := part.Part.Features[len(part.Part.Features)-1].ID
	partApply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 40, Y: 0}, Second: &workspace.SketchPoint2{X: 50, Y: 10}}}})
	partApply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	sphere := f.support(t, part, "SPHERE")
	sourceBody := sphere.PersistentSelection.SourceBodyID
	if sourceBody == body || len(part.Part.Bodies) != 2 {
		t.Fatal("sphere support was routed to the active sibling CAD Body")
	}
	child, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Nested rigid geometry"})
	if err != nil {
		t.Fatal(err)
	}
	child, err = f.service.ApplyCommand(t.Context(), child.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "Nested shared Part"})
	if err != nil {
		t.Fatal(err)
	}
	leaf := child.Product.Instances[0].ID
	q := [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}
	child, err = f.service.ApplyCommand(t.Context(), child.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{10, 2, 3}, Rotation: q})
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Owning geometry inspection"})
	if err != nil {
		t.Fatal(err)
	}
	rootApply := func(req workspace.CommandRequest) {
		t.Helper()
		req.ActorID = f.actor
		root, err = f.service.ApplyCommand(t.Context(), root.Document.ID, req)
		if err != nil {
			t.Fatal(req.Type, err)
		}
	}
	rootApply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID, Name: "Rigid child"})
	rootApply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "Direct shared Part"})
	outer, direct := root.Product.Instances[0].ID, root.Product.Instances[1].ID
	rootApply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: outer, Translation: [3]float64{31, -4, 7}, Rotation: [4]float64{math.Sin(.3), 0, 0, math.Cos(.3)}})
	path := workspace.InstancePath{RootDocumentID: root.Document.ID, Segments: []workspace.InstancePathSegment{
		{OwnerDocumentID: root.Document.ID, OwnerVersionID: root.Document.VersionID, InstanceID: outer, ReferencedDocumentID: child.Document.ID, ResolvedVersionID: child.Document.VersionID},
		{OwnerDocumentID: child.Document.ID, OwnerVersionID: child.Document.VersionID, InstanceID: leaf, ReferencedDocumentID: part.Document.ID, ResolvedVersionID: part.Document.VersionID},
	}}
	nested := sphere
	nested.InstanceID = outer
	nested.InstancePath = &path
	derived := nested
	derived.DerivedRole = "sphere-center"
	directRef := sphere
	directRef.InstanceID = direct
	frame := workspace.AssemblyGeometryRef{InstanceID: outer, Kind: "FRAME", GeometryID: "axis-system-default", InstancePath: &path}
	frameX := frame
	frameX.DerivedRole = "frame-axis-x"
	head := root.Document.VersionID
	result, err := f.service.InspectAssemblySupports(t.Context(), root.Document.ID, []workspace.AssemblyGeometryRef{nested, derived, directRef, frame, frameX})
	if err != nil || len(result.Supports) != 5 {
		t.Fatal(err)
	}
	expectedKinds := []string{"SPHERE", "POINT", "SPHERE", "FRAME", "AXIS"}
	for i, support := range result.Supports {
		if support.Status != "RESOLVED" || support.ExactType != expectedKinds[i] || support.Descriptor == nil {
			t.Fatalf("support %d: %+v", i, support)
		}
		if support.Descriptor.BodyID != outer && i != 2 {
			t.Fatal("nested leaf or CAD Body became solver body")
		}
	}
	for _, i := range []int{0, 1, 3, 4} {
		got := result.Supports[i].Descriptor.Origin
		for j, want := range [3]float64{10, 2, 3} {
			if math.Abs(got[j]-want) > 1e-10 {
				t.Fatal("nested support transformed zero/two times or to root world", got)
			}
		}
	}
	if result.Supports[2].Descriptor.Origin != ([3]float64{}) || result.Supports[2].Descriptor.BodyID != direct || result.Supports[0].Descriptor.ID == result.Supports[2].Descriptor.ID {
		t.Fatal("shared definition occurrences mixed supports")
	}
	if math.Abs(result.Supports[4].Descriptor.Direction[1]-1) > 1e-10 {
		t.Fatal("nested Frame X direction lost accepted child rotation")
	}
	opened, err := f.service.GetDocument(t.Context(), root.Document.ID)
	if err != nil || opened.Document.VersionID != head {
		t.Fatal("read-only inspection changed Head", err)
	}
	// Changing the source Head must not change accepted support or auto-navigate.
	partApply(workspace.CommandRequest{Type: "CREATE_BODY"})
	repeated, err := f.service.InspectAssemblySupports(t.Context(), root.Document.ID, []workspace.AssemblyGeometryRef{nested})
	if err != nil || repeated.Supports[0].Reference.SourceVersionID != sphere.SourceVersionID || repeated.Supports[0].Descriptor.Origin != result.Supports[0].Descriptor.Origin {
		t.Fatal("inspection substituted latest source Head", err)
	}
}
