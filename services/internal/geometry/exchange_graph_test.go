package geometry

import (
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/proto"
	"math"
	"testing"
)

func TestExchangeGraphDefinitionIdentityAndValidation(t *testing.T) {
	g := SinglePartExchangeGraph("Part", ArtifactReference{})
	g.Definitions = append(g.Definitions, &workerv1.ExchangeDefinition{Id: "assembly", Kind: "PRODUCT", Children: []*workerv1.ExchangeOccurrence{}})
	for i := 0; i < 100; i++ {
		c := proto.Clone(g.Roots[0]).(*workerv1.ExchangeOccurrence)
		c.Id = string(rune('a' + i))
		g.Definitions[1].Children = append(g.Definitions[1].Children, c)
	}
	g.Roots[0].DefinitionId = "assembly"
	if err := ValidateExchangeGraph(g); err != nil {
		t.Fatal(err)
	}
	cases := []func(*ExchangeGraph){
		func(g *ExchangeGraph) { g.Definitions[1].Children[0].DefinitionId = "missing" },
		func(g *ExchangeGraph) { g.Definitions[1].Children[0].DefinitionId = "assembly" },
		func(g *ExchangeGraph) { g.Definitions[1].Children[1].Id = g.Definitions[1].Children[0].Id },
		func(g *ExchangeGraph) { g.Roots[0].Rotation.W = 0 },
		func(g *ExchangeGraph) { g.Roots[0].Translation = &workerv1.Vec3{X: math.Inf(1)} },
		func(g *ExchangeGraph) {
			g.Definitions = append(g.Definitions, &workerv1.ExchangeDefinition{Id: "unused", Kind: "PART"})
		},
	}
	for _, change := range cases {
		invalid := proto.Clone(g).(*ExchangeGraph)
		change(invalid)
		if ValidateExchangeGraph(invalid) == nil {
			t.Fatal("invalid graph accepted")
		}
	}
}
