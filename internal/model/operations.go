package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// OperationType identifies an ADR-0002 field patch or entity deletion.
type OperationType string

const (
	OperationSet    OperationType = "set"
	OperationDelete OperationType = "delete"
)

// Operation addresses one entity. Set creates or patches an entity; JSON null
// removes an optional field. ID is supplied only in the envelope, never Fields.
type Operation struct {
	Op     OperationType                 `json:"op"`
	Kind   EntityKind                    `json:"kind"`
	ID     ID                            `json:"id"`
	Fields map[FieldName]json.RawMessage `json:"fields,omitempty"`
}

// Apply applies operations in order to a private candidate and validates its
// final state. A failure returns neither a candidate nor expanded operations.
// The receiver is unchanged even on success, so persistence can commit first.
// Expanded operations include deterministic cascades for replay by clients.
func (m *Model) Apply(operations []Operation) (*Model, []Operation, error) {
	candidate := &Model{
		workspaceID:   m.workspaceID,
		elements:      make(map[ID]Element, len(m.elements)),
		relationships: make(map[ID]Relationship, len(m.relationships)),
	}
	for id, element := range m.elements {
		candidate.elements[id] = cloneElement(element)
	}
	for id, relationship := range m.relationships {
		candidate.relationships[id] = cloneRelationship(relationship)
	}
	expanded := make([]Operation, 0, len(operations))
	for i, operation := range operations {
		applied, err := candidate.applyOperation(operation)
		if err != nil {
			return nil, nil, fmt.Errorf("operation %d: %w", i, err)
		}
		expanded = append(expanded, applied...)
	}
	result, err := New(candidate.workspaceID, candidate.Elements(), candidate.Relationships())
	if err != nil {
		return nil, nil, fmt.Errorf("batch result: %w", err)
	}
	return result, expanded, nil
}

func (m *Model) applyOperation(op Operation) ([]Operation, error) {
	if op.Kind != EntityElement && op.Kind != EntityRelationship {
		return nil, validationError(ErrorUnknownEntity, op.Kind, op.ID, "", "entity kind is not supported")
	}
	if err := op.ID.Validate(); err != nil {
		return nil, validationError(ErrorInvalidValue, op.Kind, op.ID, FieldID, "%v", err)
	}
	switch op.Op {
	case OperationSet:
		if len(op.Fields) == 0 {
			return nil, validationError(ErrorRequired, op.Kind, op.ID, "", "set requires fields")
		}
		if err := validatePatch(op); err != nil {
			return nil, err
		}
		if err := m.set(op); err != nil {
			return nil, err
		}
		return []Operation{cloneOperation(op)}, nil
	case OperationDelete:
		if op.Fields != nil {
			return nil, validationError(ErrorInvalidValue, op.Kind, op.ID, "", "delete must not contain fields")
		}
		return m.deleteEntity(op), nil
	default:
		return nil, validationError(ErrorInvalidValue, op.Kind, op.ID, "", "unknown operation %q", op.Op)
	}
}

func validatePatch(op Operation) error {
	registry := DefaultRegistry()
	names := make([]FieldName, 0, len(op.Fields))
	for name := range op.Fields {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	for _, name := range names {
		if name == FieldID {
			return validationError(ErrorInvalidValue, op.Kind, op.ID, name, "identity is only allowed in the operation envelope")
		}
		definition, ok := registry.Lookup(op.Kind, name)
		if !ok {
			return validationError(ErrorUnknownField, op.Kind, op.ID, name, "field is not registered")
		}
		value, err := patchValue(definition, op.Fields[name])
		if err != nil {
			return validationError(ErrorInvalidType, op.Kind, op.ID, name, "%v", err)
		}
		if err := registry.ValidateValue(op.Kind, name, value); err != nil {
			return withEntityID(err, op.ID)
		}
	}
	return nil
}

// Decode through JSON's value types first: encoding/json otherwise accepts null
// inside []string and map[string]string as an empty string.
func patchValue(def FieldDefinition, raw json.RawMessage) (any, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	switch def.Value {
	case ValueElementType:
		if text, ok := value.(string); ok {
			return ElementType(text), nil
		}
	case ValueStringList:
		if values, ok := value.([]any); ok {
			result := make([]string, len(values))
			for i, item := range values {
				text, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("list item must be a string")
				}
				result[i] = text
			}
			return result, nil
		}
	case ValueStringMap:
		if values, ok := value.(map[string]any); ok {
			result := make(map[string]string, len(values))
			for key, item := range values {
				text, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("property value must be a string")
				}
				result[key] = text
			}
			return result, nil
		}
	}
	return value, nil
}

func (m *Model) set(op Operation) error {
	switch op.Kind {
	case EntityElement:
		if _, exists := m.relationships[op.ID]; exists {
			return validationError(ErrorDuplicateID, op.Kind, op.ID, FieldID, "identifier is used by a relationship")
		}
		current := m.elements[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.elements[op.ID] = next
	case EntityRelationship:
		if _, exists := m.elements[op.ID]; exists {
			return validationError(ErrorDuplicateID, op.Kind, op.ID, FieldID, "identifier is used by an element")
		}
		current := m.relationships[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.relationships[op.ID] = next
	}
	return nil
}

func mergePatch[T any](current T, patch map[FieldName]json.RawMessage) (T, error) {
	var next T
	data, err := json.Marshal(current)
	if err != nil {
		return next, err
	}
	var fields map[FieldName]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return next, err
	}
	for name, value := range patch {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(fields, name)
		} else {
			fields[name] = value
		}
	}
	data, err = json.Marshal(fields)
	if err != nil {
		return next, err
	}
	err = json.Unmarshal(data, &next)
	return next, err
}

func (m *Model) deleteEntity(op Operation) []Operation {
	if op.Kind == EntityRelationship {
		// Missing deletes are retained as harmless no-ops, including during replay.
		delete(m.relationships, op.ID)
		return []Operation{op}
	}
	// Traverse a sorted child list and mark before recursing: even a temporarily
	// cyclic candidate must not hang before final batch validation.
	deleted := make(map[ID]bool)
	var order []ID
	children := make(map[ID][]ID)
	for _, element := range m.Elements() {
		children[element.ParentID] = append(children[element.ParentID], element.ID)
	}
	var visit func(ID)
	visit = func(id ID) {
		if deleted[id] {
			return
		}
		deleted[id] = true
		for _, child := range children[id] {
			visit(child)
		}
		order = append(order, id)
	}
	visit(op.ID)
	expanded := make([]Operation, 0)
	for _, relationship := range m.Relationships() {
		if deleted[relationship.SourceID] || deleted[relationship.DestinationID] {
			delete(m.relationships, relationship.ID)
			expanded = append(expanded, Operation{Op: OperationDelete, Kind: EntityRelationship, ID: relationship.ID})
		}
	}
	for _, id := range order {
		delete(m.elements, id)
		expanded = append(expanded, Operation{Op: OperationDelete, Kind: EntityElement, ID: id})
	}
	return expanded
}

func cloneOperation(op Operation) Operation {
	if op.Fields != nil {
		fields := make(map[FieldName]json.RawMessage, len(op.Fields))
		for name, value := range op.Fields {
			fields[name] = bytes.Clone(value)
		}
		op.Fields = fields
	}
	return op
}
