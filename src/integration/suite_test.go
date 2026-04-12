//go:build integration

package integration_test

import (
	"context"
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

func mustOpenIntegrationDB() *bun.DB {
	// Named in-memory database — isolated from the handler-test shared cache.
	db, err := database.New("file:integration?mode=memory&cache=shared&_pragma=foreign_keys(on)", false)
	if err != nil {
		panic(err)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		panic(err)
	}
	return db
}
