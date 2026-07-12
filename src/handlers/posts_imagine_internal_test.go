package handlers

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

func TestResolveAspectRatio(t *testing.T) {
	ig := &models.Platform{Name: "Instagram"}
	tt := &models.Platform{Name: "TikTok"}
	yt := &models.Platform{Name: "YouTube"}
	unknown := &models.Platform{Name: "Mastodon"}

	cases := []struct {
		name     string
		platform *models.Platform
		postType string
		override string
		want     string
	}{
		{"override wins over everything", ig, "story", "1:1", "1:1"},
		{"story post type -> vertical", ig, "story", "", "9:16"},
		{"reel post type -> vertical", nil, "Reel", "", "9:16"},
		{"short post type -> vertical", yt, "short", "", "9:16"},
		{"instagram feed default", ig, "feed", "", "4:5"},
		{"tiktok default", tt, "", "", "9:16"},
		{"youtube default", yt, "video", "", "16:9"},
		{"unknown platform -> 1:1", unknown, "", "", "1:1"},
		{"no platform -> 1:1", nil, "", "", "1:1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveAspectRatio(tc.platform, tc.postType, tc.override); got != tc.want {
				t.Errorf("resolveAspectRatio() = %q, want %q", got, tc.want)
			}
		})
	}
}
