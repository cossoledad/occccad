package workspace

import (
	"fmt"
	"sort"

	"github.com/occccad/occccad/internal/modelcore"
)

// References carry typed identities for navigation; display names never bind inputs.
// A blocked removal has no cascade, formula rewrite or partial ChangeSet.
type DependencyObject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}
type DependencyBlock struct {
	Subject   DependencyObject   `json:"subject"`
	Consumers []DependencyObject `json:"consumers"`
	Total     int                `json:"total"`
}
type DependencyBlockedError struct{ Block DependencyBlock }

func (e *DependencyBlockedError) Error() string {
	return fmt.Sprintf("DEPENDENCY_IN_USE: %s has %d dependent objects", e.Block.Subject.Name, e.Block.Total)
}
func (e *DependencyBlockedError) Unwrap() error                        { return ErrValidation }
func (e *DependencyBlockedError) Code() string                         { return "DEPENDENCY_IN_USE" }
func (e *DependencyBlockedError) Retryable() bool                      { return false }
func (e *DependencyBlockedError) DependencyConflict() *DependencyBlock { return &e.Block }
func dependencyBlocked(subject DependencyObject, consumers []DependencyObject) error {
	unique := map[string]DependencyObject{}
	for _, object := range consumers {
		if object.Name == "" {
			object.Name = "引用对象"
		}
		unique[object.Kind+":"+object.ID] = object
	}
	values := make([]DependencyObject, 0, len(unique))
	for _, object := range unique {
		values = append(values, object)
	}
	sort.Slice(values, func(i, j int) bool { a, b := values[i], values[j]; return a.Kind+":"+a.ID < b.Kind+":"+b.ID })
	if len(values) == 0 {
		return nil
	}
	if subject.Name == "" {
		subject.Name = "所选对象"
	}
	total := len(values)
	if total > 32 {
		values = values[:32]
	}
	return &DependencyBlockedError{Block: DependencyBlock{Subject: subject, Consumers: values, Total: total}}
}

func (d *parameterDocument) parameterDependencyObject(p modelcore.ParameterDefinition) DependencyObject {
	if d.product != nil {
		for _, c := range d.product.Constraints {
			if c.ID == p.OwnerFeatureID {
				name := c.Name
				if name == "" {
					name = p.Label
				}
				return DependencyObject{Kind: "ASSEMBLY_CONSTRAINT", ID: c.ID, Name: name}
			}
		}
	}
	if d.part != nil && p.OwnerFeatureID != "" {
		for _, f := range d.part.Features {
			if f.ID == p.OwnerFeatureID {
				return DependencyObject{Kind: "FEATURE", ID: f.ID, Name: f.Name}
			}
		}
		for _, x := range d.part.DatumPoints {
			if x.ID == p.OwnerFeatureID {
				return DependencyObject{Kind: "DATUM_POINT", ID: x.ID, Name: x.Name}
			}
		}
		for _, x := range d.part.DatumPlanes {
			if x.ID == p.OwnerFeatureID {
				return DependencyObject{Kind: "PLANE", ID: x.ID, Name: x.Name}
			}
		}
		for _, x := range d.part.DatumAxes {
			if x.ID == p.OwnerFeatureID {
				return DependencyObject{Kind: "DATUM_AXIS", ID: x.ID, Name: x.Name}
			}
		}
	}
	name := p.Key
	if name == "" {
		name = p.Label
	}
	return DependencyObject{Kind: "PARAMETER", ID: p.ParameterID, Name: name}
}
func (d *parameterDocument) parameterConsumers(ids map[modelcore.DependencyKey]bool) []DependencyObject {
	var consumers []DependencyObject
	for _, p := range d.Parameters {
		if ids[modelcore.DependencyKey("parameter:"+p.ParameterID)] || p.Source.Expression == nil {
			continue
		}
		for _, read := range p.Source.Expression.Reads {
			if ids[read] {
				consumers = append(consumers, d.parameterDependencyObject(p))
				break
			}
		}
	}
	for _, p := range d.Publications {
		if ids[modelcore.DependencyKey("parameter:"+p.Target.ParameterID)] {
			kind := "PUBLICATION"
			if d.product != nil {
				kind = "PRODUCT_PUBLICATION"
			}
			consumers = append(consumers, DependencyObject{Kind: kind, ID: p.ID, Name: p.Name})
		}
	}
	for _, input := range d.ContextInputs {
		if input.Target.Kind == "PARAMETER" && ids[modelcore.DependencyKey("parameter:"+input.Target.TargetID)] {
			consumers = append(consumers, DependencyObject{Kind: "CONTEXT_INPUT", ID: input.ID, Name: input.Name})
		}
	}
	return consumers
}
func rejectRemovedMeasurementConsumers(d *parameterDocument, before, after []Measurement) error {
	remaining := map[string]bool{}
	for _, m := range after {
		remaining[m.ID] = true
	}
	for _, m := range before {
		if remaining[m.ID] {
			continue
		}
		ids := map[modelcore.DependencyKey]bool{}
		for _, p := range d.Parameters {
			if source := p.Source.Produced; source != nil && source.Kind == "MEASUREMENT" && source.EntityID == m.ID {
				ids[modelcore.DependencyKey("parameter:"+p.ParameterID)] = true
			}
		}
		if err := dependencyBlocked(DependencyObject{Kind: "MEASUREMENT", ID: m.ID, Name: m.Name}, d.parameterConsumers(ids)); err != nil {
			return err
		}
	}
	return nil
}
