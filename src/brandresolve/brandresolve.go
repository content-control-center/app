// Package brandresolve turns a (campaign, post) pair into the Brand material a
// content-writing Genkit flow should write under (CON-245), and renders it as a
// compact prompt block. It is the single place the resolution precedence of the
// PRD §5 lives, so content_plan, draft_post, post_assistant and campaign_assistant
// all agree on which voice/audience/guardrails apply.
//
// Resolution fails open: any repository error, or simply an empty Brand library,
// yields a Resolved that falls back to the campaign's legacy tone_guidelines /
// target_persona prose — so a workspace without Brand material generates exactly
// as it did before this feature.
package brandresolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// Resolved is the outcome of resolution. Any of Voice/Audience/Guardrails may be
// nil; the legacy strings carry the campaign's prose for the fallback path.
type Resolved struct {
	Voice         *models.BrandVoice
	Audience      *models.BrandAudience
	Guardrails    *models.BrandGuardrails
	LegacyTone    string // campaign.ToneGuidelines — used when Voice is nil
	LegacyPersona string // campaign.TargetPersona — used when Audience is nil
}

// Resolve applies the PRD §5 precedence:
//
//	voice:    post.BrandVoiceID → campaign.BrandVoiceID → workspace default → (legacy prose)
//	audience: post.BrandAudienceID → campaign.BrandAudienceID → (legacy prose)
//	guardrails: always the workspace singleton.
//
// post may be nil (campaign-level resolution, e.g. draft_post batches). It never
// returns a nil *Resolved on a nil error.
func Resolve(ctx context.Context, repo repository.BrandRepository, campaign *models.Campaign, post *models.Post) (*Resolved, error) {
	r := &Resolved{}
	if campaign != nil {
		r.LegacyTone = campaign.ToneGuidelines
		r.LegacyPersona = campaign.TargetPersona
	}
	if repo == nil {
		return r, nil
	}

	data, err := repo.GetAll(ctx)
	if err != nil {
		return r, err // fail open: r still carries the legacy prose fallback
	}

	// Voice: explicit ref (post, then campaign), else the workspace default.
	if id := firstRef(refOf(post), refOf(campaign)); id != "" {
		r.Voice = findVoice(data.Voices, id)
	}
	if r.Voice == nil {
		r.Voice = defaultVoice(data.Voices)
	}

	// Audience: explicit ref (post, then campaign). No workspace default.
	if id := firstRef(audRefOf(post), audRefOf(campaign)); id != "" {
		r.Audience = findAudience(data.Audiences, id)
	}

	r.Guardrails = data.Guardrails
	return r, nil
}

// VoiceID returns the resolved voice's id, or nil when resolution landed on the
// legacy-prose path — used to stamp posts.brand_voice_id at generation time.
func (r *Resolved) VoiceID() *string {
	if r == nil || r.Voice == nil {
		return nil
	}
	id := r.Voice.ID
	return &id
}

// PromptBlock renders the resolved material as a markdown block for injection
// into a generation prompt. platformID selects the voice's per-channel note.
// Returns "" only when there is genuinely nothing to say (no voice, no audience,
// no guardrails and no legacy prose).
func (r *Resolved) PromptBlock(platformID string) string {
	if r == nil {
		return ""
	}
	var b strings.Builder

	// ── Voice ──
	if r.Voice != nil {
		v := r.Voice
		fmt.Fprintf(&b, "## Brand voice — write in this voice\n**%s**", v.Name)
		if v.WhenToUse != "" {
			fmt.Fprintf(&b, " — %s", v.WhenToUse)
		}
		b.WriteString("\n")
		if len(v.Samples) > 0 {
			b.WriteString("\nThe samples ARE the voice — match their register, rhythm and habits:\n")
			for _, s := range v.Samples {
				fmt.Fprintf(&b, "- %s\n", oneLine(s))
			}
		}
		if rules := renderRules(v.Rules); rules != "" {
			fmt.Fprintf(&b, "\nRules: %s\n", rules)
		}
		if note := v.ChannelNotes[platformID]; note != "" {
			fmt.Fprintf(&b, "On this channel: %s\n", note)
		}
	} else if strings.TrimSpace(r.LegacyTone) != "" {
		fmt.Fprintf(&b, "## Tone guidelines\n%s\n", strings.TrimSpace(r.LegacyTone))
	}

	// ── Audience ──
	if r.Audience != nil {
		a := r.Audience
		fmt.Fprintf(&b, "\n## Audience — write for this reader\n**%s**", a.Name)
		if a.Who != "" {
			fmt.Fprintf(&b, ": %s", a.Who)
		}
		b.WriteString("\n")
		writeLine(&b, "Reads on", a.ReadsOn)
		writeLine(&b, "Scrolls past when", a.ScrollsPastWhen)
		writeLine(&b, "Believes a claim when", a.BelievesWhen)
	} else if strings.TrimSpace(r.LegacyPersona) != "" {
		fmt.Fprintf(&b, "\n## Target persona\n%s\n", strings.TrimSpace(r.LegacyPersona))
	}

	// ── Guardrails (always, voice-independent) ──
	if g := r.Guardrails; g != nil {
		b.WriteString("\n## Guardrails — non-negotiable, whichever voice writes\n")
		writeList(&b, "True (rest claims on these facts)", g.Facts)
		writeList(&b, "May claim", g.MayClaim)
		writeList(&b, "NEVER claim", g.NeverClaim)
		if len(g.BannedWords) > 0 {
			fmt.Fprintf(&b, "- Never use these words: %s\n", strings.Join(g.BannedWords, ", "))
		}
		if strings.TrimSpace(g.Disclaimer) != "" {
			fmt.Fprintf(&b, "- Carry this disclaimer verbatim: %s\n", strings.TrimSpace(g.Disclaimer))
		}
	}

	return strings.TrimSpace(b.String())
}

// ── helpers ──

func refOf(x any) string {
	switch v := x.(type) {
	case *models.Post:
		if v != nil && v.BrandVoiceID != nil {
			return *v.BrandVoiceID
		}
	case *models.Campaign:
		if v != nil && v.BrandVoiceID != nil {
			return *v.BrandVoiceID
		}
	}
	return ""
}

func audRefOf(x any) string {
	switch v := x.(type) {
	case *models.Post:
		if v != nil && v.BrandAudienceID != nil {
			return *v.BrandAudienceID
		}
	case *models.Campaign:
		if v != nil && v.BrandAudienceID != nil {
			return *v.BrandAudienceID
		}
	}
	return ""
}

func firstRef(ids ...string) string {
	for _, id := range ids {
		if id != "" {
			return id
		}
	}
	return ""
}

func findVoice(vs []models.BrandVoice, id string) *models.BrandVoice {
	for i := range vs {
		if vs[i].ID == id {
			return &vs[i]
		}
	}
	return nil
}

func defaultVoice(vs []models.BrandVoice) *models.BrandVoice {
	for i := range vs {
		if vs[i].IsDefault {
			return &vs[i]
		}
	}
	return nil
}

func findAudience(as []models.BrandAudience, id string) *models.BrandAudience {
	for i := range as {
		if as[i].ID == id {
			return &as[i]
		}
	}
	return nil
}

func renderRules(r models.VoiceRules) string {
	var parts []string
	add := func(label, val string) {
		if val != "" {
			parts = append(parts, label+" "+val)
		}
	}
	add("emoji", r.Emoji)
	add("hashtags", r.Hashtags)
	add("formality", r.Formality)
	add("person", r.Person)
	add("length", r.Length)
	if r.Opening != "" {
		parts = append(parts, "opening: "+r.Opening)
	}
	if r.Closing != "" {
		parts = append(parts, "closing: "+r.Closing)
	}
	return strings.Join(parts, "; ")
}

func writeLine(b *strings.Builder, label, val string) {
	if strings.TrimSpace(val) != "" {
		fmt.Fprintf(b, "- %s: %s\n", label, strings.TrimSpace(val))
	}
}

func writeList(b *strings.Builder, label string, items models.StringSlice) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "- %s:\n", label)
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", oneLine(it))
	}
}

// oneLine collapses newlines so a pasted multi-line sample stays a single bullet.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
