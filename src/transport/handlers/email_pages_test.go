package handlers

import (
	"strings"
	"testing"
)

func TestUnsubscribePagesBranded(t *testing.T) {
	confirm := unsubscribeConfirmPage("tok123")
	for _, want := range []string{
		"#f2f1ef",                            // beige page
		"#151515",                            // ink / button
		"Unsubscribe from marketing emails?", // heading
		`action="/api/email/unsubscribe/confirm"`, // form target
		`value="tok123"`,           // token carried
		"text-transform:uppercase", // brand CTA
	} {
		if !strings.Contains(confirm, want) {
			t.Errorf("confirm page missing %q", want)
		}
	}

	ok := unsubscribeOKPage("tok123")
	if !strings.Contains(ok, `action="/api/email/resubscribe"`) || !strings.Contains(ok, `value="tok123"`) {
		t.Error("ok page missing the resubscribe form / token")
	}
	// The heading's apostrophe is HTML-escaped (You&#39;re), so match loosely.
	if !strings.Contains(ok, "unsubscribed") {
		t.Error("ok page missing heading")
	}

	// The token is HTML-escaped into the hidden field.
	if !strings.Contains(unsubscribeConfirmPage(`a"b`), `value="a&#34;b"`) {
		t.Error("token not HTML-escaped in the form")
	}

	if strings.Contains(resubscribeOKPage(), "<form") {
		t.Error("resubscribed page should carry no action form")
	}
	if !strings.Contains(unsubscribeInvalidPage(), "no longer valid") {
		t.Error("invalid page missing expected copy")
	}
}
