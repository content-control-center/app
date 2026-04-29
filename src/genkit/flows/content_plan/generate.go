package content_plan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

// jsonPostScanner incrementally scans a stream of JSON text and yields complete
// top-level JSON objects (i.e. each element of the outer array) as they arrive.
type jsonPostScanner struct {
	buf      []byte
	depth    int
	inStr    bool
	escaped  bool
	objStart int // index of the opening '{' of the current object; -1 when not inside one
}

func newJSONPostScanner() *jsonPostScanner {
	return &jsonPostScanner{objStart: -1}
}

// push appends chunk to the internal buffer and returns all newly-complete
// JSON object strings found since the last call.
func (s *jsonPostScanner) push(chunk string) []string {
	var complete []string
	for i := 0; i < len(chunk); i++ {
		c := chunk[i]
		s.buf = append(s.buf, c)
		pos := len(s.buf) - 1

		if s.escaped {
			s.escaped = false
			continue
		}
		if s.inStr {
			switch c {
			case '\\':
				s.escaped = true
			case '"':
				s.inStr = false
			}
			continue
		}
		switch c {
		case '"':
			s.inStr = true
		case '{':
			if s.depth == 0 {
				s.objStart = pos
			}
			s.depth++
		case '}':
			s.depth--
			if s.depth == 0 && s.objStart >= 0 {
				complete = append(complete, string(s.buf[s.objStart:pos+1]))
				s.objStart = -1
			}
		}
	}
	return complete
}

func generatePosts(
	ctx context.Context,
	g *genkit.Genkit,
	campaign *models.Campaign,
	platforms []resolvedPlatform,
	assets []resolvedPiece,
	cfg ContentPlanFlowConfig,
	onEvent OnEventFunc,
) ([]DraftPost, []string, error) {
	estCount := 0
	if campaign.EstimatedPostCount != nil {
		estCount = *campaign.EstimatedPostCount
	}

	dayCount := int(campaign.EndDate.Sub(*campaign.StartDate).Hours() / 24)

	phases := make([]resolvedPhase, len(campaign.CampaignType.Phases))
	for i, p := range campaign.CampaignType.Phases {
		phases[i] = resolvedPhase{
			ID:       p.ID,
			Name:     p.Name,
			Purpose:  p.Purpose,
			Sequence: p.Sequence,
		}
	}

	data := contentPlanTemplateData{
		Name:                    campaign.Name,
		Description:             campaign.Description,
		CampaignTypeLabel:       campaign.CampaignType.Label,
		CampaignTypeDescription: campaign.CampaignType.Description,
		Phases:                  phases,
		TargetPersona:           campaign.TargetPersona,
		KeyMessages:             campaign.KeyMessages,
		ToneGuidelines:          campaign.ToneGuidelines,
		Language:                campaign.Language,
		StartDate:               campaign.StartDate.Format("2006-01-02"),
		EndDate:                 campaign.EndDate.Format("2006-01-02"),
		DayCount:                dayCount,
		EstimatedPostCount:      estCount,
		Platforms:               platforms,
		Assets:                  assets,
	}

	// System prompt is identical for every batch — render once.
	systemPrompt, err := renderTemplate(cfg.systemTmpl, data)
	if err != nil {
		return nil, nil, fmt.Errorf("render system prompt: %w", err)
	}
	log.Printf("content_plan: system prompt:\n%s", systemPrompt)

	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	maxPostsPerBatch := cfg.MaxPostsPerBatch
	if maxPostsPerBatch <= 0 {
		maxPostsPerBatch = 30
	}
	maxParallel := cfg.MaxParallelBatches
	if maxParallel <= 0 {
		maxParallel = 5
	}

	modelName := "anthropic/" + cfg.ModelID

	// Single-shot fallback: no estimated count, or fewer slots than a single
	// batch. We still go through the batched path for consistency, but with
	// a single batch covering everything.
	batches := planBatches(estCount, phases, platforms, *campaign.StartDate, *campaign.EndDate, maxPostsPerBatch)
	if len(batches) == 0 {
		// EstimatedPostCount is 0 or otherwise unplannable — preserve the
		// pre-CON-67 behaviour of asking the model to decide the count
		// from campaign context.
		userPrompt, err := renderTemplate(cfg.userTmpl, data)
		if err != nil {
			return nil, nil, fmt.Errorf("render user prompt: %w", err)
		}
		log.Printf("content_plan: user prompt (no batch plan):\n%s", userPrompt)
		posts, genErr := generatePostsStreaming(ctx, g, modelName, systemPrompt, userPrompt, maxTokens, 0, onEvent)
		if genErr != nil {
			return nil, nil, genErr
		}
		return posts, nil, nil
	}

	log.Printf("content_plan: planned %d batches (totalPosts=%d, K=%d, parallel=%d)",
		len(batches), estCount, maxPostsPerBatch, maxParallel)

	gen := func(ctx context.Context, spec batchSpec, emit OnEventFunc) ([]DraftPost, error) {
		batchData := data
		batchData.Batch = &spec
		userPrompt, err := renderTemplate(cfg.userTmpl, batchData)
		if err != nil {
			return nil, fmt.Errorf("render user prompt for batch %d: %w", spec.Index, err)
		}
		log.Printf("content_plan: batch %d/%d (posts=%d window=%s..%s) user prompt:\n%s",
			spec.Index+1, len(batches), spec.PostCount, spec.DateWindow.Start, spec.DateWindow.End, userPrompt)
		return generatePostsStreaming(ctx, g, modelName, systemPrompt, userPrompt, maxTokens, spec.GlobalStartIndex, emit)
	}
	return runBatchesParallel(ctx, batches, maxParallel, gen, onEvent)
}

// runBatchesParallel fans the planned batches out to up to maxParallel
// goroutines, serialising the optional emit callback so a single-writer SSE
// sink stays safe. Aggregation is partial-success: a per-batch failure
// becomes a warning and the surviving batches' posts are returned in batch
// order. When every batch fails the function returns an AIError so the
// caller can surface a hard "error" SSE event.
//
// Extracted for unit-testability — the production gen calls into Anthropic;
// tests pass a stub that simulates timing, partial failures, and emit
// concurrency without touching the network.
func runBatchesParallel(
	ctx context.Context,
	batches []batchSpec,
	maxParallel int,
	gen func(ctx context.Context, spec batchSpec, emit OnEventFunc) ([]DraftPost, error),
	onEvent OnEventFunc,
) ([]DraftPost, []string, error) {
	if maxParallel <= 0 {
		maxParallel = 1
	}

	// onEvent is called from per-batch goroutines; the SSE writer behind it
	// is single-writer, so we must serialise. The lock is held only for the
	// duration of one event emit — negligible contention.
	var emitMu sync.Mutex
	safeEmit := onEvent
	if onEvent != nil {
		safeEmit = func(name SSEEventKind, payload any) {
			emitMu.Lock()
			defer emitMu.Unlock()
			onEvent(name, payload)
		}
	}

	type batchResult struct {
		posts []DraftPost
		err   error
	}
	results := make([]batchResult, len(batches))

	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i := range batches {
		i := i
		spec := batches[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			posts, err := gen(ctx, spec, safeEmit)
			if err != nil {
				log.Printf("content_plan: batch %d/%d failed: %v", i+1, len(batches), err)
				results[i] = batchResult{err: err}
				return
			}
			log.Printf("content_plan: batch %d/%d done (%d posts)", i+1, len(batches), len(posts))
			results[i] = batchResult{posts: posts}
		}()
	}
	wg.Wait()

	var allPosts []DraftPost
	var warnings []string
	failures := 0
	for i, r := range results {
		if r.err != nil {
			failures++
			warnings = append(warnings, fmt.Sprintf(
				"batch %d/%d (slots %d-%d) failed: %v",
				i+1, len(batches),
				batches[i].GlobalStartIndex,
				batches[i].GlobalStartIndex+batches[i].PostCount-1,
				r.err,
			))
			continue
		}
		allPosts = append(allPosts, r.posts...)
	}

	if failures == len(batches) {
		return nil, warnings, &AIError{Msg: fmt.Sprintf("all %d batches failed; first error: %v", len(batches), results[0].err)}
	}
	return allPosts, warnings, nil
}

// generatePostsStreaming attempts a streaming model call and incrementally emits
// each parsed DraftPost via onEvent. On any stream failure it falls back to a
// blocking genkit.Generate call regardless of how many partial posts were already
// emitted — the blocking response is the canonical result sent in the "complete"
// event, while the streamed posts are treated as live previews by the client.
//
// globalStartIndex is the slot index assigned to the first post produced by
// this call; emitted PostEventPayload.Index values are globalStartIndex +
// within-batch offset. Single-shot callers pass 0; batched callers pass the
// batch's GlobalStartIndex so the UI can place posts in deterministic order.
func generatePostsStreaming(
	ctx context.Context,
	g *genkit.Genkit,
	modelName, systemPrompt, userPrompt string,
	maxOutputTokens int64,
	globalStartIndex int,
	onEvent OnEventFunc,
) ([]DraftPost, error) {
	modelCfg := ai.WithConfig(anthropic.MessageNewParams{
		MaxTokens: maxOutputTokens,
	})

	scanner := newJSONPostScanner()
	var posts []DraftPost
	var streamErr error
	var chunkCount int
	var totalBytes int

	for result, err := range genkit.GenerateStream(ctx, g,
		ai.WithModelName(modelName),
		ai.WithSystem(systemPrompt),
		ai.WithPrompt(userPrompt),
		modelCfg,
	) {
		if err != nil {
			streamErr = err
			break
		}
		if result.Done {
			if result.Response != nil {
				if result.Response.Usage != nil {
					u := result.Response.Usage
					log.Printf("content_plan: tokens — input=%d output=%d total=%d",
						u.InputTokens, u.OutputTokens, u.InputTokens+u.OutputTokens)
				}
				respText := result.Response.Text()
				log.Printf("content_plan: stream finish_reason=%q posts_parsed=%d chunks=%d bytes=%d response_len=%d response_tail=%s",
					result.Response.FinishReason, len(posts), chunkCount, totalBytes, len(respText), tailOf(respText, 200))
			}
			break
		}

		chunkCount++
		chunkText := result.Chunk.Text()
		totalBytes += len(chunkText)
		for _, raw := range scanner.push(chunkText) {
			post, ok := parseAndTrimPost(raw)
			if !ok {
				log.Printf("content_plan: malformed post chunk (raw=%.100s)", raw)
				continue
			}
			emit(onEvent, SSEEventPost, PostEventPayload{Post: post, Index: globalStartIndex + len(posts)})
			posts = append(posts, post)
		}
	}

	if streamErr == nil {
		return posts, nil
	}

	// Stream failed (connection dropped mid-generation). The partial posts already emitted as
	// SSE preview events are discarded here — the blocking fallback gives the complete set,
	// which the client uses as the canonical result via the "complete" event.
	log.Printf("content_plan: stream error after %d partial posts — falling back to blocking Generate: %v", len(posts), streamErr)
	resp, err := genkit.Generate(ctx, g,
		ai.WithModelName(modelName),
		ai.WithSystem(systemPrompt),
		ai.WithPrompt(userPrompt),
		modelCfg,
	)
	if err != nil {
		return nil, &AIError{Msg: fmt.Sprintf("model call failed (stream+fallback): %v", err)}
	}

	if resp.Usage != nil {
		log.Printf("content_plan: tokens (fallback) — input=%d output=%d total=%d",
			resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.InputTokens+resp.Usage.OutputTokens)
	}

	text := strings.TrimSpace(resp.Text())
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i >= 0 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	if err := json.Unmarshal([]byte(text), &posts); err != nil {
		return nil, &AIError{Msg: fmt.Sprintf("model response not valid JSON: %v\nraw: %.200s", err, text)}
	}

	// Emit post events for the fallback results so the client still sees them.
	for i := range posts {
		posts[i].Body = trimBody(posts[i].Body)
		emit(onEvent, SSEEventPost, PostEventPayload{Post: posts[i], Index: globalStartIndex + i})
	}

	return posts, nil
}

// parseAndTrimPost unmarshals a raw JSON object string into a DraftPost and
// trims the body to maxBodyRunes. Returns (post, true) on success.
func parseAndTrimPost(raw string) (DraftPost, bool) {
	var post DraftPost
	if err := json.Unmarshal([]byte(raw), &post); err != nil {
		log.Printf("content_plan: skipping malformed post chunk: %v", err)
		return DraftPost{}, false
	}
	post.Body = trimBody(post.Body)
	return post, true
}

// trimBody truncates body to maxBodyRunes Unicode code points.
func trimBody(body string) string {
	const maxBodyRunes = 500
	if runes := []rune(body); len(runes) > maxBodyRunes {
		return string(runes[:maxBodyRunes])
	}
	return body
}

func persistDraftPosts(ctx context.Context, posts []DraftPost, campaign *models.Campaign, postRepo repository.PostRepository) error {
	if len(posts) == 0 {
		return nil
	}

	records := make([]*models.Post, 0, len(posts))
	for _, dp := range posts {
		id, err := models.NewID()
		if err != nil {
			return err
		}

		var scheduledAt *time.Time
		if t, err := time.Parse("2006-01-02", dp.PublishDate); err == nil {
			scheduledAt = &t
		}

		var phaseID *string
		if dp.PhaseID != "" {
			phaseID = &dp.PhaseID
		}

		records = append(records, &models.Post{
			ID:                  id,
			CampaignID:          campaign.ID,
			PlatformID:          dp.PlatformID,
			PlatformPostType:    dp.ContentType,
			Title:               dp.Title,
			Content:             dp.Body,
			MediaURLs:           models.StringSlice{},
			Status:              models.PostStatusDraft,
			CTAType:             models.CTATypeNone,
			CTAUrl:              "",
			TargetAudienceNotes: dp.ToneNotes,
			UsedAssetIDs:       models.StringSlice(dp.AssetRefs),
			CampaignTypePhaseID: phaseID,
			ScheduledAt:         scheduledAt,
			CreatedBy:           campaign.CreatedBy,
		})
	}

	return postRepo.CreateBatch(ctx, records)
}

// tailOf returns up to n bytes from the end of s, suitable for log output.
func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

func renderTemplate(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
