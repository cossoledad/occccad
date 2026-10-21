package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
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
	for _, ref := range scope.Selections {
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
		if _, e := s.automation.ReadSnapshot(ctx, actor, source, revision); e != nil {
			return scope, false, e
		}
		if ref.Selection != nil {
			if e := ref.Selection.Validate(); e != nil {
				return scope, false, e
			}
			if ref.Selection.SourceDocumentID != source || ref.BodyID != "" && ref.Selection.SourceBodyID != ref.BodyID {
				return scope, false, aiFault("AI_SCOPE_INVALID", "选择来源与正式持久引用不一致", false)
			}
		} else if ref.ID == "" && ref.PublicationID == "" && ref.DatumID == "" && ref.AxisID == "" && len(ref.InstancePath) == 0 {
			return scope, false, aiFault("AI_SCOPE_INVALID", "选择缺少正式稳定身份", false)
		}
	}
	return scope, current.Permission != "VIEWER" && scope.RevisionID == current.Snapshot.RevisionID, nil
}

func isQuestion(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if strings.Contains(lower, "哪些") || strings.Contains(lower, "为什么") || strings.Contains(lower, "哪一步") || strings.HasPrefix(lower, "查看") || strings.HasPrefix(lower, "解释") || strings.HasPrefix(lower, "如何") || strings.HasPrefix(lower, "怎样") || strings.HasPrefix(lower, "how ") || strings.HasPrefix(lower, "what ") || strings.HasPrefix(lower, "why ") {
		return true
	}
	for _, word := range []string{"修改", "改成", "改为", "改到", "创建", "插入", "不要", "不用", "取消后", "delete ", "create ", "change ", "set ", "make ", "add ", "insert ", "remove "} {
		if strings.Contains(lower, word) {
			return false
		}
	}
	for _, word := range []string{"?", "？", "哪些", "多少", "为什么", "参数", "状态", "进展", "解释", "查看", "查询", "how ", "what ", "why ", "show ", "list "} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

var number = `([0-9]+(?:\.[0-9]+)?)`
var rectanglePattern = regexp.MustCompile(number + `\s*[×xX*]\s*` + number + `\s*(?:mm|毫米)?(?:\s*[×xX*]\s*` + number + `\s*(?:mm|毫米)?)?`)
var extrusionPattern = regexp.MustCompile(`(?:拉伸|厚度|长度|extrud\w*|thickness|length)\s*(?:改成|改为|改到|为|成|到|is|to|=|:|：)?\s*` + number + `\s*(mm|毫米|cm|厘米|m|米)?`)
var filletPattern = regexp.MustCompile(`(?i)(?:[rR]\s*` + number + `|(?:圆角|倒圆角|fillet)\s*(?:半径|为|=|:|：)?\s*` + number + `)\s*(mm|毫米|cm|厘米)?`)
var reverseFilletPattern = regexp.MustCompile(`(?i)` + number + `\s*(mm|毫米|cm|厘米)\s*(?:的)?\s*(?:倒圆角|圆角|fillet)`)

var assemblyAxisDirection = regexp.MustCompile(`(?i)(?:轴线?(?:的)?方向|axisDirectionRelation|axis\s+direction)\s*(?:改为|改成|为|=|:|：)?\s*(same|opposite|unoriented|同向|反向|相反)`)
var assemblyPlaneDirection = regexp.MustCompile(`(?i)(?:端面(?:的)?(?:有向)?(?:法线)?(?:方向)?|planeDirectionRelation|plane\s+direction)\s*(?:改为|改成|为|=|:|：)?\s*(same|opposite|同向|反向|相反)`)
var assemblyOffset = regexp.MustCompile(`(?i)(?:偏置|offset(?:MM)?)\s*(?:改为|改成|为|=|:|：)?\s*(-?[0-9]+(?:\.[0-9]+)?)\s*(mm|毫米|cm|厘米|m|米)?`)
var assemblyDOF = regexp.MustCompile(`(?i)(?:targetDOF|目标自由度|自由度)\s*(?:改为|改成|为|=|:|：)?\s*([0-9]+)`)

func directionValue(value string) string {
	switch strings.ToLower(value) {
	case "同向", "same":
		return "SAME"
	case "反向", "相反", "opposite":
		return "OPPOSITE"
	case "unoriented":
		return "UNORIENTED"
	}
	return ""
}

func numberAt(values []string, index int) float64 {
	if index >= len(values) {
		return 0
	}
	v, _ := strconv.ParseFloat(values[index], 64)
	return v
}
func toMM(value float64, unit string) float64 {
	switch unit {
	case "cm", "厘米":
		return value * 10
	case "m", "米":
		return value * 1000
	}
	return value
}

// This is deliberately a small, source-preserving contract for the supported
// Part/shaft scenarios, not a general goal DSL. The model cannot edit these.
func reviseRequirements(previous []GoalRequirement, turn Turn) []GoalRequirement {
	byKind := map[string]GoalRequirement{}
	order := []string{}
	for _, p := range previous {
		byKind[p.Kind] = p
		order = append(order, p.Kind)
	}
	set := func(kind string, r GoalRequirement) {
		if _, ok := byKind[kind]; !ok {
			order = append(order, kind)
		}
		r.ID = uuid.NewSHA1(uuid.MustParse(turn.RunID), []byte(kind)).String()
		r.Kind = kind
		r.SourceTurnID = turn.ID
		r.Unit = "mm"
		byKind[kind] = r
	}
	text := strings.ToLower(turn.Text)
	if match := rectanglePattern.FindStringSubmatch(text); len(match) > 0 {
		set("RECTANGLE_DIMENSIONS", GoalRequirement{Quote: match[0], Width: numberAt(match, 1), Height: numberAt(match, 2)})
		if numberAt(match, 3) > 0 {
			set("EXTRUSION_LENGTH", GoalRequirement{Quote: match[0], Length: numberAt(match, 3)})
		}
	}
	if match := extrusionPattern.FindStringSubmatch(text); len(match) > 0 {
		set("EXTRUSION_LENGTH", GoalRequirement{Quote: match[0], Length: toMM(numberAt(match, 1), match[2])})
	}
	// Keep the hole as a mandatory, source-owned requirement. Otherwise the
	// base rectangle/pad/fillet could pass while the final sentence is ignored.
	hole := byKind["CENTERED_THROUGH_HOLE"]
	if hole.Kind == "" {
		hole = byKind["THROUGH_HOLE"]
	}
	explicitHole := strings.Contains(text, "贯穿孔") || strings.Contains(text, "through hole") || strings.Contains(text, "through-hole")
	if explicitHole || hole.Kind != "" && (strings.Contains(text, "孔") || strings.Contains(text, "hole")) {
		requirement := hole
		requirement.Quote = turn.Text
		sources := map[string]string{}
		for field, source := range hole.Sources {
			sources[field] = source
		}
		requirement.Sources = sources
		updated := hole.Kind == ""
		clause := text
		for _, part := range regexp.MustCompile(`[。；;\n]`).Split(text, -1) {
			if strings.Contains(part, "贯穿孔") || strings.Contains(part, "through hole") || strings.Contains(part, "through-hole") {
				clause = part
			}
		}
		if m := regexp.MustCompile(`(?:孔径\s*(?:为|是|=|：|:|改为|改成)?\s*|直径\s*(?:为|是|=|：|:|改为|改成)?\s*|[øφ⌀]\s*|diameter\s*(?:of\s*)?)([0-9]+(?:\.[0-9]+)?)\s*(mm|毫米|cm|厘米|m|米)?`).FindStringSubmatch(clause); len(m) > 0 {
			requirement.Radius = toMM(numberAt(m, 1), m[2]) / 2
			sources["radius"] = turn.ID
			updated = true
		}
		if strings.Contains(clause, "中心") || strings.Contains(clause, "center") || strings.Contains(clause, "centre") {
			requirement.Kind = "CENTERED_THROUGH_HOLE"
			sources["center"] = turn.ID
			updated = true
		} else if requirement.Kind == "" {
			requirement.Kind = "THROUGH_HOLE"
		}
		for _, plane := range []string{"XY", "YZ", "XZ"} {
			if strings.Contains(strings.ToUpper(clause), plane) {
				requirement.Plane = plane
				sources["plane"] = turn.ID
				updated = true
				break
			}
		}
		if requirement.Kind == "CENTERED_THROUGH_HOLE" {
			delete(byKind, "THROUGH_HOLE")
		}
		if updated {
			set(requirement.Kind, requirement)
		}
	}
	if strings.Contains(text, "圆角") || strings.Contains(text, "fillet") || regexp.MustCompile(`(?i)\br\s*[0-9]`).MatchString(text) {
		cancelled := false
		for _, w := range []string{"不做圆角", "不用圆角", "不要圆角", "不做倒圆角", "取消圆角", "without fillet", "no fillet"} {
			cancelled = cancelled || strings.Contains(text, w)
		}
		if cancelled {
			delete(byKind, "ALL_EDGES_FILLET")
			delete(byKind, "SELECTED_EDGES_FILLET")
			set("NO_FILLET", GoalRequirement{Quote: turn.Text})
		} else {
			radius := 0.0
			quote := ""
			if m := filletPattern.FindStringSubmatch(text); len(m) > 0 {
				radius = numberAt(m, 1)
				if radius == 0 {
					radius = numberAt(m, 2)
				}
				radius = toMM(radius, m[3])
				quote = m[0]
			} else if m := reverseFilletPattern.FindStringSubmatch(text); len(m) > 0 {
				radius = toMM(numberAt(m, 1), m[2])
				quote = m[0]
			}
			if radius > 0 {
				delete(byKind, "NO_FILLET")
				kind := "ALL_EDGES_FILLET"
				explicitAll := strings.Contains(text, "全部") || strings.Contains(text, "所有") || strings.Contains(text, "all ")
				if !explicitAll {
					for _, ref := range turn.Scope.Selections {
						if ref.Selection != nil && ref.Selection.ExpectedType == modelcore.PersistentTopologyType("EDGE") {
							kind = "SELECTED_EDGES_FILLET"
							break
						}
					}
				}
				set(kind, GoalRequirement{Quote: quote, Radius: radius})
				if kind == "ALL_EDGES_FILLET" {
					delete(byKind, "SELECTED_EDGES_FILLET")
				} else {
					delete(byKind, "ALL_EDGES_FILLET")
				}
			}
		}
	}
	axis, plane, offset, dof := assemblyAxisDirection.FindStringSubmatch(text), assemblyPlaneDirection.FindStringSubmatch(text), assemblyOffset.FindStringSubmatch(text), assemblyDOF.FindStringSubmatch(text)
	if strings.Contains(text, "同轴") || strings.Contains(text, "轴孔") || strings.Contains(text, "coaxial") || strings.Contains(text, "装配") || len(axis) > 0 || len(plane) > 0 || (byKind["ASSEMBLY_FIT"].Kind != "" && (len(offset) > 0 || len(dof) > 0)) {
		requirement := byKind["ASSEMBLY_FIT"]
		sources := map[string]string{}
		for field, source := range requirement.Sources {
			sources[field] = source
		}
		requirement.Sources = sources
		if len(axis) > 0 {
			requirement.AxisDirectionRelation = directionValue(axis[1])
			sources["axisDirectionRelation"] = turn.ID
		}
		if len(plane) > 0 {
			requirement.PlaneDirectionRelation = directionValue(plane[1])
			sources["planeDirectionRelation"] = turn.ID
		}
		if len(offset) > 0 {
			value := toMM(numberAt(offset, 1), offset[2])
			requirement.OffsetMM = &value
			sources["offsetMM"] = turn.ID
		}
		if len(dof) > 0 {
			value, _ := strconv.Atoi(dof[1])
			requirement.TargetDOF = &value
			sources["targetDOF"] = turn.ID
		} else if strings.Contains(text, "一个旋转自由度") || strings.Contains(text, "一个转动自由度") {
			value := 1
			requirement.TargetDOF = &value
			sources["targetDOF"] = turn.ID
		}
		requirement.Quote = turn.Text
		set("ASSEMBLY_FIT", requirement)
	}
	if strings.Contains(text, "保持") && (strings.Contains(text, "不变") || strings.Contains(text, "不要修改")) {
		for i := range turn.Scope.Selections {
			ref := turn.Scope.Selections[i]
			if ref.Selection != nil {
				set("PROTECTED_SELECTION:"+modelcore.ValueDigest(marshalJSON(ref.Selection)), GoalRequirement{Quote: turn.Text, Selection: &ref})
			}
		}
	}
	out := []GoalRequirement{}
	seen := map[string]bool{}
	for _, k := range order {
		if r, ok := byKind[k]; ok && !seen[k] {
			out = append(out, r)
			seen[k] = true
		}
	}
	return out
}

func (s *Service) applyFrozenSelections(ctx context.Context, r Run, baseline workspace.CommandBaseline, spec *automation.StepSpec) error {
	if spec.OperationID != "part.fillet" && spec.OperationID != "part.chamfer" {
		return nil
	}
	if spec.OperationID == "part.fillet" {
		for _, requirement := range r.Requirements {
			if requirement.Kind == "ALL_EDGES_FILLET" {
				return nil
			}
		}
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
	args, err := automation.DecodeObject(spec.Arguments)
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
