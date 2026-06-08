//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ogen-app/ogen/src/database"
	"github.com/uptrace/bun"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

// integrationDBSeq guarantees every call to mustOpenIntegrationDB gets a
// distinct database file — multiple Describes in this package each call
// this in their BeforeAll, and previously they all hit the same
// `file:integration?...` shared cache, leaking state across Describes.
// Per-call uniqueness keeps each Describe isolated.
var integrationDBSeq atomic.Uint64

// integrationTmpDir holds the temp database files for the run; cleaned
// up after the suite finishes via the BeforeSuite/AfterSuite below.
var integrationTmpDir string

var _ = BeforeSuite(func() {
	dir, err := os.MkdirTemp("", "ogen-integration-*")
	if err != nil {
		panic(err)
	}
	integrationTmpDir = dir
})

var _ = AfterSuite(func() {
	if integrationTmpDir != "" {
		_ = os.RemoveAll(integrationTmpDir)
	}
})

func mustOpenIntegrationDB() *bun.DB {
	n := integrationDBSeq.Add(1)
	// Disk-backed temp file rather than in-memory shared cache: the
	// ncruces sqlite3 driver doesn't reliably propagate the migrated
	// schema across pool connections under concurrent acquisition with
	// `mode=memory&cache=shared`. Disk + WAL is the proven path for
	// concurrent SQLite. busy_timeout=5000 lets the second writer wait
	// for the lock (instead of failing immediately) under contention,
	// which integration test #4 deliberately creates by firing
	// concurrent uploads at one post.
	path := filepath.Join(integrationTmpDir, fmt.Sprintf("integration_p%d_%d.db", GinkgoParallelProcess(), n))
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(on)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		path,
	)
	db, err := database.New(dsn, false)
	if err != nil {
		panic(err)
	}
	// Pool sized to let handler goroutines run in parallel — production
	// caps at 1 because writes serialise anyway, but the test wants to
	// observe what happens when several requests touch the DB at once.
	db.DB.SetMaxOpenConns(10)
	db.DB.SetMaxIdleConns(10)
	if err := database.Migrate(context.Background(), db); err != nil {
		panic(err)
	}
	return db
}
