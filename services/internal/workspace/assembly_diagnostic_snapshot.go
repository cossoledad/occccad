package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/encoding/protojson"
)

const assemblyDiagnosticSchema = "occccad.assembly-diagnostic.v2"
const assemblyDiagnosticBudget = 8 << 20
const assemblyAttemptBudget = 4 << 20
const assemblyAttemptLimit = 32

type assemblyDiagnosticCaptureKey struct{}
type AssemblyCompileFailure struct {
	ConstraintID string `json:"constraintId,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	Phase        string `json:"phase"`
	Error        string `json:"error"`
}
type assemblyDiagnosticCapture struct {
	failures []AssemblyCompileFailure
	invalid  map[string]bool
}

func (c *assemblyDiagnosticCapture) record(id, endpoint, phase string, err error) {
	c.failures = append(c.failures, AssemblyCompileFailure{id, endpoint, phase, err.Error()})
	c.invalid[id] = true
}

type AssemblyDiagnosticDefinition struct {
	Definition json.RawMessage `json:"definition"`
	Endpoints  map[string]int  `json:"endpoints,omitempty"`
}
type AssemblyDiagnosticFrame struct {
	Request        json.RawMessage                `json:"request"` // fields other than the table-referenced geometry and constraints
	Geometry       []int                          `json:"geometry"`
	ParticipantIDs []string                       `json:"participantIds,omitempty"`
	Constraints    []int                          `json:"constraints"`
	Result         json.RawMessage                `json:"result,omitempty"`
	Error          string                         `json:"error,omitempty"`
	Budget         *geometry.AssemblyReplayBudget `json:"budget,omitempty"`
	Missing        []string                       `json:"missing,omitempty"`
}
type AssemblyDiagnosticAttempt struct {
	DiagnosticID   string                    `json:"diagnosticId,omitempty"`
	ConstraintIDs  []string                  `json:"constraintIds"`
	BaseRevisionID string                    `json:"baseRevisionId,omitempty"`
	ManifestDigest string                    `json:"manifestDigest,omitempty"`
	Phase          string                    `json:"phase,omitempty"`
	Failure        string                    `json:"failure,omitempty"`
	Sequence       []AssemblyDiagnosticFrame `json:"sequence,omitempty"`
	Missing        []string                  `json:"missing,omitempty"`
}
type AssemblyDiagnosticDocument struct {
	DocumentID  string   `json:"documentId"`
	RevisionID  string   `json:"revisionId"`
	Type        string   `json:"type,omitempty"`
	BodyIDs     []string `json:"bodyIds,omitempty"`
	Unavailable string   `json:"unavailable,omitempty"`
}

// AssemblyDiagnosticSnapshot has one numerical authority: table-referenced RPC
// inputs. Definitions preserve saved intent/state separately from participation.
type AssemblyDiagnosticSnapshot struct {
	Schema          string                         `json:"schema"`
	Digest          string                         `json:"digest"`
	DocumentID      string                         `json:"documentId"`
	RevisionID      string                         `json:"revisionId"`
	ModelHash       string                         `json:"modelHash"`
	BuildPolicy     string                         `json:"buildPolicy"`
	Current         AssemblyDiagnosticFrame        `json:"current"`
	Geometry        []json.RawMessage              `json:"geometry"`
	Primitives      []json.RawMessage              `json:"primitives"`
	Definitions     []AssemblyDiagnosticDefinition `json:"definitions"`
	Sources         []json.RawMessage              `json:"sources"`
	Documents       []AssemblyDiagnosticDocument   `json:"documents,omitempty"`
	GroupStages     []AssemblyGroupStage           `json:"groupStages,omitempty"`
	NamingEvidence  []json.RawMessage              `json:"namingEvidence,omitempty"`
	NamingLinks     []AssemblyDiagnosticNamingLink `json:"namingLinks,omitempty"`
	CompileFailures []AssemblyCompileFailure       `json:"compileFailures,omitempty"`
	Attempts        []AssemblyDiagnosticAttempt    `json:"attempts,omitempty"`
	Missing         []string                       `json:"missing,omitempty"`
	ByteBudget      int                            `json:"byteBudget"`
}

func (s AssemblyDiagnosticSnapshot) contentDigest() string {
	s.Digest = ""
	raw, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

type diagnosticTable struct {
	values *[]json.RawMessage
	index  map[string]int
}

func newDiagnosticTable(values *[]json.RawMessage) *diagnosticTable {
	return &diagnosticTable{values: values, index: map[string]int{}}
}
func (t *diagnosticTable) add(raw json.RawMessage) (int, error) {
	// Canonical object encoding without floating-point decoding or integer loss.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return 0, err
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return 0, err
	}
	digest := sha256.Sum256(raw)
	key := string(digest[:])
	if index, ok := t.index[key]; ok {
		return index, nil
	}
	index := len(*t.values)
	*t.values = append(*t.values, raw)
	t.index[key] = index
	return index, nil
}
func packDiagnosticFrame(file geometry.AssemblyReplay, gs, cs *diagnosticTable) (AssemblyDiagnosticFrame, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(file.Request, &object); err != nil {
		return AssemblyDiagnosticFrame{}, err
	}
	frame := AssemblyDiagnosticFrame{Result: file.Result, Error: file.TransportError, Budget: file.Budget, Missing: file.Missing}
	var participants []struct {
		ID   string `json:"id"`
		Mode string `json:"mode"`
	}
	if len(object["constraints"]) > 0 {
		if err := json.Unmarshal(object["constraints"], &participants); err != nil {
			return frame, err
		}
	}
	for _, c := range participants {
		if c.Mode != "SUPPRESSED" && c.Mode != "MEASURED" {
			frame.ParticipantIDs = append(frame.ParticipantIDs, c.ID)
		}
	}
	for _, field := range []struct {
		name  string
		table *diagnosticTable
		refs  *[]int
	}{{"geometry", gs, &frame.Geometry}, {"constraints", cs, &frame.Constraints}} {
		var entries []json.RawMessage
		if len(object[field.name]) > 0 {
			if err := json.Unmarshal(object[field.name], &entries); err != nil {
				return frame, err
			}
		}
		for _, entry := range entries {
			ref, err := field.table.add(entry)
			if err != nil {
				return frame, err
			}
			*field.refs = append(*field.refs, ref)
		}
		delete(object, field.name)
	}
	var err error
	frame.Request, err = json.Marshal(object)
	return frame, err
}
func (s AssemblyDiagnosticSnapshot) ExpandFrame(frame AssemblyDiagnosticFrame) (geometry.AssemblyReplay, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(frame.Request, &object); err != nil {
		return geometry.AssemblyReplay{}, err
	}
	for _, field := range []struct {
		name  string
		table []json.RawMessage
		refs  []int
	}{{"geometry", s.Geometry, frame.Geometry}, {"constraints", s.Primitives, frame.Constraints}} {
		if _, exists := object[field.name]; exists {
			return geometry.AssemblyReplay{}, fmt.Errorf("duplicate numerical authority for %s", field.name)
		}
		entries := []json.RawMessage{}
		for _, ref := range field.refs {
			if ref < 0 || ref >= len(field.table) {
				return geometry.AssemblyReplay{}, fmt.Errorf("missing %s table reference %d", field.name, ref)
			}
			entries = append(entries, field.table[ref])
		}
		raw, err := json.Marshal(entries)
		if err != nil {
			return geometry.AssemblyReplay{}, err
		}
		object[field.name] = raw
	}
	request, err := json.Marshal(object)
	return geometry.AssemblyReplay{Schema: geometry.AssemblyReplaySchema, Units: "mm,rad; body-local geometry; quaternion xyzw", Request: request, Result: frame.Result, TransportError: frame.Error, Budget: frame.Budget, Missing: frame.Missing}, err
}
func (s AssemblyDiagnosticSnapshot) RestoreDefinitions() ([]AssemblyConstraint, error) {
	out := make([]AssemblyConstraint, 0, len(s.Definitions))
	seen := map[string]bool{}
	for _, row := range s.Definitions {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(row.Definition, &object); err != nil {
			return nil, err
		}
		for _, endpoint := range []string{"first", "second", "angleAxis"} {
			if _, exists := object[endpoint]; exists {
				return nil, fmt.Errorf("inline diagnostic endpoint duplicates source table authority")
			}
		}
		for endpoint, ref := range row.Endpoints {
			if endpoint != "first" && endpoint != "second" && endpoint != "angleAxis" {
				return nil, fmt.Errorf("unknown source endpoint %s", endpoint)
			}
			if ref < 0 || ref >= len(s.Sources) {
				return nil, fmt.Errorf("missing source endpoint %d", ref)
			}
			object[endpoint] = s.Sources[ref]
		}
		raw, err := json.Marshal(object)
		if err != nil {
			return nil, err
		}
		var definition AssemblyConstraint
		if err := json.Unmarshal(raw, &definition); err != nil {
			return nil, err
		}
		if definition.ID == "" || seen[definition.ID] {
			return nil, fmt.Errorf("duplicate or empty diagnostic constraint ID")
		}
		seen[definition.ID] = true
		out = append(out, definition)
	}
	return out, nil
}

func (s AssemblyDiagnosticSnapshot) validateReferences() error {
	definitions, err := s.RestoreDefinitions()
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, d := range definitions {
		ids[d.ID] = true
	}
	currentGeometry := map[string]bool{}
	for _, index := range s.Current.Geometry {
		if index < 0 || index >= len(s.Geometry) {
			return fmt.Errorf("missing geometry table reference %d", index)
		}
		var g struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(s.Geometry[index], &g); err != nil {
			return err
		}
		currentGeometry[g.ID] = true
	}
	for _, link := range s.NamingLinks {
		if !ids[link.ConstraintID] || (link.GeometryID != "" && !currentGeometry[link.GeometryID]) || (link.Endpoint != "FIRST" && link.Endpoint != "SECOND" && link.Endpoint != "ANGLE_AXIS") || link.Evidence < 0 || link.Evidence >= len(s.NamingEvidence) {
			return fmt.Errorf("invalid Naming evidence reference")
		}
	}
	frames := []AssemblyDiagnosticFrame{s.Current}
	for _, attempt := range s.Attempts {
		if len(attempt.Sequence) > assemblyAttemptLimit {
			return fmt.Errorf("original sequence exceeds frame budget")
		}
		for _, id := range attempt.ConstraintIDs {
			if !ids[id] {
				return fmt.Errorf("unknown original attempt constraint %s", id)
			}
		}
		frames = append(frames, attempt.Sequence...)
	}
	for frameIndex, frame := range frames {
		participants := []string{}
		seen := map[string]bool{}
		for _, index := range frame.Constraints {
			if index < 0 || index >= len(s.Primitives) {
				return fmt.Errorf("missing constraint table reference %d", index)
			}
			var primitive struct {
				ID   string `json:"id"`
				Mode string `json:"mode"`
			}
			if err := json.Unmarshal(s.Primitives[index], &primitive); err != nil {
				return err
			}
			if primitive.ID == "" || seen[primitive.ID] {
				return fmt.Errorf("duplicate or empty frame constraint ID")
			}
			seen[primitive.ID] = true
			if frameIndex == 0 && !ids[primitive.ID] {
				return fmt.Errorf("current numerical constraint has no saved definition: %s", primitive.ID)
			}
			// Original stages may also contain production-generated group links or
			// interaction drivers; their IDs are scoped to that numerical frame.
			if primitive.Mode != "SUPPRESSED" && primitive.Mode != "MEASURED" {
				participants = append(participants, primitive.ID)
			}
		}
		if !reflect.DeepEqual(participants, frame.ParticipantIDs) && !(len(participants) == 0 && len(frame.ParticipantIDs) == 0) {
			return fmt.Errorf("participant set differs from numerical constraint modes")
		}
	}
	return nil
}
func buildAssemblyDiagnosticSnapshot(frozen AssemblySolveManifest, definitions []AssemblyConstraint, failures []AssemblyCompileFailure) (AssemblyDiagnosticSnapshot, error) {
	snapshot := AssemblyDiagnosticSnapshot{Schema: assemblyDiagnosticSchema, DocumentID: frozen.RootProductDocumentID, RevisionID: frozen.RootProductRevisionID, ModelHash: frozen.ModelHash, BuildPolicy: frozen.SolverBuildPolicy, GroupStages: frozen.GroupStages, CompileFailures: failures, ByteBudget: assemblyDiagnosticBudget, Missing: []string{"DETAILED_NAMING_NOT_CAPTURED", "EVALUATION_TRACE_NOT_CAPTURED"}}
	gs, cs, sources := newDiagnosticTable(&snapshot.Geometry), newDiagnosticTable(&snapshot.Primitives), newDiagnosticTable(&snapshot.Sources)
	// The current numerical table keeps every compilable target, while modes
	// explicitly describe the accepted driving set. Saved modes live in definitions.
	saved := map[string]AssemblyConstraint{}
	for _, d := range definitions {
		saved[d.ID] = d
	}
	constraints := append([]geometry.AssemblyConstraint(nil), frozen.Constraints...)
	for i := range constraints {
		d := saved[constraints[i].ID]
		if d.Suppressed || d.EvaluationStatus != "VERIFIED" {
			constraints[i].Mode = "SUPPRESSED"
		}
	}
	raw, err := geometry.MakeAssemblyReplay("diagnostic/"+snapshot.DocumentID+"/"+snapshot.RevisionID, frozen.Bodies, frozen.Geometry, constraints, geometry.AssemblySolveOptions{Intent: frozen.Intent, AffectedBodyIDs: frozen.AffectedBodyIDs, SolverProfile: &frozen.SolverProfile, DisableConflictProbes: true})
	if err != nil {
		return snapshot, err
	}
	var numeric geometry.AssemblyReplay
	if err = json.Unmarshal(raw, &numeric); err != nil {
		return snapshot, err
	}
	snapshot.Current, err = packDiagnosticFrame(numeric, gs, cs)
	if err != nil {
		return snapshot, err
	}
	for _, definition := range definitions {
		raw, err := json.Marshal(definition)
		if err != nil {
			return snapshot, err
		}
		var object map[string]json.RawMessage
		if err = json.Unmarshal(raw, &object); err != nil {
			return snapshot, err
		}
		row := AssemblyDiagnosticDefinition{Endpoints: map[string]int{}}
		for _, endpoint := range []string{"first", "second", "angleAxis"} {
			raw, ok := object[endpoint]
			if !ok || string(raw) == "null" {
				continue
			}
			var reference AssemblyGeometryRef
			if err = json.Unmarshal(raw, &reference); err != nil {
				return snapshot, err
			}
			// Stable selection stays; expanded Naming evidence is optional, not a numerical ID.
			reference.Resolution = nil
			reference.PublicationResolution = nil
			raw, err = json.Marshal(reference)
			if err != nil {
				return snapshot, err
			}
			ref, err := sources.add(raw)
			if err != nil {
				return snapshot, err
			}
			row.Endpoints[endpoint] = ref
			delete(object, endpoint)
		}
		row.Definition, err = json.Marshal(object)
		if err != nil {
			return snapshot, err
		}
		snapshot.Definitions = append(snapshot.Definitions, row)
	}
	return snapshot, nil
}
func (service *Service) appendDiagnosticAttempts(ctx context.Context, snapshot *AssemblyDiagnosticSnapshot, definitions []AssemblyConstraint) {
	indices := map[string]int{}
	gs, cs := newDiagnosticTable(&snapshot.Geometry), newDiagnosticTable(&snapshot.Primitives)
	// Seed hash indices so current and historical inputs share table entries.
	oldG, oldC := snapshot.Geometry, snapshot.Primitives
	snapshot.Geometry = nil
	snapshot.Primitives = nil
	for _, v := range oldG {
		_, _ = gs.add(v)
	}
	for _, v := range oldC {
		_, _ = cs.add(v)
	}
	for _, definition := range definitions {
		if definition.EvaluationStatus != "NOT_UPDATED" {
			continue
		}
		id := ""
		if definition.EvaluationFailure != nil {
			id = definition.EvaluationFailure.DiagnosticID
		}
		if i, ok := indices[id]; ok && id != "" {
			snapshot.Attempts[i].ConstraintIDs = append(snapshot.Attempts[i].ConstraintIDs, definition.ID)
			continue
		}
		attempt := AssemblyDiagnosticAttempt{DiagnosticID: id, ConstraintIDs: []string{definition.ID}}
		parts := strings.Split(id, "/")
		if len(parts) != 2 || parts[0] != snapshot.DocumentID {
			attempt.Missing = []string{"ORIGINAL_ATTEMPT_UNAVAILABLE"}
		} else {
			raw, err := service.ReadOperationDiagnostic(ctx, parts[0], parts[1])
			var operation OperationDiagnostic
			if err != nil || json.Unmarshal(raw, &operation) != nil {
				attempt.Missing = []string{"ORIGINAL_ATTEMPT_UNAVAILABLE"}
			} else {
				attempt.BaseRevisionID = operation.BaseRevisionID
				attempt.ManifestDigest = operation.AssemblyManifestDigest
				attempt.Phase = operation.Stage
				attempt.Failure = operation.Failure
				attempt.Missing = operation.AssemblyMissing
				for _, raw := range operation.AssemblyAttempts {
					var numeric geometry.AssemblyReplay
					if json.Unmarshal(raw, &numeric) != nil {
						attempt.Missing = append(attempt.Missing, "ORIGINAL_REQUEST_UNREADABLE")
						continue
					}
					frame, err := packDiagnosticFrame(numeric, gs, cs)
					if err != nil {
						attempt.Missing = append(attempt.Missing, "ORIGINAL_REQUEST_UNREADABLE")
						continue
					}
					attempt.Sequence = append(attempt.Sequence, frame)
				}
				if len(attempt.Sequence) == 0 {
					attempt.Missing = append(attempt.Missing, "ORIGINAL_NUMERICAL_INPUT_UNAVAILABLE")
				}
			}
		}
		indices[id] = len(snapshot.Attempts)
		snapshot.Attempts = append(snapshot.Attempts, attempt)
	}
}
func diagnosticManifest(snapshot AssemblyDiagnosticSnapshot) (AssemblySolveManifest, error) {
	numeric, err := snapshot.ExpandFrame(snapshot.Current)
	if err != nil {
		return AssemblySolveManifest{}, err
	}
	var request workerv1.SolveAssemblyRequest
	if err = protojson.Unmarshal(numeric.Request, &request); err != nil {
		return AssemblySolveManifest{}, err
	}
	bodies, values, cs, options, err := geometry.AssemblyInputFromRequest(&request)
	if err != nil {
		return AssemblySolveManifest{}, err
	}
	definitions, err := snapshot.RestoreDefinitions()
	if err != nil {
		return AssemblySolveManifest{}, err
	}
	if request.LengthScale != 1 || request.AngleScale != 1 || options.SolverProfile == nil {
		return AssemblySolveManifest{}, fmt.Errorf("unsupported diagnostic numerical scales/profile")
	}
	manifest := AssemblySolveManifest{SchemaVersion: assemblySolveManifestSchema, DigestPolicy: assemblyManifestCanonicalJSON, RootProductDocumentID: snapshot.DocumentID, RootProductRevisionID: snapshot.RevisionID, ModelHash: snapshot.ModelHash, Purpose: "DIAGNOSTIC", Bodies: bodies, Geometry: values, Constraints: cs, Intent: options.Intent, DragTarget: options.DragTarget, AffectedBodyIDs: options.AffectedBodyIDs, SolverProfile: *options.SolverProfile, SolverBuildPolicy: snapshot.BuildPolicy, Definitions: definitions, GroupStages: snapshot.GroupStages}
	return manifest, validateAssemblySolveManifest(manifest)
}

type AssemblyDiagnosticNamingLink struct {
	ConstraintID string `json:"constraintId"`
	Endpoint     string `json:"endpoint"`
	GeometryID   string `json:"geometryId,omitempty"`
	Evidence     int    `json:"evidence"`
}

func (s *AssemblyDiagnosticSnapshot) appendNamingEvidence(evidence []AssemblyResolutionEvidence) error {
	table := newDiagnosticTable(&s.NamingEvidence)
	for _, e := range evidence {
		link := AssemblyDiagnosticNamingLink{ConstraintID: e.ConstraintID, Endpoint: e.Endpoint, GeometryID: e.GeometryID}
		e.ConstraintID = ""
		e.Endpoint = ""
		e.GeometryID = ""
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		link.Evidence, err = table.add(raw)
		if err != nil {
			return err
		}
		s.NamingLinks = append(s.NamingLinks, link)
	}
	missing := s.Missing[:0]
	for _, m := range s.Missing {
		if m != "DETAILED_NAMING_NOT_CAPTURED" {
			missing = append(missing, m)
		}
	}
	s.Missing = missing
	return nil
}

// Only diagnostic compilation partitions a failed parameter graph. Each entire
// dependency component still goes through the existing Quantity validator;
// unrelated components can be exported without accepting invalid parameters.
func resolveDiagnosticAssemblyQuantities(model *ProductModel, capture *assemblyDiagnosticCapture) {
	n := len(model.Constraints)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	union := func(a, b int) { parent[find(a)] = find(b) }
	ids, keys := map[string]int{}, map[string]int{}
	for i, c := range model.Constraints {
		if p := assemblyQuantityParameter(c); p != nil {
			if other, ok := ids[p.ParameterID]; ok {
				union(i, other)
			}
			ids[p.ParameterID] = i
			if other, ok := keys[p.Key]; ok {
				union(i, other)
			}
			keys[p.Key] = i
		}
	}
	for i, c := range model.Constraints {
		if p := assemblyQuantityParameter(c); p != nil && p.Source.Expression != nil {
			for _, read := range p.Source.Expression.Reads {
				if other, ok := ids[strings.TrimPrefix(string(read), "parameter:")]; ok {
					union(i, other)
				}
			}
			var visit func(*modelcore.ASTNode)
			visit = func(node *modelcore.ASTNode) {
				if node == nil {
					return
				}
				if other, ok := ids[node.ParameterID]; ok && node.Kind == "PARAMETER" {
					union(i, other)
				}
				visit(node.Left)
				visit(node.Right)
			}
			visit(&p.Source.Expression.CheckedAST)
		}
	}
	components := map[int][]int{}
	for i, c := range model.Constraints {
		if assemblyQuantityParameter(c) != nil {
			root := find(i)
			components[root] = append(components[root], i)
		}
	}
	// Iteration order is the saved definition order, independent of map traversal.
	for i := range model.Constraints {
		indices := components[i]
		if len(indices) == 0 {
			continue
		}
		partial := ProductModel{}
		for _, index := range indices {
			partial.Constraints = append(partial.Constraints, model.Constraints[index])
		}
		if err := resolveAssemblyQuantities(&partial); err != nil {
			for _, index := range indices {
				capture.record(model.Constraints[index].ID, "", "PARAMETERS", err)
			}
		} else {
			for j, index := range indices {
				model.Constraints[index] = partial.Constraints[j]
			}
		}
	}
}
