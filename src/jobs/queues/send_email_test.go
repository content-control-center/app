package queues

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/email"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// --- fakes ---------------------------------------------------------------

type fakeSender struct {
	sent []email.Message
	err  error
	id   string
}

func (f *fakeSender) Send(_ context.Context, msg email.Message) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, msg)
	return f.id, nil
}

type fakeUserRepo struct{ user *models.User }

func (f *fakeUserRepo) GetByIDWithTenant(_ context.Context, id string) (*models.User, error) {
	if f.user != nil && f.user.ID == id {
		return f.user, nil
	}
	return nil, sql.ErrNoRows
}
func (f *fakeUserRepo) List(context.Context) ([]models.User, error) { return nil, nil }
func (f *fakeUserRepo) Create(context.Context, *models.User) error  { return nil }
func (f *fakeUserRepo) GetByID(context.Context, string) (*models.User, error) {
	return nil, sql.ErrNoRows
}
func (f *fakeUserRepo) GetByEmail(context.Context, string) (*models.User, error) {
	return nil, sql.ErrNoRows
}
func (f *fakeUserRepo) Update(context.Context, *models.User) error   { return nil }
func (f *fakeUserRepo) Delete(context.Context, string) (bool, error) { return false, nil }

type fakeTemplateRepo struct {
	m map[string]*models.EmailTemplate
}

func (f *fakeTemplateRepo) GetByKey(_ context.Context, key string) (*models.EmailTemplate, error) {
	if t, ok := f.m[key]; ok {
		return t, nil
	}
	return nil, sql.ErrNoRows
}
func (f *fakeTemplateRepo) InsertIfAbsent(context.Context, *models.EmailTemplate) (bool, error) {
	return false, nil
}
func (f *fakeTemplateRepo) SyncVariables(context.Context, string, models.StringMap) error { return nil }
func (f *fakeTemplateRepo) List(context.Context) ([]models.EmailTemplate, error)          { return nil, nil }

// fakeSuppRepo mirrors the real SQL gate: marketing is blocked by any entry;
// transactional is blocked only by an `all`-scope entry.
type fakeSuppRepo struct {
	entries map[string]map[models.EmailSuppressionScope]bool
}

func newFakeSuppRepo() *fakeSuppRepo {
	return &fakeSuppRepo{entries: map[string]map[models.EmailSuppressionScope]bool{}}
}
func (f *fakeSuppRepo) add(email string, scope models.EmailSuppressionScope) {
	email = repository.NormalizeEmail(email)
	if f.entries[email] == nil {
		f.entries[email] = map[models.EmailSuppressionScope]bool{}
	}
	f.entries[email][scope] = true
}
func (f *fakeSuppRepo) Upsert(_ context.Context, s *models.EmailSuppression) error {
	f.add(s.Email, s.Scope)
	return nil
}
func (f *fakeSuppRepo) IsSuppressed(_ context.Context, addr string, kind models.EmailKind) (bool, error) {
	set := f.entries[repository.NormalizeEmail(addr)]
	if set == nil {
		return false, nil
	}
	if set[models.EmailSuppressionScopeAll] {
		return true, nil
	}
	if kind == models.EmailKindMarketing && set[models.EmailSuppressionScopeMarketing] {
		return true, nil
	}
	return false, nil
}
func (f *fakeSuppRepo) RemoveMarketing(_ context.Context, email string) error {
	if set := f.entries[repository.NormalizeEmail(email)]; set != nil {
		delete(set, models.EmailSuppressionScopeMarketing)
	}
	return nil
}

type fakeEmailLogRepo struct{ rows []*models.EmailLog }

func (f *fakeEmailLogRepo) Insert(_ context.Context, l *models.EmailLog) error {
	f.rows = append(f.rows, l)
	return nil
}
func (f *fakeEmailLogRepo) UpdateStatusByProviderMessageID(context.Context, string, models.EmailLogStatus) (bool, error) {
	return false, nil
}
func (f *fakeEmailLogRepo) DeleteOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }

// --- harness -------------------------------------------------------------

func testUser() *models.User {
	return &models.User{ID: "u1", Name: "Ann", Email: "Ann@Example.com", Tenant: &models.Tenant{Name: "Acme"}}
}

func newProcessor(sender email.Sender, supp *fakeSuppRepo, logs *fakeEmailLogRepo, tmplHTML string) *SendEmailProcessor {
	return &SendEmailProcessor{Deps: EmailDeps{
		Sender:       sender,
		Users:        &fakeUserRepo{user: testUser()},
		Templates:    &fakeTemplateRepo{m: map[string]*models.EmailTemplate{"welcome": {Key: "welcome", Subject: "Hi [[ .Name ]]", HTML: tmplHTML, Text: "Hi [[ .Name ]]"}, "drip_day2": {Key: "drip_day2", Subject: "Drip", HTML: tmplHTML, Text: "t [[ .UnsubscribeURL ]]"}}},
		Suppressions: supp,
		Logs:         logs,
		From:         "Ogen <hello@ogen.test>",
		AppBaseURL:   "https://app.example",
		LinkBaseURL:  "https://app.example",
		LinkSecret:   func(context.Context) (string, error) { return "link-secret", nil },
	}}
}

func task(key string, kind models.EmailKind) SendEmailTask {
	return SendEmailTask{UserID: "u1", TenantID: "t1", TemplateKey: key, EmailKind: kind, IdempotencyKey: key + ":u1"}
}

// --- tests ---------------------------------------------------------------

func TestSendEmailHappyPathTransactional(t *testing.T) {
	sender := &fakeSender{id: "msg_1"}
	logs := &fakeEmailLogRepo{}
	p := newProcessor(sender, newFakeSuppRepo(), logs, "<p>Hi [[ .Name ]] at [[ .WorkspaceName ]]</p>")

	if err := p.Process(context.Background(), task("welcome", models.EmailKindTransactional)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sent))
	}
	if sender.sent[0].To != "Ann@Example.com" {
		t.Fatalf("to: got %q", sender.sent[0].To)
	}
	if len(logs.rows) != 1 || logs.rows[0].Status != models.EmailLogSent || logs.rows[0].ProviderMessageID != "msg_1" {
		t.Fatalf("log row: %+v", logs.rows)
	}
}

func TestSendEmailDisabledWhenNoSender(t *testing.T) {
	logs := &fakeEmailLogRepo{}
	p := newProcessor(nil, newFakeSuppRepo(), logs, "<p>x</p>")

	if err := p.Process(context.Background(), task("welcome", models.EmailKindTransactional)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(logs.rows) != 1 || logs.rows[0].Status != models.EmailLogSkippedDisabled {
		t.Fatalf("want one skipped_disabled row, got %+v", logs.rows)
	}
}

func TestSendEmailMarketingSuppressed(t *testing.T) {
	sender := &fakeSender{id: "m"}
	supp := newFakeSuppRepo()
	supp.add("ann@example.com", models.EmailSuppressionScopeMarketing)
	logs := &fakeEmailLogRepo{}
	p := newProcessor(sender, supp, logs, "<p>x [[ .UnsubscribeURL ]]</p>")

	if err := p.Process(context.Background(), task("drip_day2", models.EmailKindMarketing)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatal("marketing to a marketing-suppressed address should not send")
	}
	if len(logs.rows) != 1 || logs.rows[0].Status != models.EmailLogSkippedSuppressed {
		t.Fatalf("want skipped_suppressed, got %+v", logs.rows)
	}
}

func TestSendEmailTransactionalNotBlockedByMarketingSuppression(t *testing.T) {
	sender := &fakeSender{id: "m"}
	supp := newFakeSuppRepo()
	supp.add("ann@example.com", models.EmailSuppressionScopeMarketing)
	p := newProcessor(sender, supp, &fakeEmailLogRepo{}, "<p>x</p>")

	if err := p.Process(context.Background(), task("welcome", models.EmailKindTransactional)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatal("transactional must still send to a marketing-unsubscribed address")
	}
}

func TestSendEmailAllScopeBlocksTransactional(t *testing.T) {
	sender := &fakeSender{id: "m"}
	supp := newFakeSuppRepo()
	supp.add("ann@example.com", models.EmailSuppressionScopeAll)
	logs := &fakeEmailLogRepo{}
	p := newProcessor(sender, supp, logs, "<p>x</p>")

	if err := p.Process(context.Background(), task("welcome", models.EmailKindTransactional)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatal("an all-scope suppression must block transactional mail too")
	}
	if logs.rows[0].Status != models.EmailLogSkippedSuppressed {
		t.Fatalf("want skipped_suppressed, got %s", logs.rows[0].Status)
	}
}

func TestSendEmailTransactionalNoUnsubscribeHeader(t *testing.T) {
	sender := &fakeSender{id: "t"}
	p := newProcessor(sender, newFakeSuppRepo(), &fakeEmailLogRepo{}, "<p>hi [[ .Name ]]</p>")

	if err := p.Process(context.Background(), task("welcome", models.EmailKindTransactional)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d, want 1", len(sender.sent))
	}
	if _, ok := sender.sent[0].Headers["List-Unsubscribe"]; ok {
		t.Error("transactional mail must not carry a List-Unsubscribe header")
	}
}

func TestSendEmailMarketingBuildsUnsubscribe(t *testing.T) {
	sender := &fakeSender{id: "m"}
	p := newProcessor(sender, newFakeSuppRepo(), &fakeEmailLogRepo{}, "<p>hi [[ .UnsubscribeURL ]]</p>")

	if err := p.Process(context.Background(), task("drip_day2", models.EmailKindMarketing)); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d, want 1", len(sender.sent))
	}
	msg := sender.sent[0]
	if _, ok := msg.Headers["List-Unsubscribe"]; !ok {
		t.Error("marketing message missing List-Unsubscribe header")
	}
	if msg.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Errorf("one-click header: got %q", msg.Headers["List-Unsubscribe-Post"])
	}
	if !strings.Contains(msg.HTML, "https://app.example/api/email/unsubscribe?token=") {
		t.Errorf("rendered html missing unsubscribe link: %q", msg.HTML)
	}
}
