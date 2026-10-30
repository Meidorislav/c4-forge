package model

import (
	"errors"
	"testing"
)

func TestDefaultRegistryDescribesStorageAndReferences(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		entity    EntityKind
		field     FieldName
		storage   StorageKind
		value     ValueKind
		required  bool
		reference EntityKind
	}{
		{EntityElement, FieldID, StorageColumn, ValueUUID, true, ""},
		{EntityElement, FieldParentID, StorageColumn, ValueUUID, false, EntityElement},
		{EntityElement, FieldDescription, StorageAttrs, ValueString, false, ""},
		{EntityElement, FieldTags, StorageAttrs, ValueStringList, false, ""},
		{EntityRelationship, FieldSourceID, StorageColumn, ValueUUID, true, EntityElement},
		{EntityRelationship, FieldProperties, StorageAttrs, ValueStringMap, false, ""},
	}
	for _, test := range tests {
		definition, ok := registry.Lookup(test.entity, test.field)
		if !ok {
			t.Errorf("Lookup(%s, %s) not found", test.entity, test.field)
			continue
		}
		if definition.Storage != test.storage ||
			definition.Value != test.value ||
			definition.Required != test.required ||
			definition.Reference != test.reference {
			t.Errorf("Lookup(%s, %s) = %+v", test.entity, test.field, definition)
		}
	}
}

func TestRegistryFieldsAreStableAndSorted(t *testing.T) {
	registry := DefaultRegistry()
	fields := registry.Fields(EntityRelationship)
	for i := 1; i < len(fields); i++ {
		if fields[i-1].Name >= fields[i].Name {
			t.Fatalf("Fields() not sorted at %q, %q", fields[i-1].Name, fields[i].Name)
		}
	}
	fields[0].Name = "mutated"
	again := registry.Fields(EntityRelationship)
	if again[0].Name == "mutated" {
		t.Fatal("Fields() exposed registry storage")
	}
}

func TestRegistryValidatesFieldValues(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name   string
		entity EntityKind
		field  FieldName
		value  any
		code   ErrorCode
	}{
		{"valid UUIDv7", EntityElement, FieldID, testPersonID, ""},
		{"optional reference omitted", EntityElement, FieldParentID, ID(""), ""},
		{"valid tags", EntityElement, FieldTags, []string{"A", "B"}, ""},
		{"unknown entity", "shape", FieldID, testPersonID, ErrorUnknownEntity},
		{"unknown field", EntityElement, "shape", "box", ErrorUnknownField},
		{"required nil", EntityElement, FieldID, nil, ErrorRequired},
		{"wrong runtime type", EntityElement, FieldExternal, "yes", ErrorInvalidType},
		{
			"UUIDv4 rejected",
			EntityElement,
			FieldID,
			ID("550e8400-e29b-41d4-a716-446655440000"),
			ErrorInvalidValue,
		},
		{"unknown element type", EntityElement, FieldType, ElementType("database"), ErrorInvalidValue},
		{"duplicate list value", EntityElement, FieldTags, []string{"A", "A"}, ErrorInvalidValue},
		{"empty property key", EntityElement, FieldProperties, map[string]string{"": "x"}, ErrorInvalidValue},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := registry.ValidateValue(test.entity, test.field, test.value)
			if test.code == "" {
				if err != nil {
					t.Fatalf("ValidateValue() error = %v", err)
				}
				return
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != test.code {
				t.Fatalf("ValidateValue() error = %v, want code %s", err, test.code)
			}
		})
	}
}

func TestParseID(t *testing.T) {
	got, err := ParseID(string(testPersonID))
	if err != nil || got != testPersonID {
		t.Fatalf("ParseID() = %q, %v", got, err)
	}
	for _, value := range []string{
		"019A2F00-0000-7000-8000-000000000002",
		"{019a2f00-0000-7000-8000-000000000002}",
		"urn:uuid:019a2f00-0000-7000-8000-000000000002",
		"019a2f00000070008000000000000002",
		"not-a-uuid",
		"550e8400-e29b-41d4-a716-446655440000",
	} {
		if _, err := ParseID(value); err == nil {
			t.Errorf("ParseID(%q) succeeded", value)
		}
	}
}
