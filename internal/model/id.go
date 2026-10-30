package model

import (
	"fmt"

	"github.com/google/uuid"
)

// ID is the stable identifier of a workspace entity. ADR-0003 requires UUIDv7.
type ID string

// ParseID parses and validates a UUIDv7 entity identifier.
func ParseID(value string) (ID, error) {
	id := ID(value)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

// Validate checks that the identifier is an RFC 9562 UUIDv7.
func (id ID) Validate() error {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return fmt.Errorf("parse UUID: %w", err)
	}
	if parsed.Version() != 7 {
		return fmt.Errorf("UUID version is %d, want 7", parsed.Version())
	}
	if parsed.Variant() != uuid.RFC4122 {
		return fmt.Errorf("UUID variant is %s, want RFC 4122", parsed.Variant())
	}
	return nil
}
