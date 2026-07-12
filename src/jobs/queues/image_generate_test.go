package queues

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/ogen-app/ogen/src/genkit/flows/imagegen"
	"github.com/ogen-app/ogen/src/models"
)

// ---- fakes (satisfy the image_generate worker's narrow interfaces). Names are
// ig-prefixed to avoid colliding with process_pdf_test's fakes in this package.

type igAssets struct {
	byID     map[string]*models.Asset
	updated  *models.Asset
	statuses []string
}

func (f *igAssets) GetByID(_ context.Context, id string) (*models.Asset, error) {
	return f.byID[id], nil
}
func (f *igAssets) Update(_ context.Context, a *models.Asset) error {
	f.updated = a
	f.byID[a.ID] = a
	return nil
}
func (f *igAssets) UpdateStatus(_ context.Context, id, status string) error {
	f.statuses = append(f.statuses, status)
	if a := f.byID[id]; a != nil {
		a.Status = status
	}
	return nil
}

type igFiles struct {
	byAsset  map[string]*models.AssetFile
	upserted *models.AssetFile
}

func (f *igFiles) GetByAssetID(_ context.Context, id string) (*models.AssetFile, error) {
	return f.byAsset[id], nil
}
func (f *igFiles) ListByAssetIDs(_ context.Context, ids []string) (map[string]*models.AssetFile, error) {
	out := map[string]*models.AssetFile{}
	for _, id := range ids {
		if ff := f.byAsset[id]; ff != nil {
			out[id] = ff
		}
	}
	return out, nil
}
func (f *igFiles) Upsert(_ context.Context, file *models.AssetFile) error {
	f.upserted = file
	return nil
}

type igPosts struct {
	byID    map[string]*models.Post
	updated *models.Post
}

func (p *igPosts) GetByID(_ context.Context, id string) (*models.Post, error) { return p.byID[id], nil }
func (p *igPosts) Update(_ context.Context, post *models.Post) error          { p.updated = post; return nil }

type igBlob struct {
	downloads map[string][]byte
	uploads   map[string][]byte
}

func (b *igBlob) Download(_ context.Context, key string) (io.ReadCloser, error) {
	d, ok := b.downloads[key]
	if !ok {
		return nil, fmt.Errorf("no such key %s", key)
	}
	return io.NopCloser(bytes.NewReader(d)), nil
}
func (b *igBlob) Upload(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	data, _ := io.ReadAll(r)
	if b.uploads == nil {
		b.uploads = map[string][]byte{}
	}
	b.uploads[key] = data
	return key, nil
}

func TestImageGenerate_Success(t *testing.T) {
	assets := &igAssets{byID: map[string]*models.Asset{
		"a1": {ID: "a1", Status: models.AssetStatusPending},
	}}
	files := &igFiles{byAsset: map[string]*models.AssetFile{
		"brand1": {AssetID: "brand1", S3Key: "k-brand1", MimeType: "image/png"},
		"subj1":  {AssetID: "subj1", S3Key: "k-subj1", MimeType: "image/jpeg"},
	}}
	blob := &igBlob{downloads: map[string][]byte{
		"k-brand1": []byte("brand-bytes"),
		"k-subj1":  []byte("subject-bytes"),
	}}
	posts := &igPosts{byID: map[string]*models.Post{
		"p1": {ID: "p1", UsedAssetIDs: models.StringSlice{}},
	}}

	var gotReq imagegen.ImagineRequest
	var calls int
	gen := func(_ context.Context, req imagegen.ImagineRequest) (*imagegen.ImagineResponse, error) {
		calls++
		gotReq = req
		return &imagegen.ImagineResponse{
			Image: []byte("PNGDATA"), MIMEType: "image/png",
			Model: "gemini-3.1-flash-image", AspectRatio: "4:5", Resolution: "1K",
		}, nil
	}

	p := &ImageGenerateProcessor{Deps: ImageGenDeps{Generate: gen, Storage: blob, Assets: assets, Files: files, Posts: posts}}
	if err := p.process(context.Background(), ImageGenerateTask{
		AssetID: "a1", PostID: "p1",
		BrandRefAssetIDs: []string{"brand1"}, SubjectAssetID: "subj1",
		AspectRatio: "4:5", Resolution: "1K",
	}, false); err != nil {
		t.Fatalf("process: %v", err)
	}

	if calls != 1 {
		t.Fatalf("generator called %d times, want 1", calls)
	}
	// References loaded from storage, brand-first then subject.
	if len(gotReq.BrandRefs) != 1 || string(gotReq.BrandRefs[0].Data) != "brand-bytes" {
		t.Fatalf("brand refs not passed: %+v", gotReq.BrandRefs)
	}
	if gotReq.Subject == nil || string(gotReq.Subject.Data) != "subject-bytes" {
		t.Fatalf("subject not passed: %+v", gotReq.Subject)
	}
	// Image uploaded and asset_file written.
	var upKey string
	for k := range blob.uploads {
		upKey = k
	}
	if upKey == "" || files.upserted == nil || files.upserted.S3Key != upKey || files.upserted.MimeType != "image/png" {
		t.Fatalf("generated file not persisted: up=%q file=%+v", upKey, files.upserted)
	}
	// Asset flipped to ready with provenance.
	a := assets.byID["a1"]
	if a.Status != models.AssetStatusReady || a.Type == nil || *a.Type != models.AssetTypeImage || !a.AIGenerated {
		t.Fatalf("asset not marked ready/IMG/ai: %+v", a)
	}
	if a.Generation == nil || a.Generation.Model != "gemini-3.1-flash-image" ||
		a.Generation.AspectRatio != "4:5" || !a.Generation.SynthID || !a.Generation.C2PA {
		t.Fatalf("generation metadata wrong: %+v", a.Generation)
	}
	// Post linked.
	if posts.updated == nil || len(posts.updated.UsedAssetIDs) != 1 || posts.updated.UsedAssetIDs[0] != "a1" {
		t.Fatalf("post not linked: %+v", posts.updated)
	}
}

func TestImageGenerate_ValidationErrorIsTerminal(t *testing.T) {
	assets := &igAssets{byID: map[string]*models.Asset{"a1": {ID: "a1", Status: models.AssetStatusPending}}}
	gen := func(_ context.Context, _ imagegen.ImagineRequest) (*imagegen.ImagineResponse, error) {
		return nil, &imagegen.ValidationError{Msg: "resolution \"4K\" is not supported"}
	}
	p := &ImageGenerateProcessor{Deps: ImageGenDeps{Generate: gen, Storage: &igBlob{}, Assets: assets, Files: &igFiles{byAsset: map[string]*models.AssetFile{}}}}

	// Terminal: process returns nil (no retry) but the asset lands "failed".
	if err := p.process(context.Background(), ImageGenerateTask{AssetID: "a1", AspectRatio: "16:9", Resolution: "4K"}, false); err != nil {
		t.Fatalf("process returned error, want nil terminal: %v", err)
	}
	if got := assets.byID["a1"].Status; got != models.AssetStatusFailed {
		t.Fatalf("status = %q, want failed", got)
	}
}

func TestImageGenerate_IdempotentWhenAlreadyReady(t *testing.T) {
	assets := &igAssets{byID: map[string]*models.Asset{"a1": {ID: "a1", Status: models.AssetStatusReady}}}
	calls := 0
	gen := func(_ context.Context, _ imagegen.ImagineRequest) (*imagegen.ImagineResponse, error) {
		calls++
		return &imagegen.ImagineResponse{Image: []byte("x"), MIMEType: "image/png"}, nil
	}
	p := &ImageGenerateProcessor{Deps: ImageGenDeps{Generate: gen, Storage: &igBlob{}, Assets: assets, Files: &igFiles{byAsset: map[string]*models.AssetFile{}}}}

	if err := p.process(context.Background(), ImageGenerateTask{AssetID: "a1", AspectRatio: "1:1"}, false); err != nil {
		t.Fatalf("process: %v", err)
	}
	if calls != 0 {
		t.Fatalf("generator called %d times on an already-ready asset, want 0", calls)
	}
}
