// Package platforms holds per-platform constraint maps used by both
// the soft pre-check that surfaces warnings on attachment mutations and
// the hard validation that runs immediately before a Zernio publish
// call (CON-73). The map is the single source of truth — adding a
// platform here is the only step needed to teach Ogen its image rules.
package platforms

// ImageConstraints captures the structured rules that apply to an
// image attachment for a given target platform. Add new fields here
// (alt text length, color profile, etc.) when the spec grows.
type ImageConstraints struct {
	MaxFileSizeBytes      int64
	AllowedFormats        []string // "jpeg", "png", "webp", "gif"
	AnimatedGIFSupported  bool
	MaxAttachmentsPerPost int
}

// ImageConstraintsByPlatform is keyed by Platform.ID (the Sqid stored
// in posts.platform_id). Numbers reflect the published platform docs
// at the time of writing; revisit per platform when their API
// changes.
//
// The Platform.ID values come from the seed migration
// 20240115000001_fix_platform_ids_to_sqids.up.sql.
var ImageConstraintsByPlatform = map[string]ImageConstraints{
	// LinkedIn
	"AXqWG7U2qnpt": {
		MaxFileSizeBytes:      10 << 20,
		AllowedFormats:        []string{"jpeg", "png", "gif"},
		AnimatedGIFSupported:  false,
		MaxAttachmentsPerPost: 9,
	},
	// YouTube — community posts only support a single image; treat
	// other post types the same for the MVP.
	"8S8bWQTG6qD": {
		MaxFileSizeBytes:      16 << 20,
		AllowedFormats:        []string{"jpeg", "png", "gif"},
		AnimatedGIFSupported:  true,
		MaxAttachmentsPerPost: 1,
	},
	// Facebook
	"zBU1zqVICGfk": {
		MaxFileSizeBytes:      30 << 20,
		AllowedFormats:        []string{"jpeg", "png", "gif", "webp"},
		AnimatedGIFSupported:  true,
		MaxAttachmentsPerPost: 10,
	},
	// X (Twitter)
	"81mUCmc2xsKd": {
		MaxFileSizeBytes:      5 << 20,
		AllowedFormats:        []string{"jpeg", "png", "webp", "gif"},
		AnimatedGIFSupported:  true,
		MaxAttachmentsPerPost: 4,
	},
	// Threads
	"pQ4yxT3SuE57": {
		MaxFileSizeBytes:      8 << 20,
		AllowedFormats:        []string{"jpeg", "png", "gif"},
		AnimatedGIFSupported:  true,
		MaxAttachmentsPerPost: 20,
	},
	// Instagram
	"rzgpTkARLH0L": {
		MaxFileSizeBytes:      8 << 20,
		AllowedFormats:        []string{"jpeg", "png"},
		AnimatedGIFSupported:  false,
		MaxAttachmentsPerPost: 20,
	},
}

// LookupImageConstraints returns the rule set for platformID, or
// (zero, false) when the platform is unknown. Callers treat unknown
// platforms as "no constraints to enforce" — a missing platform is a
// configuration gap, not a hard publish failure.
func LookupImageConstraints(platformID string) (ImageConstraints, bool) {
	c, ok := ImageConstraintsByPlatform[platformID]
	return c, ok
}

// MaxAttachmentsAcrossPlatforms returns the largest MaxAttachmentsPerPost
// among the supplied platform IDs. Used by the upload endpoint to gate
// the most permissive count when a post targets several platforms.
// Returns 0 when no known platform is supplied.
func MaxAttachmentsAcrossPlatforms(platformIDs []string) int {
	max := 0
	for _, id := range platformIDs {
		c, ok := ImageConstraintsByPlatform[id]
		if !ok {
			continue
		}
		if c.MaxAttachmentsPerPost > max {
			max = c.MaxAttachmentsPerPost
		}
	}
	return max
}
