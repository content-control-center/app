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

// CON-284: a thread's segment list is locked content too. A faithful mirror is
// a no-op; editing or adding a segment trips the lock. Pure logic — no DB.
func TestMutatesLockedContentThread(t *testing.T) {
	post := &models.Post{
		PlatformPostType: models.PostTypeThread,
		Content:          "root",
		ThreadSegments:   models.ThreadSegments{{Content: "root"}, {Content: "reply"}},
	}
	mirror := postRequest{
		PlatformPostType: post.PlatformPostType,
		Content:          post.Content,
		ThreadSegments:   post.ThreadSegments,
	}
	if mirror.mutatesLockedContent(post) {
		t.Fatal("mirroring the thread exactly must not count as a mutation")
	}

	edited := mirror
	edited.ThreadSegments = models.ThreadSegments{{Content: "root"}, {Content: "changed"}}
	if !edited.mutatesLockedContent(post) {
		t.Error("editing a thread segment must count as a locked-content mutation")
	}

	added := mirror
	added.ThreadSegments = models.ThreadSegments{{Content: "root"}, {Content: "reply"}, {Content: "third"}}
	if !added.mutatesLockedContent(post) {
		t.Error("adding a thread segment must count as a locked-content mutation")
	}
}

// CON-284: apply restamps Content from the root segment on a thread write, and
// demotion (empty segments) clears the segments while keeping the applied body.
func TestApplyThreadSegments(t *testing.T) {
	post := &models.Post{}
	// A thread write authors segments; Content is sent empty and derived.
	req := postRequest{
		PlatformPostType: models.PostTypeThread,
		Content:          "",
		ThreadSegments:   models.ThreadSegments{{Content: "root"}, {Content: "reply"}},
	}
	req.apply(post, models.PostStatusDraft, models.CTATypeNone)
	if post.Content != "root" {
		t.Errorf("thread apply: Content = %q, want root mirror %q", post.Content, "root")
	}
	if len(post.ThreadSegments) != 2 {
		t.Fatalf("thread apply: segments = %d, want 2", len(post.ThreadSegments))
	}

	// Demotion to a single-message type: segments cleared, body kept.
	demote := postRequest{PlatformPostType: "text-post", Content: "just this"}
	demote.apply(post, models.PostStatusDraft, models.CTATypeNone)
	if len(post.ThreadSegments) != 0 {
		t.Errorf("demote: segments = %d, want 0", len(post.ThreadSegments))
	}
	if post.Content != "just this" {
		t.Errorf("demote: Content = %q, want %q", post.Content, "just this")
	}

	// A non-thread request that still carries thread_segments must ignore them:
	// they are not persisted and must NOT overwrite the ordinary body with the
	// root segment (CON-284 — segments are meaningful only for a thread).
	stray := &models.Post{}
	strayReq := postRequest{
		PlatformPostType: "text-post",
		Content:          "ordinary body",
		ThreadSegments:   models.ThreadSegments{{Content: "root"}, {Content: "reply"}},
	}
	strayReq.apply(stray, models.PostStatusDraft, models.CTATypeNone)
	if len(stray.ThreadSegments) != 0 {
		t.Errorf("stray segments on non-thread: segments = %d, want 0", len(stray.ThreadSegments))
	}
	if stray.Content != "ordinary body" {
		t.Errorf("stray segments on non-thread: Content = %q, want %q (must not restamp from root)", stray.Content, "ordinary body")
	}
}
