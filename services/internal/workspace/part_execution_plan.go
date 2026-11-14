package workspace

import (
	"context"
	"reflect"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

// The persisted DAG qualifies reuse before preparing geometry. This is an
// operation-local plan, not a second model or a cache of mutable document Heads.
type partExecutionPlan struct {
	graph          *modelcore.DependencyGraph
	dirty          map[modelcore.DependencyKey]bool
	before         map[string]Feature
	bodies         map[string]bool
	inputs         map[modelcore.DependencyKey][]modelcore.DependencyKey
	acceptedStages map[string]string
}
type partExecutionPlanKey struct{}

func preparePartExecution(ctx context.Context, before, model PartModel, prior *modelcore.EvaluationManifest, seeds []modelcore.DependencyKey) (context.Context, error) {
	if geometry.PartRuntime(ctx).ForceCold || prior == nil || prior.EvaluatorDigest != evaluatorVersion {
		return ctx, nil
	}
	graph, err := buildPartGraph(model)
	if err != nil {
		return ctx, err
	}
	changed, err := graph.ChangedInputs(prior, evaluatorVersion, "units-mm-v1", partNodeOutput)
	if err != nil {
		return ctx, err
	}
	changed = append(changed, seeds...)
	for _, f := range model.Features {
		if f.Sketch != nil {
			support := f.Sketch.Support
			if support.OrientationRule != sketchSupportOrientationRule || support.Type == "PLANAR_FACE" && (support.DependencySnapshot == nil || support.DependencySnapshot.PolicyDigest != modelcore.TopologyNamingPolicyDigest) {
				changed = append(changed, modelcore.DependencyKey("feature:"+f.ID))
			}
		}
	}
	plan := &partExecutionPlan{graph: graph, dirty: map[modelcore.DependencyKey]bool{}, before: map[string]Feature{}, bodies: map[string]bool{}, inputs: map[modelcore.DependencyKey][]modelcore.DependencyKey{}, acceptedStages: map[string]string{}}
	for _, edge := range graph.Edges {
		plan.inputs[edge.Target] = append(plan.inputs[edge.Target], edge.Source)
	}
	for _, key := range graph.DirtyClosure(changed) {
		plan.dirty[key] = true
	}
	for _, f := range before.Features {
		plan.before[f.ID] = f
		if f.Result != nil && f.Result.GeometryKey != "" {
			plan.acceptedStages[f.ID] = f.Result.GeometryKey
		}
	}
	tips := map[string]string{}
	for _, f := range before.Features {
		if isBodyFeature(f.Type) && !f.Suppressed {
			tips[f.BodyID] = f.ID
		}
	}
	for _, body := range before.Bodies {
		if tip := tips[body.ID]; tip != "" && body.GeometryKey != "" {
			plan.acceptedStages[tip] = body.GeometryKey
		}
	}
	for _, b := range model.Bodies {
		plan.bodies[b.ID] = plan.dirty[modelcore.DependencyKey("body:"+b.ID)] || b.GeometryKey == ""
	}
	for _, f := range model.Features {
		if isBodyFeature(f.Type) && plan.dirty[modelcore.DependencyKey("feature:"+f.ID)] {
			plan.bodies[f.BodyID] = true
		}
	}
	// A removed terminal node has no outgoing edge in the candidate DAG.
	// Compare Body execution membership as well as changed surviving nodes.
	oldOrder, newOrder := map[string][]string{}, map[string][]string{}
	for _, f := range before.Features {
		if isBodyFeature(f.Type) && !f.Suppressed {
			oldOrder[f.BodyID] = append(oldOrder[f.BodyID], f.ID)
		}
	}
	for _, f := range model.Features {
		if isBodyFeature(f.Type) && !f.Suppressed {
			newOrder[f.BodyID] = append(newOrder[f.BodyID], f.ID)
		}
	}
	for _, body := range model.Bodies {
		if !reflect.DeepEqual(oldOrder[body.ID], newOrder[body.ID]) {
			plan.bodies[body.ID] = true
		}
	}
	return context.WithValue(ctx, partExecutionPlanKey{}, plan), nil
}

func executionPlan(ctx context.Context) *partExecutionPlan {
	p, _ := ctx.Value(partExecutionPlanKey{}).(*partExecutionPlan)
	return p
}
func needsPartNode(ctx context.Context, key string) bool {
	p := executionPlan(ctx)
	return p == nil || p.dirty[modelcore.DependencyKey(key)]
}
func needsPartBody(ctx context.Context, id string) bool {
	p := executionPlan(ctx)
	return p == nil || p.bodies[id]
}

// Editing entities does not change an accepted support/projection snapshot.
// Reuse it only if its definition and every upstream DAG input are unchanged.
func reusableSketchInputs(ctx context.Context, f Feature) bool {
	p := executionPlan(ctx)
	if p == nil || f.Sketch == nil || f.Sketch.Support.Status != "CONNECTED" || f.Sketch.Support.OrientationRule != sketchSupportOrientationRule {
		return false
	}
	old, ok := p.before[f.ID]
	if !ok || old.Sketch == nil || !reflect.DeepEqual(old.Sketch.Support, f.Sketch.Support) || !reflect.DeepEqual(old.Sketch.ExternalGeometry, f.Sketch.ExternalGeometry) {
		return false
	}
	if f.Sketch.Support.Type == "PLANAR_FACE" && (f.Sketch.Support.DependencySnapshot == nil || f.Sketch.Support.DependencySnapshot.PolicyDigest != modelcore.TopologyNamingPolicyDigest) {
		return false
	}
	for _, external := range f.Sketch.ExternalGeometry {
		if external.Status != "CONNECTED" || external.Snapshot == nil {
			return false
		}
	}
	key := modelcore.DependencyKey("feature:" + f.ID)
	for _, source := range p.inputs[key] {
		if p.dirty[source] {
			return false
		}
	}
	for _, external := range f.Sketch.ExternalGeometry {
		for _, source := range p.inputs[modelcore.DependencyKey("projection:"+f.ID+"/"+external.ID)] {
			if p.dirty[source] {
				return false
			}
		}
	}
	return true
}
