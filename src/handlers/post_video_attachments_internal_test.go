package handlers

import (
	"testing"

	"github.com/ogen-app/ogen/src/videoclient"
)

func TestContainerToVideoMIME(t *testing.T) {
	cases := map[string]string{
		"mp4":                       "video/mp4",
		"mov,mp4,m4a,3gp,3g2,mj2":   "video/mp4", // ffprobe's mp4 format_name
		"mov":                       "video/mp4", // quicktime demuxes to the mp4 family
		"matroska,webm":             "video/webm",
		"webm":                      "video/webm",
		"matroska":                  "video/x-matroska",
		"avi":                       "video/x-msvideo",
		"mpegts":                    "video/mpeg",
		"3gp":                       "video/3gpp",
		"":                          "",
		"totally-unknown-container": "",
	}
	for in, want := range cases {
		if got := containerToVideoMIME(in); got != want {
			t.Errorf("containerToVideoMIME(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVideoContentTypeToExt_Roundtrip(t *testing.T) {
	// Every mapped content type must round-trip back to a video MIME via the
	// extension fallback, so a disabled-probe finalize still classifies.
	for _, ct := range []string{"video/mp4", "video/quicktime", "video/webm", "video/x-msvideo", "video/x-matroska", "video/mpeg", "video/3gpp"} {
		ext := videoContentTypeToExt(ct)
		if ext == ".bin" {
			t.Errorf("videoContentTypeToExt(%q) unexpectedly unmapped", ct)
		}
		if got := extToVideoMIME(ext); got == "" {
			t.Errorf("extToVideoMIME(%q) empty for content type %q", ext, ct)
		}
	}
	if videoContentTypeToExt("video/unknown") != ".bin" {
		t.Error("unknown video content type should map to .bin")
	}
}

func TestResolveVideoMIME_Priority(t *testing.T) {
	// Probe container wins when present.
	if got := resolveVideoMIME(&videoclient.ProbeResult{Container: "matroska,webm"}, "video/mp4", "k.mp4"); got != "video/webm" {
		t.Errorf("probe container should win, got %q", got)
	}
	// Falls back to stored content-type when probe is absent.
	if got := resolveVideoMIME(nil, "video/quicktime", "k.bin"); got != "video/quicktime" {
		t.Errorf("stored content-type fallback failed, got %q", got)
	}
	// Falls back to key extension when neither probe nor content-type is a video.
	if got := resolveVideoMIME(nil, "application/octet-stream", "path/to/clip.webm"); got != "video/webm" {
		t.Errorf("extension fallback failed, got %q", got)
	}
	// Nothing identifies a video → empty (caller returns 415).
	if got := resolveVideoMIME(nil, "", "clip.bin"); got != "" {
		t.Errorf("unidentifiable video should resolve empty, got %q", got)
	}
}
