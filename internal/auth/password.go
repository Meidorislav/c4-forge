package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      uint32 = 19 * 1024
	argonTime        uint32 = 2
	argonThreads     uint8  = 1
	passwordMaxBytes        = 1024
)

// commonPasswords is the 10k worst passwords list from OWASP Passfault,
// distributed under Apache-2.0; see PASSFAULT-LICENSE.txt.
//
//go:embed common-passwords.txt
var commonPasswords string

//go:embed long-common-passwords.txt
var longCommonPasswords string

var passwordBlocklist = func() map[string]struct{} {
	result := make(map[string]struct{})
	for _, line := range strings.Split(commonPasswords+"\n"+longCommonPasswords, "\n") {
		if line != "" {
			result[strings.ToLower(strings.TrimSuffix(line, "\r"))] = struct{}{}
		}
	}
	return result
}()

func validPassword(password string) bool {
	if len(password) > passwordMaxBytes || !utf8.ValidString(password) || utf8.RuneCountInString(password) < 15 {
		return false
	}
	lower := strings.ToLower(password)
	_, blocked := passwordBlocklist[lower]
	if blocked || repeatingPassword(lower) || sequentialDigits(lower) {
		return false
	}
	return true
}

func (s *Service) hashPassword(ctx context.Context, password string) (string, error) {
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func (s *Service) verifyPassword(ctx context.Context, password, encoded string) (bool, bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, false, errors.New("invalid password hash format")
	}
	var memory, iterations uint64
	var lanes uint64
	params := strings.Split(parts[3], ",")
	if len(params) != 3 || !strings.HasPrefix(params[0], "m=") || !strings.HasPrefix(params[1], "t=") || !strings.HasPrefix(params[2], "p=") {
		return false, false, errors.New("invalid password hash parameters")
	}
	var err error
	if memory, err = strconv.ParseUint(strings.TrimPrefix(params[0], "m="), 10, 32); err != nil {
		return false, false, err
	}
	if iterations, err = strconv.ParseUint(strings.TrimPrefix(params[1], "t="), 10, 32); err != nil {
		return false, false, err
	}
	if lanes, err = strconv.ParseUint(strings.TrimPrefix(params[2], "p="), 10, 8); err != nil {
		return false, false, err
	}
	// Reject corrupt or attacker-supplied parameters before allocating memory.
	if memory < 8 || memory > 256*1024 || iterations < 1 || iterations > 10 || lanes < 1 || lanes > 8 {
		return false, false, errors.New("password hash parameters out of bounds")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, false, errors.New("invalid password salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != 32 {
		return false, false, errors.New("invalid password hash")
	}
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	case <-ctx.Done():
		return false, false, ctx.Err()
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(lanes), 32)
	matches := subtle.ConstantTimeCompare(got, want) == 1
	needsUpgrade := memory < uint64(argonMemory) || iterations < uint64(argonTime) || lanes != uint64(argonThreads)
	return matches, needsUpgrade, nil
}

// Reject exact repeated patterns and digit sequences in addition to the
// bundled list, which mostly contains passwords shorter than our minimum.
func repeatingPassword(password string) bool {
	runes := []rune(password)
	for size := 1; size <= len(runes)/2; size++ {
		match := true
		for i := size; i < len(runes); i++ {
			if runes[i] != runes[i%size] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func sequentialDigits(password string) bool {
	for i := 0; i < len(password); i++ {
		if password[i] < '0' || password[i] > '9' {
			return false
		}
		if i > 0 && password[i] != '0'+(password[i-1]-'0'+1)%10 {
			return false
		}
	}
	return len(password) > 0
}
