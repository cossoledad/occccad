package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

const systemPrompt = `You are occccad's native CAD assistant. Use the user's language. This is one durable Run, not a single fixed plan: inspect facts, plan one bounded executable stage, observe authoritative receipts/new geometry, repair or continue, then verify the user goal. The application executes all previews/commits through its existing CAD commands. Never call an executor or invent a receipt, geometry identity, selection, successful check, or displayed frame.
Use your CAD knowledge but discover operations with cad_search_operations and read exact contracts with cad_describe_operation before using them. Read nested cad://types schemas only as needed. All writes are scoped to the application's actor and target definition. Preserve units, user-confirmed hard requirements, selected occurrence, permissions, and solver diagnostics. Mandatory requirements belong to the application and cannot be removed or relaxed by you. Ask a necessary clarification when intent is ambiguous.
Prefer the compact contracts sketch.create, sketch.rectangle, sketch.circle, part.extrude, part.cut, part.fillet and part.chamfer when they fit the goal. Search using one concept or an operation ID, not a sentence combining several operations. Inspect the general command.* DTO/type graph only when a compact operation cannot express the required CAD action.
Call cad_create_task with one bounded stage of at most 32 steps, using local step labels. Later entity inputs may bind preceding outputs with bindings or declared typedBindings. Never invent an output that does not yet exist. Once the stage is applied, the same Run continues with a fresh authoritative snapshot: query its new topology before planning fillets or mates. For all edges use cad_query_topology's complete selectionSet, not the first page. Frozen selected references remain the user's targets even if the UI selection changes. Use cad_get_selection for full frozen selection pages.
For Product, resolve occurrence geometry through cad_query_occurrence using root revision and stable instance IDs, then cad_mate_candidates. Definition coordinates and world occurrence coordinates differ. Prefer existing publications/datums when suitable. Candidate ambiguity requires explanation or the user's selected faces; do not silently choose the first. Recipes and references are application data, never mesh locators. Use source/offset conventions and direction explicitly.
Use cad_verify_goal to see outstanding mandatory checks. cad_finish only requests completion; the application must verify the current goal version and actual revision before completing. A natural-language claim or a created plan does not complete a modeling goal. cad_wait_for_input asks a necessary clarification. ui.* creates a single-step handoff and waits for real interaction; UI_OPENED is only an opened tool, not a completed model. Applied, verified, and actually presented are independent.
Before tool calls, briefly tell the user what you found or will do next in the public content, in one short sentence. Give useful progress summaries, never private chain-of-thought. Use cad_wait_for_input for a necessary question. After each stage the host reports current application evidence. Inspect target identities, expected values, and observed values before deciding whether a CAD change is needed. NOT_VERIFIED means evidence is incomplete, not that the geometry is wrong. A checker can have unsupported strategies or wrong bindings; compare its scope with the actual current model. GOAL:<requirementId> targets bind the exact receipt output for that requirement. Latest SKETCH/LINEAR_EXTRUDE discovery hints may refer to a hole profile/cut, not the base rectangle/pad. A Boolean REMOVE with a circular cylinder can produce the same through hole as a linear cut. Use final geometry and feature dependencies to evaluate the result. Explain material revisions in public content using the evidence you observed. If the user intent is satisfied and checks are VERIFIED, finish concisely.
Context is a bounded working set, not the full history. Public tool facts, immutable receipts, requirements and checkpoints may rebuild it. Older detail remains queryable. Never request an entire large tree or repeat unchanged failing operations. Calls, stages and cumulative tokens have no default Run-wide limit. An optional user budget may pause the Run for adjustment; never bypass it.`

func planningTools() []Tool {
	out := []Tool{}
	for _, t := range Tools() {
		if t.Name != "cad_advance_task" && t.Name != "cad_create_task" {
			out = append(out, t)
		}
	}
	path := map[string]any{"type": "array", "items": stringSchema(), "maxItems": 32}
	typed := object(map[string]any{"target": stringSchema(), "sourceStepId": stringSchema(), "outputId": map[string]any{"type": "string", "description": "An actual stable entity ID in the source's returned outputs, never a label such as PRIMARY. Usually omit for a future step."}, "outputRole": map[string]any{"type": "string", "description": "A declared unique output role of that source operation. Only bind an output the operation actually creates/returns; existing input IDs can be supplied directly."}, "representation": map[string]any{"type": "string", "enum": []string{"ENTITY_ID", "INSTANCE_PATH", "ASSEMBLY_REF"}}}, "target", "sourceStepId", "representation")
	out = append(out,
		Tool{"cad_create_task", "Plan the next bounded executable stage; the host assigns stable identities, current scope, epoch and baseline. Applied stages will be observed before planning further.", object(map[string]any{"steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": object(map[string]any{"id": map[string]any{"type": "string", "maxLength": 40}, "operationId": stringSchema(), "version": map[string]any{"type": "integer", "minimum": 1}, "arguments": map[string]any{"type": "object"}, "bindings": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}, "typedBindings": map[string]any{"type": "array", "items": typed, "maxItems": 16}}, "id", "operationId", "version", "arguments")}}, "steps")},
		Tool{"cad_get_selection", "Read a complete page of the immutable selection frozen for this user input, never the live mouse selection.", object(map[string]any{"after": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20}})},
		Tool{"cad_query_occurrence", "Resolve a root-snapshot occurrence by stable instance IDs, including nested/PINNED accepted revisions and source permissions. Return a bounded exact geometry page or Body/INSTANCE summary.", object(map[string]any{"revisionId": stringSchema(), "instanceIds": path, "bodyId": stringSchema(), "kind": map[string]any{"type": "string", "enum": []string{"BODY", "INSTANCE", "FACE", "EDGE", "VERTEX", "DATUM", "PUBLICATION"}}, "geometryType": stringSchema(), "cursor": map[string]any{"type": "string", "maxLength": 2048}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20}}, "revisionId", "instanceIds")},
		Tool{"cad_mate_candidates", "Find explainable shaft-hole and end-plane fit candidates from exact occurrence geometry. The first occurrence moves, the second is the reference. Multiple candidates require geometric selection/clarification; recipes use existing domain commands.", object(map[string]any{"revisionId": stringSchema(), "firstInstanceIds": path, "secondInstanceIds": path, "firstBodyId": stringSchema(), "secondBodyId": stringSchema(), "axisDirectionRelation": map[string]any{"type": "string", "enum": []string{"SAME", "OPPOSITE"}}, "planeDirectionRelation": map[string]any{"type": "string", "enum": []string{"SAME", "OPPOSITE"}}, "offsetMM": map[string]any{"type": "number"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 8}, "candidateId": stringSchema(), "includeRecipe": map[string]any{"type": "boolean"}}, "revisionId", "firstInstanceIds", "secondInstanceIds")},
		Tool{"cad_verify_assembly", "Read authoritative solve and actual persisted occurrence relations for the selected moving/reference instances, never accepting hypothetical corrected poses as the result.", object(map[string]any{"revisionId": stringSchema(), "movingInstanceId": stringSchema(), "referenceInstanceId": stringSchema(), "directionRelation": map[string]any{"type": "string", "enum": []string{"SAME", "OPPOSITE"}}, "axisDirectionRelation": map[string]any{"type": "string", "enum": []string{"SAME", "OPPOSITE", "UNORIENTED"}}, "offsetMM": map[string]any{"type": "number"}, "targetDOF": map[string]any{"type": "integer", "minimum": 0}}, "revisionId", "movingInstanceId", "referenceInstanceId")},
		Tool{"cad_verify_goal", "Inspect current-goal evidence at the actual revision. FAILED is an observed mismatch; NOT_VERIFIED is unresolved/unsupported evidence, not proof of incorrect geometry. Compare expected, observed, target IDs and modeling strategy before deciding a CAD change. User requirements cannot be waived.", object(map[string]any{})},
		Tool{"cad_finish", "Request completion after all intended stages; application verification is authoritative.", object(map[string]any{"summary": map[string]any{"type": "string", "maxLength": 2000}})},
		Tool{"cad_wait_for_input", "Save progress and ask a concise necessary clarification; the same Run resumes on the user's response.", object(map[string]any{"question": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000}}, "question")},
	)
	return out
}
func (s *Service) session(key string) *planningSession {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.sessions[key] == nil {
		s.sessions[key] = &planningSession{}
	}
	return s.sessions[key]
}

func (s *Service) buildContext(ctx context.Context, r Run, question *Turn) ([]Message, automation.Context, error) {
	host := NewHost(s.automation, r.ActorID, r.DocumentID, r.AccessMode == "WRITE" && question == nil)
	if r.AccessMode == "READ" {
		host.revision = r.Scope.RevisionID
	}
	if question != nil {
		host.revision = question.Scope.RevisionID
	}
	current, err := s.automation.ContextAt(ctx, r.ActorID, r.DocumentID, host.revision)
	if err != nil {
		return nil, current, err
	}
	query := "part."
	if current.Snapshot.DocumentType == "PRODUCT" {
		query = "instance"
	}
	operations, err := s.automation.Catalog.Search(query, current.Snapshot.DocumentType, 8)
	if err != nil {
		return nil, current, err
	}
	type fact struct {
		CallID    string          `json:"callId"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Result    json.RawMessage `json:"result"`
	}
	rows, err := s.db.Query(ctx, `SELECT t.call_id::text,t.name,t.arguments,t.payload FROM occccad.assistant_tool_results t JOIN occccad.assistant_model_calls c ON c.id=t.call_id WHERE c.run_id=$1 ORDER BY c.started_at DESC,c.id DESC,t.ordinal DESC LIMIT 8`, r.ID)
	if err != nil {
		return nil, current, err
	}
	facts := []fact{}
	for rows.Next() {
		var f fact
		var result Result
		if err = rows.Scan(&f.CallID, &f.Name, &f.Arguments, &f.Result); err != nil {
			rows.Close()
			return nil, current, err
		}
		if json.Unmarshal(f.Result, &result) == nil {
			f.Result = result.Content
		}
		facts = append(facts, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, current, err
	}
	stages, err := s.Stages(ctx, r.ActorID, r.DocumentID, r.ID, 0, 0, 3)
	if err != nil {
		return nil, current, err
	}
	checkpoints := []map[string]any{}
	for _, st := range stages.Items {
		task, e := s.automation.GetTask(ctx, r.ActorID, r.DocumentID, st.TaskID)
		if e == nil { // only a bounded current-stage checkpoint, not all history
			checkpoint := stageContext(task)
			if len(marshalJSON(checkpoint)) < 24<<10 {
				checkpoints = append(checkpoints, checkpoint)
			}
		}
	}
	scope := r.Scope
	if question != nil {
		scope = question.Scope
	}
	selected := scope.Selections
	if len(selected) > 6 {
		scope.Selections = nil
	}
	history, err := s.Messages(ctx, r.ActorID, r.DocumentID, r.ID, 0, 0, 6)
	if err != nil {
		return nil, current, err
	}
	language := []map[string]any{}
	bytes := 0
	for i := len(history.Items) - 1; i >= 0; i-- {
		t := history.Items[i]
		if bytes+len(t.Text)+len(t.Reply) > 8<<10 {
			break
		}
		language = append(language, map[string]any{"id": t.ID, "sequence": t.Sequence, "kind": t.Kind, "text": t.Text, "reply": t.Reply, "goalVersion": t.GoalVersion})
		bytes += len(t.Text) + len(t.Reply)
	}
	text := r.Goal
	if question != nil {
		text = question.Text
	}
	packet := map[string]any{"current": current, "operations": operations, "goal": map[string]any{"version": r.GoalVersion, "text": r.Goal, "requirements": r.Requirements}, "run": map[string]any{"id": r.ID, "epoch": r.Epoch, "status": r.Status, "mode": r.Mode, "stageCount": r.StageCount, "stepCount": r.StepCount, "usage": r.Usage, "tokenBudget": r.TokenBudget}, "frozenScope": scope, "selectionPreview": selected[:min(len(selected), 6)], "selectionPreviewComplete": len(selected) <= 6, "selectionSummary": scopeSummary(r.Scope), "recentStages": checkpoints, "recentFacts": facts, "recentLanguage": language, "verification": r.Verification, "diagnostic": r.Failure}
	packet["targets"] = r.Targets
	packet["targetSelections"] = r.TargetSelections
	messages := []Message{{Role: "system", Content: systemPrompt}, {Role: "system", Content: ""}, {Role: "user", Content: text}}
	limit := s.requestLimits().MaxContextBytes
	for {
		packet["recentFacts"] = facts
		packet["recentStages"] = checkpoints
		packet["recentLanguage"] = language
		messages[1].Content = string(marshalJSON(packet))
		if len(marshalJSON(messages)) <= limit*3/4 {
			break
		}
		if len(facts) > 0 {
			facts = facts[:len(facts)-1]
			continue
		}
		if len(checkpoints) > 0 {
			checkpoints = checkpoints[1:]
			continue
		}
		if len(language) > 0 {
			language = language[1:]
			continue
		}
		return nil, current, aiFault("AI_ESSENTIAL_CONTEXT", "当前单次上下文配置无法容纳必要目标和选择；已保存进度，需要更大的单次窗口或较小的完整选择集", false)
	}
	return messages, current, nil
}

func (s *Service) planCall(ctx context.Context, r Run, question *Turn) error {
	messages, current, err := s.buildContext(ctx, r, question)
	if err != nil {
		if question != nil {
			return s.failQuestion(ctx, r, *question, err)
		}
		return s.deferRun(ctx, r, asFault(err))
	}
	key := r.ID
	inputID := ""
	if question != nil {
		key = "query:" + question.ID
		inputID = question.ID
	}
	session := s.session(key)
	session.mu.Lock()
	defer session.mu.Unlock()
	settings, err := s.Settings(ctx, r.ActorID)
	if err != nil {
		return err
	}
	configuration := fmt.Sprintf("%s/%s/%d", settings.Model, settings.Thinking, settings.Version)
	if session.epoch != r.Epoch || session.head != current.Snapshot.RevisionID || session.generation != configuration || len(session.messages) == 0 || len(marshalJSON(session.messages)) > s.requestLimits().MaxContextBytes*3/4 {
		session.messages = messages
		session.epoch = r.Epoch
		session.head = current.Snapshot.RevisionID
		session.generation = configuration
	}
	c, err := s.prepareCall(ctx, r, inputID, current.Snapshot.CommandBaseline, session.messages)
	if err != nil {
		return err
	}
	credential, err := s.credential(ctx, r.ActorID)
	if err != nil {
		if question != nil {
			return s.failQuestion(ctx, r, *question, err)
		}
		return s.deferRun(ctx, r, asFault(err))
	}
	generation := c.Generation
	tools := planningTools()
	if question != nil || r.AccessMode == "READ" {
		readonly := []Tool{}
		for _, t := range tools {
			if t.Name != "cad_create_task" && t.Name != "cad_finish" && t.Name != "cad_wait_for_input" {
				readonly = append(readonly, t)
			}
		}
		tools = readonly
	}
	if err = s.captureCallRequest(ctx, &c, session.messages, tools); err != nil {
		return err
	}
	if err = s.markSent(ctx, &c); err != nil {
		return err
	}
	completion, callErr := s.provider.Complete(ctx, credential, session.messages, tools, generation)
	finish, cancel := detached(ctx)
	defer cancel()
	if err = s.receiveCall(finish, &c, completion, callErr); err != nil {
		return err
	}
	if callErr != nil {
		session.messages = nil
		if question != nil {
			return s.failQuestion(finish, r, *question, callErr)
		}
		return s.deferRun(finish, r, asFault(callErr))
	}
	session.messages = append(session.messages, completion.Message)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if question != nil {
		return s.processQuestionCall(ctx, r, *question, c, session)
	}
	return s.processCall(ctx, r, c, session)
}

var localStepID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,39}$`)

func (s *Service) createStage(ctx context.Context, r Run, c CallRecord, call ToolCall) (automation.Task, error) {
	if r.AccessMode != "WRITE" || c.InputID != "" {
		return automation.Task{}, aiFault("AI_READ_ONLY", "当前输入仅有读取权限", false)
	}
	var input struct {
		Steps []automation.StepSpec `json:"steps"`
	}
	if err := decode([]byte(call.Function.Arguments), &input); err != nil {
		return automation.Task{}, err
	}
	if len(input.Steps) == 0 || len(input.Steps) > 32 {
		return automation.Task{}, aiFault("AI_STAGE_RESOURCE", "单个计划片段须为1..32步；请自动分段，不限制Run总步骤", false)
	}
	ids := map[string]string{}
	namespace := uuid.NewSHA1(uuid.MustParse(r.ID), []byte(c.ID+":"+call.ID))
	for i := range input.Steps {
		step := &input.Steps[i]
		if !localStepID.MatchString(step.ID) || ids[step.ID] != "" {
			return automation.Task{}, aiFault("INVALID_PLAN", "步骤标签必须唯一", false)
		}
		var digest string
		err := s.db.QueryRow(ctx, `SELECT digest FROM occccad.assistant_contracts WHERE run_id=$1 AND operation_id=$2`, r.ID, step.OperationID).Scan(&digest)
		if err != nil || digest != s.automation.Catalog.Digest {
			return automation.Task{}, aiFault("OPERATION_NOT_DESCRIBED", "请先读取当前操作合同："+step.OperationID, false)
		}
		for key, source := range step.Bindings {
			if ids[source] == "" {
				return automation.Task{}, aiFault("INVALID_BINDING", "只能绑定前序步骤正式输出", false)
			}
			step.Bindings[key] = ids[source]
		}
		for j := range step.TypedBindings {
			binding := &step.TypedBindings[j]
			if ids[binding.SourceStepID] == "" {
				return automation.Task{}, aiFault("INVALID_BINDING", "typed引用只允许前序正式输出", false)
			}
			binding.SourceStepID = ids[binding.SourceStepID]
		}
		original := step.ID
		step.ID = uuid.NewSHA1(namespace, []byte(original)).String()
		ids[original] = step.ID
		if err = s.applyFrozenSelections(ctx, r, c.Baseline, step); err != nil {
			return automation.Task{}, err
		}
	}
	stageGoal := r.Goal
	if len(stageGoal) > 2048 {
		stageGoal = fmt.Sprintf("持续目标 %s，第 %d 阶段；完整要求与来源保存在 Run 中", r.ID, r.StageCount+1)
	}
	task, err := s.automation.CreateTask(ctx, r.ActorID, r.DocumentID, automation.CreateTask{RequestID: "assistant:" + c.ID + ":" + call.ID, Goal: stageGoal, ExpectedHead: c.Baseline, Steps: input.Steps, ExecutionMode: r.Mode, RunID: r.ID, PlanEpoch: r.Epoch})
	if err != nil {
		return task, err
	}
	_, err = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch || terminal(current.Status) || current.Status == "PAUSED" {
			return context.Canceled
		}
		var oldSeq uint64
		e := tx.QueryRow(ctx, `SELECT sequence FROM occccad.assistant_stages WHERE run_id=$1 AND task_id=$2`, r.ID, task.ID).Scan(&oldSeq)
		if e == nil {
			current.ActiveTaskID = task.ID
			current.Status = "EXECUTING"
			return nil
		}
		if !errors.Is(e, database.ErrNoRows) {
			return e
		}
		current.StageCount++
		current.ActiveTaskID = task.ID
		current.Status = "EXECUTING"
		current.WaitingReason = ""
		current.Failure = nil
		current.LastFailureDigest = ""
		if _, e = tx.Exec(ctx, `INSERT INTO occccad.assistant_stages(run_id,sequence,epoch,goal_version,task_id,status) VALUES($1,$2,$3,$4,$5,'ACTIVE')`, r.ID, current.StageCount, current.Epoch, current.GoalVersion, task.ID); e != nil {
			return e
		}
		return s.event(ctx, tx, current, "STAGE_CREATED", map[string]any{"sequence": current.StageCount, "taskId": task.ID, "epoch": r.Epoch})
	})
	if err != nil {
		_, _ = s.automation.Control(ctx, r.ActorID, r.DocumentID, task.ID, automation.Control{Action: "stop", ExpectedVersion: task.Version, RunID: r.ID, PlanEpoch: r.Epoch})
	}
	return task, err
}

func (s *Service) processCall(ctx context.Context, r Run, c CallRecord, session *planningSession) error {
	current, err := loadRun(ctx, s.db, r.ID, false)
	if err != nil {
		return err
	}
	if current.Epoch != c.Epoch || terminal(current.Status) || current.Status == "PAUSED" {
		return context.Canceled
	}
	r = current
	if r.TokenBudget != nil && r.Usage.TotalTokens >= *r.TokenBudget {
		_, e := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
			if current.Epoch != r.Epoch {
				return context.Canceled
			}
			current.Status = "WAITING_BUDGET"
			current.WaitingReason = "已达到主动设置的累计预算；响应与进度已保存，提高或清空预算后继续"
			return s.event(ctx, tx, current, "BUDGET_WAIT", map[string]any{"reported": current.Usage.TotalTokens, "budget": current.TokenBudget})
		})
		return e
	}
	if c.Response == nil {
		return s.deferRun(ctx, r, asFault(aiFault("AI_RESPONSE_INVALID", "模型响应没有可恢复的公开内容", true)))
	}
	if len(c.Response.ToolCalls) == 0 {
		if r.AccessMode == "READ" {
			return s.finishRead(ctx, r, c, c.Response.Content)
		}
		verification, e := s.verifyGoal(ctx, r)
		if e != nil {
			return s.deferRun(ctx, r, asFault(e))
		}
		if verification.Status == "VERIFIED" {
			return s.finishGoal(ctx, r, c, c.Response.Content, verification)
		}
		// A public sentence can be a progress update. Necessary clarification
		// uses the explicit tool; narration alone must not silently stall a Run.
		digest := modelcore.ValueDigest(marshalJSON(map[string]any{"kind": "PUBLIC_REPLY_WITHOUT_ACTION", "head": c.Baseline, "goalVersion": r.GoalVersion, "tools": r.ToolCalls, "verification": verification}))
		_, e = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
			if current.Epoch != r.Epoch {
				return context.Canceled
			}
			current.PendingCallID = ""
			current.Status = "PLANNING"
			current.WaitingReason = ""
			current.Verification = verification
			current.Requirements = verification.requirements
			current.LastFailureDigest = digest
			c.Status = "APPLIED"
			if e := saveCall(ctx, tx, c); e != nil {
				return e
			}
			return s.event(ctx, tx, current, "ASSISTANT_MESSAGE", map[string]any{"callId": c.ID, "goalVersion": current.GoalVersion, "message": publicProgress(c.Response.Content)})
		})
		if e == nil && session != nil {
			session.messages = append(session.messages, Message{Role: "system", Content: "The public progress update did not execute an action. Continue with tools for outstanding checks, or cad_wait_for_input for a necessary clarification. Current application checks: " + string(marshalJSON(verification))})
		}
		return e
	}
	for i := c.ToolCursor; i < len(c.Response.ToolCalls); i++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		fresh, e := loadRun(ctx, s.db, r.ID, false)
		if e != nil {
			return e
		}
		if fresh.Epoch != r.Epoch || terminal(fresh.Status) || fresh.Status == "PAUSED" {
			return context.Canceled
		}
		r = fresh
		call := c.Response.ToolCalls[i]
		var result Result
		var stored []byte
		e = s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_tool_results WHERE call_id=$1 AND tool_id=$2`, c.ID, call.ID).Scan(&stored)
		if e == nil {
			if e = json.Unmarshal(stored, &result); e != nil {
				return e
			}
		} else if errors.Is(e, database.ErrNoRows) {
			result = s.executeTool(ctx, r, c, call)
		} else {
			return e
		}
		if result, err = s.recordTool(ctx, r, &c, i, call, result); err != nil {
			return err
		}
		if session != nil {
			session.messages = append(session.messages, Message{Role: "tool", ToolCallID: call.ID, Content: string(result.Content)})
		}
		latest, e := loadRun(ctx, s.db, r.ID, false)
		if e != nil {
			return e
		}
		if latest.Epoch != r.Epoch {
			return context.Canceled
		}
		if latest.ActiveTaskID != "" || !runnable(latest.Status) {
			return nil
		}
	}
	_, err = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch {
			return context.Canceled
		}
		current.PendingCallID = ""
		current.Status = "PLANNING"
		current.ConsecutiveTransient = 0
		c.Status = "APPLIED"
		return saveCall(ctx, tx, c)
	})
	return err
}

func (s *Service) executeTool(ctx context.Context, r Run, c CallRecord, call ToolCall) Result {
	result := Result{CallID: call.ID}
	var value any
	var err error
	switch call.Function.Name {
	case "cad_create_task":
		value, err = s.createStage(ctx, r, c, call)
	case "cad_verify_goal":
		var verified *GoalVerification
		verified, err = s.verifyGoal(ctx, r)
		value = verified
		if err == nil {
			err = s.recordGoalVerification(ctx, r, verified)
		}
	case "cad_finish":
		var input struct {
			Summary string `json:"summary"`
		}
		err = decode([]byte(call.Function.Arguments), &input)
		if err == nil {
			var verified *GoalVerification
			verified, err = s.verifyGoal(ctx, r)
			value = verified
			if err == nil && verified.Status == "VERIFIED" {
				err = s.finishGoal(ctx, r, c, input.Summary, verified)
			} else if err == nil {
				err = aiFault("AI_GOAL_UNVERIFIED", "必需目标检查尚未通过，请读取当前事实继续或修复", false)
			}
		}
	case "cad_wait_for_input":
		var input struct {
			Question string `json:"question"`
		}
		err = decode([]byte(call.Function.Arguments), &input)
		if err == nil {
			_, err = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
				if current.Epoch != r.Epoch {
					return context.Canceled
				}
				current.Status = "WAITING_INPUT"
				current.WaitingReason = input.Question
				current.Reply = input.Question
				if e := saveGoalReply(ctx, tx, *current); e != nil {
					return e
				}
				return s.event(ctx, tx, current, "INPUT_REQUIRED", input)
			})
		}
		value = input
	case "cad_get_selection":
		var input struct {
			After int `json:"after"`
			Limit int `json:"limit"`
		}
		err = decode([]byte(call.Function.Arguments), &input)
		if input.Limit == 0 {
			input.Limit = 10
		}
		if input.After < 0 || input.Limit < 1 || input.Limit > 20 || input.After > len(r.Scope.Selections) {
			err = aiFault("INVALID_QUERY", "选择页范围无效", false)
		}
		if err == nil {
			end := min(len(r.Scope.Selections), input.After+input.Limit)
			value = map[string]any{"items": r.Scope.Selections[input.After:end], "count": r.Scope.SelectionCount, "complete": r.Scope.Complete, "hasMore": end < len(r.Scope.Selections), "nextAfter": end, "scopeDigest": modelcore.ValueDigest(marshalJSON(r.Scope))}
		}
	case "cad_advance_task":
		err = aiFault("TOOL_NOT_ALLOWED", "模型没有执行或伪造显示回执的权限", false)
	default:
		allowed := false
		for _, t := range planningTools() {
			allowed = allowed || t.Name == call.Function.Name
		}
		if !allowed {
			err = aiFault("TOOL_NOT_ALLOWED", "工具不可用", false)
			break
		}
		host := NewHost(s.automation, r.ActorID, r.DocumentID, false)
		if r.AccessMode == "READ" {
			host.revision = r.Scope.RevisionID
		}
		return host.Call(ctx, Call{ID: call.ID, Name: call.Function.Name, Arguments: []byte(call.Function.Arguments)})
	}
	if err != nil {
		result.IsError = true
		value = asFault(err)
	}
	result.Content = marshalJSON(value)
	return result
}

func (s *Service) recordTool(ctx context.Context, r Run, c *CallRecord, ordinal int, call ToolCall, result Result) (Result, error) {
	result = NormalizeToolResult(result)
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if c.InputID != "" {
			if e := s.requireQuestionLease(ctx, tx, c.InputID); e != nil {
				return e
			}
		}
		inserted, e := tx.Exec(ctx, `INSERT INTO occccad.assistant_tool_results(call_id,tool_id,ordinal,name,arguments,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(call_id,tool_id) DO NOTHING`, c.ID, call.ID, ordinal, call.Function.Name, json.RawMessage(call.Function.Arguments), marshalJSON(result))
		if e != nil {
			return e
		}
		if inserted.RowsAffected() > 0 {
			current.ToolCalls++
		} else {
			// An idempotent replay uses the originally recorded feedback, even
			// if a caller supplies a different candidate after recovery.
			var stored []byte
			if e = tx.QueryRow(ctx, `SELECT payload FROM occccad.assistant_tool_results WHERE call_id=$1 AND tool_id=$2`, c.ID, call.ID).Scan(&stored); e != nil {
				return e
			}
			if e = json.Unmarshal(stored, &result); e != nil {
				return e
			}
		}
		if call.Function.Name == "cad_describe_operation" && !result.IsError {
			var input struct {
				OperationID string `json:"operationId"`
			}
			_ = json.Unmarshal([]byte(call.Function.Arguments), &input)
			if _, e = tx.Exec(ctx, `INSERT INTO occccad.assistant_contracts(run_id,operation_id,digest) VALUES($1,$2,$3) ON CONFLICT(run_id,operation_id) DO UPDATE SET digest=excluded.digest`, r.ID, input.OperationID, s.automation.Catalog.Digest); e != nil {
				return e
			}
		}
		if c.InputID == "" && result.IsError && current.Epoch == r.Epoch && (call.Function.Name == "cad_create_task" || call.Function.Name == "cad_finish") {
			digest := modelcore.ValueDigest(marshalJSON(map[string]any{"head": c.Baseline, "goal": r.GoalVersion, "tool": call.Function.Name, "arguments": call.Function.Arguments, "diagnostic": result.Content}))
			if current.LastFailureDigest == digest {
				if e := s.event(ctx, tx, current, "REPEATED_DIAGNOSTIC", map[string]string{"digest": digest, "message": "同一工具诊断再次出现，已保留输入与反馈供模型继续判断。"}); e != nil {
					return e
				}
			}
			current.LastFailureDigest = digest
		}
		c.ToolCursor = ordinal + 1
		if stored, loadErr := loadCall(ctx, tx, c.ID); loadErr == nil && stored.Status == "APPLIED" {
			c.Status = "APPLIED"
		}
		if e = saveCall(ctx, tx, *c); e != nil {
			return e
		}
		if inserted.RowsAffected() == 0 {
			return nil
		}
		return s.event(ctx, tx, current, "TOOL_RESULT", map[string]any{"callId": c.ID, "toolId": call.ID, "inputId": c.InputID, "name": call.Function.Name, "isError": result.IsError, "message": toolProgress(call.Function.Name, result), "goalVersion": r.GoalVersion, "sourceEpoch": c.Epoch, "stale": current.Epoch != c.Epoch})
	})
	return result, err
}
func (s *Service) finishGoal(ctx context.Context, r Run, c CallRecord, reply string, v *GoalVerification) error {
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		var head string
		if e := tx.QueryRow(ctx, `SELECT head_revision_id::text FROM occccad.workspaces WHERE document_id=$1 AND name='main' FOR UPDATE`, current.DocumentID).Scan(&head); e != nil {
			return e
		}
		if head != v.RevisionID {
			current.Verification = nil
			current.Status = "PLANNING"
			current.PendingCallID = ""
			current.LatestRevisionID = head
			return s.event(ctx, tx, current, "VERIFICATION_INVALIDATED", map[string]string{"reason": "workspace head changed", "revisionId": head})
		}
		if current.Epoch != r.Epoch || v.GoalVersion != current.GoalVersion || v.RevisionID != current.LatestRevisionID {
			return context.Canceled
		}
		current.Status = "COMPLETED"
		current.PendingCallID = ""
		current.Verification = v
		current.Requirements = v.requirements
		current.Reply = reply
		if reply == "" {
			current.Reply = "建模目标已应用并通过检查。"
		}
		current.WaitingReason = ""
		current.Failure = nil
		c.Status = "APPLIED"
		if e := saveCall(ctx, tx, c); e != nil {
			return e
		}
		if e := saveGoalReply(ctx, tx, *current); e != nil {
			return e
		}
		return s.event(ctx, tx, current, "GOAL_VERIFIED", v)
	})
	if err == nil {
		s.runMu.Lock()
		delete(s.sessions, r.ID)
		s.runMu.Unlock()
	}
	return err
}
func (s *Service) finishRead(ctx context.Context, r Run, c CallRecord, reply string) error {
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch {
			return context.Canceled
		}
		current.Status = "COMPLETED"
		current.Reply = reply
		current.PendingCallID = ""
		c.Status = "APPLIED"
		if e := saveCall(ctx, tx, c); e != nil {
			return e
		}
		if e := saveGoalReply(ctx, tx, *current); e != nil {
			return e
		}
		return s.event(ctx, tx, current, "ANSWERED", map[string]string{"reply": reply})
	})
	if err == nil {
		s.runMu.Lock()
		delete(s.sessions, r.ID)
		s.runMu.Unlock()
	}
	return err
}

func saveGoalReply(ctx context.Context, tx database.Tx, r Run) error {
	var id string
	if e := tx.QueryRow(ctx, `SELECT id::text FROM occccad.assistant_inputs WHERE run_id=$1 AND (kind<>'QUERY' OR sequence=1) ORDER BY sequence DESC LIMIT 1`, r.ID).Scan(&id); e != nil {
		return e
	}
	t, _, e := loadInput(ctx, tx, id)
	if e != nil {
		return e
	}
	t.Status = r.Status
	t.Reply = r.Reply
	return saveInput(ctx, tx, t)
}
