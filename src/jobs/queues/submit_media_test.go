package queues_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/publishers/zernio"
)

// fakeAttachmentRepo returns a fixed attachment list (CON-122 media tests).
type fakeAttachmentRepo struct{ atts []models.PostAttachment }

func (r *fakeAttachmentRepo) ListByPostID(context.Context, string) ([]models.PostAttachment, error) {
	return r.atts, nil
}
func (r *fakeAttachmentRepo) ListS3KeysByPostID(context.Context, string) ([]string, error) {
	return nil, nil
}
func (r *fakeAttachmentRepo) GetByID(context.Context, string) (*models.PostAttachment, error) {
	return nil, nil
}
func (r *fakeAttachmentRepo) CreateAtNextPosition(context.Context, *models.PostAttachment) error {
	return nil
}
func (r *fakeAttachmentRepo) UpdatePosition(context.Context, string, int) error   { return nil }
func (r *fakeAttachmentRepo) UpdateAltText(context.Context, string, string) error { return nil }
func (r *fakeAttachmentRepo) Delete(context.Context, string) (bool, error)        { return false, nil }

// fakeStorage serves attachment bytes by key; other methods are no-ops.
type fakeStorage struct{ objects map[string][]byte }

func (s *fakeStorage) Upload(context.Context, string, io.Reader, int64, string) (string, error) {
	return "", nil
}
func (s *fakeStorage) Copy(context.Context, string, string) error { return nil }
func (s *fakeStorage) Delete(context.Context, string) error       { return nil }
func (s *fakeStorage) PublicURL(string) string                    { return "" }
func (s *fakeStorage) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (s *fakeStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.objects[key])), nil
}

// TestSubmitUploadsMediaWithAltText proves the CON-122 media flow end-to-end:
// the worker uploads each attachment to Zernio (presign → PUT bytes) and the
// create-post body references the returned publicUrl with type + altText.
func TestSubmitUploadsMediaWithAltText(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()

	var uploaded []byte
	var uploadedCT string
	stub.handle("POST", "/media/presign", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["filename"] == "" || body["contentType"] == "" {
			t.Errorf("presign missing filename/contentType: %v", body)
		}
		writeJSON(w, http.StatusOK, zernio.MediaPresign{
			UploadURL: stub.URL + "/up/one",
			PublicURL: "https://cdn.zernio.test/one.png",
			Key:       "k1",
			ExpiresIn: 3600,
		})
	})
	stub.handle("PUT", "/up/one", func(w http.ResponseWriter, r *http.Request) {
		uploaded, _ = io.ReadAll(r.Body)
		uploadedCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})
	var submitBody zernio.SubmitRequest
	stub.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&submitBody)
		writeJSON(w, http.StatusCreated, zernio.PostEnvelope{Post: zernio.Job{
			ID: "z-1", Status: zernio.JobStatusScheduled,
		}})
	})

	deps, postRepo, _ := makeDeps(stub, map[string][]models.SocialAccount{
		"p_test": {{ID: "acc-1", Platform: "linkedin"}},
	})
	deps.Storage = &fakeStorage{objects: map[string][]byte{
		"post-attachments/post-1/att1.png": []byte("PNGBYTES"),
	}}
	deps.PostAttachmentRepo = &fakeAttachmentRepo{atts: []models.PostAttachment{{
		ID: "att1", PostID: "post-1", Position: 0,
		MimeType: "image/png", SizeBytes: 8,
		S3Key: "post-attachments/post-1/att1.png", AltText: "a red bicycle",
	}}}
	post := seedScheduledPost(postRepo)

	proc := &queues.SubmitPostProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.SubmitPostTask{PostID: post.ID}); err != nil {
		t.Fatalf("process: %v", err)
	}

	if string(uploaded) != "PNGBYTES" {
		t.Errorf("uploaded bytes = %q, want PNGBYTES", uploaded)
	}
	if uploadedCT != "image/png" {
		t.Errorf("upload content-type = %q, want image/png", uploadedCT)
	}
	if len(submitBody.MediaItems) != 1 {
		t.Fatalf("mediaItems = %d, want 1", len(submitBody.MediaItems))
	}
	m := submitBody.MediaItems[0]
	if m["url"] != "https://cdn.zernio.test/one.png" || m["type"] != "image" || m["altText"] != "a red bicycle" {
		t.Errorf("mediaItem wrong: %+v", m)
	}
}
