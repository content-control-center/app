package post_assistant

import (
	"testing"

	"github.com/ogen-app/ogen/src/domain/models"
)

func TestResolvePlatform(t *testing.T) {
	platforms := []models.Platform{
		{ID: "AXqWG7U2qnpt", Name: "LinkedIn"},
		{ID: "pQ4yxT3SuE57", Name: "Threads"},
	}

	cases := []struct {
		name    string
		ident   string
		want    string
		wantErr bool
	}{
		{"by exact id", "pQ4yxT3SuE57", "pQ4yxT3SuE57", false},
		{"by name", "Threads", "pQ4yxT3SuE57", false},
		{"by name case-insensitive", "threads", "pQ4yxT3SuE57", false},
		{"unknown", "TikTok", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePlatform(platforms, tc.ident)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolvePlatform(%q) expected error, got id %q", tc.ident, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePlatform(%q) unexpected error: %v", tc.ident, err)
			}
			if got != tc.want {
				t.Fatalf("resolvePlatform(%q) = %q, want %q", tc.ident, got, tc.want)
			}
		})
	}
}
