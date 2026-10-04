package workspace

import (
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestPatternParameterUpdateOrderSharesDefinitionsAndKeepsPinned(t *testing.T) {
	ref := func(id, revision, mode string) modelcore.ParameterDefinition {
		return modelcore.ParameterDefinition{ParameterID: id, Source: modelcore.ValueSource{External: &modelcore.ExternalParameterRef{SourceDocumentID: id, ResolvedRevisionID: revision, Revision: modelcore.ReferenceSelector{Mode: mode}}}}
	}
	models := map[string]PartModel{
		"design": {},
		"fan":    {Parameters: []modelcore.ParameterDefinition{ref("design", "design-v1", "FOLLOW_WORKSPACE_WITH_ACCEPT")}},
		"cover":  {Parameters: []modelcore.ParameterDefinition{ref("fan", "fan-v1", "FOLLOW_WORKSPACE_WITH_ACCEPT"), ref("frozen", "frozen-v1", "PINNED")}},
	}
	reads := map[string]int{}
	read := func(id string) (string, PartModel, error) {
		reads[id]++
		if id == "design" {
			return "design-v2", models[id], nil
		}
		return id + "-v1", models[id], nil
	}
	order, err := buildParameterUpdateOrder([]string{"cover", "fan", "fan"}, read)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, node := range order {
		ids = append(ids, node.DocumentID)
	}
	if !reflect.DeepEqual(ids, []string{"design", "fan", "cover"}) || reads["fan"] != 1 || reads["frozen"] != 0 {
		t.Fatal("incorrect dependency/occurrence ownership", ids, reads)
	}
	if order[0].NeedsUpdate || !order[1].NeedsUpdate || !order[2].NeedsUpdate {
		t.Fatal("transitive pending state missing", order)
	}
	models["design"] = PartModel{Parameters: []modelcore.ParameterDefinition{ref("cover", "cover-v1", "FOLLOW_WORKSPACE_WITH_ACCEPT")}}
	if _, err = buildParameterUpdateOrder([]string{"fan"}, read); err == nil || !strings.Contains(err.Error(), "CYCLE") {
		t.Fatal("cycle accepted", err)
	}
}
