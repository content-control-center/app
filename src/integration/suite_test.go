//go:build integration

package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/pgtest"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

// mustOpenIntegrationDB returns a fresh, fully-migrated Postgres database
// (CON-87 WS5). Each Describe's BeforeAll gets its own database, so suites
// stay isolated. The pool is sized up from pgtest's default so the
// concurrent-upload integration test can exercise parallel handler requests.
func mustOpenIntegrationDB() *bun.DB {
	db := pgtest.MustDB()
	db.DB.SetMaxOpenConns(10)
	db.DB.SetMaxIdleConns(5)
	return db
}
