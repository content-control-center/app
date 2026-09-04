package handlers

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

// CON-251: mutatesLockedContent is the pure comparison behind the submitted-
// post 409 content lock. It must flag a change to any content-identity field
// (body/title/media/platform/post-type/sources) and ignore everything else,
// so a no-op save or a status-only transition off a submitted post still
// passes. Pure logic — no DB needed.
func TestMutatesLockedContent(t *testing.T) {
	base := func() *models.Post {
		return &models.Post{
			Content:          "hello world",
			Title:            "a title",
			PlatformID:       "plat_1",
			PlatformPostType: "feed",
			MediaURLs:        models.StringSlice{"a", "b"},
			UsedAssetIDs:     models.StringSlice{"src_1"},
		}
	}
	// reqFor builds a request that mirrors the post exactly (a no-op save). Sources
	// are presence-aware (CON-233), so a faithful mirror sends them present.
	reqFor := func(p *models.Post) postRequest {
		return postRequest{
			Content:          p.Content,
			Title:            p.Title,
			PlatformID:       p.PlatformID,
			PlatformPostType: p.PlatformPostType,
			MediaURLs:        p.MediaURLs,
			UsedAssetIDs:     present(p.UsedAssetIDs),
		}
	}

	p := base()
	noop := reqFor(p)
	if noop.mutatesLockedContent(p) {
		t.Fatal("an identical (no-op) request must not count as a content mutation")
	}

	// Each locked field, changed in isolation, must trip the lock.
	locked := []struct {
		name string
		edit func(r *postRequest)
	}{
		{"content", func(r *postRequest) { r.Content = "rewritten" }},
		{"title", func(r *postRequest) { r.Title = "new title" }},
		{"platform", func(r *postRequest) { r.PlatformID = "plat_2" }},
		{"post_type", func(r *postRequest) { r.PlatformPostType = "story" }},
		{"media_removed", func(r *postRequest) { r.MediaURLs = models.StringSlice{"a"} }},
		{"media_reordered", func(r *postRequest) { r.MediaURLs = models.StringSlice{"b", "a"} }},
		{"sources", func(r *postRequest) { r.UsedAssetIDs = present(models.StringSlice{"src_1", "src_2"}) }},
	}
	for _, tc := range locked {
		r := reqFor(p)
		tc.edit(&r)
		if !r.mutatesLockedContent(p) {
			t.Errorf("changing %s must count as a locked-content mutation", tc.name)
		}
	}

	// CON-233: a save that OMITS used_asset_ids preserves the set (the membership
	// endpoints own it), so it must not count as touching the locked sources —
	// even though the post has sources the request doesn't restate.
	omitsSources := reqFor(p)
	omitsSources.UsedAssetIDs = Optional[models.StringSlice]{} // key absent
	if omitsSources.mutatesLockedContent(p) {
		t.Error("omitting used_asset_ids must not count as a content mutation")
	}

	// Fields the lock deliberately does NOT own (date/account/CTA/notes/status
	// are governed elsewhere) must not trip it — that's what lets a status-only
	// unschedule through on a submitted post.
	unlocked := reqFor(p)
	unlocked.Status = models.PostStatusDraft
	unlocked.CTAUrl = "https://example.com"
	unlocked.TargetAudienceNotes = "reminder"
	unlocked.SocialAccountID = "acct_9"
	if unlocked.mutatesLockedContent(p) {
		t.Error("changing only non-locked fields (status/cta/notes/account) must not count as a content mutation")
	}
}
