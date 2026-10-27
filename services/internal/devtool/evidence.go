package devtool

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Counts struct {
	Started int `json:"started"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}
type Evidence struct {
	Status string       `json:"status"`
	Reason string       `json:"reason,omitempty"`
	Counts Counts       `json:"counts"`
	Cached bool         `json:"cached"`
	Cases  []CaseResult `json:"cases,omitempty"`
}
type CaseResult struct {
	Package string `json:"package,omitempty"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}
type GoEvent struct {
	Action, Package, Test, Output string
	Elapsed                       float64
}

func redactGoOutput(raw string) string {
	var output strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		var event map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &event) == nil && event != nil {
			var detail string
			if json.Unmarshal(event["Output"], &detail) == nil {
				event["Output"], _ = json.Marshal(redact(detail))
			}
			encoded, _ := json.Marshal(event)
			output.Write(encoded)
		} else {
			output.WriteString(redact(line))
		}
		output.WriteByte('\n')
	}
	return output.String()
}

func GoEvidence(raw string, failed, required bool) Evidence {
	r := Evidence{Status: "PASS"}
	tests := map[string]*CaseResult{}
	started := map[string]bool{}
	s := bufio.NewScanner(strings.NewReader(raw))
	s.Buffer(make([]byte, 4096), 16*1024*1024)
	for s.Scan() {
		var e GoEvent
		if json.Unmarshal(s.Bytes(), &e) != nil {
			continue
		}
		if strings.Contains(e.Output, "(cached)") {
			r.Cached = true
		}
		if e.Test == "" {
			continue
		}
		key := e.Package + "/" + e.Test
		t := tests[key]
		if t == nil {
			t = &CaseResult{Package: e.Package, Name: e.Test, Status: "NOT_RUN"}
			tests[key] = t
		}
		switch e.Action {
		case "run":
			started[key] = true
		case "pass":
			t.Status = "PASS"
		case "fail":
			t.Status = "FAIL"
		case "skip":
			t.Status = "ENVIRONMENT_BLOCKED"
		case "output":
			t.Detail += e.Output
		}
	}
	for key, t := range tests {
		if started[key] {
			r.Counts.Started++
		}
		switch t.Status {
		case "PASS":
			r.Counts.Passed++
		case "FAIL":
			r.Counts.Failed++
		case "ENVIRONMENT_BLOCKED":
			r.Counts.Skipped++
			if !required && strings.Contains(t.Detail, "NOT_APPLICABLE:") {
				t.Status = "NOT_APPLICABLE"
			} else if r.Status == "PASS" {
				r.Status = "ENVIRONMENT_BLOCKED"
			}
		case "NOT_RUN":
			if started[key] {
				r.Status = "FAIL"
				r.Reason = "started test has no terminal verdict"
			}
		}
		if len(t.Detail) > 3000 {
			t.Detail = t.Detail[len(t.Detail)-3000:]
		}
		t.Detail = redact(t.Detail)
		r.Cases = append(r.Cases, *t)
	}
	if failed || s.Err() != nil || r.Counts.Failed > 0 {
		r.Status = "FAIL"
		r.Reason = "Go assertion, build or process failed"
	} else if r.Counts.Started == 0 {
		r.Status = "FAIL"
		r.Reason = "selected zero Go tests"
	} else if r.Counts.Passed == 0 && r.Counts.Skipped > 0 {
		r.Status = "ENVIRONMENT_BLOCKED"
	}
	return r
}

type xmlCase struct {
	Name    string  `xml:"name,attr"`
	Status  string  `xml:"status,attr"`
	Failure *string `xml:"failure"`
	Error   *string `xml:"error"`
	Skipped *string `xml:"skipped"`
}

func CTestEvidence(path string, failed bool) Evidence {
	r := Evidence{Status: "PASS"}
	f, e := os.Open(path)
	if e != nil {
		return Evidence{Status: "FAIL", Reason: "missing fresh CTest report"}
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	for {
		tok, e := d.Token()
		if e != nil {
			if e.Error() != "EOF" {
				r.Status = "FAIL"
				r.Reason = e.Error()
			}
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "testcase" {
			var t xmlCase
			if e = d.DecodeElement(&t, &se); e != nil {
				return Evidence{Status: "FAIL", Reason: e.Error()}
			}
			status := "PASS"
			if t.Skipped != nil || t.Status == "notrun" {
				status = "ENVIRONMENT_BLOCKED"
				r.Counts.Skipped++
			} else {
				r.Counts.Started++
				if t.Failure != nil || t.Error != nil {
					status = "FAIL"
					r.Counts.Failed++
				} else {
					r.Counts.Passed++
				}
			}
			r.Cases = append(r.Cases, CaseResult{Name: t.Name, Status: status})
		}
	}
	if failed || r.Counts.Failed > 0 || len(r.Cases) == 0 {
		r.Status = "FAIL"
	} else if r.Counts.Skipped > 0 {
		r.Status = "ENVIRONMENT_BLOCKED"
	}
	return r
}
func WebEvidence(raw string, failed bool) Evidence {
	r := Evidence{Status: "PASS"}
	if failed {
		return Evidence{Status: "FAIL", Reason: "Node/browser assertion or process failed"}
	}
	m := regexp.MustCompile(`web scenarios PASS \((\d+)/`).FindStringSubmatch(raw)
	if len(m) == 2 {
		fmt.Sscan(m[1], &r.Counts.Passed)
		r.Counts.Started = r.Counts.Passed
	} else {
		return Evidence{Status: "FAIL", Reason: "no actual scenario count in output"}
	}
	if r.Counts.Passed == 0 {
		r.Status = "FAIL"
	}
	return r
}

func NodeEvidence(raw string, failed bool) Evidence {
	r := Evidence{Status: "PASS"}
	for _, m := range regexp.MustCompile(`(?m)^# (tests|pass|fail|skipped) (\d+)`).FindAllStringSubmatch(raw, -1) {
		var n int
		fmt.Sscan(m[2], &n)
		switch m[1] {
		case "tests":
			r.Counts.Started = n
		case "pass":
			r.Counts.Passed = n
		case "fail":
			r.Counts.Failed = n
		case "skipped":
			r.Counts.Skipped = n
		}
	}
	r.Counts.Started -= r.Counts.Skipped
	if failed || r.Counts.Failed > 0 || r.Counts.Started == 0 {
		r.Status = "FAIL"
		r.Reason = "Node assertion/process failed or selected zero tests"
	} else if r.Counts.Skipped > 0 {
		r.Status = "ENVIRONMENT_BLOCKED"
	}
	return r
}
