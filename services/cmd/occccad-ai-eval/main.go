// occcccad-ai-eval is an explicitly invoked live-provider acceptance client.
// It uses the existing account's server-side credential; it never reads or
// writes an AI key and is intentionally absent from normal test execution.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/occccad/occccad/internal/config"
)

type evalCase struct {
	ID     string           `json:"id"`
	Setup  string           `json:"setup"`
	Prompt string           `json:"prompt"`
	Checks []map[string]any `json:"checks"`
}
type settings struct {
	Configured     bool   `json:"configured"`
	Provider       string `json:"provider"`
	ProviderOrigin string `json:"providerOrigin"`
	Model          string `json:"model"`
	Thinking       string `json:"thinking"`
	Version        uint64 `json:"version"`
}
type runView struct {
	ID               string            `json:"id"`
	DocumentID       string            `json:"documentId"`
	Version          uint64            `json:"version"`
	Epoch            uint64            `json:"epoch"`
	GoalVersion      uint64            `json:"goalVersion"`
	Status           string            `json:"status"`
	ModelCalls       int64             `json:"modelCalls"`
	Usage            map[string]any    `json:"usage"`
	LatestRevisionID string            `json:"latestRevisionId"`
	Targets          map[string]string `json:"targets"`
	Verification     json.RawMessage   `json:"verification"`
	Failure          *struct {
		Code string `json:"code"`
	} `json:"failure"`
}
type callSummary struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	UsageState string `json:"usageState"`
	Generation struct {
		Provider       string `json:"provider"`
		ProviderOrigin string `json:"providerOrigin"`
		Model          string `json:"model"`
		Thinking       string `json:"thinking"`
		Version        uint64 `json:"version"`
	} `json:"generation"`
	Usage *struct {
		PromptTokens     int64  `json:"prompt_tokens"`
		CompletionTokens int64  `json:"completion_tokens"`
		TotalTokens      int64  `json:"total_tokens"`
		CacheHitTokens   *int64 `json:"cacheHitTokens"`
		ReasoningTokens  *int64 `json:"reasoningTokens"`
	} `json:"usage"`
}
type caseReport struct {
	Case                 string          `json:"case"`
	Source               string          `json:"source"`
	Status               string          `json:"status"`
	Reason               string          `json:"reason,omitempty"`
	DocumentID           string          `json:"documentId,omitempty"`
	RunID                string          `json:"runId,omitempty"`
	RunStatus            string          `json:"runStatus,omitempty"`
	ProviderOrigin       string          `json:"providerOrigin,omitempty"`
	Model                string          `json:"model,omitempty"`
	Thinking             string          `json:"thinking,omitempty"`
	SettingsVersion      uint64          `json:"settingsVersion"`
	TokenBudget          int64           `json:"tokenBudget"`
	ModelCalls           int64           `json:"modelCalls"`
	Usage                map[string]any  `json:"usage,omitempty"`
	Calls                []callSummary   `json:"calls"`
	Unknowns             []string        `json:"unknowns"`
	PlanRepairs          int             `json:"planRepairs"`
	PlanRepairEventTypes map[string]int  `json:"planRepairEventTypes"`
	RunVerification      json.RawMessage `json:"runVerification,omitempty"`
	SemanticVerification json.RawMessage `json:"semanticVerification,omitempty"`
	StopStatus           string          `json:"stopStatus,omitempty"`
	WallSeconds          float64         `json:"wallSeconds"`
	Resources            []string        `json:"resources,omitempty"`
}
type report struct {
	SchemaVersion int          `json:"schemaVersion"`
	GeneratedAt   string       `json:"generatedAt"`
	BaseURL       string       `json:"baseUrl"`
	Cases         []caseReport `json:"cases"`
}
type apiClient struct {
	base *url.URL
	http *http.Client
}

func (c *apiClient) request(ctx context.Context, method, path string, input, out any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request")
		}
		body = bytes.NewReader(raw)
	}
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(c.base.Path, "/") + path
	endpoint.RawQuery = ""
	if before, query, ok := strings.Cut(endpoint.Path, "?"); ok {
		endpoint.Path, endpoint.RawQuery = before, query
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("construct API request")
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		for _, cookie := range c.http.Jar.Cookies(c.base) {
			if cookie.Name == "occccad_csrf" {
				req.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("API transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("API %s %s returned HTTP %d", method, path, res.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 2<<20))
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return fmt.Errorf("API response exceeds read budget")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("API response is not valid JSON")
	}
	return nil
}

func newClient(raw string) (*apiClient, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("base URL must have a host and no credentials, query or fragment")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return nil, fmt.Errorf("base URL must use HTTPS or loopback HTTP")
	}
	jar, _ := cookiejar.New(nil)
	return &apiClient{base: u, http: &http.Client{Jar: jar, Timeout: 120 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func providerIsLive(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	return host != "localhost" && !strings.HasSuffix(host, ".localhost") && (ip == nil || (!ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsPrivate()))
}

type documentView struct {
	Document struct {
		ID        string `json:"id"`
		VersionID string `json:"versionId"`
	} `json:"document"`
	Part *struct {
		ActiveBodyID string `json:"activeBodyId"`
		Features     []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"features"`
	} `json:"part"`
	Product *struct {
		Instances []struct {
			ID         string `json:"id"`
			DocumentID string `json:"documentId"`
		} `json:"instances"`
	} `json:"product"`
}

func (c *apiClient) createDocument(ctx context.Context, kind, name string) (documentView, error) {
	var view documentView
	err := c.request(ctx, "POST", "/api/documents", map[string]any{"requestId": uuid.NewString(), "type": kind, "name": name}, &view)
	return view, err
}
func (c *apiClient) document(ctx context.Context, id string) (documentView, error) {
	var view documentView
	err := c.request(ctx, "GET", "/api/documents/"+url.PathEscape(id), nil, &view)
	return view, err
}

type fixtureContext struct {
	Document          documentView
	Moving, Reference string
	Resources         []string
}

func (c *apiClient) setup(ctx context.Context, test evalCase) (fixtureContext, error) {
	var fixture fixtureContext
	if test.Setup == "EMPTY_PART" {
		v, e := c.createDocument(ctx, "PART", "AI live "+test.ID+" "+uuid.NewString())
		fixture.Document = v
		if v.Document.ID != "" {
			fixture.Resources = append(fixture.Resources, v.Document.ID)
		}
		return fixture, e
	}
	if test.Setup != "SHAFT_HOLE_PRODUCT" {
		return fixture, fmt.Errorf("unsupported case fixture")
	}
	ws, err := c.openRealtime(ctx)
	if err != nil {
		return fixture, err
	}
	defer ws.Close()
	makePart := func(name string, outer, inner, length float64) (documentView, error) {
		v, e := c.createDocument(ctx, "PART", "AI live "+name+" "+uuid.NewString())
		if e != nil {
			return v, e
		}
		fixture.Resources = append(fixture.Resources, v.Document.ID)
		if v.Part == nil {
			return v, fmt.Errorf("Part fixture response is incomplete")
		}
		v, e = c.fixtureCommand(ctx, ws, v.Document.ID, map[string]any{"type": "CREATE_SKETCH", "bodyId": v.Part.ActiveBodyID, "plane": "XY"})
		if e != nil {
			return v, e
		}
		sketch := ""
		for _, f := range v.Part.Features {
			if f.Type == "SKETCH" {
				if sketch != "" {
					return v, fmt.Errorf("ambiguous fixture sketch")
				}
				sketch = f.ID
			}
		}
		if sketch == "" {
			return v, fmt.Errorf("fixture sketch was not persisted")
		}
		ops := []map[string]any{{"type": "ADD_ENTITY", "entity": map[string]any{"id": uuid.NewString(), "kind": "CIRCLE", "role": "PROFILE", "center": map[string]float64{"x": 0, "y": 0}, "radius": outer}}}
		if inner > 0 {
			ops = append(ops, map[string]any{"type": "ADD_ENTITY", "entity": map[string]any{"id": uuid.NewString(), "kind": "CIRCLE", "role": "PROFILE", "center": map[string]float64{"x": 0, "y": 0}, "radius": inner}})
		}
		v, e = c.fixtureCommand(ctx, ws, v.Document.ID, map[string]any{"type": "EDIT_SKETCH", "sketchId": sketch, "operations": ops})
		if e != nil {
			return v, e
		}
		return c.fixtureCommand(ctx, ws, v.Document.ID, map[string]any{"type": "CREATE_SOLID_FEATURE", "bodyId": v.Part.ActiveBodyID, "sketchId": sketch, "generator": "LINEAR_EXTRUDE", "operation": "ADD", "length": length})
	}
	shaft, err := makePart("shaft", 5, 0, 35)
	if err != nil {
		return fixture, err
	}
	sleeve, err := makePart("sleeve", 12, 5.1, 20)
	if err != nil {
		return fixture, err
	}
	product, err := c.createDocument(ctx, "PRODUCT", "AI live shaft-hole "+uuid.NewString())
	if err != nil {
		return fixture, err
	}
	fixture.Resources = append(fixture.Resources, product.Document.ID)
	for _, part := range []documentView{sleeve, shaft} {
		product, err = c.fixtureCommand(ctx, ws, product.Document.ID, map[string]any{"type": "INSERT_INSTANCE", "referencedDocumentId": part.Document.ID, "referenceMode": "PINNED"})
		if err != nil {
			return fixture, err
		}
	}
	if product.Product == nil {
		return fixture, fmt.Errorf("Product fixture response is incomplete")
	}
	for _, instance := range product.Product.Instances {
		if instance.DocumentID == shaft.Document.ID {
			fixture.Moving = instance.ID
		}
		if instance.DocumentID == sleeve.Document.ID {
			fixture.Reference = instance.ID
		}
	}
	if fixture.Moving == "" || fixture.Reference == "" {
		return fixture, fmt.Errorf("fixture stable instances were not persisted")
	}
	angle := 37 * math.Pi / 180
	product, err = c.fixtureCommand(ctx, ws, product.Document.ID, map[string]any{"type": "MOVE_INSTANCE", "instanceId": fixture.Moving, "translation": [3]float64{47, -31, 19}, "rotation": [4]float64{0, math.Sin(angle / 2), 0, math.Cos(angle / 2)}})
	fixture.Document = product
	return fixture, err
}

type realtimeEnvelope struct {
	Protocol      string          `json:"protocol"`
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlationId,omitempty"`
	SentAt        string          `json:"sentAt,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Error         *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

func (c *apiClient) openRealtime(ctx context.Context) (*websocket.Conn, error) {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/api/realtime"
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	headers := http.Header{"Origin": []string{c.base.Scheme + "://" + c.base.Host}}
	cookies := []string{}
	csrf := ""
	for _, cookie := range c.http.Jar.Cookies(c.base) {
		cookies = append(cookies, cookie.Name+"="+cookie.Value)
		if cookie.Name == "occccad_csrf" {
			csrf = cookie.Value
		}
	}
	headers.Set("Cookie", strings.Join(cookies, "; "))
	dialer := websocket.Dialer{Subprotocols: []string{"occccad.realtime.v1"}, HandshakeTimeout: 10 * time.Second}
	ws, response, err := dialer.DialContext(ctx, u.String(), headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("fixture realtime connection failed")
	}
	ws.SetReadLimit(2 << 20)
	if _, err = exchange(ctx, ws, "connection.initialize.v1", map[string]string{"csrfToken": csrf}); err != nil {
		_ = ws.Close()
		return nil, err
	}
	return ws, nil
}
func exchange(ctx context.Context, ws *websocket.Conn, kind string, payload any) (realtimeEnvelope, error) {
	var out realtimeEnvelope
	deadline := time.Now().Add(120 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = ws.SetReadDeadline(deadline)
	_ = ws.SetWriteDeadline(deadline)
	id := uuid.NewString()
	raw, _ := json.Marshal(payload)
	if err := ws.WriteJSON(realtimeEnvelope{Protocol: "occccad.realtime.v1", ID: id, Kind: "request", Type: kind, SentAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: raw}); err != nil {
		return out, fmt.Errorf("fixture realtime send failed")
	}
	for n := 0; n < 100; n++ {
		if err := ws.ReadJSON(&out); err != nil {
			return out, fmt.Errorf("fixture realtime response failed")
		}
		if out.CorrelationID != id {
			continue
		}
		if out.Error != nil {
			return out, fmt.Errorf("fixture domain command failed: %s", out.Error.Code)
		}
		return out, nil
	}
	return out, fmt.Errorf("fixture realtime event budget reached")
}
func (c *apiClient) fixtureCommand(ctx context.Context, ws *websocket.Conn, document string, command map[string]any) (documentView, error) {
	var snapshot struct {
		Snapshot map[string]any `json:"snapshot"`
	}
	if err := c.request(ctx, "GET", "/api/documents/"+document+"/automation/context", nil, &snapshot); err != nil {
		return documentView{}, err
	}
	command["requestId"], command["expectedHead"] = uuid.NewString(), snapshot.Snapshot
	command["expectedHead"] = map[string]any{"workspaceId": snapshot.Snapshot["workspaceId"], "revisionId": snapshot.Snapshot["revisionId"], "sequence": snapshot.Snapshot["sequence"]}
	preview, err := exchange(ctx, ws, "workspace.preview.request.v1", map[string]any{"documentId": document, "interactionId": uuid.NewString(), "previewSequence": 1, "command": command})
	if err != nil {
		return documentView{}, err
	}
	var candidate struct {
		Preview struct {
			ID      string `json:"previewId"`
			Outcome string `json:"evaluationOutcome"`
		} `json:"preview"`
	}
	if json.Unmarshal(preview.Payload, &candidate) != nil || candidate.Preview.ID == "" || candidate.Preview.Outcome == "DEFINITION_ONLY" {
		return documentView{}, fmt.Errorf("fixture preview has no qualified candidate")
	}
	command["previewId"] = candidate.Preview.ID
	if _, err = exchange(ctx, ws, "workspace.command.execute.v1", map[string]any{"documentId": document, "command": command, "inlineSnapshot": false}); err != nil {
		return documentView{}, err
	}
	return c.document(ctx, document)
}

func runPath(document, id string) string {
	return "/api/documents/" + url.PathEscape(document) + "/assistant/runs/" + url.PathEscape(id)
}
func terminal(status string) bool {
	switch status {
	case "COMPLETED", "STOPPED", "BLOCKED":
		return true
	}
	return false
}
func waiting(status string) bool {
	switch status {
	case "WAITING_INPUT", "WAITING_BUDGET", "WAITING_CONFIRMATION", "WAITING_UI", "PAUSED":
		return true
	}
	return false
}
func (c *apiClient) stop(run runView) (runView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for attempt := 0; attempt < 4; attempt++ {
		if err := c.request(ctx, "GET", runPath(run.DocumentID, run.ID), nil, &run); err != nil {
			return run, err
		}
		if terminal(run.Status) {
			return run, nil
		}
		var stopped runView
		err := c.request(ctx, "POST", runPath(run.DocumentID, run.ID)+"/control", map[string]any{"action": "stop", "expectedVersion": run.Version, "epoch": run.Epoch}, &stopped)
		if err == nil {
			return stopped, nil
		}
		if attempt == 3 {
			return run, err
		}
	}
	return run, fmt.Errorf("stop could not acquire current Run version")
}

func (c *apiClient) evaluate(ctx context.Context, test evalCase, current settings, budget int64, timeout time.Duration) caseReport {
	started := time.Now()
	out := caseReport{Case: test.ID, Source: "LIVE_PROVIDER", Status: "NOT_RUN", ProviderOrigin: current.ProviderOrigin, Model: current.Model, Thinking: current.Thinking, SettingsVersion: current.Version, TokenBudget: budget, Calls: []callSummary{}, Unknowns: []string{}, PlanRepairEventTypes: map[string]int{}}
	fixture, err := c.setup(ctx, test)
	out.Resources = fixture.Resources
	if err != nil {
		out.Reason = "FIXTURE_SETUP_FAILED: " + err.Error()
		out.WallSeconds = time.Since(started).Seconds()
		return out
	}
	out.DocumentID = fixture.Document.Document.ID
	prompt := strings.NewReplacer("{{movingInstanceId}}", fixture.Moving, "{{referenceInstanceId}}", fixture.Reference).Replace(test.Prompt)
	scope := map[string]any{"rootDocumentId": out.DocumentID, "documentId": out.DocumentID, "revisionId": fixture.Document.Document.VersionID, "rootRevisionId": fixture.Document.Document.VersionID, "selections": []any{}, "selectionCount": 0, "complete": true}
	if fixture.Document.Part != nil {
		scope["workingBodyId"] = fixture.Document.Part.ActiveBodyID
	}
	var run runView
	if err = c.request(ctx, "POST", "/api/documents/"+out.DocumentID+"/assistant/runs", map[string]any{"requestId": uuid.NewString(), "text": prompt, "scope": scope, "mode": "AUTOMATIC", "tokenBudget": budget}, &run); err != nil {
		out.Reason = "RUN_CREATE_FAILED: " + err.Error()
		out.WallSeconds = time.Since(started).Seconds()
		return out
	}
	out.RunID = run.ID
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timedOut := false
	for !terminal(run.Status) && !waiting(run.Status) {
		select {
		case <-waitCtx.Done():
			timedOut = true
		case <-ticker.C:
			err = c.request(waitCtx, "GET", runPath(out.DocumentID, run.ID), nil, &run)
		}
		if timedOut || err != nil {
			timedOut = errors.Is(waitCtx.Err(), context.DeadlineExceeded)
			if stop, e := c.stop(run); e == nil {
				run = stop
				out.StopStatus = stop.Status
			} else {
				out.StopStatus = "UNKNOWN"
				out.Unknowns = append(out.Unknowns, "stop outcome unknown; inspect the retained Run")
			}
			break
		}
	}
	out.RunStatus, out.ModelCalls, out.Usage, out.RunVerification = run.Status, run.ModelCalls, run.Usage, run.Verification
	out.Status = "NOT_PASSED"
	if timedOut {
		out.Status = "TIMED_OUT"
		out.Reason = "evaluation wait timeout; production Run has no imposed lifetime deadline"
	} else if err != nil {
		out.Reason = "POLL_FAILED: " + err.Error()
	} else if waiting(run.Status) {
		out.Reason = "RUN_REQUIRES_INTERVENTION"
	} else if run.Status != "COMPLETED" {
		out.Reason = "RUN_DID_NOT_COMPLETE"
	}
	if run.Failure != nil {
		out.Reason += " " + run.Failure.Code
	}
	if run.Status == "COMPLETED" {
		var semantic map[string]any
		if test.Setup == "EMPTY_PART" {
			checks := make([]map[string]any, 0, len(test.Checks))
			for _, check := range test.Checks {
				copy := map[string]any{}
				for k, v := range check {
					copy[k] = v
				}
				copy["bodyId"] = fixture.Document.Part.ActiveBodyID
				checks = append(checks, copy)
			}
			created := []string{}
			for _, role := range []string{"SKETCH", "LINEAR_EXTRUDE", "FILLET", "CHAMFER"} {
				if id := run.Targets[role]; id != "" {
					created = append(created, id)
				}
			}
			err = c.request(ctx, "POST", "/api/documents/"+out.DocumentID+"/automation/verify-part", map[string]any{"revisionId": run.LatestRevisionID, "goalVersion": run.GoalVersion, "createdEntityIds": created, "checks": checks}, &semantic)
		} else {
			err = c.request(ctx, "POST", "/api/documents/"+out.DocumentID+"/automation/verify-assembly", map[string]any{"revisionId": run.LatestRevisionID, "movingInstanceId": fixture.Moving, "referenceInstanceId": fixture.Reference, "directionRelation": "SAME", "axisDirectionRelation": "SAME", "offsetMM": 0, "targetDOF": 1}, &semantic)
		}
		if err != nil {
			out.Reason = "INDEPENDENT_VERIFICATION_UNAVAILABLE"
			out.Unknowns = append(out.Unknowns, err.Error())
		} else {
			out.SemanticVerification, _ = json.Marshal(semantic)
			if passed, ok := semantic["passed"].(bool); ok && passed {
				out.Status = "PASSED"
				out.Reason = ""
			} else {
				out.Reason = "INDEPENDENT_GOAL_CHECK_FAILED"
			}
		}
	}
	c.collect(ctx, run, &out)
	out.WallSeconds = time.Since(started).Seconds()
	return out
}

func (c *apiClient) collect(parent context.Context, run runView, out *caseReport) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 20*time.Second)
	defer cancel()
	cursor := ""
	for page := 0; page < 20; page++ {
		var response struct {
			Items   []callSummary `json:"items"`
			HasMore bool          `json:"hasMore"`
			Next    string        `json:"nextCursor"`
		}
		if err := c.request(ctx, "GET", runPath(run.DocumentID, run.ID)+"/calls?limit=50&cursor="+url.QueryEscape(cursor), nil, &response); err != nil {
			out.Unknowns = append(out.Unknowns, "model-call ledger unavailable")
			break
		}
		out.Calls = append(out.Calls, response.Items...)
		if !response.HasMore {
			break
		}
		if response.Next == "" || response.Next == cursor || page == 19 {
			out.Unknowns = append(out.Unknowns, "model-call ledger pagination incomplete")
			break
		}
		cursor = response.Next
	}
	unknown, cacheUnknown, reasoningUnknown := 0, 0, 0
	if len(out.Calls) == 0 {
		out.Source = "UNATTESTED_PROVIDER"
		out.Status = "NOT_PASSED"
		out.Unknowns = append(out.Unknowns, "no recorded model attempt available to attest live provider execution")
	}
	for _, call := range out.Calls {
		if !providerIsLive(call.Generation.ProviderOrigin) {
			out.Source = "UNATTESTED_PROVIDER"
			out.Status = "NOT_PASSED"
			out.Unknowns = append(out.Unknowns, "a model attempt did not record a nonlocal HTTPS provider origin")
		}
		if call.Usage == nil {
			if call.Status != "PREPARED" {
				unknown++
			}
			continue
		}
		if call.Usage.CacheHitTokens == nil {
			cacheUnknown++
		}
		if call.Usage.ReasoningTokens == nil {
			reasoningUnknown++
		}
	}
	if unknown > 0 {
		out.Unknowns = append(out.Unknowns, fmt.Sprintf("provider token usage unknown for %d attempts; unknown is not zero", unknown))
	}
	if cacheUnknown > 0 {
		out.Unknowns = append(out.Unknowns, fmt.Sprintf("cache breakdown not reported for %d calls", cacheUnknown))
	}
	if reasoningUnknown > 0 {
		out.Unknowns = append(out.Unknowns, fmt.Sprintf("reasoning breakdown not reported for %d calls", reasoningUnknown))
	}
	after := uint64(0)
	for page := 0; page < 20; page++ {
		var response struct {
			Items []struct {
				Sequence uint64 `json:"sequence"`
				Type     string `json:"type"`
			} `json:"items"`
			HasMore bool   `json:"hasMore"`
			Next    uint64 `json:"nextAfter"`
		}
		if err := c.request(ctx, "GET", fmt.Sprintf("%s/events?limit=50&after=%d", runPath(run.DocumentID, run.ID), after), nil, &response); err != nil {
			out.Unknowns = append(out.Unknowns, "repair event history unavailable")
			break
		}
		for _, event := range response.Items {
			switch event.Type {
			case "PLAN_REPAIRED", "PLAN_REPAIR", "PLAN_REVISED", "REPLAN_REQUIRED", "STAGE_REPAIR":
				out.PlanRepairs++
				out.PlanRepairEventTypes[event.Type]++
			}
		}
		if !response.HasMore {
			break
		}
		if response.Next <= after || page == 19 {
			out.Unknowns = append(out.Unknowns, "event pagination incomplete")
			break
		}
		after = response.Next
	}
	var latest runView
	if err := c.request(ctx, "GET", runPath(run.DocumentID, run.ID), nil, &latest); err == nil {
		out.ModelCalls, out.Usage = latest.ModelCalls, latest.Usage
	} else {
		out.Unknowns = append(out.Unknowns, "final Run usage snapshot unavailable; late call accounting may arrive later")
	}
}

func saveReport(path string, value report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func repositoryRoot(envFile string) (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, candidate := range []string{start, filepath.Dir(envFile)} {
		for directory := candidate; directory != ""; {
			if _, err := os.Stat(filepath.Join(directory, "tasks", "01.task.txt")); err == nil {
				return directory, nil
			}
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
			directory = parent
		}
	}
	return "", fmt.Errorf("run from the repository or select its environment file")
}
func defaultBase(configuration config.Config) string {
	host, port, err := net.SplitHostPort(configuration.ListenAddress)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func main() {
	envFile, err := config.LoadEnvironment()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load current development environment failed")
		os.Exit(2)
	}
	configuration := config.Load()
	root, err := repositoryRoot(envFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	base := flag.String("base-url", defaultBase(configuration), "existing CAD API origin")
	email := flag.String("email", configuration.AdminEmail, "existing account email (default current environment)")
	password := flag.String("password", "", "account password; defaults to current environment without printing its value")
	selected := flag.String("case", "box", "box, box-fillet, shaft-hole or all")
	output := flag.String("output", filepath.Join(root, "build", "acceptance", "ai-eval.json"), "JSON evidence output")
	tokenBudget := flag.Int64("token-budget", 20000, "explicit cumulative Run token budget")
	timeout := flag.Duration("timeout", 5*time.Minute, "evaluation wait timeout; explicit stop on expiration")
	flag.Parse()
	if *password == "" {
		*password = configuration.AdminPassword
	}
	if *tokenBudget <= 0 || *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "token-budget and timeout must be positive")
		os.Exit(2)
	}
	raw, err := os.ReadFile(filepath.Join(root, "tasks", "ai-eval-cases.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read evaluation cases failed")
		os.Exit(2)
	}
	var catalog struct {
		SchemaVersion int        `json:"schemaVersion"`
		Cases         []evalCase `json:"cases"`
	}
	if json.Unmarshal(raw, &catalog) != nil || catalog.SchemaVersion != 1 {
		fmt.Fprintln(os.Stderr, "invalid evaluation cases")
		os.Exit(2)
	}
	cases := []evalCase{}
	for _, item := range catalog.Cases {
		if *selected == "all" || *selected == item.ID {
			cases = append(cases, item)
		}
	}
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "unknown evaluation case")
		os.Exit(2)
	}
	client, err := newClient(*base)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	evidence := report{SchemaVersion: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), BaseURL: client.base.String(), Cases: []caseReport{}}
	var current settings
	notRun := ""
	if *password == "" {
		notRun = "ACCOUNT_PASSWORD_NOT_CONFIGURED"
	} else if err = client.request(ctx, "POST", "/api/auth/login", map[string]string{"email": *email, "password": *password}, nil); err != nil {
		notRun = "LOGIN_UNAVAILABLE"
	} else if err = client.request(ctx, "GET", "/api/assistant/settings", nil, &current); err != nil {
		notRun = "ASSISTANT_SETTINGS_UNAVAILABLE"
	} else if !current.Configured {
		notRun = "API_KEY_NOT_CONFIGURED"
	} else if !providerIsLive(current.ProviderOrigin) {
		notRun = "LIVE_PROVIDER_ORIGIN_NOT_ATTESTED"
	}
	for _, test := range cases {
		var result caseReport
		if notRun != "" {
			result = caseReport{Case: test.ID, Source: "NOT_RUN", Status: "NOT_RUN", Reason: notRun, Calls: []callSummary{}, Unknowns: []string{}, TokenBudget: *tokenBudget}
		} else {
			result = client.evaluate(ctx, test, current, *tokenBudget, *timeout)
		}
		evidence.Cases = append(evidence.Cases, result)
		if err = saveReport(*output, evidence); err != nil {
			fmt.Fprintln(os.Stderr, "save evaluation evidence failed")
			os.Exit(1)
		}
		fmt.Printf("%s: %s (Run %s, calls %d, %.2fs)\n", test.ID, result.Status, result.RunID, result.ModelCalls, result.WallSeconds)
		if ctx.Err() != nil {
			break
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		fmt.Fprintln(os.Stderr, "evaluation interrupted; inspect retained Run and stop status")
	}
	fmt.Printf("Evidence: %s\n", *output)
}
