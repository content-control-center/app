package platforms

import (
	"fmt"
	"strings"

	"github.com/ogen-app/ogen/src/models"
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
	if p == nil {
		return nil
	}
	switch AttachmentKind(att.MimeType) {
	case KindImage:
		return validateImageAttachment(att, p)
	case KindPDF:
		return validatePDFAttachment(att, p)
	}
	return nil
}

func validateImageAttachment(att *models.PostAttachment, p *models.Platform) []ValidationError {
	if p.ImageConstraints.IsZero() {
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

func validatePDFAttachment(att *models.PostAttachment, p *models.Platform) []ValidationError {
	// Platforms that haven't opted into PDFs surface a single explicit
	// "not supported" warning rather than a cascade of size/format
	// failures from a zero-valued rule set.
	if p.PDFConstraints.IsZero() {
		return []ValidationError{{
			Platform:     p.ID,
			AttachmentID: att.ID,
			Rule:         RulePDFNotSupported,
			Expected:     "no PDF attachments",
			Actual:       "pdf",
			Message:      fmt.Sprintf("PDF attachments are not supported on %s", p.Name),
		}}
	}
	c := p.PDFConstraints
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
	if c.MaxPages > 0 && att.PageCount > c.MaxPages {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
			AttachmentID: att.ID,
			Rule:         RuleMaxPages,
			Expected:     fmt.Sprintf("<= %d", c.MaxPages),
			Actual:       fmt.Sprintf("%d", att.PageCount),
			Message:      fmt.Sprintf("PDF has %d pages; platform allows up to %d", att.PageCount, c.MaxPages),
		})
	}
	return errs
}

// ValidatePostAttachments runs rules for every attachment plus the
// post-level count rules (per kind) and the mixed-kind warning. A nil
// platform returns nil. A platform that defines neither image nor PDF
// rules also returns nil.
func ValidatePostAttachments(atts []models.PostAttachment, p *models.Platform) []ValidationError {
	if p == nil {
		return nil
	}
	hasImageRules := !p.ImageConstraints.IsZero()
	hasPDFRules := !p.PDFConstraints.IsZero()
	if !hasImageRules && !hasPDFRules {
		// Still surface the explicit PDF-not-supported warning if the
		// post has any PDFs — image-only platforms with zero image
		// rules are out of scope and stay silent.
		var errs []ValidationError
		for i := range atts {
			if AttachmentKind(atts[i].MimeType) == KindPDF {
				errs = append(errs, validatePDFAttachment(&atts[i], p)...)
			}
		}
		return errs
	}

	var errs []ValidationError

	images, pdfs := splitByKind(atts)

	if len(images) > 0 && len(pdfs) > 0 {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
			AttachmentID: "",
			Rule:         RuleAttachmentMix,
			Expected:     "images or PDF, not both",
			Actual:       fmt.Sprintf("%d image(s) + %d pdf(s)", len(images), len(pdfs)),
			Message:      "mixing image and PDF attachments on a single post is not supported",
		})
	}
	if hasImageRules && len(images) > p.ImageConstraints.MaxAttachmentsPerPost {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
			AttachmentID: "",
			Rule:         RuleMaxAttachmentsCount,
			Expected:     fmt.Sprintf("<= %d images", p.ImageConstraints.MaxAttachmentsPerPost),
			Actual:       fmt.Sprintf("%d images", len(images)),
			Message:      fmt.Sprintf("post has %d image attachments; platform allows up to %d", len(images), p.ImageConstraints.MaxAttachmentsPerPost),
		})
	}
	if hasPDFRules && len(pdfs) > p.PDFConstraints.MaxAttachmentsPerPost {
		errs = append(errs, ValidationError{
			Platform:     p.ID,
			AttachmentID: "",
			Rule:         RuleMaxAttachmentsCount,
			Expected:     fmt.Sprintf("<= %d pdf(s)", p.PDFConstraints.MaxAttachmentsPerPost),
			Actual:       fmt.Sprintf("%d pdf(s)", len(pdfs)),
			Message:      fmt.Sprintf("post has %d PDF attachments; platform allows up to %d", len(pdfs), p.PDFConstraints.MaxAttachmentsPerPost),
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
// Platforms with neither image nor PDF rules are absent from the
// returned map.
func ValidateForPublish(atts []models.PostAttachment, ps []*models.Platform) map[string][]ValidationError {
	out := map[string][]ValidationError{}
	for _, p := range ps {
		if p == nil {
			continue
		}
		if p.ImageConstraints.IsZero() && p.PDFConstraints.IsZero() {
			continue
		}
		out[p.ID] = ValidatePostAttachments(atts, p)
	}
	return out
}

func splitByKind(atts []models.PostAttachment) (images, pdfs []models.PostAttachment) {
	for i := range atts {
		switch AttachmentKind(atts[i].MimeType) {
		case KindImage:
			images = append(images, atts[i])
		case KindPDF:
			pdfs = append(pdfs, atts[i])
		}
	}
	return
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
	case "application/pdf":
		return "pdf"
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
