package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	secretsv1 "github.com/ogen-app/ogen/gen/secrets/v1"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/secrets"
)

// secretsService adapts secrets.Store to the generated SecretsServiceServer.
// It holds nothing but the Store — the crypto, allowlist, and hot-reload
// notifications all live behind it.
type secretsService struct {
	secretsv1.UnimplementedSecretsServiceServer
	store secrets.Store
}

func newSecretsService(store secrets.Store) *secretsService {
	return &secretsService{store: store}
}

// List returns one entry per allowlisted name (in allowlist order), marking
// each as set/unset. The allowlist stays authoritative in this repo — callers
// never hardcode it.
func (s *secretsService) List(ctx context.Context, _ *secretsv1.ListRequest) (*secretsv1.ListResponse, error) {
	rows, err := s.store.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "secrets grpc list", logging.AttrComponent, "grpcserver", logging.AttrError, err)
		return nil, status.Error(codes.Internal, "list failed")
	}
	byName := make(map[string]secrets.Metadata, len(rows))
	for _, m := range rows {
		byName[m.Name] = m
	}
	out := make([]*secretsv1.SecretMetadata, 0, len(secrets.AllowedNames))
	for _, name := range secrets.AllowedNames {
		if m, ok := byName[name]; ok {
			out = append(out, metadataToProto(m, true))
		} else {
			out = append(out, &secretsv1.SecretMetadata{Name: name, Set: false})
		}
	}
	return &secretsv1.ListResponse{Secrets: out}, nil
}

// Set upserts the plaintext value. store.Set enforces the allowlist and the
// surface validations, encrypts, persists, and notifies subscribers — so a
// rotated key is picked up by Genkit/Zernio without a restart.
func (s *secretsService) Set(ctx context.Context, req *secretsv1.SetRequest) (*secretsv1.SetResponse, error) {
	meta, created, err := s.store.Set(ctx, req.GetName(), req.GetValue())
	if err != nil {
		return nil, mapStoreErr(ctx, "set", req.GetName(), err)
	}
	return &secretsv1.SetResponse{Secret: metadataToProto(meta, true), Created: created}, nil
}

// Delete removes a stored secret. Missing name → NotFound.
func (s *secretsService) Delete(ctx context.Context, req *secretsv1.DeleteRequest) (*secretsv1.DeleteResponse, error) {
	if err := s.store.Delete(ctx, req.GetName()); err != nil {
		return nil, mapStoreErr(ctx, "delete", req.GetName(), err)
	}
	return &secretsv1.DeleteResponse{}, nil
}

func metadataToProto(m secrets.Metadata, set bool) *secretsv1.SecretMetadata {
	return &secretsv1.SecretMetadata{
		Name:        m.Name,
		Set:         set,
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
		KekVersion:  int32(m.KEKVersion),
		Algorithm:   m.Algorithm,
		Decryptable: m.Decryptable,
	}
}

// mapStoreErr translates Store sentinels to gRPC status codes, mirroring the
// REST handler's mapping (src/handlers/secrets.go). ErrInvalidValue's message
// is safe to surface — the Store guarantees it never contains the value.
// Everything else is logged (with name, never value) and returned as a generic
// Internal so we don't leak internals across the wire.
func mapStoreErr(ctx context.Context, op, name string, err error) error {
	switch {
	case errors.Is(err, secrets.ErrUnknownName):
		return status.Error(codes.InvalidArgument, "unknown secret name")
	case errors.Is(err, secrets.ErrNotFound):
		return status.Error(codes.NotFound, "secret not found")
	case errors.Is(err, secrets.ErrInvalidValue):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		slog.ErrorContext(ctx, "secrets grpc "+op, logging.AttrComponent, "grpcserver", "name", name, logging.AttrError, err)
		return status.Error(codes.Internal, op+" failed")
	}
}
