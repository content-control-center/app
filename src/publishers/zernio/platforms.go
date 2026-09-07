package zernio

// SupportedPlatform describes one entry in the Phase 1 allowlist. The
// Zernio API uses its own platform identifier on the wire (e.g.
// "twitter") which doesn't always match Ogen's local platforms table
// (which uses "x-twitter"). OgenID is the join key into the existing
// platforms row so the GET /platforms endpoint can attach the
// authoritative supportedPostTypes list.
type SupportedPlatform struct {
	ZernioID string // identifier sent to Zernio (POST body, list filter)
	Label    string // human label for picker UI
	OgenID   string // primary key in Ogen's platforms table

	// SupportedPostTypes is the subset of Ogen post-type slugs (as
	// keys of models.Platform.PostTypes) that Zernio can publish to
	// for this platform. Curated by hand from Zernio's per-platform
	// docs because Zernio's API does not expose a programmatic
	// capability matrix; refresh whenever Zernio adds support for a
	// new format.
	SupportedPostTypes []string
}

// supportedPlatforms is the Phase 1 allowlist from the ticket.
//
// Adding a platform here requires (1) confirming Zernio supports it,
// (2) confirming its connect flow is redirect-based — Bluesky is
// excluded because it requires app-password credentials, breaking the
// "open URL in browser" UX, (3) ensuring the Ogen platforms table has
// a corresponding row.
//
// The SupportedPostTypes lists are best-effort starting points based
// on each platform's standard OAuth posting capabilities. Verify
// against Zernio's platform docs before relying on a slug — adding a
// post type Zernio doesn't actually support will fail at publish time.
var supportedPlatforms = []SupportedPlatform{
	{
		ZernioID: "twitter", Label: "X (Twitter)", OgenID: "x-twitter",
		SupportedPostTypes: []string{"text-post", "image-post", "video", "thread"},
	},
	{
		ZernioID: "linkedin", Label: "LinkedIn", OgenID: "linkedin",
		SupportedPostTypes: []string{"text-post", "image-post", "carousel", "video", "article"},
	},
	{
		ZernioID: "facebook", Label: "Facebook", OgenID: "facebook",
		SupportedPostTypes: []string{"text-post", "image-post", "video", "reel", "link-post"},
	},
	{
		ZernioID: "instagram", Label: "Instagram", OgenID: "instagram",
		SupportedPostTypes: []string{"image-post", "carousel", "reel", "story"},
	},
	{
		ZernioID: "youtube", Label: "YouTube", OgenID: "youtube",
		SupportedPostTypes: []string{"video", "short"},
	},
	{
		ZernioID: "threads", Label: "Threads", OgenID: "threads",
		// "thread" (CON-284): Threads publishes native reply chains via the same
		// platformSpecificData.threadItems payload as X.
		SupportedPostTypes: []string{"text-post", "image-post", "carousel", "video", "thread"},
	},
}

// SupportedPlatforms returns a defensive copy of the allowlist.
func SupportedPlatforms() []SupportedPlatform {
	out := make([]SupportedPlatform, len(supportedPlatforms))
	copy(out, supportedPlatforms)
	return out
}

// LookupSupportedPlatform returns the allowlist entry for zernioID, or
// nil when the platform is not in the Phase 1 allowlist.
func LookupSupportedPlatform(zernioID string) *SupportedPlatform {
	for i := range supportedPlatforms {
		if supportedPlatforms[i].ZernioID == zernioID {
			return &supportedPlatforms[i]
		}
	}
	return nil
}

// LookupSupportedByOgenID returns the allowlist entry whose OgenID
// matches, or nil. Used by the publishers adapter to enrich the
// /api/platforms response.
func LookupSupportedByOgenID(ogenID string) *SupportedPlatform {
	for i := range supportedPlatforms {
		if supportedPlatforms[i].OgenID == ogenID {
			return &supportedPlatforms[i]
		}
	}
	return nil
}

// sqidToZernioID maps the post-Sqid-migration platform.id (the
// 12-character Sqid) to its ZernioID. Migration
// 20240115000001_fix_platform_ids_to_sqids.up.sql is the source of
// truth for this mapping; CON-69 needs it because the post row
// carries a Sqid but Zernio's API + the auto-publish allowlist key
// off the Zernio platform name.
var sqidToZernioID = map[string]string{
	"AXqWG7U2qnpt": "linkedin",
	"8S8bWQTG6qD":  "youtube",
	"zBU1zqVICGfk": "facebook",
	"81mUCmc2xsKd": "twitter",
	"pQ4yxT3SuE57": "threads",
	"rzgpTkARLH0L": "instagram",
}

// LookupSupportedBySqid returns the allowlist entry that corresponds
// to a platform.id (Sqid form, post-CON-65 migration), or nil when
// the Sqid is unknown.
func LookupSupportedBySqid(sqid string) *SupportedPlatform {
	zernioID, ok := sqidToZernioID[sqid]
	if !ok {
		return nil
	}
	return LookupSupportedPlatform(zernioID)
}

// LookupSqidByZernioID returns the platform.id Sqid for a Zernio
// platform identifier (the inverse of the sqidToZernioID map), or ""
// when the Zernio id has no mapped Sqid. CON-130 uses it to resolve
// the {platform:"linkedin"} convert-to-manual request — which speaks
// Zernio ids like the allowlist does — to the Sqid stored on posts.
func LookupSqidByZernioID(zernioID string) string {
	for sqid, zid := range sqidToZernioID {
		if zid == zernioID {
			return sqid
		}
	}
	return ""
}
