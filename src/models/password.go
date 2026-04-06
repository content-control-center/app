package models

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2Params holds the cost parameters used for hashing.
// These are stored inside the encoded hash so verification always uses
// the same parameters the hash was produced with.
type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultArgon2Params = &argon2Params{
	memory:      64 * 1024, // 64 MiB
	iterations:  3,
	parallelism: 2,
	saltLength:  16,
	keyLength:   32,
}

// HashPassword hashes password using argon2id and returns a PHC-encoded string
// that includes the salt and all cost parameters.
func HashPassword(password string) (string, error) {
	p := defaultArgon2Params

	salt := make([]byte, p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.memory, p.iterations, p.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword returns true when password matches the PHC-encoded hash.
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, hash, err := decodeArgon2Hash(encoded)
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	candidate := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)
	return subtle.ConstantTimeCompare(hash, candidate) == 1, nil
}

func decodeArgon2Hash(encoded string) (*argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// Expected format: ["", "argon2id", "v=19", "m=65536,t=3,p=2", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, nil, fmt.Errorf("invalid hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, fmt.Errorf("parse version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, nil, fmt.Errorf("unsupported argon2 version %d", version)
	}

	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return nil, nil, nil, fmt.Errorf("parse params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decode salt: %w", err)
	}
	p.saltLength = uint32(len(salt))

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decode hash: %w", err)
	}
	p.keyLength = uint32(len(hash))

	return &p, salt, hash, nil
}
