package workspace

import (
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

func parameterPresentation(model PartModel, parameter modelcore.ParameterDefinition) modelcore.ParameterDefinition {
	if parameter.OwnerFeatureID == "" {
		parameter.DisplayName = parameter.Label
		if parameter.DisplayName == "" {
			parameter.DisplayName = parameter.Key
		}
		parameter.QualifiedDisplayPath = parameter.DisplayName
		parameter.DisplayAlias = parameter.Key
		return parameter
	}
	var feature *Feature
	for i := range model.Features {
		if model.Features[i].ID == parameter.OwnerFeatureID {
			feature = &model.Features[i]
			break
		}
	}
	if feature == nil {
		parameter.DisplayName, parameter.QualifiedDisplayPath = parameter.Label, parameter.Label
		return parameter
	}
	featureName := feature.Name
	if featureName == "" {
		featureName = strings.Title(strings.ToLower(feature.Type))
	}
	bodyName := "几何体"
	for _, body := range model.Bodies {
		if body.ID == feature.BodyID {
			bodyName = body.Name
			break
		}
	}
	group, name := "", parameter.Label
	switch parameter.PropertySlot {
	case "length":
		group, name = "第一限制", "长度"
	default:
		if parameter.Lifecycle == "SKETCH_DIMENSION" {
			group = "尺寸约束"
			switch strings.ToUpper(parameter.Label) {
			case "DISTANCE":
				name = "距离"
			case "LENGTH":
				name = "长度"
			case "RADIUS":
				name = "半径"
			case "DIAMETER":
				name = "直径"
			case "ANGLE":
				name = "角度"
			}
		}
	}
	if name == "" {
		name = "参数"
	}
	parameter.DisplayName = name
	parts := []string{bodyName, featureName}
	if group != "" {
		parts = append(parts, group)
	}
	parameter.QualifiedDisplayPath = strings.Join(append(parts, name), "\\")
	if parameter.Key != "" && !strings.HasPrefix(parameter.Key, strings.NewReplacer("-", "_", ":", "_").Replace(parameter.OwnerFeatureID)+"_") {
		parameter.DisplayAlias = parameter.Key
	}
	return parameter
}

func resolveQualifiedParameterSource(model PartModel, source string) (string, error) {
	wanted := strings.TrimSpace(source)
	var found string
	for _, parameter := range model.Parameters {
		if parameterPresentation(model, parameter).QualifiedDisplayPath != wanted {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("%w: parameter display path is ambiguous", ErrValidation)
		}
		found = parameter.Key
	}
	if found != "" {
		return found, nil
	}
	return source, nil
}

func presentParameters(model *PartModel) {
	for i := range model.Parameters {
		model.Parameters[i] = parameterPresentation(*model, model.Parameters[i])
	}
}
