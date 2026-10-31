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

// Change records an expanded operation and the values it replaced (ADR-0003).
// Existed distinguishes creation from an existing entity with optional fields
// absent. Before contains patched fields for set, or all fields for delete;
// absent values are JSON null. Changes own their maps and JSON bytes.
type Change struct {
	Operation
	Existed bool                          `json:"existed"`
	Before  map[FieldName]json.RawMessage `json:"before"`
}

// Apply applies a batch and returns its expanded operations. See
// ApplyWithChanges for history, atomicity and persistence semantics.
func (m *Model) Apply(operations []Operation) (*Model, []Operation, error) {
	result, changes, err := m.ApplyWithChanges(operations)
	if err != nil {
		return nil, nil, err
	}
	expanded := make([]Operation, 0, len(changes))
	for _, change := range changes {
		expanded = append(expanded, change.Operation)
	}
	return result, expanded, nil
}

// ApplyWithChanges applies operations in order to a private candidate and
// validates its final state. Failure returns no candidate or changes. The
// receiver is unchanged, even on success: storage must commit before publishing
// the candidate. Cascades and their previous values have deterministic order.
func (m *Model) ApplyWithChanges(operations []Operation) (*Model, []Change, error) {
	candidate := &Model{
		workspaceID:   m.workspaceID,
		elements:      make(map[ID]Element, len(m.elements)),
		relationships: make(map[ID]Relationship, len(m.relationships)),
		views:         make(map[ID]View, len(m.views)),
		placements:    make(map[ID]Placement, len(m.placements)),
		shapes:        make(map[ID]Shape, len(m.shapes)),
	}
	for id, element := range m.elements {
		candidate.elements[id] = cloneElement(element)
	}
	for id, relationship := range m.relationships {
		candidate.relationships[id] = cloneRelationship(relationship)
	}
	for id, v := range m.views {
		candidate.views[id] = v
	}
	for id, p := range m.placements {
		candidate.placements[id] = clonePlacement(p)
	}
	for id, s := range m.shapes {
		candidate.shapes[id] = cloneShape(s)
	}
	expanded := make([]Change, 0, len(operations))
	for i, operation := range operations {
		applied, err := candidate.applyOperation(operation)
		if err != nil {
			return nil, nil, fmt.Errorf("operation %d: %w", i, err)
		}
		expanded = append(expanded, applied...)
	}
	result, err := NewWithPresentation(candidate.workspaceID, candidate.Elements(), candidate.Relationships(), candidate.Views(), candidate.Placements(), candidate.Shapes())
	if err != nil {
		return nil, nil, fmt.Errorf("batch result: %w", err)
	}
	return result, expanded, nil
}

func (m *Model) applyOperation(op Operation) ([]Change, error) {
	if op.Kind != EntityElement && op.Kind != EntityRelationship && op.Kind != EntityView && op.Kind != EntityPlacement && op.Kind != EntityShape {
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
		change := m.changeBefore(op)
		if err := m.set(op); err != nil {
			return nil, err
		}
		return []Change{change}, nil
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
	kinds := []EntityKind{EntityElement, EntityRelationship, EntityView, EntityPlacement, EntityShape}
	for _, kind := range kinds {
		if kind != op.Kind && m.contains(kind, op.ID) {
			return validationError(ErrorDuplicateID, op.Kind, op.ID, FieldID, "identifier is used by %s", kind)
		}
	}
	switch op.Kind {
	case EntityElement:
		current := m.elements[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.elements[op.ID] = next
	case EntityRelationship:
		current := m.relationships[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.relationships[op.ID] = next
	case EntityView:
		current := m.views[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.views[op.ID] = next
	case EntityPlacement:
		current := m.placements[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.placements[op.ID] = next
	case EntityShape:
		current := m.shapes[op.ID]
		current.ID = op.ID
		next, err := mergePatch(current, op.Fields)
		if err != nil {
			return err
		}
		m.shapes[op.ID] = next
	}
	return nil
}

func (m *Model) contains(kind EntityKind, id ID) bool {
	switch kind {
	case EntityElement:
		_, ok := m.elements[id]
		return ok
	case EntityRelationship:
		_, ok := m.relationships[id]
		return ok
	case EntityView:
		_, ok := m.views[id]
		return ok
	case EntityPlacement:
		_, ok := m.placements[id]
		return ok
	case EntityShape:
		_, ok := m.shapes[id]
		return ok
	}
	return false
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

func (m *Model) deleteEntity(op Operation) []Change {
	if op.Kind == EntityPlacement || op.Kind == EntityShape {
		change := m.changeBefore(op)
		if op.Kind == EntityPlacement {
			delete(m.placements, op.ID)
		} else {
			delete(m.shapes, op.ID)
		}
		return []Change{change}
	}
	if op.Kind == EntityView {
		return m.deleteView(op.ID)
	}
	if op.Kind == EntityRelationship {
		// Missing deletes are retained as harmless no-ops, including during replay.
		change := m.changeBefore(op)
		delete(m.relationships, op.ID)
		return []Change{change}
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
	expanded := make([]Change, 0)
	// Remove scoped views and their contents before deleting elements.
	for _, view := range m.Views() {
		if deleted[view.ScopeID] {
			expanded = append(expanded, m.deleteView(view.ID)...)
		}
	}
	for _, placement := range m.Placements() {
		if deleted[placement.ElementID] {
			expanded = append(expanded, m.deleteEntity(Operation{Op: OperationDelete, Kind: EntityPlacement, ID: placement.ID})...)
		}
	}
	for _, relationship := range m.Relationships() {
		if deleted[relationship.SourceID] || deleted[relationship.DestinationID] {
			expanded = append(expanded, m.changeBefore(Operation{Op: OperationDelete, Kind: EntityRelationship, ID: relationship.ID}))
			delete(m.relationships, relationship.ID)
		}
	}
	for _, id := range order {
		expanded = append(expanded, m.changeBefore(Operation{Op: OperationDelete, Kind: EntityElement, ID: id}))
		delete(m.elements, id)
	}
	return expanded
}

func (m *Model) deleteView(id ID) []Change {
	changes := make([]Change, 0)
	for _, shape := range m.Shapes() {
		if shape.ViewID == id {
			changes = append(changes, m.deleteEntity(Operation{Op: OperationDelete, Kind: EntityShape, ID: shape.ID})...)
		}
	}
	for _, p := range m.Placements() {
		if p.ViewID == id {
			changes = append(changes, m.deleteEntity(Operation{Op: OperationDelete, Kind: EntityPlacement, ID: p.ID})...)
		}
	}
	op := Operation{Op: OperationDelete, Kind: EntityView, ID: id}
	changes = append(changes, m.changeBefore(op))
	delete(m.views, id)
	return changes
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

// Entity fields are structs containing only JSON-compatible values; marshaling
// cannot fail. Reading the candidate before each operation preserves intermediate
// values when a batch touches the same field more than once.
func (m *Model) changeBefore(op Operation) Change {
	change := Change{Operation: cloneOperation(op), Before: make(map[FieldName]json.RawMessage)}
	var entity any
	switch op.Kind {
	case EntityElement:
		value, ok := m.elements[op.ID]
		change.Existed, entity = ok, value
	case EntityRelationship:
		value, ok := m.relationships[op.ID]
		change.Existed, entity = ok, value
	case EntityView:
		value, ok := m.views[op.ID]
		change.Existed, entity = ok, value
	case EntityPlacement:
		value, ok := m.placements[op.ID]
		change.Existed, entity = ok, value
	case EntityShape:
		value, ok := m.shapes[op.ID]
		change.Existed, entity = ok, value
	}
	values := make(map[FieldName]json.RawMessage)
	if change.Existed {
		data, _ := json.Marshal(entity)
		_ = json.Unmarshal(data, &values)
	}
	for _, field := range DefaultRegistry().Fields(op.Kind) {
		if op.Op == OperationSet {
			if _, patched := op.Fields[field.Name]; !patched {
				continue
			}
		}
		previous, ok := values[field.Name]
		if !ok {
			previous = json.RawMessage("null")
		}
		change.Before[field.Name] = previous
	}
	return change
}
