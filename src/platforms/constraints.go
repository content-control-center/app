// Package platforms holds the per-platform attachment validator used
// by both the soft pre-check that surfaces warnings on attachment
// mutations and the hard validation that runs immediately before a
// Zernio publish call (CON-73).
//
// Constraint values themselves live on the `platforms` row — see
// models.Platform.ImageConstraints. This package consumes those rules;
// it does not own them.
package platforms

import "strings"

// Validation rule identifiers. They are stable strings so the frontend
// and audit log can switch on them without reading the human message.
const (
	RuleMaxFileSize         = "max_file_size_bytes"
	RuleAllowedFormat       = "allowed_formats"
	RuleAnimatedGIF         = "animated_gif_supported"
	RuleMaxAttachmentsCount = "max_attachments_per_post"
	RuleMaxPages            = "max_pages"             // CON-75: PDF page-count cap
	RuleAttachmentMix       = "attachment_kind_mix"   // CON-75: image+PDF on the same post
	RulePDFNotSupported     = "pdf_not_supported"     // CON-75: platform has no PDF rules
	RulePostTypeUnknown     = "post_type_unknown"     // CON-74: slug not in Platform.PostTypes
	RuleRequiresContent     = "requires_content"      // CON-74: post type needs non-empty content
	RuleMinAttachments      = "min_attachments"       // CON-74: too few attachments for the type
	RuleMaxAttachments      = "max_attachments"       // CON-74: too many attachments for the type
	RuleAttachmentKind      = "attachment_kind"       // CON-74: wrong attachment kind for the type
)

// Attachment kinds, returned by AttachmentKind. PDF, image, and video
// attachments take different validation paths.
const (
	KindImage = "image"
	KindPDF   = "pdf"
	KindVideo = "video"
)

// AttachmentKind returns the kind label for the given MIME type.
// Unknown MIMEs return an empty string; the validator treats that as
// "unclassified" and skips kind-specific rules.
func AttachmentKind(mime string) string {
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return KindImage
	case "application/pdf":
		return KindPDF
	}
	if strings.HasPrefix(mime, "video/") {
		return KindVideo
	}
	return ""
}
