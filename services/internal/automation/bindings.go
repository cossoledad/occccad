package automation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

// TypedOutput is an application-constructed reference, never a provider supplied
// path into arbitrary JSON. Multiple outputs are selected by identity or role.
type TypedOutput struct {
	Role         string                         `json:"role"`
	Entity       EntityRef                      `json:"entity"`
	InstancePath *workspace.InstancePath        `json:"instancePath,omitempty"`
	AssemblyRef  *workspace.AssemblyGeometryRef `json:"assemblyRef,omitempty"`
}

type InputBinding struct {
	Target         string `json:"target"`
	SourceStepID   string `json:"sourceStepId"`
	OutputID       string `json:"outputId,omitempty"`
	OutputRole     string `json:"outputRole,omitempty"`
	Representation string `json:"representation"` // ENTITY_ID | INSTANCE_PATH | ASSEMBLY_REF
}

func (s *Service) decorateOutputs(ctx context.Context, actor, document string, result *ActionResult) error {
	entities := append([]EntityRef{}, result.Outputs...)
	if len(entities) == 0 && result.Output != nil {
		entities = append(entities, *result.Output)
	}
	var product workspace.ProductModel
	loaded := false
	for _, entity := range entities {
		role := entity.Kind
		if len(entities) == 1 {
			role = "PRIMARY"
		}
		output := TypedOutput{Role: role, Entity: entity}
		if entity.Kind == "INSTANCE" {
			if !loaded {
				snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, result.Receipt.RevisionID)
				if err != nil {
					return err
				}
				if err = json.Unmarshal(snapshot.Model, &product); err != nil {
					return err
				}
				loaded = true
			}
			for _, instance := range product.Instances {
				if instance.ID != entity.ID {
					continue
				}
				if err := s.authorize(ctx, actor, instance.ReferencedDocumentID, access.RoleViewer); err != nil {
					return err
				}
				revision := instance.ReferencedVersionID
				// The immutable Product records the revision actually evaluated for
				// this occurrence even in FOLLOW_HEAD mode. A later source Head
				// must not rewrite the meaning of an already committed output.
				if revision == "" {
					return fault("INSTANCE_SNAPSHOT_UNAVAILABLE", "created occurrence has no accepted dependency revision")
				}
				source, err := s.workspace.ReadModelSnapshot(ctx, instance.ReferencedDocumentID, revision)
				if err != nil {
					return err
				}
				path := workspace.InstancePath{RootDocumentID: document, Canonical: instance.ID, Display: instance.Name, Segments: []workspace.InstancePathSegment{{OwnerDocumentID: document, OwnerVersionID: result.Receipt.RevisionID, InstanceID: instance.ID, InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID, ResolvedVersionID: source.RevisionID}}}
				output.InstancePath = &path
				output.AssemblyRef = &workspace.AssemblyGeometryRef{InstanceID: instance.ID, InstancePath: &path, Kind: "BODY", SourceVersionID: source.RevisionID}
				break
			}
		}
		result.TypedOutputs = append(result.TypedOutputs, output)
	}
	return nil
}

// A finite list of writable typed slots deliberately excludes generic paths,
// array offsets and expansion of user expressions.
func bindingSlot(target string) (container, leaf, kind string) {
	if kind = bindingKind(target); kind != "" {
		return "", target, kind
	}
	switch target {
	case "firstAssemblyRef", "secondAssemblyRef", "angleAxis":
		return "", target, "ASSEMBLY_REF"
	case "firstAssemblyRef.instanceId", "secondAssemblyRef.instanceId", "angleAxis.instanceId":
		parts := strings.Split(target, ".")
		return parts[0], parts[1], "INSTANCE"
	case "firstAssemblyRef.instancePath", "secondAssemblyRef.instancePath", "angleAxis.instancePath":
		parts := strings.Split(target, ".")
		return parts[0], parts[1], "INSTANCE_PATH"
	case "instancePath":
		return "", target, "INSTANCE_PATH"
	}
	return "", "", ""
}

func validateInputBinding(op Operation, args map[string]any, binding InputBinding, known map[string]bool) error {
	container, leaf, kind := bindingSlot(binding.Target)
	if kind == "" || !known[binding.SourceStepID] || binding.OutputID != "" && binding.OutputRole != "" {
		return fault("INVALID_BINDING", "typed binding requires a declared slot, a preceding step, and one output selector")
	}
	fieldName := leaf
	if container != "" {
		fieldName = container
	}
	declared := false
	for _, field := range op.Fields {
		if field.Name == fieldName {
			declared = true
		}
	}
	if !declared {
		return fault("INVALID_BINDING", "operation has no declared input slot: "+binding.Target)
	}
	if container == "" {
		if _, literal := args[leaf]; literal {
			return fault("INVALID_BINDING", "typed input also has a literal")
		}
	} else if literal, ok := args[container]; ok {
		object, ok := literal.(map[string]any)
		if !ok {
			return fault("INVALID_BINDING", "nested typed slot requires an object")
		}
		if _, exists := object[leaf]; exists {
			return fault("INVALID_BINDING", "nested typed input also has a literal")
		}
	}
	switch kind {
	case "INSTANCE_PATH":
		if binding.Representation != "INSTANCE_PATH" {
			return fault("INVALID_BINDING", "InstancePath slot requires INSTANCE_PATH")
		}
	case "ASSEMBLY_REF":
		if binding.Representation != "ASSEMBLY_REF" {
			return fault("INVALID_BINDING", "AssemblyRef slot requires ASSEMBLY_REF")
		}
	default:
		if binding.Representation != "ENTITY_ID" {
			return fault("INVALID_BINDING", "entity input requires ENTITY_ID")
		}
	}
	return nil
}

func bindInputs(task *Task, step *Step, args map[string]any) error {
	bindings := append([]InputBinding{}, step.TypedBindings...)
	for field, source := range step.Bindings {
		bindings = append(bindings, InputBinding{Target: field, SourceStepID: source, Representation: "ENTITY_ID"})
	}
	for _, binding := range bindings {
		var outputs []TypedOutput
		for _, prior := range task.Steps {
			if prior.ID != binding.SourceStepID || prior.Result == nil {
				continue
			}
			outputs = prior.Result.TypedOutputs
			if len(outputs) == 0 && prior.Result.Output != nil {
				outputs = []TypedOutput{{Role: "PRIMARY", Entity: *prior.Result.Output}}
			}
		}
		matches := []TypedOutput{}
		for _, output := range outputs {
			if binding.OutputID != "" && output.Entity.ID != binding.OutputID || binding.OutputRole != "" && output.Role != binding.OutputRole {
				continue
			}
			matches = append(matches, output)
		}
		if len(matches) != 1 {
			return fault("INVALID_BINDING", "output is absent or ambiguous; select a stable output ID or unique role")
		}
		output := matches[0]
		container, leaf, kind := bindingSlot(binding.Target)
		var value any
		switch binding.Representation {
		case "ENTITY_ID":
			if kind != "ENTITY" && kind != output.Entity.Kind {
				return fault("INVALID_BINDING", "output entity kind does not match the declared input")
			}
			value = output.Entity.ID
		case "INSTANCE_PATH":
			if output.InstancePath == nil {
				return fault("INVALID_BINDING", "output has no resolved InstancePath")
			}
			value = output.InstancePath
		case "ASSEMBLY_REF":
			if output.AssemblyRef == nil {
				return fault("INVALID_BINDING", "output has no formal AssemblyRef")
			}
			value = output.AssemblyRef
		default:
			return fault("INVALID_BINDING", "unsupported typed representation")
		}
		if container == "" {
			args[leaf] = value
		} else {
			object, _ := args[container].(map[string]any)
			if object == nil {
				object = map[string]any{}
			}
			object[leaf] = value
			args[container] = object
		}
	}
	return nil
}
