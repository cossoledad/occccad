package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// parameterDocument owns the common parameter domain. The document adapters
// retain their own geometry, assembly and publication models.
type parameterDocument struct {
	Parameters    []modelcore.ParameterDefinition
	Publications  []Publication
	ContextInputs []ContextInput
	part          *PartModel
	product       *ProductModel
}

// Constraint quantities keep their owning constraint's single persistent slot,
// while every parameter consumer sees the same document-wide namespace.
func productParameterDefinitions(model ProductModel) []modelcore.ParameterDefinition {
	parameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	for _, c := range model.Constraints {
		if c.QuantityParameter == nil {
			continue
		}
		p := *c.QuantityParameter
		p.OwnerFeatureID, p.Lifecycle = c.ID, "ASSEMBLY_REQUIRED"
		p.Label = constraintDisplayName(c, func(id string) string {
			for _, i := range model.Instances {
				if i.ID == id {
					return i.Name
				}
			}
			return id
		}, model.Constraints...)
		p.QualifiedDisplayPath = p.Label
		p.Role = "DRIVING"
		if c.Mode == "MEASURED" {
			p.Role, p.EvaluatedValue = "MEASURED", nil
			if c.MeasuredValue != nil && !c.Suppressed && c.EvaluationStatus == modelcore.AssemblyConstraintVerified {
				spec, _ := assemblyQuantitySpecification(c)
				q, err := modelcore.NewQuantity(*c.MeasuredValue, spec.unit)
				if err == nil {
					p.EvaluatedValue = &q
				}
			}
		}
		parameters = append(parameters, p)
	}
	return parameters
}

func setProductParameterDefinitions(model *ProductModel, parameters []modelcore.ParameterDefinition) {
	owned := map[string]int{}
	for i, c := range model.Constraints {
		if c.QuantityParameter != nil {
			owned[c.QuantityParameter.ParameterID] = i
		}
	}
	model.Parameters = nil
	for _, p := range parameters {
		if i, ok := owned[p.ParameterID]; ok {
			p.Role = "DRIVING" // Preserved source; measured evidence lives on the constraint.
			model.Constraints[i].QuantityParameter = &p
		} else {
			model.Parameters = append(model.Parameters, p)
		}
	}
}

func productParameterHistory(model *ProductModel, address modelcore.PropertyAddress, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	parameters := productParameterDefinitions(*model)
	result, handled, err := parameterHistory(&parameters, address, value, write)
	if handled && write && err == nil {
		setProductParameterDefinitions(model, parameters)
	}
	return result, handled, err
}

func parameterPartContext(d *parameterDocument) PartModel {
	if d.part != nil {
		return *d.part
	}
	model := PartModel{Parameters: append([]modelcore.ParameterDefinition(nil), d.Parameters...), Measurements: d.product.Measurements}
	for _, p := range d.product.Publications {
		if p.Type == "PARAMETER" && p.Target.ParameterID == "" && len(p.Target.InstancePath.Segments) > 0 && p.Contract.Dimension != nil {
			segment := p.Target.InstancePath.Segments[len(p.Target.InstancePath.Segments)-1]
			p.Target.ParameterID = "forwarded:" + p.ID
			model.Parameters = append(model.Parameters, modelcore.ParameterDefinition{ParameterID: p.Target.ParameterID, Lifecycle: "PUBLICATION_FORWARD", ValueType: p.Contract.ValueType, Dimension: *p.Contract.Dimension, Source: modelcore.ValueSource{External: &modelcore.ExternalParameterRef{SourceDocumentID: segment.ReferencedDocumentID, ResolvedRevisionID: segment.ResolvedVersionID, PublicationID: p.Target.PublicationID, Revision: modelcore.ReferenceSelector{Mode: "PINNED"}}}})
		}
		model.Publications = append(model.Publications, Publication{ID: p.ID, Name: p.Name, Type: p.Type, CompatibilityVersion: p.CompatibilityVersion, Contract: p.Contract, Resolution: p.Resolution, Target: PublicationTarget{Kind: "PARAMETER", ParameterID: p.Target.ParameterID}})
	}
	return model
}

func resolveProductParameters(model *ProductModel, revisionID string) error {
	if err := syncMeasurementParameters(&model.Parameters, model.Measurements); err != nil {
		return err
	}
	if err := resolveAssemblyQuantities(model); err != nil {
		return err
	}
	for i := range model.Publications {
		publication := &model.Publications[i]
		if publication.Target.ParameterID == "" {
			continue
		}
		publication.Resolution = parameterPublicationResolution(productParameterDefinitions(*model), publication.Target.ParameterID, publication.Contract, revisionID)
	}
	return nil
}

func productParameterPublication(model ProductModel, request CommandRequest) (ProductPublication, error) {
	for _, p := range productParameterDefinitions(model) {
		if p.ParameterID != request.TargetID {
			continue
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			names := []string{}
			for _, pub := range model.Publications {
				names = append(names, pub.Name)
			}
			name = nextScopedName("Parameter.", names)
		}
		publication := ProductPublication{ID: commandEntityID("product-publication", request.RequestID), Name: name, Type: "PARAMETER", SemanticPurpose: request.SemanticPurpose, CompatibilityVersion: "1", Target: ProductPublicationTarget{ParameterID: p.ParameterID}, Contract: PublicationContract{ValueType: p.ValueType, Dimension: &p.Dimension, UnitPolicy: "SI_CANONICAL"}}
		return publication, nil
	}
	return ProductPublication{}, fmt.Errorf("%w: parameter does not exist", ErrValidation)
}

func readParameterDocument(raw json.RawMessage) (*parameterDocument, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	d := &parameterDocument{}
	// These required model fields are validated by the owning document adapter.
	if _, product := fields["instances"]; product {
		d.product = &ProductModel{}
		if err := json.Unmarshal(raw, d.product); err != nil {
			return nil, err
		}
		d.Parameters = productParameterDefinitions(*d.product)
		d.Publications = parameterPartContext(&parameterDocument{product: d.product, Parameters: d.Parameters}).Publications
	} else {
		d.part = &PartModel{}
		if err := json.Unmarshal(raw, d.part); err != nil {
			return nil, err
		}
		normalizePartModel(d.part)
		d.Parameters, d.Publications, d.ContextInputs = d.part.Parameters, d.part.Publications, d.part.ContextInputs
	}
	return d, nil
}

func (d *parameterDocument) resolve() error {
	if d.part != nil {
		d.part.Parameters = d.Parameters
		if err := validateAndResolvePartParameters(d.part); err != nil {
			return err
		}
		d.Parameters = d.part.Parameters
		return nil
	}
	setProductParameterDefinitions(d.product, d.Parameters)
	if err := resolveProductParameters(d.product, ""); err != nil {
		return err
	}
	d.Parameters = productParameterDefinitions(*d.product)
	return nil
}

func (d *parameterDocument) MarshalJSON() ([]byte, error) {
	if d.part != nil {
		d.part.Parameters = d.Parameters
		return json.Marshal(d.part)
	}
	setProductParameterDefinitions(d.product, d.Parameters)
	return json.Marshal(d.product)
}

func resolveDocumentParameterSource(d *parameterDocument, source string) (string, error) {
	if d.part != nil {
		d.part.Parameters = d.Parameters
		return resolveQualifiedParameterSource(*d.part, source)
	}
	wanted := strings.TrimSpace(source)
	for _, p := range d.Parameters {
		if wanted == p.Label || wanted == p.QualifiedDisplayPath {
			return p.Key, nil
		}
	}
	return source, nil
}

func parameterHistory(parameters *[]modelcore.ParameterDefinition, address modelcore.PropertyAddress, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	if address.SlotID != "parameter.entity" && address.SlotID != "parameter.source" && address.SlotID != "parameter.key" {
		return nil, false, nil
	}
	index := -1
	for i, p := range *parameters {
		if p.ParameterID == address.EntityID {
			index = i
			break
		}
	}
	if !write {
		if index < 0 {
			return nil, true, nil
		}
		p := (*parameters)[index]
		var result any = parameterHistoryDefinition(p)
		if address.SlotID == "parameter.source" {
			result = p.Source
		}
		if address.SlotID == "parameter.key" {
			result = p.Key
		}
		raw, err := json.Marshal(result)
		return raw, true, err
	}
	if address.SlotID == "parameter.entity" {
		if len(value) == 0 || string(value) == "null" {
			if index >= 0 {
				*parameters = append((*parameters)[:index], (*parameters)[index+1:]...)
			}
			return nil, true, nil
		}
		var p modelcore.ParameterDefinition
		if err := json.Unmarshal(value, &p); err != nil {
			return nil, true, err
		}
		if p.ParameterID != address.EntityID {
			return nil, true, fmt.Errorf("%w: parameter history identity mismatch", ErrValidation)
		}
		if index < 0 {
			*parameters = append(*parameters, p)
		} else {
			(*parameters)[index] = p
		}
		return nil, true, nil
	}
	if index < 0 {
		return nil, true, fmt.Errorf("%w: parameter history target missing", ErrValidation)
	}
	var err error
	if address.SlotID == "parameter.source" {
		(*parameters)[index].Source = modelcore.ValueSource{}
		err = json.Unmarshal(value, &(*parameters)[index].Source)
	} else {
		err = json.Unmarshal(value, &(*parameters)[index].Key)
	}
	return nil, true, err
}

func parameterChangesOnly(changes modelcore.ChangeSet) bool {
	if len(changes.Changes) == 0 {
		return false
	}
	for _, change := range changes.Changes {
		if !strings.HasPrefix(change.Target.SlotID, "parameter.") {
			return false
		}
	}
	return true
}

func productParameterChangesOnly(model ProductModel, changes modelcore.ChangeSet) bool {
	if !parameterChangesOnly(changes) {
		return false
	}
	dirty := map[string]bool{}
	for _, c := range changes.Changes {
		if c.Target.SlotID != "parameter.key" {
			dirty[c.Target.EntityID] = true
		}
	}
	parameters := productParameterDefinitions(model)
	for changed := true; changed; {
		changed = false
		for _, p := range parameters {
			if !dirty[p.ParameterID] && p.Source.Expression != nil {
				for _, read := range p.Source.Expression.Reads {
					if dirty[strings.TrimPrefix(string(read), "parameter:")] {
						dirty[p.ParameterID], changed = true, true
						break
					}
				}
			}
		}
	}
	for _, c := range model.Constraints {
		if c.QuantityParameter != nil && dirty[c.QuantityParameter.ParameterID] {
			return false
		}
	}
	return true
}

func parameterHistoryDefinition(p modelcore.ParameterDefinition) modelcore.ParameterDefinition {
	p.EvaluatedValue = nil
	p.CurrencyStatus = ""
	p.DisplayName = ""
	p.DisplayAlias = ""
	p.QualifiedDisplayPath = ""
	return p
}

func parameterGraph(parameters []modelcore.ParameterDefinition) ([]modelcore.DependencyNode, []modelcore.DependencyEdge) {
	nodes := []modelcore.DependencyNode{}
	edges := []modelcore.DependencyEdge{}
	for _, p := range parameters {
		key := modelcore.DependencyKey("parameter:" + p.ParameterID)
		raw, _ := json.Marshal(p.Source)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 1, Type: "PARAMETER", CanonicalInput: raw, Derived: p.Source.Produced != nil})
		if p.Source.External != nil {
			source := modelcore.DependencyKey("external-parameter:" + p.ParameterID)
			nodes = append(nodes, modelcore.DependencyNode{Key: source, Type: "EXTERNAL_PARAMETER_SNAPSHOT", CanonicalInput: raw})
			edges = append(edges, modelcore.DependencyEdge{Source: source, Target: key, Kind: modelcore.ReadValue})
		}
		if p.Source.Expression != nil {
			for _, read := range p.Source.Expression.Reads {
				edges = append(edges, modelcore.DependencyEdge{Source: read, Target: key, Kind: modelcore.ReadValue})
			}
		}
	}
	return nodes, edges
}

func presentDocumentParameters(parameters []modelcore.ParameterDefinition) {
	for i := range parameters {
		p := &parameters[i]
		p.DisplayAlias = p.Key
		p.DisplayName = p.Label
		if p.DisplayName == "" {
			p.DisplayName = p.Key
		}
		p.QualifiedDisplayPath = p.DisplayName
	}
}

func documentParameterStructure(parameters []modelcore.ParameterDefinition, path, documentID, versionID string) DocumentStructureNode {
	root := DocumentStructureNode{ID: path + "/parameters", Kind: "PARAMETER_SET", Name: "参数", DocumentID: documentID, VersionID: versionID}
	for _, p := range parameters {
		name := p.DisplayName
		if name == "" {
			name = p.Label
		}
		if name == "" {
			name = p.Key
		}
		capabilities := []string{"EDIT"}
		if p.Lifecycle == "USER" {
			capabilities = append(capabilities, "DELETE")
		}
		root.Children = append(root.Children, DocumentStructureNode{ID: root.ID + "/parameter:" + p.ParameterID, Kind: "PARAMETER", EntityID: p.ParameterID, Name: name, ParameterAlias: p.Key, OwnerEntityID: p.OwnerFeatureID, DocumentID: documentID, OwnerDocumentID: documentID, VersionID: versionID, Capabilities: capabilities})
	}
	return root
}

// Connection describes identity/contract; currency describes whether the cached
// value may be consumed. A stale measurement remains inspectable and connected.
func parameterPublicationResolution(parameters []modelcore.ParameterDefinition, id string, contract PublicationContract, revisionID string) PublicationResolution {
	for _, p := range parameters {
		if p.ParameterID != id {
			continue
		}
		if contract.Dimension == nil || !contract.Dimension.Equal(p.Dimension) || contract.ValueType != p.ValueType {
			return PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: "PUBLICATION_PARAMETER_CONTRACT_INCOMPATIBLE"}
		}
		currency := p.CurrencyStatus
		if currency == "" {
			currency = "CURRENT"
		}
		result := PublicationResolution{Status: "CONNECTED", CurrencyStatus: currency, ResolvedVersionID: revisionID}
		if p.EvaluatedValue == nil {
			result.CurrencyStatus = "BLOCKED"
			return result
		}
		value := *p.EvaluatedValue
		if bounds := contract.Bounds; bounds != nil && ((bounds.Minimum != nil && value.SIValue < bounds.Minimum.SIValue) || (bounds.Maximum != nil && value.SIValue > bounds.Maximum.SIValue)) {
			return PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: "PUBLICATION_PARAMETER_OUT_OF_BOUNDS"}
		}
		result.Value = &value
		result.ValueDigest = resolvedDigest(value)
		result.SourceDigest = resolvedDigest(struct {
			ID    string
			Value modelcore.Quantity
		}{id, value})
		return result
	}
	return PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: "PUBLICATION_PARAMETER_MISSING"}
}
func refreshPartParameterPublications(model *PartModel, revisionID string) {
	for i := range model.Publications {
		p := &model.Publications[i]
		if p.Target.Kind == "PARAMETER" {
			p.Resolution = parameterPublicationResolution(model.Parameters, p.Target.ParameterID, p.Contract, revisionID)
		}
	}
}
