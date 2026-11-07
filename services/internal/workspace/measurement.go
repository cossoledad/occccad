package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeSetMeasurements = "occccad://document/measurements/set"
const measurementPolicy = "exact-boundary-owner-frame-v1"
const measurementEvaluatorVersion = "measurement-v3-facets"
const measurementSlot = "document.measurements"

// Display picks may carry locators. Saved definitions contain only typed stable
// references and the source Revision needed for semantic continuation.
type MeasurementSupport struct {
	Kind             string                         `json:"kind"`
	BodyID           string                         `json:"bodyId,omitempty"`
	InstancePath     *InstancePath                  `json:"instancePath,omitempty"`
	Selection        *modelcore.PersistentSelection `json:"persistentSelection,omitempty"`
	SourceRevisionID string                         `json:"sourceRevisionId,omitempty"`
	Mode             string                         `json:"mode"`
	GeometryKey      string                         `json:"geometryKey,omitempty"`
	LocalID          uint64                         `json:"localId,omitempty"`
}
type Measurement struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	Kind           string                `json:"kind"`
	Supports       []MeasurementSupport  `json:"supports"`
	Visible        bool                  `json:"visible"`
	ShowComponents bool                  `json:"showComponents"`
	Result         *MeasurementEvidence  `json:"result,omitempty"`
	Status         string                `json:"status,omitempty"`
	OutputCurrency map[string]string     `json:"outputCurrency,omitempty"`
	DisplayAnchors map[string][3]float64 `json:"displayAnchors,omitempty"`
}
type MeasurementEvidence struct {
	EvaluatorVersion string                     `json:"evaluatorVersion,omitempty"`
	Reads            []modelcore.DependencyRead `json:"reads,omitempty"`
	InputStamps      map[string]string          `json:"inputStamps,omitempty"`
	AnchorFrame      InstancePose               `json:"anchorFrame"`
	SourceRevisionID string                     `json:"sourceRevisionId"`
	SourceStamp      string                     `json:"sourceStamp"`
	Policy           string                     `json:"policy"`
	geometry.MeasurementResult
}
type MeasurementRequest struct {
	RevisionID string      `json:"revisionId"`
	Definition Measurement `json:"definition"`
}
type MeasurementReport struct {
	DocumentID string              `json:"documentId"`
	RevisionID string              `json:"revisionId"`
	Definition Measurement         `json:"definition"`
	Result     MeasurementEvidence `json:"result"`
}
type measurementHandler struct{}

func (measurementHandler) TypeURI() string                   { return typeSetMeasurements }
func (measurementHandler) SupportedSchemaVersions() []uint32 { return []uint32{1} }
func (measurementHandler) TargetDocumentTypes() []string     { return []string{"PART", "PRODUCT"} }
func (measurementHandler) Apply(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	return applyMeasurements(raw, payload)
}

func measurementError(code string) error { return fmt.Errorf("%w: %s", ErrValidation, code) }
func measurementCount(kind string) int {
	switch kind {
	case "ITEM":
		return 1
	case "BETWEEN":
		return 2
	case "ANGLE_POINTS":
		return 3
	}
	return 0
}
func measurementValues(raw json.RawMessage) ([]Measurement, error) {
	var m struct {
		Measurements []Measurement `json:"measurements"`
	}
	err := json.Unmarshal(raw, &m)
	return m.Measurements, err
}
func applyMeasurements(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	before, err := measurementValues(raw)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	var next []Measurement
	if err = json.Unmarshal(payload, &next); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if len(next) > 128 {
		return nil, modelcore.ChangeSet{}, measurementError("MEASUREMENT_LIMIT")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, m := range next {
		if uuid.Validate(m.ID) != nil || ids[m.ID] || names[scopedNameKey(m.Name)] || validateScopedName(m.Name) != nil || measurementCount(m.Kind) != len(m.Supports) || len(m.Supports) == 0 || m.Result == nil || !m.Result.Complete || m.Result.Policy != measurementPolicy || m.Result.SourceStamp == "" || m.Result.KernelBuild == "" || m.Status != "" {
			return nil, modelcore.ChangeSet{}, measurementError("MEASUREMENT_DEFINITION_INVALID")
		}
		ids[m.ID] = true
		names[scopedNameKey(m.Name)] = true
		for _, s := range m.Supports {
			if s.GeometryKey != "" || s.LocalID != 0 || s.Mode != "GEOMETRY" && s.Mode != "CENTER" && s.Mode != "AXIS" {
				return nil, modelcore.ChangeSet{}, measurementError("MEASUREMENT_REFERENCE_INVALID")
			}
			if s.Kind != "BODY" && s.Kind != "OCCURRENCE" && (s.Selection == nil || s.Selection.Validate() != nil || s.SourceRevisionID == "") {
				return nil, modelcore.ChangeSet{}, measurementError("MEASUREMENT_REFERENCE_INVALID")
			}
		}
	}
	if reflect.DeepEqual(before, next) {
		return raw, modelcore.ChangeSet{}, nil
	}
	change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "document", SlotID: measurementSlot}, before, next)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	doc, err := readParameterDocument(raw)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	beforeParameters := append([]modelcore.ParameterDefinition(nil), doc.Parameters...)
	if err = rejectRemovedMeasurementConsumers(doc, before, next); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err = syncMeasurementParameters(&doc.Parameters, next); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	storedParameters := doc.Parameters
	if doc.product != nil {
		setProductParameterDefinitions(doc.product, doc.Parameters)
		storedParameters = doc.product.Parameters
	}
	fields["parameters"], _ = json.Marshal(storedParameters)
	fields["measurements"], _ = json.Marshal(next)
	out, err := json.Marshal(fields)
	changes, _ := appendParameterLifecycleChanges([]modelcore.ModelChange{change}, nil, beforeParameters, doc.Parameters)
	seeds := []modelcore.DependencyKey{}
	for _, prior := range before {
		for _, current := range next {
			if current.ID == prior.ID && !reflect.DeepEqual(prior.Result, current.Result) {
				seeds = append(seeds, modelcore.DependencyKey("measurement:"+current.ID))
			}
		}
	}
	return out, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
}
func measurementsOnly(c modelcore.ChangeSet) bool {
	if len(c.Changes) == 0 || len(c.ImpactSeeds) > 0 {
		return false
	}
	for _, v := range c.Changes {
		if v.Target.SlotID == "parameter.entity" {
			valid := true
			for _, raw := range []json.RawMessage{v.Before, v.After} {
				if len(raw) == 0 || string(raw) == "null" {
					continue
				}
				var p modelcore.ParameterDefinition
				if json.Unmarshal(raw, &p) != nil || p.Source.Produced == nil || p.Source.Produced.Kind != "MEASUREMENT" {
					valid = false
				}
			}
			if valid {
				continue
			}
		}
		if v.Target.SlotID != measurementSlot || v.Target.EntityID != "document" {
			return false
		}
	}
	return true
}

// Revision snapshots in paths are evidence. Continuation still verifies every
// typed owner/member/document identity, then resolves the current accepted edge.
func measurementPathMatches(a, b InstancePath) bool {
	if a.RootDocumentID != b.RootDocumentID || len(a.Segments) != len(b.Segments) {
		return false
	}
	for i, x := range a.Segments {
		y := b.Segments[i]
		if x.OwnerDocumentID != y.OwnerDocumentID || x.InstanceID != y.InstanceID || x.ReferencedDocumentID != y.ReferencedDocumentID {
			return false
		}
	}
	return true
}

type frozenMeasurement struct {
	inputs     []geometry.AnalysisGeometry
	operands   [][]geometry.MeasurementElement
	definition Measurement
	stamp      string
	stamps     map[string]string
	frame      InstancePose
	reads      []modelcore.DependencyRead
}

type measurementSnapshotKey struct{}
type measurementSnapshot struct {
	documentID, revisionID, kind string
	raw                          json.RawMessage
}

func (s *Service) measurementModel(ctx context.Context, doc, rev string) (string, json.RawMessage, error) {
	if snapshot, ok := ctx.Value(measurementSnapshotKey{}).(measurementSnapshot); ok && snapshot.documentID == doc && snapshot.revisionID == rev {
		return snapshot.kind, snapshot.raw, nil
	}
	var kind string
	var raw []byte
	err := s.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.document_id=d.id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, doc, rev).Scan(&kind, &raw)
	if errors.Is(err, database.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	return kind, raw, err
}
func (s *Service) freezeMeasurement(ctx context.Context, doc, rev string, m Measurement, bind bool) (frozenMeasurement, error) {
	f := frozenMeasurement{definition: m}
	f.definition.Result = nil
	f.definition.Status = ""
	f.definition.OutputCurrency = nil
	f.definition.DisplayAnchors = nil
	required := measurementCount(m.Kind)
	if required == 0 || len(m.Supports) != required {
		return f, measurementError("MEASUREMENT_INPUT_INVALID")
	}
	kind, raw, err := s.measurementModel(ctx, doc, rev)
	if err != nil {
		return f, err
	}
	ctx = withAssemblyReadCache(ctx)
	items := []expandedOccurrence{{DocumentID: doc, RevisionID: rev, DocumentType: kind, Pose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}}
	if kind == "PRODUCT" {
		var candidate ProductModel
		if err = json.Unmarshal(raw, &candidate); err != nil {
			return f, err
		}
		items, err = s.expandProductContext(ctx, doc, rev, candidate)
		if err != nil {
			return f, err
		}
	}
	parts := map[string]PartModel{}
	inputs := map[string]geometry.AnalysisGeometry{}
	reads := []modelcore.DependencyRead{}
	placements := []analysisPlacement{}
	elementCount := 0
	inputBytes := int64(0)
	inputDigests := map[string]bool{}
	for i, source := range m.Supports {
		if source.Mode == "" {
			source.Mode = "GEOMETRY"
		}
		if source.Mode != "GEOMETRY" && source.Mode != "CENTER" && source.Mode != "AXIS" {
			return f, measurementError("MEASUREMENT_MODE_INVALID")
		}
		selected := items[:1]
		if kind == "PRODUCT" {
			if source.InstancePath == nil || validateNonRootInstancePath(*source.InstancePath, doc) != nil {
				return f, measurementError("MEASUREMENT_OCCURRENCE_REQUIRED")
			}
			target, ok := occurrenceByCanonical(items, source.InstancePath.Canonical)
			if !ok || !measurementPathMatches(*source.InstancePath, target.Path) {
				return f, measurementError("MEASUREMENT_OCCURRENCE_BROKEN")
			}
			if bind && source.Selection == nil && source.SourceRevisionID == "" {
				if err = validateResolvedInstancePath(*source.InstancePath, target.Path); err != nil {
					return f, err
				}
			}
			source.InstancePath = &target.Path
			selected = []expandedOccurrence{target}
			if source.Kind == "OCCURRENCE" {
				selected = nil
				for _, item := range items {
					if item.DocumentType == "PART" && (item.Path.Canonical == target.Path.Canonical || strings.HasPrefix(item.Path.Canonical, target.Path.Canonical+"/")) {
						selected = append(selected, item)
					}
				}
			}
		} else if kind != "PART" || source.InstancePath != nil && len(source.InstancePath.Segments) > 0 {
			return f, measurementError("MEASUREMENT_OWNER_INVALID")
		}
		if len(selected) == 0 {
			return f, measurementError("MEASUREMENT_EMPTY_SUPPORT")
		}
		op := []geometry.MeasurementElement{}
		for _, item := range selected {
			if item.DocumentType != "PART" {
				return f, measurementError("MEASUREMENT_PART_REQUIRED")
			}
			key := item.DocumentID + "/" + item.RevisionID
			part, ok := parts[key]
			if !ok {
				var pRaw json.RawMessage
				if item.DocumentID == doc {
					pRaw = raw
				} else {
					_, pRaw, err = s.measurementModel(ctx, item.DocumentID, item.RevisionID)
					if err != nil {
						return f, err
					}
				}
				if err = json.Unmarshal(pRaw, &part); err != nil {
					return f, err
				}
				normalizePartModel(&part)
				parts[key] = part
			}
			// Context variants require their own naming snapshot. Never quietly measure
			// the reusable base Part when the displayed occurrence is specialized.
			for _, owner := range items {
				for _, b := range owner.ContextBindings {
					if b.OwningInstancePath.Canonical == item.Path.Canonical {
						return f, measurementError("MEASUREMENT_CONTEXT_VARIANT_UNSUPPORTED")
					}
				}
			}
			bodyID := source.BodyID
			if source.Selection != nil {
				bodyID = source.Selection.SourceBodyID
				source.BodyID = bodyID
			}
			candidates := []PartBody{}
			for _, b := range part.Bodies {
				if !b.Consumed && (source.Kind == "OCCURRENCE" || b.ID == bodyID) {
					candidates = append(candidates, b)
				}
			}
			if len(candidates) == 0 {
				return f, measurementError("MEASUREMENT_BODY_UNAVAILABLE")
			}
			for _, b := range candidates {
				elementCount++
				if elementCount > 128 {
					return f, measurementError("MEASUREMENT_SCOPE_LIMIT")
				}
				if b.GeometryKey == "" {
					return f, measurementError("MEASUREMENT_BREP_UNAVAILABLE")
				}
				for _, feature := range part.Features {
					if feature.BodyID == b.ID && !feature.Suppressed && (feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED") {
						return f, measurementError("MEASUREMENT_BODY_NOT_UPDATED")
					}
				}
				element := geometry.MeasurementElement{Kind: source.Kind, Mode: source.Mode}
				if source.Kind == "OCCURRENCE" {
					element.Kind = "BODY"
				}
				if element.Kind != "BODY" {
					if element.Kind != "FACE" && element.Kind != "EDGE" && element.Kind != "VERTEX" {
						return f, measurementError("MEASUREMENT_SUPPORT_INVALID")
					}
					if source.Selection == nil {
						if !bind {
							return f, measurementError("MEASUREMENT_REFERENCE_UNBOUND")
						}
						bound, e := s.BindPersistentSelection(ctx, item.DocumentID, BindPersistentSelectionRequest{BodyID: b.ID, SourceVersionID: item.RevisionID, GeometryKey: source.GeometryKey, Kind: source.Kind, LocalID: source.LocalID})
						if e != nil {
							return f, e
						}
						source.Selection = &bound
						source.SourceRevisionID = item.RevisionID
					}
					resolution, e := s.ResolvePersistentSelection(ctx, item.DocumentID, ResolvePersistentSelectionRequest{Selection: *source.Selection, SourceVersionID: source.SourceRevisionID, TargetVersionID: item.RevisionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
					if e != nil {
						return f, e
					}
					if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
						return f, measurementError("MEASUREMENT_SUPPORT_" + string(resolution.Status))
					}
					c := resolution.Candidates[0]
					if c.GeometryKey != b.GeometryKey || string(c.Type) != source.Kind {
						return f, measurementError("MEASUREMENT_SUPPORT_MISMATCH")
					}
					element.LocalID = c.LocalID
				}
				id := item.Path.Canonical + "/" + item.DocumentID + "/" + b.ID
				element.InputID = id
				input, ok := inputs[id]
				if !ok {
					input = geometry.AnalysisGeometry{ID: id, Pose: geometry.AssemblyPose(item.Pose)}
					if err = s.database.QueryRow(ctx, `SELECT geometry_id FROM occccad.geometry_artifacts WHERE geometry_key=$1`, b.GeometryKey).Scan(&input.GeometryID); err != nil {
						return f, err
					}
					input.BRep, err = s.brepArtifactReference(ctx, b.GeometryKey)
					if err != nil {
						return f, err
					}
					if input.BRep.Size <= 0 || input.BRep.SHA256 == "" {
						return f, measurementError("MEASUREMENT_BREP_UNAVAILABLE")
					}
					if !inputDigests[input.BRep.SHA256] {
						inputBytes += input.BRep.Size
						inputDigests[input.BRep.SHA256] = true
					}
					if inputBytes > 256*1024*1024 {
						return f, measurementError("MEASUREMENT_SCOPE_LIMIT")
					}
					inputs[id] = input
					f.inputs = append(f.inputs, input)
				}
				op = append(op, element)
				reads = append(reads, modelcore.DependencyRead{DocumentID: item.DocumentID, Occurrence: item.Path.Canonical, EntityID: b.ID, Facet: modelcore.IntrinsicGeometry, Slot: fmt.Sprintf("operand:%d", i), Digest: b.GeometryKey, RevisionID: item.RevisionID}, modelcore.DependencyRead{DocumentID: item.DocumentID, Occurrence: item.Path.Canonical, EntityID: b.ID, Facet: modelcore.TopologySupport, Slot: fmt.Sprintf("operand:%d", i), Digest: resolvedDigest(element), RevisionID: item.RevisionID})
				placements = append(placements, analysisPlacement{item.DocumentID, item.Path.Canonical, b.ID, item.RevisionID, item.Pose})
			}
		}
		if source.SourceRevisionID == "" {
			source.SourceRevisionID = selected[0].RevisionID
		}
		source.GeometryKey = ""
		source.LocalID = 0
		f.definition.Supports = append([]MeasurementSupport(nil), f.definition.Supports...)
		f.definition.Supports[i] = source
		f.operands = append(f.operands, op)
	}
	totalBytes := int64(0)
	unique := map[string]bool{}
	for _, g := range f.inputs {
		if !unique[g.BRep.SHA256] {
			totalBytes += g.BRep.Size
			unique[g.BRep.SHA256] = true
		}
	}
	n := 0
	for _, op := range f.operands {
		n += len(op)
	}
	if n > 128 || len(f.inputs) > 128 || totalBytes > 256*1024*1024 {
		return f, measurementError("MEASUREMENT_SCOPE_LIMIT")
	}
	f.stamps, f.frame, err = measurementInputStamps(m.Kind, reads, placements)
	f.reads = reads
	if err != nil {
		return f, err
	}
	facet := measurementPrimaryFacet(m)
	f.stamp = f.stamps[facet]
	return f, nil
}
func (s *Service) Measure(ctx context.Context, doc string, r MeasurementRequest) (MeasurementReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var head string
	if err := s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, doc).Scan(&head); err != nil {
		return MeasurementReport{}, err
	}
	if head != r.RevisionID {
		return MeasurementReport{}, measurementError("MEASUREMENT_STALE_REVISION")
	}
	f, err := s.freezeMeasurement(ctx, doc, r.RevisionID, r.Definition, true)
	if err != nil {
		return MeasurementReport{}, err
	}
	result, err := s.worker.MeasureGeometry(ctx, newID("measure"), r.Definition.Kind, f.inputs, f.operands)
	if err != nil {
		return MeasurementReport{}, err
	}
	evidence := MeasurementEvidence{EvaluatorVersion: measurementEvaluatorVersion, Reads: f.reads, InputStamps: f.stamps, AnchorFrame: f.frame, SourceRevisionID: r.RevisionID, SourceStamp: f.stamp, Policy: measurementPolicy, MeasurementResult: result}
	return MeasurementReport{DocumentID: doc, RevisionID: r.RevisionID, Definition: f.definition, Result: evidence}, nil
}
func (s *Service) prepareMeasurements(ctx context.Context, doc string, raw json.RawMessage, r CommandRequest) ([]Measurement, error) {
	values, err := measurementValues(raw)
	if err != nil {
		return nil, err
	}
	clearMeasurementProjection(values)
	if r.Type == "DELETE_MEASUREMENT" {
		index := slices.IndexFunc(values, func(m Measurement) bool { return m.ID == r.MeasurementID })
		if index < 0 {
			return nil, ErrNotFound
		}
		return slices.Delete(values, index, index+1), nil
	}
	if r.Measurement == nil {
		return nil, measurementError("MEASUREMENT_REQUIRED")
	}
	m := *r.Measurement
	m.Result = nil
	m.Status = ""
	m.OutputCurrency = nil
	m.DisplayAnchors = nil
	index := slices.IndexFunc(values, func(v Measurement) bool { return v.ID == m.ID })
	if m.ID != "" && index < 0 {
		return nil, ErrNotFound
	}
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	names := []string{}
	for _, v := range values {
		if v.ID != m.ID {
			names = append(names, v.Name)
		}
	}
	if strings.TrimSpace(m.Name) == "" {
		m.Name = nextScopedName("测量", names)
	}
	if err = ensureUniqueScopedName(m.Name, names, ""); err != nil {
		return nil, err
	}
	if index >= 0 && reflect.DeepEqual(values[index].Supports, m.Supports) && values[index].Kind == m.Kind && r.Type != "UPDATE_MEASUREMENT" {
		m.Result = values[index].Result
	} else {
		report, e := s.Measure(ctx, doc, MeasurementRequest{RevisionID: r.VersionID, Definition: m})
		if e != nil {
			return nil, e
		}
		if !report.Result.Complete {
			return nil, measurementError(report.Result.Diagnostic)
		}
		m.Supports = report.Definition.Supports
		m.Result = &report.Result
	}
	if index < 0 {
		values = append(values, m)
	} else {
		values[index] = m
	}
	return values, nil
}
func (s *Service) projectMeasurements(ctx context.Context, doc, rev string, values []Measurement) {
	for i := range values {
		m := &values[i]
		m.Status = "STALE"
		if m.Result == nil {
			m.Status = "MISSING"
			continue
		}
		f, err := s.freezeMeasurement(ctx, doc, rev, *m, false)
		if err != nil {
			m.Status = "UNKNOWN"
			if errors.Is(err, ErrValidation) || errors.Is(err, ErrNotFound) || errors.Is(err, database.ErrNoRows) {
				m.Status = "BROKEN"
			}
			continue
		}
		m.Status = modelcore.DerivedCurrency(m.Result.SourceStamp, f.stamp, true)
		m.OutputCurrency = map[string]string{}
		for key := range m.Result.Values {
			facet := measurementOutputFacet(m.Kind, key)
			m.OutputCurrency[key] = modelcore.DerivedCurrency(m.Result.InputStamps[facet], f.stamps[facet], true)
		}
		if m.Status == "CURRENT" {
			transform := composeInstancePose(f.frame, inverseRelativePose(InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, m.Result.AnchorFrame))
			m.DisplayAnchors = map[string][3]float64{}
			for key, point := range m.Result.Anchors {
				if key == "direction" || key == "firstDirection" || key == "secondDirection" {
					m.DisplayAnchors[key] = rotateByPose(transform, point)
				} else {
					m.DisplayAnchors[key] = pointByPose(transform, point)
				}
			}
		}
	}
}
func measurementStructure(doc, rev, parent string, values []Measurement, parameters []modelcore.ParameterDefinition) DocumentStructureNode {
	group := DocumentStructureNode{ID: parent + "/measurements", Kind: "MEASUREMENT_GROUP", EntityID: "measurements", LocalVisible: boolPointer(slices.ContainsFunc(values, func(m Measurement) bool { return m.Visible })), Name: "测量", DocumentID: doc, VersionID: rev, Capabilities: []string{"REFRESH"}}
	for _, m := range values {
		node := DocumentStructureNode{ID: group.ID + "/" + m.ID, Kind: "MEASUREMENT", EntityID: m.ID, EntityType: m.Kind, Name: m.Name, DocumentID: doc, OwnerDocumentID: doc, VersionID: rev, EvaluationStatus: m.Status, CurrencyStatus: m.Status, LocalVisible: boolPointer(m.Visible), Capabilities: []string{"EDIT", "REFRESH", "DELETE"}}
		for _, p := range parameters {
			if p.Source.Produced == nil || p.Source.Produced.EntityID != m.ID {
				continue
			}
			child := DocumentStructureNode{ID: node.ID + "/parameter:" + p.ParameterID, Kind: "PARAMETER", EntityID: p.ParameterID, Name: measurementOutputLabel(p.PropertySlot), ParameterAlias: p.Key, OwnerEntityID: m.ID, DocumentID: doc, OwnerDocumentID: doc, VersionID: rev, CurrencyStatus: p.CurrencyStatus, EvaluationStatus: p.CurrencyStatus, Capabilities: []string{"EDIT"}}
			if p.EvaluatedValue != nil {
				value, _ := quantityInDisplayUnit(*p.EvaluatedValue, p.DisplayUnit)
				child.Name += fmt.Sprintf(" = %.7g %s", value, p.DisplayUnit)
			}
			node.Children = append(node.Children, child)
		}
		group.Children = append(group.Children, node)
	}
	return group
}
