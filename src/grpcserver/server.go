// Package grpcserver hosts Ogen's internal, operator-facing gRPC surface.
//
// Today it serves a single service — SecretsService — so Harbor (Ogen's
// operations dashboard) can list / rotate / clear the third-party API keys in
// the `secret` table through the SAME secrets.Store the rest of the app uses.
// Going through the Store (rather than the table) reuses the envelope crypto,
// the name allowlist, and the hot-reload subscription hooks with zero
// duplication.
//
// The listener is internal-only and every call is gated by a shared bearer
// token (constant-time compared). Transport is plain (insecure) like Ogen's
// pdf/video gRPC clients, so the token is only as safe as the network it
// crosses: the default bind is loopback-only (config.GRPCAddr) and, when
// exposed across hosts, the operator is expected to keep it on a private
// network / behind a NetworkPolicy and add mTLS at the infra or transport layer
// (grpc.Creds). Never carry the token over an untrusted path in cleartext.
package grpcserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	secretsv1 "github.com/ogen-app/ogen/gen/secrets/v1"
	"github.com/ogen-app/ogen/src/secrets"
)

// authMetadataKey is the (lowercased) gRPC metadata key carrying the shared
// bearer token. gRPC lowercases metadata keys, so callers may send
// "Authorization" — it arrives here as "authorization".
const authMetadataKey = "authorization"

// New builds the internal gRPC server: a token-authenticated SecretsService
// backed by store. An empty token is rejected — the caller must decide NOT to
// start the server at all rather than run it unauthenticated.
func New(token string, store secrets.Store) (*grpc.Server, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("grpcserver: auth token is required")
	}
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(tokenAuthInterceptor(token)),
	)
	secretsv1.RegisterSecretsServiceServer(srv, newSecretsService(store))
	return srv, nil
}

// tokenAuthInterceptor rejects any unary call whose `authorization` metadata
// is not exactly `Bearer <token>`. The comparison is constant-time and the
// token is never logged.
func tokenAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	want := []byte("Bearer " + token)
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		vals := md.Get(authMetadataKey)
		if len(vals) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization")
		}
		// ConstantTimeCompare returns 0 on any length mismatch, so this is safe
		// against both value and length side channels.
		if subtle.ConstantTimeCompare([]byte(vals[0]), want) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		return handler(ctx, req)
	}
}
