package workspace

import (
	"errors"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"testing"
)

func TestImportGraphBudgetsCountSharedExpansion(t *testing.T) {
	graph := geometry.SinglePartExchangeGraph("Part", geometry.ArtifactReference{})
	target := "part"
	for depth := 0; depth < 14; depth++ {
		id := fmt.Sprintf("assembly-%d", depth)
		graph.Definitions = append(graph.Definitions, &workerv1.ExchangeDefinition{Id: id, Kind: "PRODUCT", Children: []*workerv1.ExchangeOccurrence{{Id: "a", DefinitionId: target, Rotation: &workerv1.Quaternion{W: 1}}, {Id: "b", DefinitionId: target, Rotation: &workerv1.Quaternion{W: 1}}}})
		target = id
	}
	graph.Roots[0].DefinitionId = target
	if err := ValidateImportGraph(graph); !errors.Is(err, ErrValidation) {
		t.Fatalf("exponential expansion accepted: %v", err)
	}
}
func TestExchangeNameRestoresSourceUntilUserRenames(t *testing.T) {
	instance := ProductInstance{Name: "Pin.1", ImportedName: &ImportedOccurrenceName{Source: "Pin", Assigned: "Pin.1"}}
	if exchangeInstanceName(instance) != "Pin" {
		t.Fatal("source name lost")
	}
	instance.Name = "User Pin"
	if exchangeInstanceName(instance) != "User Pin" {
		t.Fatal("user rename lost")
	}
}
