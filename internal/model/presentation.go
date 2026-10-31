package model

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func (m *Model) addPresentation(ids map[ID]EntityKind, views []View, placements []Placement, shapes []Shape) error {
	add := func(kind EntityKind, id ID, value any) error {
		if previous, exists := ids[id]; exists {
			return validationError(ErrorDuplicateID, kind, id, FieldID, "identifier is already used by %s", previous)
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return validationError(ErrorInvalidValue, kind, id, "", "invalid entity: %v", err)
		}
		var fields map[FieldName]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		registry := DefaultRegistry()
		for _, def := range registry.Fields(kind) {
			raw, exists := fields[def.Name]
			if !exists {
				raw = json.RawMessage("null")
			}
			value, err := patchValue(def, raw)
			if err != nil {
				return validationError(ErrorInvalidType, kind, id, def.Name, "%v", err)
			}
			if err := registry.ValidateValue(kind, def.Name, value); err != nil {
				return withEntityID(err, id)
			}
		}
		ids[id] = kind
		return nil
	}
	keys := map[string]ID{}
	for _, view := range views {
		if err := add(EntityView, view.ID, view); err != nil {
			return err
		}
		if _, ok := keys[view.Key]; ok {
			return validationError(ErrorInvalidValue, EntityView, view.ID, FieldKey, "view key is already used")
		}
		keys[view.Key] = view.ID
		var expected ElementType
		switch view.Type {
		case "systemLandscape":
			if view.ScopeID != "" {
				return validationError(ErrorInvalidValue, EntityView, view.ID, FieldScopeID, "landscape view must not have a scope")
			}
		case "systemContext", "container":
			expected = ElementSoftwareSystem
		case "component":
			expected = ElementContainer
		default:
			return validationError(ErrorInvalidValue, EntityView, view.ID, FieldType, "unknown view type %q", view.Type)
		}
		if expected != "" {
			if view.ScopeID == "" {
				return validationError(ErrorRequired, EntityView, view.ID, FieldScopeID, "scope is required")
			}
			element, ok := m.elements[view.ScopeID]
			if !ok {
				return validationError(ErrorInvalidReference, EntityView, view.ID, FieldScopeID, "scope element does not exist")
			}
			if element.Type != expected {
				return validationError(ErrorInvalidValue, EntityView, view.ID, FieldScopeID, "scope must be %s", expected)
			}
		}
		m.views[view.ID] = view
	}
	pairs := map[[2]ID]bool{}
	for _, p := range placements {
		if err := add(EntityPlacement, p.ID, p); err != nil {
			return err
		}
		if _, ok := m.views[p.ViewID]; !ok {
			return validationError(ErrorInvalidReference, EntityPlacement, p.ID, FieldViewID, "view does not exist")
		}
		if _, ok := m.elements[p.ElementID]; !ok {
			return validationError(ErrorInvalidReference, EntityPlacement, p.ID, FieldElementID, "element does not exist")
		}
		pair := [2]ID{p.ViewID, p.ElementID}
		if pairs[pair] {
			return validationError(ErrorInvalidValue, EntityPlacement, p.ID, FieldElementID, "element is already placed on view")
		}
		pairs[pair] = true
		if p.Width != nil && *p.Width <= 0 {
			return validationError(ErrorInvalidValue, EntityPlacement, p.ID, FieldWidth, "width must be positive")
		}
		if p.Height != nil && *p.Height <= 0 {
			return validationError(ErrorInvalidValue, EntityPlacement, p.ID, FieldHeight, "height must be positive")
		}
		m.placements[p.ID] = clonePlacement(p)
	}
	for _, shape := range shapes {
		if err := add(EntityShape, shape.ID, shape); err != nil {
			return err
		}
		if _, ok := m.views[shape.ViewID]; !ok {
			return validationError(ErrorInvalidReference, EntityShape, shape.ID, FieldViewID, "view does not exist")
		}
		switch shape.Type {
		case "sticky", "rectangle", "ellipse", "text", "arrow":
		default:
			return validationError(ErrorInvalidValue, EntityShape, shape.ID, FieldType, "unknown shape type %q", shape.Type)
		}
		m.shapes[shape.ID] = cloneShape(shape)
	}
	return nil
}

func cloneNumber(p *float64) *float64 {
	if p == nil {
		return nil
	}
	n := *p
	return &n
}

func clonePlacement(p Placement) Placement {
	p.X = cloneNumber(p.X)
	p.Y = cloneNumber(p.Y)
	p.Width = cloneNumber(p.Width)
	p.Height = cloneNumber(p.Height)
	return p
}

func cloneShape(s Shape) Shape {
	s.Geometry = canonicalObject(s.Geometry)
	s.Style = canonicalObject(s.Style)
	s.Z = cloneNumber(s.Z)
	return s
}

// View returns one view.
func (m *Model) View(id ID) (View, bool) { v, ok := m.views[id]; return v, ok }

// Placement returns a defensive copy of one placement.
func (m *Model) Placement(id ID) (Placement, bool) {
	p, ok := m.placements[id]
	return clonePlacement(p), ok
}

// Shape returns a defensive copy of one shape.
func (m *Model) Shape(id ID) (Shape, bool) { s, ok := m.shapes[id]; return cloneShape(s), ok }

// Views returns views sorted by ID.
func (m *Model) Views() []View {
	r := make([]View, 0, len(m.views))
	for _, v := range m.views {
		r = append(r, v)
	}
	sort.Slice(r, func(i, j int) bool { return r[i].ID < r[j].ID })
	return r
}

// Placements returns defensive copies sorted by ID.
func (m *Model) Placements() []Placement {
	r := make([]Placement, 0, len(m.placements))
	for _, p := range m.placements {
		r = append(r, clonePlacement(p))
	}
	sort.Slice(r, func(i, j int) bool { return r[i].ID < r[j].ID })
	return r
}

// Shapes returns defensive copies sorted by ID.
func (m *Model) Shapes() []Shape {
	r := make([]Shape, 0, len(m.shapes))
	for _, s := range m.shapes {
		r = append(r, cloneShape(s))
	}
	sort.Slice(r, func(i, j int) bool { return r[i].ID < r[j].ID })
	return r
}

func canonicalObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return bytes.Clone(raw)
	}
	data, err := json.Marshal(normalizeJSONNumbers(value))
	if err != nil {
		return bytes.Clone(raw)
	}
	return data
}

// Normalize decimal spellings without converting through float64. PostgreSQL
// JSONB may rewrite exponents, but must not change a geometry's numeric value.
func normalizeJSONNumbers(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeJSONNumbers(item)
		}
	case []any:
		for i, item := range v {
			v[i] = normalizeJSONNumbers(item)
		}
	case json.Number:
		text := string(v)
		sign := ""
		if strings.HasPrefix(text, "-") {
			sign = "-"
			text = text[1:]
		}
		parts := strings.SplitN(strings.ToLower(text), "e", 2)
		exponent := 0
		if len(parts) == 2 {
			var err error
			exponent, err = strconv.Atoi(parts[1])
			if err != nil {
				return v
			}
		}
		decimal := strings.SplitN(parts[0], ".", 2)
		digits := decimal[0]
		if len(decimal) == 2 {
			digits += decimal[1]
			exponent -= len(decimal[1])
		}
		digits = strings.TrimLeft(digits, "0")
		if digits == "" {
			return json.Number("0")
		}
		trimmed := strings.TrimRight(digits, "0")
		exponent += len(digits) - len(trimmed)
		digits = trimmed
		if exponent != 0 {
			digits += "e" + strconv.Itoa(exponent)
		}
		return json.Number(sign + digits)
	}
	return value
}
