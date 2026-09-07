package platforms

import (
	"strings"
	"testing"

	"github.com/ogen-app/ogen/src/domain/models"
)

// videoPlatform is a platform seeded with a representative video rule set
// (mirrors the Instagram-style seed: 1 GB / 900s / 3s floor / 1080p /
// vertical+square+wide, single attachment).
func videoPlatform() *models.Platform {
	return &models.Platform{
		ID:   "vidplat",
		Name: "VidPlat",
		VideoConstraints: models.VideoConstraints{
			MaxFileSizeBytes:      1 << 30, // 1 GiB
			AllowedFormats:        []string{"mp4", "mov"},
			MaxDurationSeconds:    900,
			MinDurationSeconds:    3,
			MaxWidth:              1920,
			MaxHeight:             1920,
			AllowedAspectRatios:   []string{"9:16", "1:1", "16:9"},
			MaxAttachmentsPerPost: 1,
		},
	}
}

func hasRule(errs []ValidationError, rule string) bool {
	for _, e := range errs {
		if e.Rule == rule {
			return true
		}
	}
	return false
}

func TestValidateVideoAttachment_NotSupported(t *testing.T) {
	// Platform with no video rules → single explicit not-supported warning.
	p := &models.Platform{ID: "img", Name: "ImgOnly", ImageConstraints: models.ImageConstraints{MaxFileSizeBytes: 1000, AllowedFormats: []string{"jpeg"}, MaxAttachmentsPerPost: 4}}
	att := &models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100}
	errs := ValidateAttachment(att, p)
	if len(errs) != 1 || !hasRule(errs, RuleVideoNotSupported) {
		t.Fatalf("want single video_not_supported, got %+v", errs)
	}
}

func TestValidateVideoAttachment_SizeAndFormat(t *testing.T) {
	p := videoPlatform()
	att := &models.PostAttachment{ID: "a1", MimeType: "video/x-matroska", SizeBytes: 2 << 30} // mkv, 2 GiB
	errs := validateVideoAttachment(att, p)
	if !hasRule(errs, RuleMaxFileSize) {
		t.Errorf("want max_file_size, got %+v", errs)
	}
	if !hasRule(errs, RuleAllowedFormat) {
		t.Errorf("want allowed_formats (mkv not allowed), got %+v", errs)
	}
}

func TestValidateVideoAttachment_Duration(t *testing.T) {
	p := videoPlatform()

	tooLong := &models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 1_000_000} // 1000s > 900
	if errs := validateVideoAttachment(tooLong, p); !hasRule(errs, RuleMaxDuration) {
		t.Errorf("want max_duration, got %+v", errs)
	}

	tooShort := &models.PostAttachment{ID: "a2", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 1_000} // 1s < 3s floor
	if errs := validateVideoAttachment(tooShort, p); !hasRule(errs, RuleMinDuration) {
		t.Errorf("want min_duration, got %+v", errs)
	}

	ok := &models.PostAttachment{ID: "a3", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000} // 30s
	if errs := validateVideoAttachment(ok, p); len(errs) != 0 {
		t.Errorf("want no duration errors for 30s, got %+v", errs)
	}
}

func TestValidateVideoAttachment_GracefulDegradation(t *testing.T) {
	// Unprobed upload (video-service unavailable): duration=0, dims=0.
	// Only size/format are checkable; duration/resolution/aspect skipped.
	p := videoPlatform()
	att := &models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100} // no duration, no dims
	if errs := validateVideoAttachment(att, p); len(errs) != 0 {
		t.Fatalf("unprobed valid video should pass, got %+v", errs)
	}
}

func TestValidateVideoAttachment_Resolution(t *testing.T) {
	p := videoPlatform()
	att := &models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, Width: 3840, Height: 2160, DurationMs: 30_000}
	if errs := validateVideoAttachment(att, p); !hasRule(errs, RuleMaxResolution) {
		t.Errorf("want max_resolution for 4K on 1080p platform, got %+v", errs)
	}
}

func TestValidateVideoAttachment_UnboundedAxisLabel(t *testing.T) {
	// Width unbounded (0), height capped: the violation message must render the
	// unbounded axis as "∞", not a misleading "0".
	p := videoPlatform()
	p.VideoConstraints.MaxWidth = 0
	p.VideoConstraints.MaxHeight = 1080
	att := &models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, Width: 9999, Height: 2160, DurationMs: 30_000}

	var res *ValidationError
	for _, e := range validateVideoAttachment(att, p) {
		if e.Rule == RuleMaxResolution {
			e := e
			res = &e
			break
		}
	}
	if res == nil {
		t.Fatal("want max_resolution error")
	}
	if !strings.Contains(res.Expected, "∞") || strings.Contains(res.Expected, "0x") {
		t.Errorf("unbounded width should render as ∞, got Expected=%q", res.Expected)
	}
	if !strings.Contains(res.Message, "∞") {
		t.Errorf("message should render unbounded width as ∞, got %q", res.Message)
	}
}

func TestValidateVideoAttachment_AspectRatio(t *testing.T) {
	p := videoPlatform()

	// 1080x1920 == 9:16 (allowed) — exact.
	if errs := validateVideoAttachment(&models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, Width: 1080, Height: 1920, DurationMs: 30_000}, p); hasRule(errs, RuleAspectRatio) {
		t.Errorf("9:16 should be allowed, got %+v", errs)
	}
	// 1920x1088 ~ 16:9 within tolerance — allowed.
	if errs := validateVideoAttachment(&models.PostAttachment{ID: "a2", MimeType: "video/mp4", SizeBytes: 100, Width: 1920, Height: 1088, DurationMs: 30_000}, p); hasRule(errs, RuleAspectRatio) {
		t.Errorf("1920x1088 should pass 16:9 within tolerance, got %+v", errs)
	}
	// 1920x800 (2.4:1 cinematic) — not allowed.
	if errs := validateVideoAttachment(&models.PostAttachment{ID: "a3", MimeType: "video/mp4", SizeBytes: 100, Width: 1920, Height: 800, DurationMs: 30_000}, p); !hasRule(errs, RuleAspectRatio) {
		t.Errorf("2.4:1 should be flagged, got %+v", errs)
	}
}

func TestValidatePostAttachments_VideoCountCap(t *testing.T) {
	p := videoPlatform()
	atts := []models.PostAttachment{
		{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000, Width: 1080, Height: 1920},
		{ID: "a2", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000, Width: 1080, Height: 1920},
	}
	errs := ValidatePostAttachments(atts, p)
	if !hasRule(errs, RuleMaxAttachmentsCount) {
		t.Fatalf("want max_attachments_per_post for 2 videos (cap 1), got %+v", errs)
	}
}

func TestValidatePostAttachments_ImageVideoMixNotFlagged(t *testing.T) {
	// story allows image+video; the attachment-level mix warning must not fire.
	p := videoPlatform()
	p.ImageConstraints = models.ImageConstraints{MaxFileSizeBytes: 1 << 20, AllowedFormats: []string{"jpeg"}, MaxAttachmentsPerPost: 1}
	atts := []models.PostAttachment{
		{ID: "a1", MimeType: "image/jpeg", SizeBytes: 100},
		{ID: "a2", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000, Width: 1080, Height: 1920},
	}
	if errs := ValidatePostAttachments(atts, p); hasRule(errs, RuleAttachmentMix) {
		t.Fatalf("image+video must not be flagged as a mix, got %+v", errs)
	}
}

func TestValidatePostAttachments_VideoPDFMixFlagged(t *testing.T) {
	p := videoPlatform()
	p.PDFConstraints = models.PDFConstraints{MaxFileSizeBytes: 1 << 20, AllowedFormats: []string{"pdf"}, MaxAttachmentsPerPost: 1}
	atts := []models.PostAttachment{
		{ID: "a1", MimeType: "application/pdf", SizeBytes: 100},
		{ID: "a2", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000, Width: 1080, Height: 1920},
	}
	if errs := ValidatePostAttachments(atts, p); !hasRule(errs, RuleAttachmentMix) {
		t.Fatalf("video+pdf must be flagged as a mix, got %+v", errs)
	}
}

func TestValidatePostType_RequiresVideoTitle(t *testing.T) {
	p := videoPlatform()
	p.Name = "YouTube"
	p.VideoConstraints.RequiresVideoTitle = true
	p.PostTypes = models.PostTypeMap{"video": "Video"}
	att := models.PostAttachment{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000, Width: 1080, Height: 1920}

	// Untitled YouTube video → blocked.
	untitled := &models.Post{PlatformPostType: "video", Title: ""}
	if errs := ValidatePostType(untitled, p, []models.PostAttachment{att}); !hasRule(errs, RuleRequiresVideoTitle) {
		t.Fatalf("untitled YouTube video must be blocked, got %+v", errs)
	}
	// Titled → passes the title rule.
	titled := &models.Post{PlatformPostType: "video", Title: "My clip"}
	if errs := ValidatePostType(titled, p, []models.PostAttachment{att}); hasRule(errs, RuleRequiresVideoTitle) {
		t.Fatalf("titled YouTube video must pass, got %+v", errs)
	}
	// A platform that doesn't require a title never blocks on it.
	p.VideoConstraints.RequiresVideoTitle = false
	if errs := ValidatePostType(untitled, p, []models.PostAttachment{att}); hasRule(errs, RuleRequiresVideoTitle) {
		t.Fatalf("non-requiring platform must not block on title, got %+v", errs)
	}
}

func TestValidateForPublish_IncludesVideoOnlyPlatform(t *testing.T) {
	// A platform with only video rules must appear in the publish map
	// (previously skipped because it had neither image nor PDF rules).
	p := videoPlatform()
	atts := []models.PostAttachment{{ID: "a1", MimeType: "video/mp4", SizeBytes: 100, DurationMs: 30_000, Width: 1080, Height: 1920}}
	out := ValidateForPublish(atts, []*models.Platform{p})
	if _, ok := out[p.ID]; !ok {
		t.Fatalf("video-only platform must be present in ValidateForPublish map, got keys %+v", out)
	}
}
