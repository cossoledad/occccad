package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// This opt-in timing test deliberately uses the production Router/Worker path.
// A mocked unit evaluator cannot establish a geometry performance baseline.
// Setup is excluded; all 100 measured steps mutate one evolving Part and commit.
func TestContinuousComplexModeling100BaselineThroughRouter(t *testing.T) {
	service, artifacts, db, _ := featureAssociationTestService(t)
	setupStart := time.Now()
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "100-step complex modeling baseline"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) time.Duration {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-baseline-%03d", view.Document.ID, seq)
		before := view.Document.VersionID
		start := time.Now()
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		elapsed := time.Since(start)
		if e != nil {
			t.Fatalf("command %d %s: %v", seq, r.Type, e)
		}
		if v.Document.VersionID == before {
			t.Fatal("modeling command did not commit a new Revision")
		}
		view = v
		for _, f := range view.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatalf("%s: %s", f.Type, f.Diagnostic)
			}
		}
		return elapsed
	}
	last := func() workspace.Feature { return view.Part.Features[len(view.Part.Features)-1] }
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	stockSketch := last().ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: stockSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -30, Y: -30}, Second: &workspace.SketchPoint2{X: 30, Y: 30}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stockSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 8})
	stock := last()
	circle := func(plane string, radius float64, constrain bool) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
		id := last().ID
		ops := []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "section-circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 10}, Radius: radius}}}
		if constrain {
			ops = append(ops, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "section-radius", Kind: "RADIUS", Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "section-circle", SubElement: "WHOLE"}}, Value: &radius}})
		}
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: ops})
		return id
	}
	bottom := circle("datum-xy", 2, true)
	axis := workspace.DatumReference{Kind: "AXIS_SYSTEM", EntityID: view.Part.AxisSystems[0].ID, Axis: "Z"}
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "associated loft section", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: "datum-xy"}, TranslationDirection: &axis, Distance: 20}})
	plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
	top := circle(plane, 1, false)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", BodyID: stock.BodyID, Operation: "REMOVE", Sections: []workspace.LoftSection{{SketchID: bottom}, {SketchID: top}}}})
	seed := last()
	apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: stock.BodyID, Operation: "REMOVE", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 2, Direction: [3]float64{0, 0, 1}}, SourceKind: "GENERATOR_TOOL", Source: workspace.FeatureStageRef{BodyID: stock.BodyID, FeatureID: seed.ID}, ResultMode: "COMBINE"}}})
	pattern := last()
	member := findP7TopologyPick(t, service, view, "EDGE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == pattern.ID && strings.HasPrefix(s.Anchor.OutputSlot, "MEMBER/1/") && p.GeometryType == "CIRCLE" && math.Abs(s.CreationEvidence.Centroid[2]) < 1e-5
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: stock.BodyID, Length: 0.15, Selections: []workspace.FeatureSelection{{Selection: member.Selection, SourceVersionID: member.SourceVersionID}}}})
	chamfer := last()
	setupMS := float64(time.Since(setupStart)) / float64(time.Millisecond)

	report := modelingBaselineReport{SchemaVersion: 1, Workload: "complex-parametric-part-100-v1", CreatedAt: time.Now().UTC().Format(time.RFC3339), GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), SetupMS: setupMS, Steps: make([]modelingBaselineStep, 0, 100)}
	report.OCCTVersion = activeBodyArtifact(t, view).OCCTVersion
	seriesStart := time.Now()
	for round := 1; round <= 20; round++ {
		updates := []struct {
			name, id, unit string
			value          float64
		}{
			{"stock-height", "parameter:" + stock.ID + ":length", "mm", 8 + float64(round)*0.01},
			{"loft-profile-radius", "parameter:" + bottom + ":constraint:section-radius:value", "mm", 2 + float64(round)*0.01},
			{"associated-datum-height", "parameter:" + plane + ":datum:distance", "mm", 20 + float64(round)*0.05},
			{"pattern-member-count", "parameter:" + pattern.ID + ":pattern:count", "1", float64(2 + round%3)},
			{"member-chamfer", "parameter:" + chamfer.ID + ":length", "mm", 0.15 + float64(round)*0.002},
		}
		for _, u := range updates {
			before := activeBodyArtifact(t, view).GeometryID
			elapsed := apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: u.id, Unit: u.unit, Value: u.value})
			a := activeBodyArtifact(t, view)
			if !(a.Volume > 0) || math.IsNaN(a.Volume) || math.IsInf(a.Volume, 0) || p6TopologyCount(t, a.Topology, "solids") != 1 {
				t.Fatal("invalid final solid")
			}
			if a.GeometryID == before {
				t.Fatalf("step %d %s did not change actual geometry", len(report.Steps)+1, u.name)
			}
			var raw []byte
			if err := db.QueryRow(t.Context(), `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var manifest modelcore.EvaluationManifest
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			var execution workspace.EvaluationRuntime
			if len(manifest.Runtime) == 0 {
				t.Fatal("actual execution evidence missing")
			}
			if err := json.Unmarshal(manifest.Runtime, &execution); err != nil {
				t.Fatal(err)
			}
			step := modelingBaselineStep{Index: len(report.Steps) + 1, Round: round, Operation: u.name, Value: u.value, Unit: u.unit, WallMS: float64(elapsed) / float64(time.Millisecond), GeometryID: a.GeometryID, Volume: a.Volume}
			for _, w := range execution.WorkerRuns {
				step.Executed += int(w.StagesExecuted)
				step.Reused += int(w.StagesReused)
				step.Generators += int(w.GeneratorCalls)
				step.Modifiers += int(w.ModifierCalls)
				step.BodyOperations += int(w.BodyOperationCalls)
				step.ExactMS += w.ExactMs
				step.NamingMS += w.NamingMs
				step.MeshMS += w.MeshMs
				step.EncodingMS += w.EncodingMs
				step.QueueMS += w.QueueMs
				step.IOMS += w.ArtifactIoMs
				step.PeakRSSBytes = max(step.PeakRSSBytes, w.PeakRssBytes)
				step.StageCacheBytes = max(step.StageCacheBytes, w.StageCacheBytes)
			}
			for key, n := range execution.Nodes {
				if strings.HasPrefix(string(key), "profile:") {
					step.Profiles += n.Executions
				}
				if strings.HasPrefix(string(key), "loft-correspondence:") {
					step.Correspondence += n.Executions
				}
			}
			if step.Executed == 0 || step.Generators+step.Modifiers+step.BodyOperations == 0 {
				t.Fatalf("step %d reused a response without geometry computation", step.Index)
			}
			report.EvaluatorVersion = manifest.EvaluatorDigest
			report.Steps = append(report.Steps, step)
			t.Logf("BASELINE_STEP index=%d operation=%s wall_ms=%.2f executed=%d reused=%d exact_ms=%.2f naming_ms=%.2f", step.Index, step.Operation, step.WallMS, step.Executed, step.Reused, step.ExactMS, step.NamingMS)
		}
		t.Logf("BASELINE_PROGRESS committed=%d/100 elapsed_s=%.2f", len(report.Steps), time.Since(seriesStart).Seconds())
	}
	report.SeriesMS = float64(time.Since(seriesStart)) / float64(time.Millisecond)
	for _, s := range report.Steps {
		report.CommandMS += s.WallMS
		report.PeakRSSBytes = max(report.PeakRSSBytes, s.PeakRSSBytes)
	}
	report.Overall = summarizeModelingBaseline(report.Steps)
	report.ByOperation = map[string]modelingBaselineDistribution{}
	for _, s := range report.Steps {
		if _, ok := report.ByOperation[s.Operation]; !ok {
			var group []modelingBaselineStep
			for _, other := range report.Steps {
				if other.Operation == s.Operation {
					group = append(group, other)
				}
			}
			report.ByOperation[s.Operation] = summarizeModelingBaseline(group)
		}
	}
	// Only the final state is force-cold verified, outside the measured 100 commits.
	// No shared prepare/stage/response/display cache participates in this oracle.
	final := activeBodyArtifact(t, view)
	coldStart := time.Now()
	cold, err := service.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: "baseline-final-cold", Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + chamfer.ID + ":length", Unit: "mm", Value: 0.19})
	if err != nil {
		t.Fatal(err)
	}
	report.ColdCheckMS = float64(time.Since(coldStart)) / float64(time.Millisecond)
	if cold.Artifact == nil || cold.Artifact.GeometryID != final.GeometryID || math.Abs(cold.Artifact.Volume-final.Volume) > 1e-7 {
		t.Fatal("final state differs from real cold rebuild")
	}
	read := func(a workspace.Artifact, role string) []byte {
		t.Helper()
		_, r, e := artifacts.Open(t.Context(), a.Representations[role].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		b, e := io.ReadAll(r)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if !slices.Equal(read(final, role), read(*cold.Artifact, role)) {
			t.Fatal("cold rebuild differs in", role)
		}
	}
	service.DiscardPreview(view.Document.ID, p6Actor, cold.PreviewID)
	if current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor); e != nil || current.Document.VersionID != view.Document.VersionID {
		t.Fatal("cold oracle changed final Head", e)
	}
	report.ColdEquivalent = true
	summary, _ := json.Marshal(report.Overall)
	t.Logf("BASELINE_SUMMARY workload=%s steps=%d setup_ms=%.2f command_ms=%.2f series_ms=%.2f peak_rss_bytes=%d distribution=%s", report.Workload, len(report.Steps), report.SetupMS, report.CommandMS, report.SeriesMS, report.PeakRSSBytes, summary)
	if path := os.Getenv("OCCCCAD_MODELING_BASELINE_OUTPUT"); path != "" {
		if !filepath.IsAbs(path) {
			t.Fatal("OCCCCAD_MODELING_BASELINE_OUTPUT must be absolute")
		}
		data, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, append(data, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
		t.Logf("BASELINE_REPORT %s", path)
	}
}

type modelingBaselineStep struct {
	Index           int     `json:"index"`
	Round           int     `json:"round"`
	Operation       string  `json:"operation"`
	Value           float64 `json:"value"`
	Unit            string  `json:"unit"`
	WallMS          float64 `json:"wallMs"`
	GeometryID      string  `json:"geometryId"`
	Volume          float64 `json:"volumeMm3"`
	Executed        int     `json:"stagesExecuted"`
	Reused          int     `json:"stagesReused"`
	Generators      int     `json:"generatorCalls"`
	Modifiers       int     `json:"modifierCalls"`
	BodyOperations  int     `json:"bodyOperationCalls"`
	Profiles        int     `json:"profileCalls"`
	Correspondence  int     `json:"correspondenceCalls"`
	ExactMS         float64 `json:"exactMs"`
	NamingMS        float64 `json:"namingMs"`
	MeshMS          float64 `json:"meshMs"`
	EncodingMS      float64 `json:"encodingMs"`
	QueueMS         float64 `json:"queueMs"`
	IOMS            float64 `json:"ioMs"`
	PeakRSSBytes    uint64  `json:"workerLifetimePeakRssBytes"`
	StageCacheBytes uint64  `json:"stageCacheBytes"`
}
type modelingBaselineDistribution struct {
	Count  int     `json:"count"`
	MeanMS float64 `json:"meanMs"`
	P50MS  float64 `json:"p50Ms"`
	P95MS  float64 `json:"p95Ms"`
	P99MS  float64 `json:"p99Ms"`
	MinMS  float64 `json:"minMs"`
	MaxMS  float64 `json:"maxMs"`
}
type modelingBaselineReport struct {
	SchemaVersion    int                                     `json:"schemaVersion"`
	Workload         string                                  `json:"workload"`
	CreatedAt        string                                  `json:"createdAt"`
	GoVersion        string                                  `json:"goVersion"`
	Platform         string                                  `json:"platform"`
	LogicalCPUs      int                                     `json:"logicalCpus"`
	OCCTVersion      string                                  `json:"occtVersion"`
	EvaluatorVersion string                                  `json:"evaluatorVersion"`
	SetupMS          float64                                 `json:"setupMs"`
	CommandMS        float64                                 `json:"commandMs"`
	SeriesMS         float64                                 `json:"seriesMs"`
	ColdCheckMS      float64                                 `json:"coldCheckMs"`
	ColdEquivalent   bool                                    `json:"coldEquivalent"`
	PeakRSSBytes     uint64                                  `json:"workerLifetimePeakRssBytes"`
	Overall          modelingBaselineDistribution            `json:"overall"`
	ByOperation      map[string]modelingBaselineDistribution `json:"byOperation"`
	Steps            []modelingBaselineStep                  `json:"steps"`
}

func summarizeModelingBaseline(steps []modelingBaselineStep) modelingBaselineDistribution {
	samples := make([]float64, len(steps))
	var total float64
	for i, s := range steps {
		samples[i] = s.WallMS
		total += s.WallMS
	}
	slices.Sort(samples)
	percentile := func(p float64) float64 { return samples[int(math.Ceil(p*float64(len(samples))))-1] }
	return modelingBaselineDistribution{Count: len(samples), MeanMS: total / float64(len(samples)), P50MS: percentile(.5), P95MS: percentile(.95), P99MS: percentile(.99), MinMS: samples[0], MaxMS: samples[len(samples)-1]}
}
