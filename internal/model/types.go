// Package model defines the validated in-memory C4 metamodel.
package model

// EntityKind identifies a kind of entity in the field registry and validation errors.
type EntityKind string

const (
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
