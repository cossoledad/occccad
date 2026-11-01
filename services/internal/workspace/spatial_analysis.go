package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/occccad/occccad/internal/database"
	"io"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeSetSpatialAnalyses = "occccad://product/spatial-analyses/set"
const spatialPolicy = "accepted-spatial-manual-v1"

// Definitions and accepted evidence are persistent; currency is a read projection.
// No mesh, triangles or BREP are stored in the parameter model.
type SpatialAnalysis struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	Kind            string                  `json:"kind"`
	Scope           string                  `json:"scope"`
	Selected        []InstancePath          `json:"selected,omitempty"`
	SecondSelection []InstancePath          `json:"secondSelection,omitempty"`
	ClearanceMM     float64                 `json:"clearanceMm"`
	SectionType     string                  `json:"sectionType,omitempty"`
	Frame           InstancePose            `json:"frame"`
	Size            [3]float64              `json:"size"`
	CutMode         string                  `json:"cutMode,omitempty"`
	MinimumMM       float64                 `json:"minimumMm"`
	MaximumMM       float64                 `json:"maximumMm"`
	AccuracyMM      float64                 `json:"accuracyMm"`
	Result          *SpatialResultReference `json:"result,omitempty"`
	Status          string                  `json:"status,omitempty"`
}
type SpatialResultReference struct {
	ArtifactID       string `json:"artifactId"`
	SHA256           string `json:"sha256"`
	SourceRevisionID string `json:"sourceRevisionId"`
	SourceStamp      string `json:"sourceStamp"`
	Policy           string `json:"policy"`
}
type SpatialAnalysisRequest struct {
	RevisionID string          `json:"revisionId"`
	Definition SpatialAnalysis `json:"definition"`
}
type SpatialAnalysisReport struct {
	DocumentID  string                          `json:"documentId"`
	RevisionID  string                          `json:"revisionId"`
	Definition  SpatialAnalysis                 `json:"definition"`
	SourceStamp string                          `json:"sourceStamp"`
	Policy      string                          `json:"policy"`
	Check       StaticInterferenceReport        `json:"check"`
	Geometry    *geometry.SpatialGeometryReport `json:"geometry,omitempty"`
	Complete    bool                            `json:"complete"`
}

func spatialMutation(t string) bool {
	return t == "SAVE_SPATIAL_ANALYSIS" || t == "UPDATE_SPATIAL_ANALYSES" || t == "DELETE_SPATIAL_ANALYSIS"
}
func spatialOnlyHistory(before, after ProductModel, c modelcore.ChangeSet) bool {
	if len(c.Changes) == 0 || len(c.ImpactSeeds) > 0 {
		return false
	}
	for _, v := range c.Changes {
		if v.Target.SlotID != "product.spatial-analyses" || v.Target.EntityID != "product" {
			return false
		}
	}
	before.SpatialAnalyses = nil
	after.SpatialAnalyses = nil
	return reflect.DeepEqual(before, after)
}
func applySpatialAnalyses(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var m ProductModel
	var values []SpatialAnalysis
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if len(values) > 128 {
		return nil, modelcore.ChangeSet{}, spatialError("SPATIAL_ANALYSIS_LIMIT")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, a := range values {
		if uuid.Validate(a.ID) != nil || ids[a.ID] || names[scopedNameKey(a.Name)] || strings.TrimSpace(a.Name) == "" || a.Result == nil || a.Result.Policy != spatialPolicy {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid spatial analysis definition", ErrValidation)
		}
		ids[a.ID] = true
		names[scopedNameKey(a.Name)] = true
	}
	if reflect.DeepEqual(m.SpatialAnalyses, values) {
		return raw, modelcore.ChangeSet{}, nil
	}
	change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "product", SlotID: "product.spatial-analyses"}, m.SpatialAnalyses, values)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	m.SpatialAnalyses = values
	next, err := json.Marshal(m)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}}, err
}
func normalizeSpatial(a SpatialAnalysis) SpatialAnalysis {
	if a.Scope == "ALL" {
		a.Selected = nil
		a.SecondSelection = nil
	} else if a.Scope != "BETWEEN" {
		a.SecondSelection = nil
	}
	return a
}
func spatialDefinition(a SpatialAnalysis) SpatialAnalysis {
	a = normalizeSpatial(a)
	canonical := func(paths []InstancePath) []InstancePath {
		values := []InstancePath{}
		for _, p := range paths {
			values = append(values, InstancePath{RootDocumentID: p.RootDocumentID, Canonical: p.Canonical})
		}
		slices.SortFunc(values, func(a, b InstancePath) int { return strings.Compare(a.Canonical, b.Canonical) })
		return values
	}
	a.Selected = canonical(a.Selected)
	a.SecondSelection = canonical(a.SecondSelection)
	a.Result = nil
	a.Status = ""
	a.Name = ""
	a.ID = ""
	return a
}
func spatialError(message string) error { return fmt.Errorf("%w: %s", ErrValidation, message) }
func validateSpatial(a SpatialAnalysis) error {
	if !slices.Contains([]string{"CLASH", "SECTION", "DISTANCE", "BAND"}, a.Kind) || !slices.Contains([]string{"ALL", "SELECTED", "AGAINST_ALL", "BETWEEN"}, a.Scope) {
		return spatialError("SPATIAL_TYPE_INVALID")
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for _, v := range append(append(append([]float64{}, a.Frame.Translation[:]...), a.Frame.Rotation[:]...), append(a.Size[:], a.ClearanceMM, a.MinimumMM, a.MaximumMM, a.AccuracyMM)...) {
		if !finite(v) {
			return spatialError("SPATIAL_NUMBER_INVALID")
		}
	}
	if a.ClearanceMM < 0 {
		return spatialError("SPATIAL_CLEARANCE_INVALID")
	}
	if a.Kind == "SECTION" {
		if a.Scope != "ALL" && a.Scope != "SELECTED" {
			return spatialError("SPATIAL_SECTION_SCOPE_INVALID")
		}
		if !slices.Contains([]string{"PLANE", "SLICE", "BOX"}, a.SectionType) || !slices.Contains([]string{"NONE", "KEEP_INSIDE", "KEEP_OUTSIDE"}, a.CutMode) {
			return spatialError("SPATIAL_SECTION_TYPE_INVALID")
		}
		if a.Size[0] <= 0 || a.Size[1] <= 0 || a.Size[2] <= 0 {
			return spatialError("SPATIAL_SECTION_SIZE_INVALID")
		}
		q := a.Frame.Rotation
		n := q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3]
		if math.Abs(n-1) > 1e-6 {
			return spatialError("SPATIAL_FRAME_INVALID")
		}
	}
	if (a.Kind == "SECTION" || a.Kind == "BAND") && (a.AccuracyMM < 0.01 || a.AccuracyMM > 1000) {
		return spatialError("SPATIAL_ACCURACY_INVALID")
	}
	if a.Kind == "BAND" {
		for _, first := range a.Selected {
			for _, second := range a.SecondSelection {
				if first.Canonical == second.Canonical || strings.HasPrefix(first.Canonical, second.Canonical+"/") || strings.HasPrefix(second.Canonical, first.Canonical+"/") {
					return spatialError("SPATIAL_BAND_OVERLAP")
				}
			}
		}
	}
	if a.Kind == "BAND" && (a.Scope != "BETWEEN" || a.MinimumMM < 0 || a.MaximumMM <= a.MinimumMM) {
		return spatialError("SPATIAL_BAND_RANGE_INVALID")
	}
	return nil
}

// This stamp deliberately excludes owning Revision, labels and analysis metadata.
// Only accepted source versions, context values and saved component poses matter.
func (s *Service) spatialSourceStamp(ctx context.Context, doc, rev string, a SpatialAnalysis) (string, error) {
	items, err := s.expandProductContext(withAssemblyReadCache(ctx), doc, rev)
	if err != nil {
		return "", err
	}
	paths := append(append([]InstancePath{}, a.Selected...), a.SecondSelection...)
	for _, p := range paths {
		item, ok := occurrenceByCanonical(items, p.Canonical)
		if !ok {
			return "", spatialError("SPATIAL_SOURCE_MISSING")
		}
		if len(p.Segments) != len(item.Path.Segments) {
			return "", spatialError("SPATIAL_SOURCE_CHANGED")
		}
		for i, v := range p.Segments {
			if v.InstanceID != item.Path.Segments[i].InstanceID {
				return "", spatialError("SPATIAL_SOURCE_CHANGED")
			}
		}
	}
	type source struct {
		Path, Document string
		Geometry       []string
		Pose           InstancePose
		Bindings       []ContextBinding
	}
	sources := []source{}
	reads := []modelcore.DependencyRead{}
	var base *InstancePose
	for _, v := range items {
		if v.DocumentType != "PART" {
			continue
		}
		included := a.Scope == "ALL" || a.Scope == "AGAINST_ALL"
		for _, p := range paths {
			if p.Canonical == v.Path.Canonical || strings.HasPrefix(v.Path.Canonical, p.Canonical+"/") {
				included = true
			}
		}
		if !included {
			continue
		}
		bindings := []ContextBinding{}
		for _, owner := range items {
			for _, binding := range owner.ContextBindings {
				if binding.OwningInstancePath.Canonical == v.Path.Canonical {
					bindings = append(bindings, binding)
				}
			}
		}
		slices.SortFunc(bindings, func(a, b ContextBinding) int { return strings.Compare(a.ID, b.ID) })
		for i := range bindings {
			b := &bindings[i]
			b.Name = ""
			b.OwningInstancePath.Display = ""
			b.SourceInstancePath.Display = ""
			b.Accepted.RootProductRevisionID = ""
			if len(b.OwningInstancePath.Segments) > 0 {
				b.OwningInstancePath.Segments = append([]InstancePathSegment(nil), b.OwningInstancePath.Segments...)
				b.OwningInstancePath.Segments[0].OwnerVersionID = ""
				for i := range b.OwningInstancePath.Segments {
					b.OwningInstancePath.Segments[i].InstanceName = ""
				}
			}
			if len(b.SourceInstancePath.Segments) > 0 {
				b.SourceInstancePath.Segments = append([]InstancePathSegment(nil), b.SourceInstancePath.Segments...)
				b.SourceInstancePath.Segments[0].OwnerVersionID = ""
				for i := range b.SourceInstancePath.Segments {
					b.SourceInstancePath.Segments[i].InstanceName = ""
				}
			}
		}
		_, raw, err := s.measurementModel(ctx, v.DocumentID, v.RevisionID)
		if err != nil {
			return "", err
		}
		var part PartModel
		if err = json.Unmarshal(raw, &part); err != nil {
			return "", err
		}
		geometries := []string{}
		for _, body := range part.Bodies {
			if body.Consumed {
				continue
			}
			geometries = append(geometries, body.ID+"/"+body.GeometryKey)
		}
		slices.Sort(geometries)
		pose := v.Pose
		if base == nil {
			copy := pose
			base = &copy
		}
		if a.Kind == "CLASH" || a.Kind == "DISTANCE" {
			pose = inverseRelativePose(pose, *base)
		}
		sources = append(sources, source{v.Path.Canonical, v.DocumentID, geometries, pose, bindings})
		reads = append(reads, modelcore.DependencyRead{DocumentID: v.DocumentID, Occurrence: v.Path.Canonical, EntityID: v.DocumentID, Facet: modelcore.IntrinsicGeometry, Digest: resolvedDigest(geometries), RevisionID: v.RevisionID}, modelcore.DependencyRead{DocumentID: v.DocumentID, Occurrence: v.Path.Canonical, EntityID: v.DocumentID, Facet: modelcore.Placement, Digest: resolvedDigest(pose)}, modelcore.DependencyRead{DocumentID: v.DocumentID, Occurrence: v.Path.Canonical, EntityID: v.DocumentID, Facet: modelcore.Configuration, Digest: resolvedDigest(bindings)})
	}
	// Definition path snapshots are checked/rebased independently; the stamp uses stable paths.
	d := spatialDefinition(a)
	d.Selected = nil
	d.SecondSelection = nil
	reads = append(reads, modelcore.DependencyRead{DocumentID: doc, EntityID: "analysis-scope", Facet: modelcore.ScopeMembership, Digest: resolvedDigest(sources)})
	return analysisInputStamp("spatial-v3-facets", spatialPolicy, struct {
		Definition       SpatialAnalysis
		Selected, Second []string
	}{d, spatialCanonicals(a.Selected), spatialCanonicals(a.SecondSelection)}, reads)
}
func spatialCanonicals(paths []InstancePath) []string {
	out := []string{}
	for _, p := range paths {
		out = append(out, p.Canonical)
	}
	slices.Sort(out)
	return out
}
func (s *Service) rebaseSpatial(ctx context.Context, doc, rev string, a SpatialAnalysis) (SpatialAnalysis, error) {
	items, err := s.expandProductContext(withAssemblyReadCache(ctx), doc, rev)
	if err != nil {
		return a, err
	}
	rebase := func(paths []InstancePath) ([]InstancePath, error) {
		out := []InstancePath{}
		seen := map[string]bool{}
		for _, p := range paths {
			v, ok := occurrenceByCanonical(items, p.Canonical)
			if !ok {
				return nil, spatialError("SPATIAL_SOURCE_MISSING")
			}
			if p.RootDocumentID != doc || len(p.Segments) != len(v.Path.Segments) {
				return nil, spatialError("SPATIAL_SOURCE_CHANGED")
			}
			for i, x := range p.Segments {
				if x.InstanceID != v.Path.Segments[i].InstanceID {
					return nil, spatialError("SPATIAL_SOURCE_CHANGED")
				}
			}
			if !seen[v.Path.Canonical] {
				out = append(out, v.Path)
				seen[v.Path.Canonical] = true
			}
		}
		return out, nil
	}
	a.Selected, err = rebase(a.Selected)
	if err != nil {
		return a, err
	}
	a.SecondSelection, err = rebase(a.SecondSelection)
	return a, err
}
func (s *Service) AnalyzeSpatial(ctx context.Context, doc string, r SpatialAnalysisRequest) (SpatialAnalysisReport, error) {
	ctx = withAssemblyReadCache(ctx)
	a := normalizeSpatial(r.Definition)
	out := SpatialAnalysisReport{DocumentID: doc, RevisionID: r.RevisionID, Definition: a, Policy: spatialPolicy}
	if err := validateSpatial(a); err != nil {
		return out, err
	}
	if err := s.staticInterferenceRevision(ctx, doc, r.RevisionID); err != nil {
		return out, err
	}
	stamp, err := s.spatialSourceStamp(ctx, doc, r.RevisionID, a)
	if err != nil {
		return out, err
	}
	out.SourceStamp = stamp
	input := StaticInterferenceRequest{RevisionID: r.RevisionID, Scope: a.Scope, Selected: a.Selected, SecondSelection: a.SecondSelection, ClearanceMM: a.ClearanceMM}
	frozen := frozenInterference{singleBody: a.Kind == "SECTION"}
	report, err := s.analyzeInterference(ctx, doc, input, nil, &frozen)
	out.Check = report
	if err != nil {
		return out, err
	}
	if len(report.Issues) > 0 {
		return out, spatialError("SPATIAL_INPUT_UNAVAILABLE")
	}
	if len(frozen.inputs) == 0 {
		return out, spatialError("SPATIAL_MATERIAL_MISSING")
	}
	if a.Kind == "CLASH" || a.Kind == "DISTANCE" {
		if len(frozen.pairs) == 0 {
			return out, spatialError("SPATIAL_PAIR_REQUIRED")
		}
		computed, e := s.worker.AnalyzeInterference(ctx, newID("spatial"), frozen.inputs, frozen.pairs, a.ClearanceMM, staticInterferenceToleranceMM, geometry.InterferenceOptions{})
		if e != nil {
			return out, e
		}
		out.Check.Pairs = computed.Pairs
		out.Check.KernelBuild = computed.KernelBuild
		out.Check.CheckedPairs = 0
		for _, p := range computed.Pairs {
			if p.Complete {
				out.Check.CheckedPairs++
			}
		}
		out.Check.Complete = computed.Complete
		out.Complete = computed.Complete
	} else {
		sourceIDs, targetIDs := []string{}, []string{}
		for _, b := range report.Bodies {
			source := a.Kind == "SECTION"
			for _, p := range a.Selected {
				if p.Canonical == b.InstancePath.Canonical || strings.HasPrefix(b.InstancePath.Canonical, p.Canonical+"/") {
					source = true
				}
			}
			if source {
				sourceIDs = append(sourceIDs, b.ID)
			} else {
				targetIDs = append(targetIDs, b.ID)
			}
		}
		if a.Kind == "SECTION" {
			targetIDs = nil
		}
		if a.Kind == "BAND" && len(sourceIDs) > 16 {
			return out, spatialError("SPATIAL_BAND_SOURCE_LIMIT")
		}
		g, e := s.worker.AnalyzeSpatialGeometry(ctx, newID("spatial"), frozen.inputs, geometry.SpatialOptions{Kind: a.Kind, SourceIDs: sourceIDs, TargetIDs: targetIDs, Frame: geometry.AssemblyPose(a.Frame), SectionType: a.SectionType, Size: a.Size, Minimum: a.MinimumMM, Maximum: a.MaximumMM, Accuracy: a.AccuracyMM})
		if e != nil {
			return out, e
		}
		out.Geometry = &g
		out.Complete = g.Complete
	}
	if err := s.staticInterferenceRevision(ctx, doc, r.RevisionID); err != nil {
		return out, err
	}
	out.Definition.Result = nil
	out.Definition.Status = ""
	return out, nil
}
func (s *Service) prepareSpatialAnalyses(ctx context.Context, doc string, raw json.RawMessage, r CommandRequest) ([]SpatialAnalysis, error) {
	var m ProductModel
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := append([]SpatialAnalysis(nil), m.SpatialAnalyses...)
	if r.Type == "DELETE_SPATIAL_ANALYSIS" {
		if !slices.ContainsFunc(out, func(a SpatialAnalysis) bool { return a.ID == r.TargetID }) {
			return nil, ErrNotFound
		}
		return slices.DeleteFunc(out, func(a SpatialAnalysis) bool { return a.ID == r.TargetID }), nil
	}
	ids := map[string]bool{}
	for _, id := range r.SpatialAnalysisIDs {
		ids[id] = true
	}
	if r.Type == "SAVE_SPATIAL_ANALYSIS" {
		if r.SpatialAnalysis == nil {
			return nil, spatialError("SPATIAL_DEFINITION_REQUIRED")
		}
		a := normalizeSpatial(*r.SpatialAnalysis)
		index := slices.IndexFunc(out, func(v SpatialAnalysis) bool { return v.ID == a.ID })
		if a.ID == "" {
			a.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(doc+"/spatial/"+r.RequestID)).String()
		}
		if uuid.Validate(a.ID) != nil {
			return nil, spatialError("SPATIAL_DEFINITION_INVALID")
		}
		if strings.TrimSpace(a.Name) == "" && index >= 0 {
			a.Name = out[index].Name
		}
		if strings.TrimSpace(a.Name) == "" {
			prefix := map[string]string{"CLASH": "Clash", "SECTION": "Section", "DISTANCE": "Distance", "BAND": "Distance"}[a.Kind]
			for n := 1; ; n++ {
				name := fmt.Sprintf("%s.%d", prefix, n)
				if !slices.ContainsFunc(out, func(v SpatialAnalysis) bool { return scopedNameKey(v.Name) == scopedNameKey(name) }) {
					a.Name = name
					break
				}
			}
		}
		a.Result = nil
		a.Status = ""
		if index >= 0 {
			old := out[index]
			if old.Kind != a.Kind && !(slices.Contains([]string{"DISTANCE", "BAND"}, old.Kind) && slices.Contains([]string{"DISTANCE", "BAND"}, a.Kind)) {
				return nil, spatialError("SPATIAL_TYPE_CHANGED")
			}
			if reflect.DeepEqual(spatialDefinition(old), spatialDefinition(a)) && old.Result != nil && old.Result.Policy == spatialPolicy {
				stamp, e := s.spatialSourceStamp(ctx, doc, r.VersionID, a)
				if e == nil && stamp == old.Result.SourceStamp {
					a.Result = old.Result
				}
			}
			out[index] = a
		} else {
			out = append(out, a)
		}
		ids[a.ID] = true
	}
	found := 0
	for i := range out {
		a := out[i]
		if len(ids) > 0 && !ids[a.ID] {
			continue
		}
		found++
		if r.Type == "SAVE_SPATIAL_ANALYSIS" && a.Result != nil {
			continue
		}
		if r.Type == "UPDATE_SPATIAL_ANALYSES" && a.Result != nil {
			stamp, e := s.spatialSourceStamp(ctx, doc, r.VersionID, a)
			if e == nil && stamp == a.Result.SourceStamp && a.Result.Policy == spatialPolicy {
				if _, e = s.GetSpatialResult(ctx, doc, r.VersionID, a.ID); e == nil {
					continue
				}
			}
		}
		var err error
		a, err = s.rebaseSpatial(ctx, doc, r.VersionID, a)
		if err != nil {
			return nil, err
		}
		result, err := s.AnalyzeSpatial(ctx, doc, SpatialAnalysisRequest{r.VersionID, a})
		if err != nil {
			return nil, err
		}
		if !result.Complete {
			return nil, spatialError("SPATIAL_INCOMPLETE")
		}
		if s.artifacts == nil {
			return nil, spatialError("SPATIAL_STORAGE_UNAVAILABLE")
		}
		data, err := json.Marshal(result)
		if len(data) > 32*1024*1024 {
			return nil, spatialError("SPATIAL_GRAPHICS_LIMIT")
		}
		if err != nil {
			return nil, err
		}
		object, err := s.artifacts.Put(ctx, artifact.KindSpatialAnalysis, "application/json", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		a.Result = &SpatialResultReference{ArtifactID: object.ID, SHA256: object.SHA256, SourceRevisionID: r.VersionID, SourceStamp: result.SourceStamp, Policy: spatialPolicy}
		a.Status = ""
		out[i] = a
	}
	if len(ids) > 0 && found != len(ids) {
		return nil, ErrNotFound
	}
	return out, nil
}
func (s *Service) projectSpatialStatus(ctx context.Context, doc, rev string, m *ProductModel) {
	for i := range m.SpatialAnalyses {
		a := &m.SpatialAnalyses[i]
		a.Status = "STALE"
		if a.Result == nil {
			a.Status = "MISSING"
			continue
		}
		stamp, err := s.spatialSourceStamp(ctx, doc, rev, *a)
		if err != nil {
			a.Status = "UNKNOWN"
			if errors.Is(err, ErrValidation) || errors.Is(err, database.ErrNoRows) || errors.Is(err, ErrNotFound) {
				a.Status = "BROKEN"
			}
		} else if stamp == a.Result.SourceStamp && a.Result.Policy == spatialPolicy {
			a.Status = "CURRENT"
		}
	}
}
func (s *Service) GetSpatialResult(ctx context.Context, doc, rev, id string) (SpatialAnalysisReport, error) {
	var raw []byte
	if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, doc, rev).Scan(&raw); err != nil {
		return SpatialAnalysisReport{}, err
	}
	var m ProductModel
	if err := json.Unmarshal(raw, &m); err != nil {
		return SpatialAnalysisReport{}, err
	}
	index := slices.IndexFunc(m.SpatialAnalyses, func(a SpatialAnalysis) bool { return a.ID == id })
	if index < 0 || m.SpatialAnalyses[index].Result == nil {
		return SpatialAnalysisReport{}, ErrNotFound
	}
	ref := m.SpatialAnalyses[index].Result
	if s.artifacts == nil {
		return SpatialAnalysisReport{}, ErrNotFound
	}
	object, reader, err := s.artifacts.Open(ctx, ref.ArtifactID)
	if err != nil {
		return SpatialAnalysisReport{}, err
	}
	defer reader.Close()
	if object.Kind != artifact.KindSpatialAnalysis || object.SHA256 != ref.SHA256 || object.Size > 32*1024*1024 {
		return SpatialAnalysisReport{}, spatialError("SPATIAL_RESULT_UNAVAILABLE")
	}
	data, err := io.ReadAll(io.LimitReader(reader, 32*1024*1024+1))
	if err != nil {
		return SpatialAnalysisReport{}, err
	}
	if len(data) > 32*1024*1024 {
		return SpatialAnalysisReport{}, spatialError("SPATIAL_GRAPHICS_LIMIT")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != ref.SHA256 {
		return SpatialAnalysisReport{}, spatialError("SPATIAL_RESULT_UNAVAILABLE")
	}
	var out SpatialAnalysisReport
	if err = json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	if out.DocumentID != doc || out.RevisionID != ref.SourceRevisionID || out.SourceStamp != ref.SourceStamp || out.Policy != ref.Policy {
		return out, spatialError("SPATIAL_RESULT_UNAVAILABLE")
	}
	if stamp, e := s.spatialSourceStamp(ctx, doc, rev, m.SpatialAnalyses[index]); e == nil && stamp == ref.SourceStamp {
		out.Check.RevisionID = rev
	}
	return out, nil
}
func spatialStructure(doc, rev, parent string, values []SpatialAnalysis) DocumentStructureNode {
	group := DocumentStructureNode{ID: parent + "/spatial-analyses", Kind: "SPATIAL_ANALYSIS_GROUP", Name: "空间分析", DocumentID: doc, VersionID: rev, Capabilities: []string{"REFRESH"}}
	for _, a := range values {
		status := "STALE"
		if a.Status != "" {
			status = a.Status
		}
		group.Children = append(group.Children, DocumentStructureNode{ID: group.ID + "/" + a.ID, Kind: "SPATIAL_ANALYSIS", EntityID: a.ID, EntityType: a.Kind, Name: a.Name, DocumentID: doc, OwnerDocumentID: doc, VersionID: rev, EvaluationStatus: status, CurrencyStatus: status, Capabilities: []string{"EDIT", "REFRESH", "DELETE"}})
	}
	return group
}
