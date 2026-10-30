package model

import (
	"fmt"
	"sort"
	"strings"
)

// FieldName is the stable wire/storage name of an entity field.
type FieldName string

const (
	FieldID            FieldName = "id"
	FieldType          FieldName = "type"
	FieldParentID      FieldName = "parentId"
	FieldNameValue     FieldName = "name"
	FieldDescription   FieldName = "description"
	FieldTechnology    FieldName = "technology"
	FieldTags          FieldName = "tags"
	FieldURL           FieldName = "url"
	FieldProperties    FieldName = "properties"
	FieldExternal      FieldName = "external"
	FieldDSLID         FieldName = "dslId"
	FieldSourceID      FieldName = "sourceId"
	FieldDestinationID FieldName = "destinationId"
)

// StorageKind identifies where ADR-0003 persists a field.
type StorageKind string

const (
	StorageColumn StorageKind = "column"
	StorageAttrs  StorageKind = "attrs"
)

// ValueKind identifies the runtime value accepted by a registered field.
type ValueKind string

const (
	ValueUUID        ValueKind = "uuid"
	ValueString      ValueKind = "string"
	ValueBool        ValueKind = "bool"
	ValueStringList  ValueKind = "stringList"
	ValueStringMap   ValueKind = "stringMap"
	ValueElementType ValueKind = "elementType"
)

// FieldDefinition declares how one field is stored and validated. A non-empty
// Reference identifies the kind of entity its UUID must reference.
type FieldDefinition struct {
	Name      FieldName
	Storage   StorageKind
	Value     ValueKind
	Required  bool
	Reference EntityKind
}

// Registry is the immutable field registry required by ADR-0003.
type Registry struct {
	fields map[EntityKind]map[FieldName]FieldDefinition
}

// DefaultRegistry returns the field registry for roadmap step 2.1.
func DefaultRegistry() Registry {
	return Registry{fields: map[EntityKind]map[FieldName]FieldDefinition{
		EntityElement: definitions(
			FieldDefinition{FieldID, StorageColumn, ValueUUID, true, ""},
			FieldDefinition{FieldType, StorageColumn, ValueElementType, true, ""},
			FieldDefinition{FieldParentID, StorageColumn, ValueUUID, false, EntityElement},
			FieldDefinition{FieldNameValue, StorageColumn, ValueString, true, ""},
			FieldDefinition{FieldDSLID, StorageColumn, ValueString, true, ""},
			FieldDefinition{FieldDescription, StorageAttrs, ValueString, false, ""},
			FieldDefinition{FieldTechnology, StorageAttrs, ValueString, false, ""},
			FieldDefinition{FieldTags, StorageAttrs, ValueStringList, false, ""},
			FieldDefinition{FieldURL, StorageAttrs, ValueString, false, ""},
			FieldDefinition{FieldProperties, StorageAttrs, ValueStringMap, false, ""},
			FieldDefinition{FieldExternal, StorageAttrs, ValueBool, false, ""},
		),
		EntityRelationship: definitions(
			FieldDefinition{FieldID, StorageColumn, ValueUUID, true, ""},
			FieldDefinition{FieldSourceID, StorageColumn, ValueUUID, true, EntityElement},
			FieldDefinition{FieldDestinationID, StorageColumn, ValueUUID, true, EntityElement},
			FieldDefinition{FieldDescription, StorageAttrs, ValueString, false, ""},
			FieldDefinition{FieldTechnology, StorageAttrs, ValueString, false, ""},
			FieldDefinition{FieldTags, StorageAttrs, ValueStringList, false, ""},
			FieldDefinition{FieldURL, StorageAttrs, ValueString, false, ""},
			FieldDefinition{FieldProperties, StorageAttrs, ValueStringMap, false, ""},
		),
	}}
}

func definitions(fields ...FieldDefinition) map[FieldName]FieldDefinition {
	result := make(map[FieldName]FieldDefinition, len(fields))
	for _, field := range fields {
		result[field.Name] = field
	}
	return result
}

// Lookup returns the definition of one registered field.
func (r Registry) Lookup(entity EntityKind, field FieldName) (FieldDefinition, bool) {
	fields, ok := r.fields[entity]
	if !ok {
		return FieldDefinition{}, false
	}
	definition, ok := fields[field]
	return definition, ok
}

// Fields returns an entity's definitions sorted by stable field name.
func (r Registry) Fields(entity EntityKind) []FieldDefinition {
	fields := r.fields[entity]
	result := make([]FieldDefinition, 0, len(fields))
	for _, field := range fields {
		result = append(result, field)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// ValidateValue validates a field value without applying cross-entity rules.
func (r Registry) ValidateValue(entity EntityKind, field FieldName, value any) error {
	fields, ok := r.fields[entity]
	if !ok {
		return validationError(ErrorUnknownEntity, entity, "", field, "entity kind is not registered")
	}
	definition, ok := fields[field]
	if !ok {
		return validationError(ErrorUnknownField, entity, "", field, "field is not registered")
	}
	if value == nil {
		if definition.Required {
			return validationError(ErrorRequired, entity, "", field, "value is required")
		}
		return nil
	}

	switch definition.Value {
	case ValueUUID:
		return validateUUIDValue(entity, definition, value)
	case ValueString:
		text, ok := value.(string)
		if !ok {
			return wrongValueType(entity, definition, value)
		}
		if definition.Required && strings.TrimSpace(text) == "" {
			return validationError(ErrorRequired, entity, "", field, "value is required")
		}
	case ValueBool:
		if _, ok := value.(bool); !ok {
			return wrongValueType(entity, definition, value)
		}
	case ValueStringList:
		values, ok := value.([]string)
		if !ok {
			return wrongValueType(entity, definition, value)
		}
		return validateStringList(entity, field, values)
	case ValueStringMap:
		values, ok := value.(map[string]string)
		if !ok {
			return wrongValueType(entity, definition, value)
		}
		for key := range values {
			if strings.TrimSpace(key) == "" {
				return validationError(ErrorInvalidValue, entity, "", field, "property key must not be empty")
			}
		}
	case ValueElementType:
		elementType, ok := value.(ElementType)
		if !ok {
			return wrongValueType(entity, definition, value)
		}
		if !elementType.Valid() {
			return validationError(ErrorInvalidValue, entity, "", field, "unknown element type %q", elementType)
		}
	default:
		return validationError(ErrorInvalidValue, entity, "", field, "unknown registry value kind %q", definition.Value)
	}
	return nil
}

func validateUUIDValue(entity EntityKind, field FieldDefinition, value any) error {
	var id ID
	switch typed := value.(type) {
	case ID:
		id = typed
	case string:
		id = ID(typed)
	default:
		return wrongValueType(entity, field, value)
	}
	if id == "" && !field.Required {
		return nil
	}
	if id == "" {
		return validationError(ErrorRequired, entity, "", field.Name, "value is required")
	}
	if err := id.Validate(); err != nil {
		return validationError(ErrorInvalidValue, entity, "", field.Name, "%v", err)
	}
	return nil
}

func validateStringList(entity EntityKind, field FieldName, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return validationError(ErrorInvalidValue, entity, "", field, "list values must not be empty")
		}
		if _, duplicate := seen[value]; duplicate {
			return validationError(ErrorInvalidValue, entity, "", field, "duplicate value %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func wrongValueType(entity EntityKind, field FieldDefinition, value any) error {
	return validationError(
		ErrorInvalidType,
		entity,
		"",
		field.Name,
		"got %T, want %s",
		value,
		field.Value,
	)
}

func (d FieldDefinition) String() string {
	return fmt.Sprintf("%s.%s", d.Storage, d.Name)
}
