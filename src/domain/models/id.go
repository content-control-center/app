package models

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"github.com/sqids/sqids-go"
)

var sq *sqids.Sqids

func init() {
	s, err := sqids.New()
	if err != nil {
		panic(fmt.Sprintf("init sqids: %v", err))
	}
	sq = s
}

// NewID generates a unique Sqid from a cryptographically random uint64.
func NewID() (string, error) {
	var n uint64
	if err := binary.Read(rand.Reader, binary.BigEndian, &n); err != nil {
		return "", fmt.Errorf("generate random id: %w", err)
	}
	id, err := sq.Encode([]uint64{n})
	if err != nil {
		return "", fmt.Errorf("encode sqid: %w", err)
	}
	return id, nil
}
