//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/content-control-center/app/src/database"
	"github.com/uptrace/bun"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

// integrationDBSeq guarantees every call to mustOpenIntegrationDB gets a
// distinct in-memory database — same fix as the handler-suite helper
// (CON-70). Multiple Describes in this package each call this in their
// BeforeAll, and they used to all hit the same `file:integration?...`
// shared cache → cross-Describe state leaks. Per-call uniqueness keeps
// each Describe isolated.
var integrationDBSeq atomic.Uint64

func mustOpenIntegrationDB() *bun.DB {
	n := integrationDBSeq.Add(1)
	dsn := fmt.Sprintf(
		"file:integration_p%d_%d?mode=memory&cache=shared&_pragma=foreign_keys(on)",
		GinkgoParallelProcess(), n,
	)
	db, err := database.New(dsn, false)
	if err != nil {
		panic(err)
	}
	// Same connection-pool override as the handler-suite helper —
	// production SQLite caps at one connection (correct for WAL on
	// disk), but the integration tests run handlers + workers + flow
	// callbacks concurrently against this DB and need headroom.
	db.DB.SetMaxOpenConns(10)
	db.DB.SetMaxIdleConns(10)
	if err := database.Migrate(context.Background(), db); err != nil {
		panic(err)
	}
	return db
}
