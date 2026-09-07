package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func svixSecretAndSig(secretKey, id, ts string, body []byte) (secret, sig string) {
	secret = "whsec_" + base64.StdEncoding.EncodeToString([]byte(secretKey))
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	sig = "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return secret, sig
}

func TestVerifySvixSignatureValid(t *testing.T) {
	now := time.Now()
	id := "msg_1"
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"email.delivered"}`)
	secret, sig := svixSecretAndSig("supersecretkey", id, ts, body)

	if err := verifySvixSignature(secret, id, ts, sig, body, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifySvixSignatureTampered(t *testing.T) {
	now := time.Now()
	id := "msg_1"
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"email.delivered"}`)
	secret, sig := svixSecretAndSig("supersecretkey", id, ts, body)

	if err := verifySvixSignature(secret, id, ts, sig, []byte(`{"type":"email.bounced"}`), now); err == nil {
		t.Fatal("tampered body accepted; want error")
	}
}

func TestVerifySvixSignatureStale(t *testing.T) {
	now := time.Now()
	id := "msg_1"
	ts := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	body := []byte(`{"type":"email.delivered"}`)
	secret, sig := svixSecretAndSig("supersecretkey", id, ts, body)

	if err := verifySvixSignature(secret, id, ts, sig, body, now); err == nil {
		t.Fatal("stale timestamp accepted; want error")
	}
}

func TestVerifySvixSignatureMissingHeaders(t *testing.T) {
	if err := verifySvixSignature("whsec_x", "", "", "", nil, time.Now()); err == nil {
		t.Fatal("missing headers accepted; want error")
	}
}
