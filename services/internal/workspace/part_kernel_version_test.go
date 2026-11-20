package workspace

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPartEvaluatorIncludesLockedKernelBuild(t *testing.T) {
	data, err := os.ReadFile("../../../conan.lock")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct{ Requires []string }
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	for _, ref := range lock.Requires {
		if strings.HasPrefix(ref, "opencascade/") {
			recipe, _, _ := strings.Cut(ref, "%")
			_, revision, ok := strings.Cut(recipe, "#")
			if !ok || revision == "" || !strings.HasSuffix(evaluatorVersion, "-occt-"+revision) {
				t.Fatalf("Part evaluator %q must identify locked kernel %s", evaluatorVersion, recipe)
			}
			return
		}
	}
	t.Fatal("missing locked OCCT package")
}
