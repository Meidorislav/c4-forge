// Package model defines the validated in-memory C4 metamodel.
package model

import "encoding/json"

// EntityKind identifies a kind of entity in the field registry and validation errors.
type EntityKind string

const (
	EntityView         EntityKind = "view"
	EntityPlacement    EntityKind = "placement"
	EntityShape        EntityKind = "shape"
	EntityWorkspace    EntityKind = "workspace"
	EntityElement      EntityKind = "element"
	EntityRelationship EntityKind = "relationship"
)

// ElementType identifies an MVP C4 element type (MD-1).
type ElementType string

const (
	ElementPerson         ElementType = "person"
	ElementSoftwareSystem ElementType = "softwareSystem"
	ElementContainer      ElementType = "container"
	ElementComponent      ElementType = "component"
)

// Valid reports whether the element type is part of the MVP metamodel.
func (t ElementType) Valid() bool {
	switch t {
	case ElementPerson, ElementSoftwareSystem, ElementContainer, ElementComponent:
		return true
	default:
		return false
	}
}

// Element is a model-backed C4 element. ParentID is required only for
// containers and components.
type Element struct {
	ID          ID                `json:"id"`
	Type        ElementType       `json:"type"`
	ParentID    ID                `json:"parentId,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Technology  string            `json:"technology,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	URL         string            `json:"url,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	External    bool              `json:"external,omitempty"`
	DSLID       string            `json:"dslId"`
}

// Relationship is a directed link between two model elements. Multiple
// relationships may have the same source and destination (MD-5).
type Relationship struct {
	ID            ID                `json:"id"`
	SourceID      ID                `json:"sourceId"`
	DestinationID ID                `json:"destinationId"`
	Description   string            `json:"description,omitempty"`
	Technology    string            `json:"technology,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	URL           string            `json:"url,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
}

// View describes one C4 diagram. ScopeID is empty for a landscape view.
type View struct {
	ID          ID     `json:"id"`
	Type        string `json:"type"`
	Key         string `json:"key"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ScopeID     ID     `json:"scopeId,omitempty"`
}

// Placement positions one element on a view. Coordinates are required, while
// dimensions are optional. Pointers distinguish zero from an absent value.
type Placement struct {
	ID        ID       `json:"id"`
	ViewID    ID       `json:"viewId"`
	ElementID ID       `json:"elementId"`
	X         *float64 `json:"x"`
	Y         *float64 `json:"y"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
}

// Shape is a freeform annotation, independent of C4 elements and relationships.
type Shape struct {
	ID       ID              `json:"id"`
	ViewID   ID              `json:"viewId"`
	Type     string          `json:"type"`
	Geometry json.RawMessage `json:"geometry"`
	Style    json.RawMessage `json:"style,omitempty"`
	Text     string          `json:"text,omitempty"`
	Z        *float64        `json:"z,omitempty"`
}
