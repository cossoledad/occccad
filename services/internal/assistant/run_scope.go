package assistant

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func (s *Service) freezeScope(ctx context.Context, actor, document string, input *FrozenScope, body string) (FrozenScope, bool, error) {
	current, err := s.automation.Context(ctx, actor, document)
	if err != nil {
		return FrozenScope{}, false, err
	}
	scope := FrozenScope{RootDocumentID: document, DocumentID: document, RevisionID: current.Snapshot.RevisionID, WorkingBodyID: current.WorkingBodyID, Selections: []SelectedReference{}, Complete: true}
	if input != nil {
		if err = json.Unmarshal(marshalJSON(input), &scope); err != nil {
			return scope, false, err
		}
	}
	if body != "" {
		scope.WorkingBodyID = body
	}
	if scope.DocumentID == "" {
		scope.DocumentID = document
	}
	if scope.RootDocumentID == "" {
		scope.RootDocumentID = document
	}
	if scope.RevisionID == "" {
		scope.RevisionID = current.Snapshot.RevisionID
	}
	if scope.DocumentID != document || !scope.Complete || scope.SelectionCount != len(scope.Selections) || len(scope.Selections) > 256 || len(marshalJSON(scope)) > 512<<10 {
		return scope, false, aiFault("AI_SCOPE_INVALID", "发送目标必须是完整、受限且属于当前定义的正式选择集", false)
	}
	snapshot, err := s.automation.ReadSnapshot(ctx, actor, document, scope.RevisionID)
	if err != nil {
		return scope, false, err
	}
	if scope.RootDocumentID != document {
		root, e := s.automation.ReadSnapshot(ctx, actor, scope.RootDocumentID, scope.RootRevisionID)
		if e != nil {
			return scope, false, e
		}
		scope.RootRevisionID = root.RevisionID
		occurrence, e := s.automation.QueryOccurrence(ctx, actor, scope.RootDocumentID, automation.OccurrenceQuery{RevisionID: root.RevisionID, InstanceIDs: scope.InstancePath})
		if e != nil {
			return scope, false, e
		}
		if occurrence.SourceDocumentID != document || occurrence.SourceRevisionID != scope.RevisionID {
			return scope, false, aiFault("AI_SCOPE_INVALID", "编辑目标不是该根快照中的正式 occurrence", false)
		}
	}
	if scope.WorkingBodyID != "" {
		page, e := s.automation.Query(ctx, actor, document, automation.Query{RevisionID: snapshot.RevisionID, Kind: "BODY", ID: scope.WorkingBodyID, Limit: 1})
		if e != nil {
			return scope, false, e
		}
		if page.MatchedCount != 1 {
			return scope, false, aiFault("AI_SCOPE_INVALID", "工作实体不属于发送时模型", false)
		}
	}
	parts := map[string]*workspace.PartModel{}
	for index := range scope.Selections {
		ref := &scope.Selections[index]
		source := ref.SourceDocumentID
		if source == "" {
			source = document
		}
		revision := ref.SourceVersionID
		if revision == "" {
			revision = scope.RevisionID
		}
		if len(ref.InstancePath) > 0 {
			rootRevision := scope.RootRevisionID
			if scope.RootDocumentID == document {
				rootRevision = scope.RevisionID
			}
			occurrence, e := s.automation.QueryOccurrence(ctx, actor, scope.RootDocumentID, automation.OccurrenceQuery{RevisionID: rootRevision, InstanceIDs: ref.InstancePath})
			if e != nil {
				return scope, false, e
			}
			if occurrence.SourceDocumentID != source || occurrence.SourceRevisionID != revision {
				return scope, false, aiFault("AI_SCOPE_INVALID", "选区来源与根快照接受的正式实例路径不一致", false)
			}
		}
		key := source + ":" + revision
		part, loaded := parts[key]
		if !loaded {
			observed, e := s.automation.ReadSnapshot(ctx, actor, source, revision)
			if e != nil {
				return scope, false, e
			}
			if observed.DocumentType == "PART" {
				part = &workspace.PartModel{}
				if e = json.Unmarshal(observed.Model, part); e != nil {
					return scope, false, e
				}
			}
			parts[key] = part
		}
		ref.SourceDocumentID, ref.SourceVersionID = source, revision
		if ref.Kind == "INSTANCE" {
			if len(ref.InstancePath) == 0 || ref.ID != ref.InstancePath[len(ref.InstancePath)-1] {
				return scope, false, aiFault("AI_SCOPE_INVALID", "实例选择缺少一致的正式路径", false)
			}
			ref.EntityType = "INSTANCE"
		} else {
			selectionMetadata(ref, part)
		}
		if ref.Selection != nil {
			if e := ref.Selection.Validate(); e != nil {
				return scope, false, e
			}
			if ref.Selection.SourceDocumentID != source || ref.BodyID != "" && ref.Selection.SourceBodyID != ref.BodyID {
				return scope, false, aiFault("AI_SCOPE_INVALID", "选择来源与正式持久引用不一致", false)
			}
		} else if ref.ID == "" && ref.PublicationID == "" && ref.DatumID == "" && ref.AxisID == "" && len(ref.InstancePath) == 0 && !(ref.FeatureID != "" && ref.SketchReference != nil) {
			return scope, false, aiFault("AI_SCOPE_INVALID", "选择缺少正式稳定身份", false)
		}
	}
	return scope, current.Permission != "VIEWER" && scope.RevisionID == current.Snapshot.RevisionID, nil
}

func (s *Service) applyFrozenSelections(ctx context.Context, r Run, baseline workspace.CommandBaseline, spec *automation.StepSpec) error {
	if spec.OperationID != "part.fillet" && spec.OperationID != "part.chamfer" {
		return nil
	}
	args, err := automation.DecodeObject(spec.Arguments)
	if err != nil {
		return err
	}
	// An explicit model-selected collection already has a formal scope and
	// resolver. Frozen edges are a fallback, never an inferred all-edge intent.
	if token, _ := args["selectionSet"].(string); token != "" {
		return nil
	}
	refs := []SelectedReference{}
	for _, ref := range r.Scope.Selections {
		if ref.Selection != nil && ref.Selection.ExpectedType == modelcore.PersistentTopologyType("EDGE") {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	// A changed snapshot requires formal relocation, never a rewritten revision.
	ids := []string{}
	body := ""
	for _, ref := range refs {
		if ref.Selection.SourceDocumentID != r.DocumentID {
			return aiFault("AI_SCOPE_INVALID", "所选边属于其他定义", false)
		}
		if body != "" && body != ref.Selection.SourceBodyID {
			return aiFault("AI_SCOPE_INVALID", "一次特征不能跨实体修改选择", false)
		}
		body = ref.Selection.SourceBodyID
		original := workspace.FeatureSelection{Selection: *ref.Selection, SourceVersionID: ref.SourceVersionID}
		if original.SourceVersionID == "" {
			original.SourceVersionID = r.Scope.RevisionID
		}
		resolved, err := s.automation.ResolveSelectionForSnapshotWithEvidence(ctx, r.ActorID, r.DocumentID, baseline.RevisionID, original)
		if err != nil {
			return err
		}
		ids = append(ids, resolved.TopologyID)
	}
	if len(ids) > 100 {
		return aiFault("AI_SELECTION_RESOURCE", "单次精确边选择超过当前工具集合容量，请选择整个实体的全部边或更小完整集合", false)
	}
	result, err := s.automation.QueryTopology(ctx, r.ActorID, r.DocumentID, automation.TopologyQuery{RevisionID: baseline.RevisionID, BodyID: body, Kind: "EDGE", IDs: ids, Limit: 1})
	if err != nil {
		return err
	}
	args["selectionSet"] = result["selectionSet"]
	args["bodyId"] = body
	spec.Arguments = marshalJSON(args)
	return nil
}
func scopeSummary(scope FrozenScope) string {
	return fmt.Sprintf("%d selected formal objects, complete=%t, definition=%s, observed=%s", scope.SelectionCount, scope.Complete, scope.DocumentID, scope.RevisionID)
}
