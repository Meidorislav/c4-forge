package model

import (
	"errors"
	"sort"
)

// Model is an immutable, validated in-memory snapshot of one workspace.
type Model struct {
	workspaceID   ID
	elements      map[ID]Element
	relationships map[ID]Relationship
}

// New validates and copies a complete workspace model. Input order does not
// matter: references are checked after all entities have been indexed.
func New(workspaceID ID, elements []Element, relationships []Relationship) (*Model, error) {
	if err := workspaceID.Validate(); err != nil {
		return nil, validationError(ErrorInvalidValue, EntityWorkspace, workspaceID, FieldID, "%v", err)
	}

	registry := DefaultRegistry()
	result := &Model{
		workspaceID:   workspaceID,
		elements:      make(map[ID]Element, len(elements)),
		relationships: make(map[ID]Relationship, len(relationships)),
	}
	allIDs := make(map[ID]EntityKind, len(elements)+len(relationships))
	dslIDs := make(map[string]ID, len(elements))

	for _, input := range elements {
		if err := validateElementFields(registry, input); err != nil {
			return nil, err
		}
		if previous, duplicate := allIDs[input.ID]; duplicate {
			return nil, validationError(
				ErrorDuplicateID,
				EntityElement,
				input.ID,
				FieldID,
				"identifier is already used by %s",
				previous,
			)
		}
		if previous, duplicate := dslIDs[input.DSLID]; duplicate {
			return nil, validationError(
				ErrorDuplicateDSLID,
				EntityElement,
				input.ID,
				FieldDSLID,
				"DSL identifier %q is already used by element %s",
				input.DSLID,
				previous,
			)
		}
		allIDs[input.ID] = EntityElement
		dslIDs[input.DSLID] = input.ID
		result.elements[input.ID] = cloneElement(input)
	}

	// Stable error selection also matters when a batch violates several hierarchy rules.
	for _, element := range result.Elements() {
		if err := validateHierarchy(element, result.elements); err != nil {
			return nil, err
		}
	}

	for _, input := range relationships {
		if err := validateRelationshipFields(registry, input); err != nil {
			return nil, err
		}
		if previous, duplicate := allIDs[input.ID]; duplicate {
			return nil, validationError(
				ErrorDuplicateID,
				EntityRelationship,
				input.ID,
				FieldID,
				"identifier is already used by %s",
				previous,
			)
		}
		if _, exists := result.elements[input.SourceID]; !exists {
			return nil, validationError(
				ErrorInvalidReference,
				EntityRelationship,
				input.ID,
				FieldSourceID,
				"source element %s does not exist",
				input.SourceID,
			)
		}
		if _, exists := result.elements[input.DestinationID]; !exists {
			return nil, validationError(
				ErrorInvalidReference,
				EntityRelationship,
				input.ID,
				FieldDestinationID,
				"destination element %s does not exist",
				input.DestinationID,
			)
		}
		allIDs[input.ID] = EntityRelationship
		result.relationships[input.ID] = cloneRelationship(input)
	}

	return result, nil
}

func validateElementFields(registry Registry, element Element) error {
	values := map[FieldName]any{
		FieldID:          element.ID,
		FieldType:        element.Type,
		FieldParentID:    element.ParentID,
		FieldNameValue:   element.Name,
		FieldDSLID:       element.DSLID,
		FieldDescription: element.Description,
		FieldTechnology:  element.Technology,
		FieldTags:        element.Tags,
		FieldURL:         element.URL,
		FieldProperties:  element.Properties,
		FieldExternal:    element.External,
	}
	for _, definition := range registry.Fields(EntityElement) {
		if err := registry.ValidateValue(EntityElement, definition.Name, values[definition.Name]); err != nil {
			return withEntityID(err, element.ID)
		}
	}
	if (element.Type == ElementPerson || element.Type == ElementSoftwareSystem) && element.Technology != "" {
		return validationError(
			ErrorInvalidValue,
			EntityElement,
			element.ID,
			FieldTechnology,
			"technology is only valid for containers and components",
		)
	}
	return nil
}

func validateRelationshipFields(registry Registry, relationship Relationship) error {
	values := map[FieldName]any{
		FieldID:            relationship.ID,
		FieldSourceID:      relationship.SourceID,
		FieldDestinationID: relationship.DestinationID,
		FieldDescription:   relationship.Description,
		FieldTechnology:    relationship.Technology,
		FieldTags:          relationship.Tags,
		FieldURL:           relationship.URL,
		FieldProperties:    relationship.Properties,
	}
	for _, definition := range registry.Fields(EntityRelationship) {
		if err := registry.ValidateValue(EntityRelationship, definition.Name, values[definition.Name]); err != nil {
			return withEntityID(err, relationship.ID)
		}
	}
	return nil
}

func withEntityID(err error, entityID ID) error {
	var validation *ValidationError
	if !errors.As(err, &validation) {
		return err
	}
	cloned := *validation
	cloned.EntityID = entityID
	return &cloned
}

func validateHierarchy(element Element, elements map[ID]Element) error {
	switch element.Type {
	case ElementPerson, ElementSoftwareSystem:
		if element.ParentID != "" {
			return validationError(
				ErrorInvalidHierarchy,
				EntityElement,
				element.ID,
				FieldParentID,
				"%s must not have a parent",
				element.Type,
			)
		}
		return nil
	case ElementContainer:
		return validateParentType(element, elements, ElementSoftwareSystem)
	case ElementComponent:
		return validateParentType(element, elements, ElementContainer)
	default:
		return validationError(
			ErrorInvalidValue,
			EntityElement,
			element.ID,
			FieldType,
			"unknown element type %q",
			element.Type,
		)
	}
}

func validateParentType(element Element, elements map[ID]Element, expected ElementType) error {
	if element.ParentID == "" {
		return validationError(
			ErrorRequired,
			EntityElement,
			element.ID,
			FieldParentID,
			"%s requires a %s parent",
			element.Type,
			expected,
		)
	}
	parent, exists := elements[element.ParentID]
	if !exists {
		return validationError(
			ErrorInvalidReference,
			EntityElement,
			element.ID,
			FieldParentID,
			"parent element %s does not exist",
			element.ParentID,
		)
	}
	if parent.Type != expected {
		return validationError(
			ErrorInvalidHierarchy,
			EntityElement,
			element.ID,
			FieldParentID,
			"%s parent has type %s, want %s",
			element.Type,
			parent.Type,
			expected,
		)
	}
	return nil
}

// WorkspaceID returns the workspace that owns this model.
func (m *Model) WorkspaceID() ID {
	return m.workspaceID
}

// Element returns a defensive copy of an element.
func (m *Model) Element(id ID) (Element, bool) {
	element, ok := m.elements[id]
	return cloneElement(element), ok
}

// Relationship returns a defensive copy of a relationship.
func (m *Model) Relationship(id ID) (Relationship, bool) {
	relationship, ok := m.relationships[id]
	return cloneRelationship(relationship), ok
}

// Elements returns defensive copies sorted by ID for deterministic consumers.
func (m *Model) Elements() []Element {
	result := make([]Element, 0, len(m.elements))
	for _, element := range m.elements {
		result = append(result, cloneElement(element))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// Relationships returns defensive copies sorted by ID for deterministic consumers.
func (m *Model) Relationships() []Relationship {
	result := make([]Relationship, 0, len(m.relationships))
	for _, relationship := range m.relationships {
		result = append(result, cloneRelationship(relationship))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func cloneElement(element Element) Element {
	element.Tags = append([]string(nil), element.Tags...)
	element.Properties = cloneStringMap(element.Properties)
	return element
}

func cloneRelationship(relationship Relationship) Relationship {
	relationship.Tags = append([]string(nil), relationship.Tags...)
	relationship.Properties = cloneStringMap(relationship.Properties)
	return relationship
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
