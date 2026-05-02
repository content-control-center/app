package zernio

import (
	"encoding/json"
	"testing"
)

// Zernio returns profileId as either a bare ObjectId string or a
// populated profile sub-document depending on populate state. Both
// shapes must decode to a usable string ID so the reconciler's
// per-profile filter works.
func TestAccountUnmarshalProfileIDBareString(t *testing.T) {
	raw := []byte(`{"_id":"acc1","platform":"linkedin","profileId":"p_test","username":"alice"}`)
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ProfileID != "p_test" {
		t.Errorf("ProfileID: got %q want p_test", a.ProfileID)
	}
	if a.ID != "acc1" || a.Platform != "linkedin" || a.Username != "alice" {
		t.Errorf("other fields wrong: %+v", a)
	}
}

func TestAccountUnmarshalProfileIDPopulatedObject(t *testing.T) {
	raw := []byte(`{
        "_id":"acc1",
        "platform":"linkedin",
        "profileId":{"_id":"p_test","name":"Ogen integration","color":"#ff00ff"},
        "username":"alice"
    }`)
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ProfileID != "p_test" {
		t.Errorf("ProfileID: got %q want p_test (from populated _id)", a.ProfileID)
	}
}

func TestAccountUnmarshalMissingProfileID(t *testing.T) {
	raw := []byte(`{"_id":"acc1","platform":"linkedin"}`)
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ProfileID != "" {
		t.Errorf("ProfileID: got %q want empty string", a.ProfileID)
	}
}

func TestAccountUnmarshalNullProfileID(t *testing.T) {
	raw := []byte(`{"_id":"acc1","platform":"linkedin","profileId":null}`)
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ProfileID != "" {
		t.Errorf("ProfileID: got %q want empty string", a.ProfileID)
	}
}

func TestAccountUnmarshalMalformedProfileID(t *testing.T) {
	// A number is neither a string nor a {_id} object — should error.
	raw := []byte(`{"_id":"acc1","platform":"linkedin","profileId":42}`)
	var a Account
	if err := json.Unmarshal(raw, &a); err == nil {
		t.Fatalf("expected error on numeric profileId, got %+v", a)
	}
}
