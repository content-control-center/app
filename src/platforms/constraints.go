// Package platforms holds the per-platform attachment validator used
// by both the soft pre-check that surfaces warnings on attachment
// mutations and the hard validation that runs immediately before a
// Zernio publish call (CON-73).
//
// Constraint values themselves live on the `platforms` row — see
// models.Platform.ImageConstraints. This package consumes those rules;
// it does not own them.
package platforms

// Validation rule identifiers. They are stable strings so the frontend
// and audit log can switch on them without reading the human message.
const (
	RuleMaxFileSize         = "max_file_size_bytes"
	RuleAllowedFormat       = "allowed_formats"
	RuleAnimatedGIF         = "animated_gif_supported"
	RuleMaxAttachmentsCount = "max_attachments_per_post"
)
