package devtool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) Docs(o Options) error {
	args := []string{"node", filepath.Join(a.Root, "tools/documentation-audit.mjs")}
	if o.Verbose {
		args = append(args, "--verbose")
	}
	if o.Base != "" {
		args = append(args, "--baseline", o.Base)
	}
	if o.JSON {
		args = append(args, "--json")
	}
	out, e := a.capture(a.Root, nil, args...)
	fmt.Fprint(a.Out, out)
	return e
}

type Recipe struct {
	Goal                        string      `json:"goal"`
	Sources                     [][2]string `json:"sources"`
	Documents, Obligations      []string
	Focused, Complete, Escalate string
}

func (a *App) TaskContext(o Options) error {
	start := time.Now()
	var recipes map[string]Recipe
	if e := loadJSON(filepath.Join(a.Root, "dev/recipes.json"), &recipes); e != nil {
		return e
	}
	r, ok := recipes[o.Recipe]
	if !ok {
		return fmt.Errorf("unknown recipe; choose property, rpc, selection, feature, diagnostic")
	}
	sources := []any{}
	snippets := []any{}
	returned := 0
	for _, s := range r.Sources {
		b, e := os.ReadFile(filepath.Join(a.Root, s[0]))
		if e != nil {
			return e
		}
		query := s[1]
		if o.Symbol != "" {
			query = o.Symbol
		}
		lines := strings.Split(string(b), "\n")
		hits := []int{}
		for i, l := range lines {
			if strings.Contains(l, query) {
				hits = append(hits, i+1)
				if len(hits) == 3 {
					break
				}
			}
		}
		sources = append(sources, map[string]any{"path": s[0], "entry": s[1], "query": query, "lines": hits})
		if o.Read && len(hits) > 0 && len(snippets) < 2 {
			first, last := max(0, hits[0]-9), min(len(lines), hits[0]+16)
			excerpt := ""
			for i := first; i < last; i++ {
				excerpt += fmt.Sprintf("%d: %s\n", i+1, lines[i])
			}
			truncated := len(excerpt) > 5000
			if truncated {
				excerpt = strings.ToValidUTF8(excerpt[:5000], "")
			}
			returned += len(excerpt)
			snippets = append(snippets, map[string]any{"path": s[0], "text": excerpt, "truncated": truncated})
		}
	}
	v := map[string]any{"schema": "occccad.task-context.v1", "recipe": o.Recipe, "goal": r.Goal, "sources": sources, "documents": r.Documents, "obligations": r.Obligations, "validation": map[string]string{"focused": r.Focused, "complete": r.Complete, "escalate": r.Escalate}, "snippets": snippets, "metrics": map[string]any{"searchCalls": len(r.Sources), "returnedSourceBytes": returned, "discoverySeconds": time.Since(start).Seconds(), "modelTokens": nil, "boundary": "packet only; not model token measurements"}}
	destination := filepath.Join(a.Root, "build/agent-workflow", time.Now().UTC().Format("20060102T150405.000000000")+".json")
	v["artifact"] = destination
	if e := writeJSON(destination, v); e != nil {
		return e
	}
	return a.emit(v)
}
