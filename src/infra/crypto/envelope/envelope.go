package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// AlgorithmAES256GCM is the only algorithm name written to the
// `secret.algorithm` column today. Persisted alongside each row so a
// future migration can recognise pre-rotation rows and dispatch to a
// matching unwrapper without ambiguity.
const AlgorithmAES256GCM = "AES-256-GCM"

// CurrentKEKVersion is the integer written to `secret.kek_version` on
// new rows. Bump on KEK rotation; old rows stay decryptable as long as
// the keyring still holds the matching version.
const CurrentKEKVersion = 1

// gcmNonceSize is the AES-GCM standard nonce length. crypto/cipher's
// GCM construction will reject anything else, but we hard-code it so
// the wire format is locked rather than implicit.
const gcmNonceSize = 12

// ErrAuthFailed is returned when AES-GCM rejects a ciphertext — either
// the bytes were tampered with, the wrong KEK was used, or the wrapped
// DEK doesn't actually unwrap to the DEK that produced this ciphertext.
// We never return the underlying GCM error verbatim because Go's
// stdlib message ("cipher: message authentication failed") is fine to
// surface but we want a single typed error callers can switch on.
var ErrAuthFailed = errors.New("envelope: authentication failed")

// ErrMalformed is returned when a Record has structurally wrong sizes
// (nonce length, missing fields). Distinguishes "bad row" from "wrong
// KEK" so an operator looking at the failure can tell whether to
// re-seed the row or recover the KEK.
var ErrMalformed = errors.New("envelope: malformed record")

// ErrAlgorithmUnsupported is returned when a row's algorithm column
// names something this build doesn't know how to decrypt. Reserved for
// the rotation seam — today only AlgorithmAES256GCM is recognised.
var ErrAlgorithmUnsupported = errors.New("envelope: unsupported algorithm")

// Record is the wire format persisted to the secret table. Every field
// is needed for decryption; losing any one bricks the row. Algorithm
// and KEKVersion are forward-compat seams (see package doc).
type Record struct {
	Ciphertext []byte
	Nonce      []byte
	WrappedDEK []byte
	DEKNonce   []byte
	Algorithm  string
	KEKVersion int
}

// Cipher binds a KEK to the Encrypt/Decrypt operations. One Cipher per
// process is the intended usage — the KEK is held in memory for the
// process lifetime. All methods are safe for concurrent use because
// the underlying cipher.Block is stateless.
type Cipher struct {
	kekBlock cipher.Block
	kekGCM   cipher.AEAD
}

// NewCipher constructs a Cipher from a 32-byte KEK. Returns an error
// if the key length is wrong; the caller should fail boot.
func NewCipher(kek []byte) (*Cipher, error) {
	if len(kek) != KeySize {
		return nil, fmt.Errorf("envelope: KEK must be %d bytes, got %d", KeySize, len(kek))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("envelope: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope: gcm: %w", err)
	}
	return &Cipher{kekBlock: block, kekGCM: gcm}, nil
}

// Encrypt produces a Record from plaintext. A fresh DEK and two fresh
// nonces are drawn from crypto/rand on every call — no nonce is ever
// reused under the same key, eliminating the catastrophic GCM nonce
// reuse failure mode by construction.
//
// Plaintext is treated as opaque bytes. Callers passing strings should
// scope the resulting record narrowly; we cannot zero the input string
// for them.
func (c *Cipher) Encrypt(plaintext []byte) (Record, error) {
	if c == nil {
		return Record{}, errors.New("envelope: nil Cipher")
	}

	dek := make([]byte, KeySize)
	if _, err := rand.Read(dek); err != nil {
		return Record{}, fmt.Errorf("envelope: generate DEK: %w", err)
	}

	dataBlock, err := aes.NewCipher(dek)
	if err != nil {
		return Record{}, fmt.Errorf("envelope: data cipher: %w", err)
	}
	dataGCM, err := cipher.NewGCM(dataBlock)
	if err != nil {
		return Record{}, fmt.Errorf("envelope: data gcm: %w", err)
	}

	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return Record{}, fmt.Errorf("envelope: nonce: %w", err)
	}
	ciphertext := dataGCM.Seal(nil, nonce, plaintext, nil)

	dekNonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(dekNonce); err != nil {
		return Record{}, fmt.Errorf("envelope: dek nonce: %w", err)
	}
	wrappedDEK := c.kekGCM.Seal(nil, dekNonce, dek, nil)

	return Record{
		Ciphertext: ciphertext,
		Nonce:      nonce,
		WrappedDEK: wrappedDEK,
		DEKNonce:   dekNonce,
		Algorithm:  AlgorithmAES256GCM,
		KEKVersion: CurrentKEKVersion,
	}, nil
}

// Decrypt is the inverse of Encrypt. Returns ErrAuthFailed for any
// AES-GCM authentication failure (tampered ciphertext, tampered
// wrapped DEK, wrong KEK), ErrMalformed for structural problems,
// ErrAlgorithmUnsupported for rows from a future build.
//
// We deliberately collapse "bad ciphertext" and "bad wrapped DEK" into
// a single ErrAuthFailed because distinguishing them would let an
// attacker probe which layer of the envelope they're attacking.
func (c *Cipher) Decrypt(r Record) ([]byte, error) {
	if c == nil {
		return nil, errors.New("envelope: nil Cipher")
	}
	if r.Algorithm != "" && r.Algorithm != AlgorithmAES256GCM {
		return nil, fmt.Errorf("%w: %s", ErrAlgorithmUnsupported, r.Algorithm)
	}
	if len(r.Nonce) != gcmNonceSize || len(r.DEKNonce) != gcmNonceSize {
		return nil, ErrMalformed
	}
	if len(r.WrappedDEK) == 0 || len(r.Ciphertext) == 0 {
		return nil, ErrMalformed
	}

	dek, err := c.kekGCM.Open(nil, r.DEKNonce, r.WrappedDEK, nil)
	if err != nil {
		return nil, ErrAuthFailed
	}
	if len(dek) != KeySize {
		return nil, ErrMalformed
	}

	dataBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope: data cipher: %w", err)
	}
	dataGCM, err := cipher.NewGCM(dataBlock)
	if err != nil {
		return nil, fmt.Errorf("envelope: data gcm: %w", err)
	}

	plaintext, err := dataGCM.Open(nil, r.Nonce, r.Ciphertext, nil)
	if err != nil {
		return nil, ErrAuthFailed
	}
	return plaintext, nil
}
