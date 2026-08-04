package post_assistant

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsPostRemovedFKViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "post_assistant_messages post_id fk violation",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "post_assistant_messages_post_id_fkey"},
			want: true,
		},
		{
			name: "post_versions post_id fk violation",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "post_versions_post_id_fkey"},
			want: true,
		},
		{
			name: "wrapped fk violation is still detected",
			err:  fmt.Errorf("persist user message: %w", &pgconn.PgError{Code: "23503", ConstraintName: "post_assistant_messages_post_id_fkey"}),
			want: true,
		},
		{
			name: "fk violation on a different column is ignored",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "post_assistant_messages_tenant_id_fkey"},
			want: false,
		},
		{
			name: "unique violation on post_id is not an fk violation",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "some_post_id_fkey"},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("boom"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPostRemovedFKViolation(tc.err); got != tc.want {
				t.Fatalf("isPostRemovedFKViolation() = %v, want %v", got, tc.want)
			}
		})
	}
}
