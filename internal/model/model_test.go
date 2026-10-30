package model

import (
	"errors"
	"testing"
)

const (
	testWorkspaceID    ID = "019a2f00-0000-7000-8000-000000000001"
	testPersonID       ID = "019a2f00-0000-7000-8000-000000000002"
	testSystemID       ID = "019a2f00-0000-7000-8000-000000000003"
	testContainerID    ID = "019a2f00-0000-7000-8000-000000000004"
	testComponentID    ID = "019a2f00-0000-7000-8000-000000000005"
	testRelationshipID ID = "019a2f00-0000-7000-8000-000000000006"
	testSecondRelID    ID = "019a2f00-0000-7000-8000-000000000007"
	testMissingID      ID = "019a2f00-0000-7000-8000-000000000099"
)

func validFixture() ([]Element, []Relationship) {
	// The child-first order proves that validation indexes the complete input
	// before resolving hierarchy references.
	elements := []Element{
		{
			ID:          testComponentID,
			Type:        ElementComponent,
			ParentID:    testContainerID,
			Name:        "API handler",
			Description: "Handles model operations.",
			Technology:  "Go",
			Tags:        []string{"Component", "Backend"},
			Properties:  map[string]string{"owner": "platform"},
			DSLID:       "apiHandler",
		},
		{
			ID:         testContainerID,
			Type:       ElementContainer,
			ParentID:   testSystemID,
			Name:       "Web application",
			Technology: "Go and React",
			Tags:       []string{"Container"},
			DSLID:      "webApplication",
		},
		{
			ID:       testPersonID,
			Type:     ElementPerson,
			Name:     "Architect",
			External: true,
			DSLID:    "architect",
		},
		{
			ID:          testSystemID,
			Type:        ElementSoftwareSystem,
			Name:        "c4-forge",
			Description: "Collaborative C4 modeling.",
			URL:         "https://example.test/c4-forge",
			DSLID:       "c4forge",
		},
	}
	relationships := []Relationship{
		{
			ID:            testRelationshipID,
			SourceID:      testPersonID,
			DestinationID: testSystemID,
			Description:   "Creates diagrams with",
			Technology:    "HTTPS",
			Tags:          []string{"Synchronous"},
		},
		{
			ID:            testSecondRelID,
			SourceID:      testPersonID,
			DestinationID: testSystemID,
			Description:   "Reviews models in",
		},
	}
	return elements, relationships
}

func TestNewAcceptsValidModel(t *testing.T) {
	elements, relationships := validFixture()
	got, err := New(testWorkspaceID, elements, relationships)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got.WorkspaceID() != testWorkspaceID {
		t.Errorf("WorkspaceID() = %s, want %s", got.WorkspaceID(), testWorkspaceID)
	}
	if len(got.Elements()) != 4 {
		t.Errorf("len(Elements()) = %d, want 4", len(got.Elements()))
	}
	if len(got.Relationships()) != 2 {
		t.Errorf("len(Relationships()) = %d, want 2", len(got.Relationships()))
	}
}

func TestModelOwnsDefensiveCopies(t *testing.T) {
	elements, relationships := validFixture()
	got, err := New(testWorkspaceID, elements, relationships)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	elements[0].Tags[0] = "mutated input"
	elements[0].Properties["owner"] = "mutated input"
	relationships[0].Tags[0] = "mutated input"

	component, ok := got.Element(testComponentID)
	if !ok {
		t.Fatal("Element(component) not found")
	}
	if component.Tags[0] != "Component" || component.Properties["owner"] != "platform" {
		t.Errorf("model changed through input: %+v", component)
	}
	component.Tags[0] = "mutated output"
	component.Properties["owner"] = "mutated output"

	again, _ := got.Element(testComponentID)
	if again.Tags[0] != "Component" || again.Properties["owner"] != "platform" {
		t.Errorf("model changed through accessor result: %+v", again)
	}
}

func TestNewRejectsInvalidModels(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ID, *[]Element, *[]Relationship)
		code   ErrorCode
		field  FieldName
	}{
		{
			name: "workspace is not UUIDv7",
			mutate: func(workspace *ID, _ *[]Element, _ *[]Relationship) {
				*workspace = "550e8400-e29b-41d4-a716-446655440000"
			},
			code:  ErrorInvalidValue,
			field: FieldID,
		},
		{
			name: "element ID is invalid",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].ID = "not-a-uuid"
			},
			code:  ErrorInvalidValue,
			field: FieldID,
		},
		{
			name: "unknown element type",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].Type = "database"
			},
			code:  ErrorInvalidValue,
			field: FieldType,
		},
		{
			name: "name is required",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].Name = "  "
			},
			code:  ErrorRequired,
			field: FieldNameValue,
		},
		{
			name: "DSL ID is required",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].DSLID = ""
			},
			code:  ErrorRequired,
			field: FieldDSLID,
		},
		{
			name: "duplicate entity ID",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[1].ID = (*elements)[0].ID
			},
			code:  ErrorDuplicateID,
			field: FieldID,
		},
		{
			name: "relationship ID duplicates element",
			mutate: func(_ *ID, elements *[]Element, relationships *[]Relationship) {
				(*relationships)[0].ID = (*elements)[0].ID
			},
			code:  ErrorDuplicateID,
			field: FieldID,
		},
		{
			name: "duplicate DSL ID",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[1].DSLID = (*elements)[0].DSLID
			},
			code:  ErrorDuplicateDSLID,
			field: FieldDSLID,
		},
		{
			name: "root element has parent",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[2].ParentID = testSystemID
			},
			code:  ErrorInvalidHierarchy,
			field: FieldParentID,
		},
		{
			name: "container has no parent",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[1].ParentID = ""
			},
			code:  ErrorRequired,
			field: FieldParentID,
		},
		{
			name: "container parent is missing",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[1].ParentID = testMissingID
			},
			code:  ErrorInvalidReference,
			field: FieldParentID,
		},
		{
			name: "container parent has wrong type",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[1].ParentID = testPersonID
			},
			code:  ErrorInvalidHierarchy,
			field: FieldParentID,
		},
		{
			name: "component parent has wrong type",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].ParentID = testSystemID
			},
			code:  ErrorInvalidHierarchy,
			field: FieldParentID,
		},
		{
			name: "person has technology",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[2].Technology = "Human"
			},
			code:  ErrorInvalidValue,
			field: FieldTechnology,
		},
		{
			name: "tag is empty",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].Tags = []string{"Component", " "}
			},
			code:  ErrorInvalidValue,
			field: FieldTags,
		},
		{
			name: "tag is duplicated",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].Tags = []string{"Component", "Component"}
			},
			code:  ErrorInvalidValue,
			field: FieldTags,
		},
		{
			name: "property key is empty",
			mutate: func(_ *ID, elements *[]Element, _ *[]Relationship) {
				(*elements)[0].Properties = map[string]string{" ": "value"}
			},
			code:  ErrorInvalidValue,
			field: FieldProperties,
		},
		{
			name: "relationship source is required",
			mutate: func(_ *ID, _ *[]Element, relationships *[]Relationship) {
				(*relationships)[0].SourceID = ""
			},
			code:  ErrorRequired,
			field: FieldSourceID,
		},
		{
			name: "relationship source is missing",
			mutate: func(_ *ID, _ *[]Element, relationships *[]Relationship) {
				(*relationships)[0].SourceID = testMissingID
			},
			code:  ErrorInvalidReference,
			field: FieldSourceID,
		},
		{
			name: "relationship destination is missing",
			mutate: func(_ *ID, _ *[]Element, relationships *[]Relationship) {
				(*relationships)[0].DestinationID = testMissingID
			},
			code:  ErrorInvalidReference,
			field: FieldDestinationID,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspaceID := testWorkspaceID
			elements, relationships := validFixture()
			test.mutate(&workspaceID, &elements, &relationships)

			_, err := New(workspaceID, elements, relationships)
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("New() error = %v, want *ValidationError", err)
			}
			if validation.Code != test.code || validation.Field != test.field {
				t.Errorf(
					"New() error = (%s, %s), want (%s, %s): %v",
					validation.Code,
					validation.Field,
					test.code,
					test.field,
					err,
				)
			}
		})
	}
}
