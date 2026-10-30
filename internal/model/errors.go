package model

import "fmt"

// ErrorCode classifies model validation failures without requiring callers to
// parse error text.
type ErrorCode string

const (
	ErrorUnknownEntity    ErrorCode = "unknown_entity"
	ErrorUnknownField     ErrorCode = "unknown_field"
	ErrorRequired         ErrorCode = "required"
	ErrorInvalidType      ErrorCode = "invalid_type"
	ErrorInvalidValue     ErrorCode = "invalid_value"
	ErrorDuplicateID      ErrorCode = "duplicate_id"
	ErrorDuplicateDSLID   ErrorCode = "duplicate_dsl_id"
	ErrorInvalidReference ErrorCode = "invalid_reference"
	ErrorInvalidHierarchy ErrorCode = "invalid_hierarchy"
)

// ValidationError identifies the entity and field that violate a model rule.
type ValidationError struct {
	Code       ErrorCode
	EntityKind EntityKind
	EntityID   ID
	Field      FieldName
	Message    string
}

func (e *ValidationError) Error() string {
	location := string(e.EntityKind)
	if e.EntityID != "" {
		location += " " + string(e.EntityID)
	}
	if e.Field != "" {
		location += "." + string(e.Field)
	}
	return fmt.Sprintf("%s: %s: %s", location, e.Code, e.Message)
}

func validationError(
	code ErrorCode,
	entityKind EntityKind,
	entityID ID,
	field FieldName,
	format string,
	args ...any,
) error {
	return &ValidationError{
		Code:       code,
		EntityKind: entityKind,
		EntityID:   entityID,
		Field:      field,
		Message:    fmt.Sprintf(format, args...),
	}
}
