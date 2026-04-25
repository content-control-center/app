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
}

// supportedPlatforms is the Phase 1 allowlist from the ticket.
//
// Adding a platform here requires (1) confirming Zernio supports it,
// (2) confirming its connect flow is redirect-based — Bluesky is
// excluded because it requires app-password credentials, breaking the
// "open URL in browser" UX, (3) ensuring the Ogen platforms table has
// a corresponding row.
var supportedPlatforms = []SupportedPlatform{
	{ZernioID: "twitter", Label: "X (Twitter)", OgenID: "x-twitter"},
	{ZernioID: "linkedin", Label: "LinkedIn", OgenID: "linkedin"},
	{ZernioID: "facebook", Label: "Facebook", OgenID: "facebook"},
	{ZernioID: "instagram", Label: "Instagram", OgenID: "instagram"},
	{ZernioID: "youtube", Label: "YouTube", OgenID: "youtube"},
	{ZernioID: "threads", Label: "Threads", OgenID: "threads"},
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
