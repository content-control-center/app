package templates

import (
	"strings"
	"testing"
)

// TestRenderConnectionExpiringStages proves the connection_expiring template
// (subject + html + text) parses under the custom delimiters and branches on
// Stage, interpolating the account / platform / reconnect vars for each stage.
func TestRenderConnectionExpiringStages(t *testing.T) {
	tmpl := defaultByKey(t, KeyConnectionExpiring)

	t.Run("expiring_soon", func(t *testing.T) {
		r, err := Render(tmpl, Data{
			Name:          "Ann",
			WorkspaceName: "Acme",
			Platform:      "LinkedIn",
			AccountName:   "acme-corp",
			Stage:         StageExpiringSoon,
			ExpiresAt:     "September 15, 2026",
			ExpiresIn:     "in 6 days",
			ReconnectURL:  "https://app.example/settings/accounts?reconnect=acc1",
		})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if !strings.Contains(r.Subject, "expires soon") {
			t.Errorf("expiring_soon subject: %q", r.Subject)
		}
		for _, body := range []string{r.HTML, r.Text} {
			for _, want := range []string{"LinkedIn", "acme-corp", "in 6 days", "reconnect=acc1"} {
				if !strings.Contains(body, want) {
					t.Errorf("expiring_soon body missing %q in %q", want, body)
				}
			}
			if strings.Contains(body, "has expired") {
				t.Errorf("expiring_soon body should not use the action-required copy: %q", body)
			}
		}
	})

	t.Run("action_required", func(t *testing.T) {
		r, err := Render(tmpl, Data{
			Name:          "Ann",
			WorkspaceName: "Acme",
			Platform:      "X (Twitter)",
			AccountName:   "acme",
			Stage:         StageActionRequired,
			ReconnectURL:  "https://app.example/settings/accounts?reconnect=acc2",
		})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if !strings.Contains(r.Subject, "Action needed") {
			t.Errorf("action_required subject: %q", r.Subject)
		}
		for _, body := range []string{r.HTML, r.Text} {
			for _, want := range []string{"X (Twitter)", "acme", "has expired", "reconnect=acc2"} {
				if !strings.Contains(body, want) {
					t.Errorf("action_required body missing %q in %q", want, body)
				}
			}
		}
	})
}
