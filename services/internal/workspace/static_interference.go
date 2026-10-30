package workspace

import (
	"context"
	"errors"
	"fmt"
	"github.com/occccad/occccad/internal/database"
	"math"
	"sort"

	"github.com/occccad/occccad/internal/geometry"
)

// These are the existing AnalyzeInterference Worker budgets. Check the entire
// requested scope before starting; never turn a truncated scope into a pass.
const staticInterferenceBodies = 128
const staticInterferencePairs = 8192
const staticInterferenceBytes = 256 * 1024 * 1024
const staticInterferenceToleranceMM = 1e-7

type StaticInterferenceRequest struct {
	RevisionID  string         `json:"revisionId"`
	Scope       string         `json:"scope"`
	Selected    []InstancePath `json:"selected,omitempty"`
	ClearanceMM float64        `json:"clearanceMm"`
}
type StaticInterferenceBody struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	DocumentID     string       `json:"documentId"`
	RevisionID     string       `json:"revisionId"`
	InstancePath   InstancePath `json:"instancePath"`
	BodyID         string       `json:"bodyId"`
	VariantKey     string       `json:"variantKey,omitempty"`
	GeometryKey    string       `json:"geometryKey,omitempty"`
	Pose           InstancePose `json:"pose"`
	DiagnosticCode string       `json:"diagnosticCode,omitempty"`
	Diagnostic     string       `json:"diagnostic,omitempty"`
}
type StaticInterferenceIssue struct {
	Name           string `json:"name"`
	Occurrence     string `json:"occurrence"`
	DiagnosticCode string `json:"diagnosticCode"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}
type StaticInterferenceReport struct {
	DocumentID     string                      `json:"documentId"`
	RevisionID     string                      `json:"revisionId"`
	InputDigest    string                      `json:"inputDigest"`
	Policy         string                      `json:"policy"`
	Scope          string                      `json:"scope"`
	ClearanceMM    float64                     `json:"clearanceMm"`
	ToleranceMM    float64                     `json:"toleranceMm"`
	Bodies         []StaticInterferenceBody    `json:"bodies"`
	Pairs          []geometry.InterferencePair `json:"pairs"`
	Issues         []StaticInterferenceIssue   `json:"issues"`
	Complete       bool                        `json:"complete"`
	CheckedPairs   int                         `json:"checkedPairs"`
	LayoutParts    int                         `json:"layoutParts"`
	ConsumedBodies int                         `json:"consumedBodies"`
	KernelBuild    string                      `json:"kernelBuild,omitempty"`
}

func staticInterferenceLimit() error {
	return fmt.Errorf("%w: STATIC_INTERFERENCE_LIMIT: at most 128 Bodies, 8192 pairs and 256 MiB unique BREP", ErrValidation)
}

// Only the current immutable owning Product and its saved reference edges are
// read. In particular GetDocument's FOLLOW_HEAD update path is not invoked.
func (s *Service) staticInterferenceRevision(ctx context.Context, doc, rev string) error {
	var head, kind string
	if err := s.database.QueryRow(ctx, `SELECT head_version_id::text,document_type FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, doc).Scan(&head, &kind); err != nil {
		return err
	}
	if kind != "PRODUCT" || head != rev || rev == "" {
		return fmt.Errorf("%w: STATIC_INTERFERENCE_STALE", ErrValidation)
	}
	return ctx.Err()
}

func (s *Service) AnalyzeStaticInterference(ctx context.Context, doc string, r StaticInterferenceRequest) (StaticInterferenceReport, error) {
	out := StaticInterferenceReport{DocumentID: doc, RevisionID: r.RevisionID, Scope: r.Scope, ClearanceMM: r.ClearanceMM, ToleranceMM: staticInterferenceToleranceMM, Policy: "accepted-solid-dmu-v1", Bodies: []StaticInterferenceBody{}, Pairs: []geometry.InterferencePair{}, Issues: []StaticInterferenceIssue{}}
	if math.IsNaN(r.ClearanceMM) || math.IsInf(r.ClearanceMM, 0) || r.ClearanceMM < 0 || (r.Scope != "ALL" && r.Scope != "SELECTED") || r.Scope == "ALL" && len(r.Selected) > 0 {
		return out, fmt.Errorf("%w: STATIC_INTERFERENCE_INPUT_INVALID", ErrValidation)
	}
	if err := s.staticInterferenceRevision(ctx, doc, r.RevisionID); err != nil {
		return out, err
	}
	ctx = withAssemblyReadCache(ctx)
	items, err := s.expandProductContext(ctx, doc, r.RevisionID)
	if err != nil {
		return out, err
	}
	selected := map[string]bool{}
	for _, path := range r.Selected {
		item, ok := occurrenceByCanonical(items, path.Canonical)
		if !ok || item.DocumentType != "PART" {
			return out, fmt.Errorf("%w: select accepted Part leaves", ErrValidation)
		}
		if err := validateResolvedInstancePath(path, item.Path); err != nil {
			return out, err
		}
		selected[path.Canonical] = true
	}
	if r.Scope == "SELECTED" && len(selected) < 2 {
		return out, fmt.Errorf("%w: STATIC_INTERFERENCE_SELECTION_REQUIRED", ErrValidation)
	}
	chosen := map[string]PartModel{}
	// Establish expected material from definitions, not resolved display rows.
	expected := 0
	for _, item := range items {
		if item.DocumentType != "PART" || r.Scope == "SELECTED" && !selected[item.Path.Canonical] {
			continue
		}
		source, e := s.readAssemblyPartModel(ctx, item.DocumentID, item.RevisionID)
		if e != nil {
			return out, e
		}
		chosen[item.Path.Canonical] = source.model
		for _, body := range source.model.Bodies {
			if body.Consumed {
				continue
			}
			for _, f := range source.model.Features {
				if f.BodyID == body.ID && isBodyFeature(f.Type) && !f.Suppressed {
					expected++
					break
				}
			}
		}
		if expected > staticInterferenceBodies {
			return out, staticInterferenceLimit()
		}
	}
	// Reuse accepted projections, including nested bindings rebased by expand.
	// Unselected variants are not computed and cannot consume this scope's budget.
	scoped := make([]expandedOccurrence, 0, len(items))
	for _, item := range items {
		if item.DocumentType == "PART" {
			if _, ok := chosen[item.Path.Canonical]; ok {
				scoped = append(scoped, item)
			}
			continue
		}
		bindings := []ContextBinding{}
		for _, b := range item.ContextBindings {
			if _, ok := chosen[b.OwningInstancePath.Canonical]; ok {
				bindings = append(bindings, b)
			}
		}
		item.ContextBindings = bindings
		scoped = append(scoped, item)
	}
	variants, err := s.acceptedContextVariants(ctx, scoped)
	if err != nil {
		return out, err
	}
	byPath := map[string]ContextVariantSnapshot{}
	for _, v := range variants {
		byPath[v.OwningInstancePath.Canonical] = v
	}
	valid := map[string]geometry.AnalysisGeometry{}
	for _, item := range items {
		model, ok := chosen[item.Path.Canonical]
		if !ok {
			continue
		}
		variant, hasVariant := byPath[item.Path.Canonical]
		invalid := hasVariant && (variant.Status != "READY" || variant.Part == nil)
		if hasVariant && !invalid {
			model = *variant.Part
		}
		name := item.Path.Display
		if name == "" {
			name = item.Name
		}
		if invalid {
			out.Issues = append(out.Issues, StaticInterferenceIssue{Name: name, Occurrence: item.Path.Canonical, DiagnosticCode: "VARIANT_UNAVAILABLE", Diagnostic: variant.Diagnostic})
		}
		material := false
		for _, body := range model.Bodies {
			if body.Consumed {
				out.ConsumedBodies++
				continue
			}
			required, failed := false, false
			for _, f := range model.Features {
				if f.BodyID == body.ID && !f.Suppressed && isBodyFeature(f.Type) {
					required = true
					failed = failed || f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED"
				}
			}
			if !required {
				continue
			}
			material = true
			b := StaticInterferenceBody{ID: item.Path.Canonical + "/body:" + body.ID, Name: name + " / " + body.Name, DocumentID: item.DocumentID, RevisionID: item.RevisionID, InstancePath: item.Path, BodyID: body.ID, Pose: item.Pose, GeometryKey: body.GeometryKey, VariantKey: variant.VariantKey}
			var input geometry.AnalysisGeometry
			switch {
			case invalid:
				b.DiagnosticCode = "VARIANT_UNAVAILABLE"
			case failed:
				b.DiagnosticCode = "BODY_EVALUATION_FAILED"
			case body.GeometryKey == "":
				b.DiagnosticCode = "BREP_MISSING"
			default:
				input = geometry.AnalysisGeometry{ID: b.ID, Pose: geometry.AssemblyPose(item.Pose)}
				e := s.database.QueryRow(ctx, `SELECT geometry_id FROM occccad.geometry_artifacts WHERE geometry_key=$1`, body.GeometryKey).Scan(&input.GeometryID)
				if e == nil {
					input.BRep, e = s.brepArtifactReference(ctx, body.GeometryKey)
				}
				if e != nil && !errors.Is(e, database.ErrNoRows) {
					return out, e
				}
				if e != nil || input.GeometryID == "" || input.BRep.Size <= 0 || input.BRep.SHA256 == "" {
					b.DiagnosticCode = "BREP_MISSING"
					if e != nil {
						b.Diagnostic = e.Error()
					}
				} else {
					valid[b.ID] = input
				}
			}
			if b.DiagnosticCode != "" {
				out.Issues = append(out.Issues, StaticInterferenceIssue{Name: b.Name, Occurrence: item.Path.Canonical, DiagnosticCode: b.DiagnosticCode, Diagnostic: b.Diagnostic})
			}
			out.Bodies = append(out.Bodies, b)
		}
		if !material && !invalid {
			out.LayoutParts++
		}
	}
	sort.Slice(out.Bodies, func(i, j int) bool { return out.Bodies[i].ID < out.Bodies[j].ID })
	if len(out.Bodies) > staticInterferenceBodies {
		return out, staticInterferenceLimit()
	}
	pairs := []geometry.AnalysisPair{}
	used := map[string]bool{}
	for i, a := range out.Bodies {
		for _, b := range out.Bodies[i+1:] {
			if a.InstancePath.Canonical == b.InstancePath.Canonical {
				continue
			}
			out.Pairs = append(out.Pairs, geometry.InterferencePair{FirstID: a.ID, SecondID: b.ID, Classification: "INCONCLUSIVE", Diagnostic: "INPUT_UNAVAILABLE"})
			if _, ok := valid[a.ID]; !ok {
				continue
			}
			if _, ok := valid[b.ID]; !ok {
				continue
			}
			pairs = append(pairs, geometry.AnalysisPair{FirstID: a.ID, SecondID: b.ID})
			used[a.ID], used[b.ID] = true, true
		}
	}
	if len(out.Pairs) > staticInterferencePairs {
		return out, staticInterferenceLimit()
	}
	inputs := []geometry.AnalysisGeometry{}
	unique := map[string]bool{}
	var bytes int64
	for _, b := range out.Bodies {
		if !used[b.ID] {
			continue
		}
		g := valid[b.ID]
		inputs = append(inputs, g)
		key := g.BRep.ObjectKey + ":" + g.BRep.SHA256
		if !unique[key] {
			if g.BRep.Size > staticInterferenceBytes-bytes {
				return out, staticInterferenceLimit()
			}
			bytes += g.BRep.Size
			unique[key] = true
		}
	}
	if bytes > staticInterferenceBytes {
		return out, staticInterferenceLimit()
	}
	out.InputDigest = resolvedDigest(struct {
		Revision, Policy, Scope string
		Clearance               float64
		Bodies                  []StaticInterferenceBody
		Inputs                  []geometry.AnalysisGeometry
	}{r.RevisionID, out.Policy, r.Scope, r.ClearanceMM, out.Bodies, inputs})
	if len(pairs) > 0 {
		report, e := s.worker.AnalyzeInterference(ctx, newID("static-analysis"), inputs, pairs, r.ClearanceMM, out.ToleranceMM)
		if e != nil {
			return out, e
		}
		computed := map[geometry.AnalysisPair]geometry.InterferencePair{}
		for _, p := range report.Pairs {
			computed[geometry.AnalysisPair{FirstID: p.FirstID, SecondID: p.SecondID}] = p
		}
		for i, p := range out.Pairs {
			if v, ok := computed[geometry.AnalysisPair{FirstID: p.FirstID, SecondID: p.SecondID}]; ok {
				out.Pairs[i] = v
				if v.Complete {
					out.CheckedPairs++
				}
			}
		}
		out.KernelBuild = report.KernelBuild
		out.Complete = report.Complete && len(out.Issues) == 0 && len(out.Pairs) > 0 && out.CheckedPairs == len(out.Pairs)
	}
	if err := s.staticInterferenceRevision(ctx, doc, r.RevisionID); err != nil {
		return out, err
	}
	return out, nil
}
