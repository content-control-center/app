package handlers

import "encoding/json"

// Optional distinguishes the three JSON states a request field can be in:
// absent, explicit null, and a concrete value. A plain pointer field collapses
// the first two into nil, so a full-replace PUT cannot tell "leave this alone"
// (key omitted) apart from "clear this" (key set to null).
//
// That distinction only matters for fields the server writes on its own, with
// no client in the loop: the brand refs (CON-245) are stamped by content_plan /
// draft_post, so an ordinary save that omits them would otherwise null a value
// the client may never have seen. Optional keeps absent and null apart so those
// fields survive omission. Use it sparingly — client-authored fields are
// correctly full-replace and should stay plain.
//
// The zero value is the absent state (Present=false), which is what stdlib
// encoding/json leaves a field at when its key is missing; a present key —
// including one whose value is null — invokes UnmarshalJSON and sets Present.
type Optional[T any] struct {
	Present bool // the key appeared in the request body
	Value   *T   // nil when the value was explicit null
}

// UnmarshalJSON records that the key was present and captures its value (nil for
// an explicit null). encoding/json only calls this for keys that appear in the
// body, so an omitted key leaves the zero value (Present=false).
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Present = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// applyTo overwrites *dst only when the field was present in the body: an
// omitted key leaves the existing value untouched, while a present value or an
// explicit null replaces it (the null clearing the ref).
func (o Optional[T]) applyTo(dst **T) {
	if o.Present {
		*dst = o.Value
	}
}
