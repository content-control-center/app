package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// UnsubscribeTokenMaxAge bounds how long a signed unsubscribe link stays valid.
// Generous by design — an unsubscribe capability is low-risk and links may sit
// in an inbox for a long time — but bounded so a leaked token can't authorise
// forever.
const UnsubscribeTokenMaxAge = 365 * 24 * time.Hour

// ErrInvalidToken / ErrExpiredToken are returned by VerifyUnsubscribe. Handlers
// map both to a generic 400 without echoing the address (no enumeration).
var (
	ErrInvalidToken = errors.New("email: invalid unsubscribe token")
	ErrExpiredToken = errors.New("email: expired unsubscribe token")
)

// SignUnsubscribe returns a URL-safe token authorising unsubscribing email from
// marketing mail. It is an HMAC-SHA256 over "email:issuedUnix" keyed by secret
// (email_link_secret), so it cannot be forged without the key. The caller
// should pass an already-normalised (lower-cased) address.
func SignUnsubscribe(secret, email string, issued time.Time) string {
	msg := email + ":" + strconv.FormatInt(issued.Unix(), 10)
	sig := signHMAC(secret, msg)
	return b64([]byte(msg)) + "." + b64(sig)
}

// VerifyUnsubscribe validates token against secret and returns the email it
// authorises. Fails closed on a bad signature, malformed payload, or a token
// older than UnsubscribeTokenMaxAge.
func VerifyUnsubscribe(secret, token string, now time.Time) (string, error) {
	dot := strings.IndexByte(token, '.')
	if dot <= 0 {
		return "", ErrInvalidToken
	}
	msgRaw, err := unb64(token[:dot])
	if err != nil {
		return "", ErrInvalidToken
	}
	sigRaw, err := unb64(token[dot+1:])
	if err != nil {
		return "", ErrInvalidToken
	}
	if !hmac.Equal(sigRaw, signHMAC(secret, string(msgRaw))) {
		return "", ErrInvalidToken
	}

	msg := string(msgRaw)
	colon := strings.LastIndexByte(msg, ':')
	if colon <= 0 {
		return "", ErrInvalidToken
	}
	email := msg[:colon]
	issuedUnix, err := strconv.ParseInt(msg[colon+1:], 10, 64)
	if err != nil {
		return "", ErrInvalidToken
	}
	if now.Sub(time.Unix(issuedUnix, 0)) > UnsubscribeTokenMaxAge {
		return "", ErrExpiredToken
	}
	return email, nil
}

func signHMAC(secret, msg string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}

func b64(b []byte) string            { return base64.RawURLEncoding.EncodeToString(b) }
func unb64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
