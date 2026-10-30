package model

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

type fixtureModel struct {
	Elements      []Element      `json:"elements"`
	Relationships []Relationship `json:"relationships"`
}

type operationFixture struct {
	Name       string      `json:"name"`
	Initial    string      `json:"initial"`
	Operations []Operation `json:"operations"`
	Expected   string      `json:"expected"`
	Expanded   []Operation `json:"expanded"`
	Error      *struct {
		Code  ErrorCode `json:"code"`
		Field FieldName `json:"field"`
	} `json:"error"`
}

func TestOperationFixtures(t *testing.T) {
	data, err := os.ReadFile("../../testdata/model-operations/suite.json")
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		WorkspaceID ID                      `json:"workspaceId"`
		Models      map[string]fixtureModel `json:"models"`
		Cases       []operationFixture      `json:"cases"`
	}
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if len(suite.Cases) == 0 {
		t.Fatal("fixture suite is empty")
	}
	snapshot := func(t *testing.T, name string) *Model {
		t.Helper()
		data, ok := suite.Models[name]
		if !ok {
			t.Fatalf("unknown fixture model %q", name)
		}
		result, err := New(suite.WorkspaceID, data.Elements, data.Relationships)
		if err != nil {
			t.Fatalf("fixture model %q: %v", name, err)
		}
		return result
	}
	for _, fixture := range suite.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			original := snapshot(t, fixture.Initial)
			before := snapshot(t, fixture.Initial)
			result, expanded, err := original.Apply(fixture.Operations)
			if !reflect.DeepEqual(original, before) {
				t.Fatal("Apply mutated original snapshot")
			}
			if fixture.Error != nil {
				var validation *ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("Apply error = %v, want ValidationError", err)
				}
				if validation.Code != fixture.Error.Code || validation.Field != fixture.Error.Field {
					t.Fatalf("Apply error = %v, want (%s, %s)", err, fixture.Error.Code, fixture.Error.Field)
				}
				if result != nil || expanded != nil {
					t.Fatal("rejected batch leaked candidate or expanded operations")
				}
				_, _, againErr := original.Apply(fixture.Operations)
				if againErr == nil || againErr.Error() != err.Error() {
					t.Fatalf("nondeterministic rejection: %v, then %v", err, againErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := snapshot(t, fixture.Expected)
			if !reflect.DeepEqual(result, expected) {
				t.Fatalf("unexpected result\nelements: %+v\nrelationships: %+v", result.Elements(), result.Relationships())
			}
			assertOperationJSON(t, expanded, fixture.Expanded)
			replay, _, err := original.Apply(expanded)
			if err != nil {
				t.Fatalf("replay: %v", err)
			}
			if !reflect.DeepEqual(replay, expected) {
				t.Fatal("replay differs from expected snapshot")
			}
			// Repeated application to the same source must not depend on map order.
			again, againExpanded, err := original.Apply(fixture.Operations)
			if err != nil || !reflect.DeepEqual(again, result) {
				t.Fatalf("nondeterministic Apply: %v", err)
			}
			assertOperationJSON(t, againExpanded, expanded)
		})
	}
}

func assertOperationJSON(t *testing.T, got, want []Operation) {
	t.Helper()
	// RawMessage preserves whitespace; compare decoded JSON values instead.
	decode := func(value []Operation) any {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	if !reflect.DeepEqual(decode(got), decode(want)) {
		t.Fatalf("expanded operations = %+v, want %+v", got, want)
	}
}

func TestApplyOwnsOperationAndSnapshotCopies(t *testing.T) {
	elements, relationships := validFixture()
	original, err := New(testWorkspaceID, elements, relationships)
	if err != nil {
		t.Fatal(err)
	}
	operations := []Operation{{Op: OperationSet, Kind: EntityElement, ID: testContainerID, Fields: map[FieldName]json.RawMessage{
		FieldTags: json.RawMessage(`["New"]`), FieldProperties: json.RawMessage(`{"owner":"new"}`),
	}}}
	result, expanded, err := original.Apply(operations)
	if err != nil {
		t.Fatal(err)
	}
	operations[0].Fields[FieldTags][2] = 'X'
	operations[0].Fields[FieldProperties] = json.RawMessage(`{}`)
	if string(expanded[0].Fields[FieldTags]) != `["New"]` {
		t.Fatal("expanded operation aliases input bytes")
	}
	expanded[0].Fields[FieldTags][2] = 'Y'
	resultElement, _ := result.Element(testContainerID)
	if resultElement.Tags[0] != "New" || resultElement.Properties["owner"] != "new" {
		t.Fatal("snapshot aliases operation fields")
	}
	resultElement.Tags[0] = "changed"
	resultElement.Properties["owner"] = "changed"
	originalElement, _ := original.Element(testContainerID)
	if originalElement.Tags[0] != "Container" || originalElement.Properties != nil {
		t.Fatal("result aliases original")
	}
}

func TestApplyRejectsMalformedRawJSON(t *testing.T) {
	original, err := New(testWorkspaceID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []json.RawMessage{nil, {}, []byte(`{"a":`)} {
		result, expanded, err := original.Apply([]Operation{{Op: OperationSet, Kind: EntityElement, ID: testPersonID, Fields: map[FieldName]json.RawMessage{FieldNameValue: raw}}})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != ErrorInvalidType || result != nil || expanded != nil {
			t.Fatalf("malformed raw JSON: %v", err)
		}
	}
}
