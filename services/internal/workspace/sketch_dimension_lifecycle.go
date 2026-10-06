package workspace

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

func replaceSketchConstraint(sketch *SketchFeature, operation SketchOperation) error {
	if operation.Constraint == nil || operation.ConstraintID == "" || operation.Constraint.ID != operation.ConstraintID {
		return fmt.Errorf("%w: constraint update must retain its stable identity", ErrValidation)
	}
	for i, prior := range sketch.Constraints {
		if prior.ID != operation.ConstraintID {
			continue
		}
		oldDimension, newDimension := isDimensionalConstraint(prior.Kind), isDimensionalConstraint(operation.Constraint.Kind)
		if oldDimension != newDimension {
			return fmt.Errorf("%w: editing a relation cannot implicitly convert between dimensions and logic constraints", ErrValidation)
		}
		if operation.Constraint.Internal != prior.Internal {
			return fmt.Errorf("%w: generated relation ownership cannot change through definition editing", ErrValidation)
		}
		if operation.Constraint.ParameterID != "" && operation.Constraint.ParameterID != prior.ParameterID {
			return fmt.Errorf("%w: dimension must retain ParameterId", ErrValidation)
		}
		replacement := *operation.Constraint
		replacement.ParameterID = prior.ParameterID
		replacement.Internal = prior.Internal
		replacement.SelfMirrorMode = prior.SelfMirrorMode
		if !oldDimension {
			if (replacement.Kind == "MIRROR" || replacement.Kind == "SAME_SUPPORT") && replacement.Kind != prior.Kind {
				return fmt.Errorf("%w: generated relation must be created by its formal geometry operation", ErrValidation)
			}
			counts := map[string]int{"COINCIDENT": 2, "HORIZONTAL": 1, "VERTICAL": 1, "PARALLEL": 2, "PERPENDICULAR": 2, "COLLINEAR": 2, "FIXED": 1, "FIXED_POINT": 1, "TANGENT": 2, "EQUAL": 2, "CONCENTRIC": 2, "POINT_ON_OBJECT": 2, "MIDPOINT": 2, "SYMMETRY": 3, "MIRROR": 3, "SAME_SUPPORT": 2}
			expected, known := counts[replacement.Kind]
			if !known || len(replacement.References) != expected {
				return fmt.Errorf("%w: logical constraint kind/reference count is invalid even when disabled", ErrValidation)
			}
			kinds := map[string]string{}
			for _, entity := range sketch.Entities {
				kinds[entity.ID] = entity.Kind
			}
			for _, external := range sketch.ExternalGeometry {
				kind := external.GeometryKind
				if external.Snapshot != nil {
					kind = external.Snapshot.Kind
				}
				kinds[external.ID] = kind
			}
			for _, ref := range replacement.References {
				if (ref.Target == "ENTITY" || ref.Target == "EXTERNAL") && (ref.EntityID == "" || kinds[ref.EntityID] == "") {
					return fmt.Errorf("%w: logical reference does not resolve to known sketch geometry", ErrValidation)
				}
				if ref.Target != "ENTITY" && ref.Target != "EXTERNAL" && ref.Target != "SKETCH_ORIGIN" && ref.Target != "SKETCH_X_AXIS" && ref.Target != "SKETCH_Y_AXIS" {
					return fmt.Errorf("%w: invalid logical geometry target", ErrValidation)
				}
			}
			normalized := *sketch
			normalized.Entities = append([]SketchEntity(nil), sketch.Entities...)
			normalized.Constraints = []SketchConstraint{replacement}
			if err := normalizeSketchPointIdentities(&normalized); err != nil {
				return err
			}
			replacement = normalized.Constraints[0]
			if !constraintReferencesCompatible(replacement, kinds) {
				return fmt.Errorf("%w: logical constraint has incompatible references even when disabled", ErrValidation)
			}
			if operation.ParameterSource != nil || operation.ParameterKey != "" || operation.RestoreMode != "" || replacement.Reference || replacement.Value != nil || replacement.Unit != "" {
				return fmt.Errorf("%w: logical relation cannot carry dimension parameter edits", ErrValidation)
			}
			if prior.Internal {
				if replacement.Kind != prior.Kind {
					return fmt.Errorf("%w: generated relation kind belongs to its source operation", ErrValidation)
				}
				if !reflect.DeepEqual(replacement.References, prior.References) {
					if prior.Kind != "MIRROR" || len(prior.References) != 3 || len(replacement.References) != 3 || !reflect.DeepEqual(prior.References[0], replacement.References[0]) || !reflect.DeepEqual(prior.References[2], replacement.References[2]) {
						return fmt.Errorf("%w: generated relation references require the source operation's explicit impact policy", ErrValidation)
					}
				}
			}
			if replacement.Kind != "FIXED_POINT" {
				replacement.FixedPoint = nil
			}
		}
		sketch.Constraints[i] = replacement
		return nil
	}
	return fmt.Errorf("%w: selected constraint does not exist", ErrValidation)
}

func applySketchDimensionLifecycle(model *PartModel, sketchID string, before SketchFeature, operations []SketchOperation) error {
	priorConstraints := map[string]SketchConstraint{}
	for _, constraint := range before.Constraints {
		priorConstraints[constraint.ID] = constraint
	}
	var sketch *SketchFeature
	for i := range model.Features {
		if model.Features[i].ID == sketchID {
			sketch = model.Features[i].Sketch
		}
	}
	if sketch == nil {
		return fmt.Errorf("%w: sketch missing", ErrValidation)
	}
	for _, operation := range operations {
		if operation.Type == "FILLET_ENTITIES" || operation.Type == "CHAMFER_ENTITIES" {
			label := "radius"
			if operation.Type == "CHAMFER_ENTITIES" {
				label = "first-length"
			}
			operation.ConstraintID = macroID(operation.OperationID, "corner/"+label)
			operation.Type = "ADD_CONSTRAINT"
		}
		if operation.Type != "ADD_CONSTRAINT" && operation.Type != "UPDATE_CONSTRAINT" && operation.Type != "UPDATE_CONSTRAINT_VALUE" {
			continue
		}
		id := operation.ConstraintID
		if operation.Constraint != nil {
			id = operation.Constraint.ID
		}
		index := -1
		for i := range sketch.Constraints {
			if sketch.Constraints[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			continue
		}
		constraint := &sketch.Constraints[index]
		if !isDimensionalConstraint(constraint.Kind) {
			continue
		}
		parameterIndex := -1
		for i := range model.Parameters {
			if model.Parameters[i].ParameterID == constraint.ParameterID {
				parameterIndex = i
				break
			}
		}
		if parameterIndex < 0 {
			return fmt.Errorf("%w: dimension parameter missing", ErrValidation)
		}
		parameter := &model.Parameters[parameterIndex]
		prior, exists := priorConstraints[id]
		if parameter.Source.External != nil && (operation.ParameterSource != nil || operation.Value != nil || operation.RestoreMode == "MEASUREMENT") {
			return fmt.Errorf("%w: dimension uses a read-only external parameter source", ErrValidation)
		}
		if constraint.Reference && operation.Type == "UPDATE_CONSTRAINT_VALUE" {
			return fmt.Errorf("%w: reference dimension is measured; its value cannot be edited", ErrValidation)
		}
		if exists && prior.Reference && !constraint.Reference {
			switch operation.RestoreMode {
			case "ORIGINAL":
				if parameter.Source.Literal == nil && parameter.Source.Expression == nil && parameter.Source.External == nil {
					return fmt.Errorf("%w: reference dimension has no original driving definition; explicitly use its measurement", ErrValidation)
				}
			case "MEASUREMENT":
				if prior.Value == nil || !validSketchDimensionValue(constraint.Kind, *prior.Value) {
					return fmt.Errorf("%w: reference dimension has no valid measurement for driving", ErrValidation)
				}
				quantity, err := modelcore.NewQuantity(*prior.Value, constraint.Unit)
				if err != nil {
					return err
				}
				parameter.Source = modelcore.ValueSource{Literal: &quantity}
			default:
				return fmt.Errorf("%w: switching to driving requires ORIGINAL or MEASUREMENT policy", ErrValidation)
			}
		}
		if constraint.Reference && operation.ParameterSource != nil {
			return fmt.Errorf("%w: reference mode preserves its original driving source; switch to driving to edit it", ErrValidation)
		}
		if operation.ParameterSource != nil {
			source, err := compileSketchDimensionSource(*model, *operation.ParameterSource, parameter.Dimension)
			if err != nil {
				return err
			}
			parameter.Source = source
		} else if operation.Type == "UPDATE_CONSTRAINT_VALUE" && operation.Value != nil {
			if parameter.Source.Expression != nil {
				return fmt.Errorf("%w: dimension is formula driven; edit its expression explicitly", ErrValidation)
			}
			quantity, err := modelcore.NewQuantity(*operation.Value, parameter.DisplayUnit)
			if err != nil {
				return err
			}
			parameter.Source = modelcore.ValueSource{Literal: &quantity}
		} else if operation.Type == "UPDATE_CONSTRAINT" && !constraint.Reference && exists && !prior.Reference && constraint.Value != nil && prior.Value != nil && *constraint.Value != *prior.Value {
			return fmt.Errorf("%w: changing a driving dimension requires an explicit parameter source", ErrValidation)
		}
		if operation.ParameterKey != "" {
			key := strings.TrimSpace(operation.ParameterKey)
			if !validParameterKey(key) {
				return fmt.Errorf("%w: dimension name must be an ASCII identifier", ErrValidation)
			}
			for _, other := range model.Parameters {
				if other.ParameterID != parameter.ParameterID && other.Key == key {
					return fmt.Errorf("%w: parameter name already exists", ErrValidation)
				}
			}
			parameter.Key = key
		}
	}
	keys := map[string]string{}
	for _, parameter := range model.Parameters {
		keys[parameter.ParameterID] = parameter.Key
	}
	for i := range model.Parameters {
		if expression := model.Parameters[i].Source.Expression; expression != nil {
			formatted, err := modelcore.FormatExpression(*expression, keys)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrValidation, err)
			}
			expression.SourceText = formatted
		}
	}
	return validateSketchReferenceParameterDependencies(*model)
}

func compileSketchDimensionSource(model PartModel, text string, dimension modelcore.Dimension) (modelcore.ValueSource, error) {
	names := map[string]modelcore.ParameterBinding{}
	for _, parameter := range model.Parameters {
		names[parameter.Key] = modelcore.ParameterBinding{ParameterID: parameter.ParameterID, Dimension: parameter.Dimension}
	}
	source, err := resolveQualifiedParameterSource(model, strings.TrimSpace(text))
	if err != nil {
		return modelcore.ValueSource{}, err
	}
	expression, err := modelcore.CompileExpression(source, names, dimension, defaultParameterUnit(dimension))
	if err != nil {
		return modelcore.ValueSource{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if expression.CheckedAST.Kind == "LITERAL" && expression.CheckedAST.Quantity != nil {
		return modelcore.ValueSource{Literal: expression.CheckedAST.Quantity}, nil
	}
	return modelcore.ValueSource{Expression: &expression}, nil
}

func validateSketchReferenceParameterDependencies(model PartModel) error {
	measured := map[string]bool{}
	for _, feature := range model.Features {
		if feature.Sketch != nil {
			for _, constraint := range feature.Sketch.Constraints {
				if constraint.Reference {
					measured[constraint.ParameterID] = true
				}
			}
		}
	}
	for _, parameter := range model.Parameters {
		if expression := parameter.Source.Expression; expression != nil {
			for _, read := range expression.Reads {
				if measured[strings.TrimPrefix(string(read), "parameter:")] {
					return fmt.Errorf("%w: expressions cannot depend on reference dimensions in this evaluator phase", ErrValidation)
				}
			}
		}
	}
	return nil
}

func refreshSketchReferenceMeasurements(model *PartModel) {
	for featureIndex := range model.Features {
		sketch := model.Features[featureIndex].Sketch
		if sketch == nil {
			continue
		}
		entities := map[string]SketchEntity{}
		for _, entity := range sketch.Entities {
			if !entity.Suppressed {
				entities[entity.ID] = entity
			}
		}
		for _, external := range sketch.ExternalGeometry {
			if external.Status == "CONNECTED" && external.Snapshot != nil {
				s := external.Snapshot
				entities[external.ID] = SketchEntity{ID: external.ID, Kind: s.Kind, Point: s.Point, Start: s.Start, End: s.End, Center: s.Center, Radius: s.Radius}
			}
		}
		for i := range sketch.Constraints {
			constraint := &sketch.Constraints[i]
			if !constraint.Reference {
				continue
			}
			constraint.Value = nil
			value, ok := measureSketchConstraint(*constraint, entities)
			if constraint.Suppressed {
				ok = false
			}
			for j := range model.Parameters {
				parameter := &model.Parameters[j]
				if parameter.ParameterID != constraint.ParameterID {
					continue
				}
				parameter.EvaluatedValue = nil
				if ok {
					quantity, err := modelcore.NewQuantity(value, constraint.Unit)
					if err == nil {
						constraint.Value = &value
						parameter.EvaluatedValue = &quantity
					}
				}
			}
		}
	}
}

func measureSketchConstraint(constraint SketchConstraint, entities map[string]SketchEntity) (float64, bool) {
	refs := constraint.References
	point := func(ref SketchGeometryRef) (SketchPoint2, bool) {
		if ref.PointID != "" {
			entity, exists := entities[ref.EntityID]
			expected := ""
			if ref.SubElement == "START" {
				expected = entity.StartPointID
			} else if ref.SubElement == "END" {
				expected = entity.EndPointID
			}
			if !exists || ref.PointID != expected {
				return SketchPoint2{}, false
			}
		}
		if ref.SubElement == "CONTROL" {
			e, ok := entities[ref.EntityID]
			if !ok {
				return SketchPoint2{}, false
			}
			points := e.ControlPoints
			ids := e.ControlPointIDs
			if e.Mode == "CONTROL" {
				points = e.Poles
				ids = e.PoleIDs
			}
			index := -1
			if ref.ControlPointID != "" {
				for i, id := range ids {
					if id == ref.ControlPointID {
						index = i
						break
					}
				}
			} else if ref.ControlPointIndex != nil {
				index = *ref.ControlPointIndex
			}
			if index >= 0 && index < len(points) {
				return points[index], true
			}
			return SketchPoint2{}, false
		}
		return constraintReferencePoint(ref, entities)
	}
	line := func(ref SketchGeometryRef) (SketchPoint2, SketchPoint2, bool) {
		if ref.Target == "ENTITY" || ref.Target == "EXTERNAL" {
			if ref.SubElement != "WHOLE" && ref.SubElement != "DIRECTION" {
				return SketchPoint2{}, SketchPoint2{}, false
			}
		}
		return constraintLine(ref, entities)
	}
	if len(refs) == 0 {
		return 0, false
	}
	if constraint.Kind == "HORIZONTAL_DISTANCE" || constraint.Kind == "VERTICAL_DISTANCE" {
		if len(refs) != 2 {
			return 0, false
		}
		a, ok := point(refs[0])
		b, other := point(refs[1])
		if !ok || !other {
			return 0, false
		}
		if constraint.Kind == "HORIZONTAL_DISTANCE" {
			return b.X - a.X, true
		}
		return b.Y - a.Y, true
	}
	if constraint.Kind == "DISTANCE" && len(refs) == 2 {
		a1, a2, aLine := line(refs[0])
		b1, b2, bLine := line(refs[1])
		if aLine && bLine {
			a, b := sub2(a2, a1), sub2(b2, b1)
			la, lb := math.Hypot(a.X, a.Y), math.Hypot(b.X, b.Y)
			if la == 0 || lb == 0 || math.Abs(a.X*b.Y-a.Y*b.X) > 1e-10*la*lb {
				return 0, false
			}
			delta := sub2(b1, a1)
			return math.Abs(a.X*delta.Y-a.Y*delta.X) / la, true
		}
		if aLine || bLine {
			var p SketchPoint2
			var ok bool
			if aLine {
				p, ok = point(refs[1])
			} else {
				p, ok = point(refs[0])
				a1, a2 = b1, b2
			}
			d := sub2(a2, a1)
			length := math.Hypot(d.X, d.Y)
			if !ok || length == 0 {
				return 0, false
			}
			delta := sub2(p, a1)
			return math.Abs(d.X*delta.Y-d.Y*delta.X) / length, true
		}
		a, ok := point(refs[0])
		b, other := point(refs[1])
		if ok && other {
			return math.Hypot(b.X-a.X, b.Y-a.Y), true
		}
	}
	if constraint.Kind == "LENGTH" {
		a, b, ok := line(refs[0])
		if ok {
			return math.Hypot(b.X-a.X, b.Y-a.Y), true
		}
	}
	entity, exists := entities[refs[0].EntityID]
	if exists && (constraint.Kind == "RADIUS" || constraint.Kind == "DIAMETER") && entity.Radius > 0 {
		factor := 1.0
		if constraint.Kind == "DIAMETER" {
			factor = 2
		}
		return entity.Radius * factor, true
	}
	if exists && (constraint.Kind == "MAJOR_RADIUS" || constraint.Kind == "MINOR_RADIUS") {
		value := entity.MajorRadius
		if constraint.Kind == "MINOR_RADIUS" {
			value = entity.MinorRadius
		}
		return value, value > 0
	}
	if constraint.Kind == "ANGLE" && len(refs) == 2 {
		a1, a2, ok := line(refs[0])
		b1, b2, other := line(refs[1])
		a, b := sub2(a2, a1), sub2(b2, b1)
		length := math.Hypot(a.X, a.Y) * math.Hypot(b.X, b.Y)
		if ok && other && length > 0 {
			return math.Acos(math.Max(-1, math.Min(1, (a.X*b.X+a.Y*b.Y)/length))) * 180 / math.Pi, true
		}
	}
	return 0, false
}

func appendSketchParameterDefinitionChanges(changes []modelcore.ModelChange, seeds []modelcore.DependencyKey, before, after []modelcore.ParameterDefinition) ([]modelcore.ModelChange, []modelcore.DependencyKey, map[string]bool) {
	priorByID := map[string]modelcore.ParameterDefinition{}
	changed := map[string]bool{}
	for _, parameter := range before {
		priorByID[parameter.ParameterID] = parameter
	}
	for _, parameter := range after {
		prior, exists := priorByID[parameter.ParameterID]
		if !exists {
			continue
		}
		if prior.Key == parameter.Key && prior.Role == parameter.Role && prior.DisplayUnit == parameter.DisplayUnit && prior.Dimension.Equal(parameter.Dimension) {
			continue
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.entity"}, prior, parameter)
		changes = append(changes, change)
		seeds = append(seeds, "parameter:"+modelcore.DependencyKey(parameter.ParameterID))
		changed[parameter.ParameterID] = true
	}
	return changes, seeds, changed
}

// Deleting a driving/reference dimension removes its managed parameter during
// ensureFeatureParameters. Dependency validation then rejects any dangling AST
// read in the same staged command. Supporting design relations remain explicit.
func deleteSketchConstraint(sketch *SketchFeature, op SketchOperation) error {
	if op.ConstraintID == "" {
		return fmt.Errorf("%w: select a constraint by stable identity", ErrValidation)
	}
	for i, c := range sketch.Constraints {
		if c.ID != op.ConstraintID {
			continue
		}
		if c.Internal && c.Kind != "MIRROR" {
			explicit := false
			for _, id := range op.DetachConstraintIDs {
				if id != c.ID {
					return fmt.Errorf("%w: constraint deletion may only release the selected generated relation", ErrValidation)
				}
				if explicit {
					return fmt.Errorf("%w: duplicate generated relation release", ErrValidation)
				}
				explicit = true
			}
			if !explicit {
				return fmt.Errorf("%w: this relation belongs to a generated operation; explicitly release it through that operation's impact options", ErrValidation)
			}
		} else if len(op.DetachConstraintIDs) > 0 {
			return fmt.Errorf("%w: selected relation does not require generated-operation release", ErrValidation)
		}
		sketch.Constraints = append(sketch.Constraints[:i], sketch.Constraints[i+1:]...)
		return nil
	}
	return fmt.Errorf("%w: selected constraint does not exist", ErrValidation)
}
