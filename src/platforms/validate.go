package platforms

import (
	"fmt"
	"strings"

	"github.com/content-control-center/app/src/models"
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

// ValidateAttachment runs all rules carried by the platform row for
// one attachment. Returns an empty slice when the attachment passes
// every rule. A nil platform — e.g. a draft post that has not picked
// one yet — short-circuits to nil; callers treat that as "nothing to
// validate yet".
func ValidateAttachment(att *models.PostAttachment, p *models.Platform) []ValidationError {
	if p == nil || p.ImageConstraints.IsZero() {
		return nil
	}
	c := p.ImageConstraints
	var errs []ValidationError
	if att.SizeBytes > c.MaxFileSizeBytes {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
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
			Platform:     p.ID,
			AttachmentID: att.ID,
			Rule:         RuleAllowedFormat,
			Expected:     strings.Join(c.AllowedFormats, ","),
			Actual:       format,
			Message:      fmt.Sprintf("format %q is not allowed for this platform", format),
		})
	}
	if att.IsAnimated && !c.AnimatedGIFSupported {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
			AttachmentID: att.ID,
			Rule:         RuleAnimatedGIF,
			Expected:     "static",
			Actual:       "animated",
			Message:      "animated GIFs are not supported on this platform",
		})
	}
	return errs
}

// ValidatePostAttachments runs rules for every attachment plus the
// post-level count rule. A nil platform returns nil.
func ValidatePostAttachments(atts []models.PostAttachment, p *models.Platform) []ValidationError {
	if p == nil || p.ImageConstraints.IsZero() {
		return nil
	}
	c := p.ImageConstraints
	var errs []ValidationError
	if len(atts) > c.MaxAttachmentsPerPost {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
			AttachmentID: "",
			Rule:         RuleMaxAttachmentsCount,
			Expected:     fmt.Sprintf("<= %d", c.MaxAttachmentsPerPost),
			Actual:       fmt.Sprintf("%d", len(atts)),
			Message:      fmt.Sprintf("post has %d attachments; platform allows up to %d", len(atts), c.MaxAttachmentsPerPost),
		})
	}
	for i := range atts {
		errs = append(errs, ValidateAttachment(&atts[i], p)...)
	}
	return errs
}

// ValidateForPublish runs the publish-time hard check across every
// target platform. The future Zernio publish path uses the returned
// map to skip platforms whose value is non-empty while still
// proceeding with platforms whose value is empty (CON-73 §2.4).
// Platforms with no image rules (zero ImageConstraints) are absent
// from the returned map.
func ValidateForPublish(atts []models.PostAttachment, ps []*models.Platform) map[string][]ValidationError {
	out := map[string][]ValidationError{}
	for _, p := range ps {
		if p == nil || p.ImageConstraints.IsZero() {
			continue
		}
		out[p.ID] = ValidatePostAttachments(atts, p)
	}
	return out
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
