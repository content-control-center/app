package handlers

import (
	"database/sql"
	"errors"

	"github.com/gofiber/fiber/v2"
)

// bindAndValidate parses the request body into dst (a pointer to the request
// struct) and runs struct validation, returning the canonical 400 responses
// used across the handlers: the raw parser error for a malformed body, and the
// humanized validationError for a failed constraint. It collapses the repeated
// BodyParser + validate.Struct preamble into one call; behavior is identical to
// the inline blocks it replaces.
func bindAndValidate(c *fiber.Ctx, dst any) error {
	if err := c.BodyParser(dst); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(dst); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	return nil
}

// notFound maps a repository lookup error to an HTTP response: a missing row
// (sql.ErrNoRows) becomes a 404 carrying msg, any other error is returned
// unchanged (surfacing as a 500). It replaces the repeated
// "if errors.Is(err, sql.ErrNoRows) { return 404 } return err" block — callers
// pass the exact message they used before, so responses are unchanged.
func notFound(err error, msg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, msg)
	}
	return err
}
