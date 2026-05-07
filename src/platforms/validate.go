package platforms

import (
	"fmt"
	"strings"

	"github.com/content-control-center/app/src/models"
)

// Validation rule identifiers. They are stable strings so the frontend
// and audit log can switch on them without reading the human message.
const (
	RuleMaxFileSize         = "max_file_size_bytes"
	RuleAllowedFormat       = "allowed_formats"
	RuleAnimatedGIF         = "animated_gif_supported"
	RuleMaxAttachmentsCount = "max_attachments_per_post"
)

// ValidationError describes a single rule failure for one
// (attachment, platform) pair, exactly per CON-73 §2.4.
type ValidationError struct {
	Platform     string `json:"platform"`
	AttachmentID string `json:"attachment_id"`
	Rule         string `json:"rule"`
	Expected     string `json:"expected"`
	Actual       string `json:"actual"`
	Message      string `json:"message"`
}

// ValidateAttachment runs all rules for one (attachment, platform).
// Returns an empty slice when the attachment passes every rule.
// Unknown platforms are treated as "no rules" — the caller decides
// what to do with that.
func ValidateAttachment(att *models.PostAttachment, platformID string) []ValidationError {
	c, ok := LookupImageConstraints(platformID)
	if !ok {
		return nil
	}
	var errs []ValidationError
	if att.SizeBytes > c.MaxFileSizeBytes {
		errs = append(errs, ValidationError{
			Platform:     platformID,
			AttachmentID: att.ID,
			Rule:         RuleMaxFileSize,
			Expected:     fmt.Sprintf("<= %d", c.MaxFileSizeBytes),
			Actual:       fmt.Sprintf("%d", att.SizeBytes),
			Message:      fmt.Sprintf("file is %d bytes; platform allows up to %d", att.SizeBytes, c.MaxFileSizeBytes),
		})
	}
	format := mimeToFormat(att.MimeType)
	if !contains(c.AllowedFormats, format) {
		errs = append(errs, ValidationError{
			Platform:     platformID,
			AttachmentID: att.ID,
			Rule:         RuleAllowedFormat,
			Expected:     strings.Join(c.AllowedFormats, ","),
			Actual:       format,
			Message:      fmt.Sprintf("format %q is not allowed for this platform", format),
		})
	}
	if att.IsAnimated && !c.AnimatedGIFSupported {
		errs = append(errs, ValidationError{
			Platform:     platformID,
			AttachmentID: att.ID,
			Rule:         RuleAnimatedGIF,
			Expected:     "static",
			Actual:       "animated",
			Message:      "animated GIFs are not supported on this platform",
		})
	}
	return errs
}

// ValidateForPublish runs the publish-time hard check for every
// (attachment, platformID) pair across all targetPlatformIDs. The
// caller — the future Zernio publish path — uses the returned map to
// short-circuit publishing for any platform whose value is non-empty,
// while still proceeding with the platforms whose value is empty
// (CON-73 §2.4). Platforms not in the constraint table are absent
// from the returned map.
func ValidateForPublish(atts []models.PostAttachment, targetPlatformIDs []string) map[string][]ValidationError {
	out := map[string][]ValidationError{}
	for _, pid := range targetPlatformIDs {
		if _, ok := LookupImageConstraints(pid); !ok {
			continue
		}
		errs := ValidatePostAttachments(atts, pid)
		out[pid] = errs
	}
	return out
}

// ValidatePostAttachments runs rules for every (attachment, platformID)
// pair for one post, plus the count rule. Returns the flat list of
// failures; an empty slice means the post is publishable everywhere.
func ValidatePostAttachments(atts []models.PostAttachment, platformID string) []ValidationError {
	c, ok := LookupImageConstraints(platformID)
	if !ok {
		return nil
	}
	var errs []ValidationError
	if len(atts) > c.MaxAttachmentsPerPost {
		errs = append(errs, ValidationError{
			Platform:     platformID,
			AttachmentID: "",
			Rule:         RuleMaxAttachmentsCount,
			Expected:     fmt.Sprintf("<= %d", c.MaxAttachmentsPerPost),
			Actual:       fmt.Sprintf("%d", len(atts)),
			Message:      fmt.Sprintf("post has %d attachments; platform allows up to %d", len(atts), c.MaxAttachmentsPerPost),
		})
	}
	for i := range atts {
		errs = append(errs, ValidateAttachment(&atts[i], platformID)...)
	}
	return errs
}

func mimeToFormat(mime string) string {
	switch mime {
	case "image/jpeg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	}
	return mime
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
