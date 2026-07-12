package imagegen

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/firebase/genkit/go/ai"
)

func TestValidateSize(t *testing.T) {
	const (
		flash        = "gemini-3.1-flash-image"
		pro          = "gemini-3-pro-image"
		proPreview   = "gemini-3-pro-image-preview"
		flashPreview = "gemini-3.1-flash-image-preview"
		unknown      = "some-other-model"
	)
	cases := []struct {
		name       string
		model      string
		ratio, res string
		wantErr    bool
	}{
		{"flash common ratio 1K", flash, "4:5", "1K", false},
		{"flash extra wide ratio", flash, "1:4", "1K", false},
		{"flash draft 512", flash, "4:5", "512", false},
		{"flash 2K", flash, "21:9", "2K", false},
		{"flash rejects 4K (pro-only)", flash, "16:9", "4K", true},
		{"pro 4K", pro, "16:9", "4K", false},
		{"pro rejects flash-only ratio", pro, "1:4", "1K", true},
		{"pro rejects 512 draft", pro, "1:1", "512", true},
		{"pro-preview keeps pro caps", proPreview, "16:9", "4K", false},
		{"flash-preview keeps flash caps", flashPreview, "1:4", "512", false},
		{"unknown model -> base caps ok", unknown, "1:1", "1K", false},
		{"unknown model rejects 4K", unknown, "1:1", "4K", true},
		{"unknown model rejects extra ratio", unknown, "1:4", "1K", true},
		{"invalid ratio", flash, "2:1", "1K", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSize(tc.model, tc.ratio, tc.res)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSize(%q,%q,%q) err=%v, wantErr=%v", tc.model, tc.ratio, tc.res, err, tc.wantErr)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Run("brand and subject", func(t *testing.T) {
		out, err := Render(PromptData{
			Platform: "Instagram", BrandCount: 3, HasSubject: true,
			SubjectDesc: "a ceramic mug", Instruction: "warm tones",
			AspectRatio: "4:5", Resolution: "1K",
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Instagram", "BRAND STYLE", "images 1–3", "POST SUBJECT", "a ceramic mug", "warm tones", "4:5", "1K"} {
			if !strings.Contains(out, want) {
				t.Errorf("prompt missing %q\n---\n%s", want, out)
			}
		}
	})

	t.Run("brand only, no subject", func(t *testing.T) {
		out, err := Render(PromptData{Platform: "X", BrandCount: 1, AspectRatio: "1:1", Resolution: "1K"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "BRAND STYLE") {
			t.Errorf("expected BRAND STYLE block\n%s", out)
		}
		if strings.Contains(out, "POST SUBJECT") {
			t.Errorf("did not expect POST SUBJECT with no subject\n%s", out)
		}
	})

	t.Run("no references", func(t *testing.T) {
		out, err := Render(PromptData{Platform: "LinkedIn", AspectRatio: "16:9", Resolution: "1K"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "BRAND STYLE") || strings.Contains(out, "POST SUBJECT") {
			t.Errorf("expected no reference blocks\n%s", out)
		}
		for _, want := range []string{"Task:", "Constraints:", "16:9"} {
			if !strings.Contains(out, want) {
				t.Errorf("prompt missing %q\n%s", want, out)
			}
		}
	})
}

func TestExtractImage(t *testing.T) {
	raw := []byte("\x89PNG\r\n\x1a\nfake-image-bytes")
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)

	t.Run("image present among text parts", func(t *testing.T) {
		resp := &ai.ModelResponse{Message: ai.NewUserMessage(
			ai.NewTextPart("Here is your image."),
			ai.NewMediaPart("image/png", dataURL),
		)}
		got, mime, err := extractImage(resp)
		if err != nil {
			t.Fatal(err)
		}
		if mime != "image/png" || string(got) != string(raw) {
			t.Fatalf("got mime=%q bytes=%q", mime, got)
		}
	})

	t.Run("text-only surfaces commentary", func(t *testing.T) {
		resp := &ai.ModelResponse{Message: ai.NewUserMessage(
			ai.NewTextPart("I can't generate that."),
		)}
		_, _, err := extractImage(resp)
		if err == nil || !strings.Contains(err.Error(), "can't generate") {
			t.Fatalf("expected commentary in error, got %v", err)
		}
	})

	t.Run("nil response", func(t *testing.T) {
		if _, _, err := extractImage(nil); err == nil {
			t.Fatal("expected error for nil response")
		}
	})
}

func TestMediaPartRoundTrip(t *testing.T) {
	raw := []byte{0, 1, 2, 3, 255, 128}
	part := mediaPart(ReferenceImage{MIMEType: "image/jpeg", Data: raw})
	got, mime, err := decodeDataURL(part)
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" || string(got) != string(raw) {
		t.Fatalf("round-trip got mime=%q bytes=%v", mime, got)
	}
}
