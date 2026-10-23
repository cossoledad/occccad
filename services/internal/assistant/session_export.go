package assistant

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

// One SELECT gives PostgreSQL and SQLite one statement snapshot, including
// mutable call/tool/task records. Rows stream without an application write
// transaction or a growing in-memory transcript. No UI page limit is applied.
const exportRecordsSQL = `WITH calls AS (
 SELECT id,run_id,status,payload,started_at,COALESCE(payload->>'turnId','') AS turn_id
 FROM occccad.assistant_model_calls WHERE run_id=$1
), tasks AS (
 SELECT t.*,COALESCE(c.turn_id,'') AS turn_id,c.id AS call_id FROM occccad.automation_tasks t
 LEFT JOIN calls c ON t.request_id LIKE ('assistant:' || c.id::text || ':%')
 WHERE t.document_id=$3 AND t.actor_id=$4 AND t.payload->>'runId'=$1::text
), records AS (
 SELECT 0 AS category,'run' AS kind,r.id::text AS id,'' AS parent,'' AS turn_id,0 AS sequence,r.version,
 r.status,now()::text AS name,r.payload::text AS payload,
 CAST((SELECT COUNT(*) FROM calls WHERE turn_id='') AS text) AS arguments
 FROM occccad.assistant_runs r WHERE r.id=$1 AND r.document_id=$3 AND r.actor_id=$4
 UNION ALL SELECT 1,'input',i.id::text,'',i.id::text,i.sequence,0,i.status,'',i.payload::text,'null'
 FROM occccad.assistant_inputs i WHERE i.run_id=$1 AND ($2='' OR i.id::text=$2)
 UNION ALL SELECT 2,'call',c.id::text,'',c.turn_id,0,0,c.status,'',c.payload::text,
 CASE WHEN EXISTS (SELECT 1 FROM occccad.assistant_run_events e WHERE e.run_id=$1 AND e.sequence=COALESCE((c.payload->>'requestEventSequence')::bigint,0) AND e.payload->>'type'='MODEL_REQUEST_CAPTURED' AND e.payload->'data'->>'callId'=c.id::text) THEN 'true' ELSE 'false' END
 FROM calls c WHERE $2='' OR c.turn_id=$2
 UNION ALL SELECT 3,'event',e.sequence::text,'',COALESCE(e.payload->>'turnId',''),e.sequence,e.epoch,'','',e.payload::text,'null'
 FROM occccad.assistant_run_events e WHERE e.run_id=$1 AND ($2='' OR e.payload->>'turnId'=$2 OR (e.payload->>'type'='MODEL_REQUEST_CAPTURED' AND e.payload->'data'->>'callId' IN (SELECT id::text FROM calls WHERE turn_id=$2)))
 UNION ALL SELECT 4,'tool',t.tool_id,c.id::text,c.turn_id,t.ordinal,0,'',t.name,t.payload::text,t.arguments::text
 FROM occccad.assistant_tool_results t JOIN calls c ON c.id=t.call_id WHERE $2='' OR c.turn_id=$2
 UNION ALL SELECT 5,'stage',s.task_id::text,COALESCE(t.call_id::text,''),COALESCE(t.turn_id,''),s.sequence,s.goal_version,s.status,'',
 jsonb_build_object('runId',s.run_id::text,'sequence',s.sequence,'epoch',s.epoch,'goalVersion',s.goal_version,'taskId',s.task_id::text,'status',s.status)::text,'null'
 FROM occccad.assistant_stages s LEFT JOIN tasks t ON t.id=s.task_id WHERE s.run_id=$1 AND ($2='' OR t.turn_id=$2)
 UNION ALL SELECT 6,'task',t.id::text,COALESCE(t.call_id::text,''),t.turn_id,0,t.version,'','',t.payload::text,'null'
 FROM tasks t WHERE $2='' OR t.turn_id=$2
) SELECT kind,id,parent,turn_id,sequence,version,status,name,payload,arguments FROM records ORDER BY category,sequence,id`

type ExportMetadata struct {
	PendingCalls      int64  `json:"pendingCalls"`
	UnknownUsageCalls int64  `json:"unknownUsageCalls"`
	Schema            string `json:"schema"`
	RunID             string `json:"runId"`
	DocumentID        string `json:"documentId"`
	TurnID            string `json:"turnId,omitempty"`
	AsOf              string `json:"asOf"`
	RunStatus         string `json:"runStatus"`
	WorkUnfinished    bool   `json:"workUnfinished"`
	UnattributedCalls int64  `json:"unattributedCalls"`
	Boundary          string `json:"boundary"`
}
type ExportRecord struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	ParentID  string          `json:"parentId,omitempty"`
	TurnID    string          `json:"turnId,omitempty"`
	Sequence  uint64          `json:"sequence,omitempty"`
	Version   uint64          `json:"version,omitempty"`
	Status    string          `json:"status,omitempty"`
	Name      string          `json:"name,omitempty"`
	Data      json.RawMessage `json:"data"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Missing   []string        `json:"missing,omitempty"`
}
type SessionExport struct {
	Metadata ExportMetadata
	spool    *os.File
}

func scanExportRecord(rows database.Rows) (ExportRecord, error) {
	var v ExportRecord
	err := rows.Scan(&v.Kind, &v.ID, &v.ParentID, &v.TurnID, &v.Sequence, &v.Version, &v.Status, &v.Name, &v.Data, &v.Arguments)
	return v, err
}

func (s *Service) OpenExport(ctx context.Context, actor, document, run, turn string) (*SessionExport, error) {
	if _, err := s.GetRun(ctx, actor, document, run); err != nil {
		return nil, err
	}
	if turn != "" {
		if _, err := uuid.Parse(turn); err != nil {
			return nil, aiFault("AI_INPUT_INVALID", "导出轮次无效", false)
		}
		t, _, err := loadInput(ctx, s.db, turn)
		if err != nil || t.RunID != run {
			return nil, aiFault("AI_INPUT_INVALID", "导出轮次不属于该会话", false)
		}
	}
	rows, err := s.db.Query(ctx, exportRecordsSQL, run, turn, document, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		rows.Close()
		return nil, aiFault("AI_RUN_NOT_FOUND", "导出会话不可用", false)
	}
	first, err := scanExportRecord(rows)
	if err != nil {
		rows.Close()
		return nil, err
	}
	var state Run
	if first.Kind != "run" || json.Unmarshal(first.Data, &state) != nil {
		rows.Close()
		return nil, aiFault("AI_EXPORT_INVALID", "导出快照损坏", false)
	}
	unknown, _ := strconv.ParseInt(string(first.Arguments), 10, 64)
	meta := ExportMetadata{PendingCalls: state.Usage.PendingCalls, UnknownUsageCalls: state.Usage.UnknownAttempts, Schema: "occccad.assistant-export.v1", RunID: run, DocumentID: document, TurnID: turn, AsOf: first.Name, RunStatus: first.Status, WorkUnfinished: !terminal(first.Status) || state.ActiveTaskID != "" || state.RetiringTaskID != "" || state.PendingCallID != "" || state.Usage.PendingCalls > 0, UnattributedCalls: unknown, Boundary: "仅包含已保存的公开记录；不包含 API Key、私有思考或未保存的历史。无法归属的历史调用不按 Epoch 猜测归入本轮。"}
	first.Name = ""
	first.Arguments = nil
	if turn != "" {
		first.Data = marshalJSON(map[string]any{"id": state.ID, "documentId": state.DocumentID, "version": state.Version, "status": state.Status, "createdAt": state.CreatedAt})
	}
	root, err := config.ProjectRoot()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, "build", "assistant-exports")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(directory, "session-*.jsonl")
	if err != nil {
		return nil, err
	}
	archive := &SessionExport{Metadata: meta, spool: file}
	ok := false
	defer func() {
		if !ok {
			archive.Close()
		}
	}()
	buffer := bufio.NewWriterSize(file, 64<<10)
	encode := func(v ExportRecord) error {
		v, err := publicExportRecord(v)
		if err != nil {
			return err
		}
		return writeExportJSON(buffer, v)
	}
	if err = encode(first); err != nil {
		return nil, err
	}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		v, e := scanExportRecord(rows)
		if e != nil {
			return nil, e
		}
		if err = encode(v); err != nil {
			return nil, err
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = buffer.Flush(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ok = true
	return archive, nil
}
func (e *SessionExport) Close() {
	_ = e.spool.Close()
	_ = os.Remove(e.spool.Name())
}

func publicExportRecord(v ExportRecord) (ExportRecord, error) {
	if v.Kind == "call" {
		var c CallRecord
		if err := json.Unmarshal(v.Data, &c); err != nil {
			return v, err
		}
		c.Status = v.Status
		if c.Response != nil {
			c.Response.ReasoningContent = ""
		}
		v.Data = marshalJSON(c)
		if c.TurnID == "" {
			v.Missing = append(v.Missing, "TURN_ATTRIBUTION_UNAVAILABLE")
		}
		if c.RequestEventSequence == 0 || string(v.Arguments) != "true" {
			v.Missing = append(v.Missing, "REQUEST_SNAPSHOT_UNAVAILABLE")
		}
		v.Arguments = nil
		if c.Usage == nil {
			v.Missing = append(v.Missing, "USAGE_NOT_REPORTED")
		}
	}
	if v.Kind == "task" {
		var task automation.Task
		if err := json.Unmarshal(v.Data, &task); err != nil {
			return v, err
		}
		if task.EventsTruncated {
			v.Missing = append(v.Missing, "TASK_EVENT_HISTORY_TRUNCATED_AT_SOURCE")
		}
	}
	if (v.Kind == "task" || v.Kind == "stage") && v.TurnID == "" {
		v.Missing = append(v.Missing, "TURN_ATTRIBUTION_UNAVAILABLE")
	}
	if v.Kind == "input" {
		var t Turn
		if err := json.Unmarshal(v.Data, &t); err != nil {
			return v, err
		}
		t.Status = v.Status
		v.Data = marshalJSON(t)
		if t.Usage == nil {
			v.Missing = append(v.Missing, "TURN_USAGE_UNAVAILABLE")
		}
	}
	if !json.Valid(v.Data) {
		return v, aiFault("AI_EXPORT_INVALID", "导出记录不是有效JSON", false)
	}
	if len(v.Arguments) > 0 && !json.Valid(v.Arguments) {
		v.Arguments = marshalJSON(string(v.Arguments))
	}
	return v, nil
}

func writeExportJSON(w io.Writer, value any) error { return json.NewEncoder(w).Encode(value) }
func writeExportMarkdown(w io.Writer, title string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	// Arbitrary public text must not close the surrounding Markdown code block.
	fence := "```"
	for strings.Contains(string(raw), fence) {
		fence += "`"
	}
	_, err = fmt.Fprintf(w, "\n## %s\n\n%sjson\n%s\n%s\n", title, fence, raw, fence)
	return err
}

func writeExportText(w io.Writer, title, content string) error {
	fence := "```"
	for strings.Contains(content, fence) {
		fence += "`"
	}
	_, err := fmt.Fprintf(w, "\n### %s\n\n%stext\n%s\n%s\n", title, fence, content, fence)
	return err
}

// Both formats use the same full snapshot iterator. A final complete marker is
// written only after the cursor is exhausted, so a broken download is apparent.
func (e *SessionExport) WriteTo(w io.Writer, format string) error {
	if format != "json" && format != "markdown" {
		return aiFault("AI_INPUT_INVALID", "导出格式必须为JSON或Markdown", false)
	}
	if format == "json" {
		if _, err := io.WriteString(w, "{\"metadata\":"); err != nil {
			return err
		}
		if err := writeExportJSON(w, e.Metadata); err != nil {
			return err
		}
		if _, err := io.WriteString(w, ",\"records\":["); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "# 建模助手会话导出\n\n截至：%s\n\n会话状态：%s；读取快照时尚有未完成工作：%t。\n\n", e.Metadata.AsOf, e.Metadata.RunStatus, e.Metadata.WorkUnfinished); err != nil {
			return err
		}
		if err := writeExportMarkdown(w, "导出范围与记录边界", e.Metadata); err != nil {
			return err
		}
	}
	count := 0
	write := func(v ExportRecord) error {
		var err error
		if format == "markdown" {
			err = writeExportMarkdown(w, fmt.Sprintf("%s · %s", v.Kind, v.ID), v)
			if err != nil {
				return err
			}
			if v.Kind == "input" {
				var t Turn
				if json.Unmarshal(v.Data, &t) == nil {
					err = writeExportText(w, "用户输入", t.Text)
					if err == nil && t.Reply != "" {
						err = writeExportText(w, "已保存回复", t.Reply)
					}
				}
			}
			if v.Kind == "call" {
				var c CallRecord
				if json.Unmarshal(v.Data, &c) == nil && c.Response != nil && c.Response.Content != "" {
					err = writeExportText(w, "公开模型内容", c.Response.Content)
				}
			}
		} else {
			if count > 0 {
				if _, err = io.WriteString(w, ","); err != nil {
					return err
				}
			}
			err = writeExportJSON(w, v)
		}
		if err == nil {
			count++
		}
		return err
	}
	decoder := json.NewDecoder(bufio.NewReaderSize(e.spool, 64<<10))
	for {
		var v ExportRecord
		err := decoder.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err = write(v); err != nil {
			return err
		}
	}
	if format == "markdown" {
		return writeExportMarkdown(w, "导出完成", map[string]any{"complete": true, "recordCount": count})
	}
	_, err := fmt.Fprintf(w, "],\"complete\":true,\"recordCount\":%d}\n", count)
	return err
}
