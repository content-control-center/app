---
name: add-grpc-service
description: Build a Go gRPC service or client the way pdf-service (CON-103) does — buf-managed proto codegen, client-streaming for large payloads, a health service, graceful shutdown, gRPC status error mapping (terminal vs transient), and a thin private-network client with a nil-disabled sentinel and typed error classification. Use when the user asks to add a gRPC service/microservice, a proto-defined RPC API, a streaming RPC, a gRPC client, split work into an internally-networked Go service, or wire Railway private networking between services.
tools: Read, Edit, Write, Glob, Grep, Bash
---

# Add gRPC Service / Client Skill

How this project builds internal gRPC services, distilled from **CON-103**
(`pdf-service`). There are two sides, in two repos:

- **Server** — `ogen-app/pdf-service` (a separate repo). Owns the proto contract,
  generates Go stubs with **buf** managed mode, serves `pdf.v1.PdfService` plus a
  standard `grpc.health.v1.Health`, and is reached **only** over the private
  network (plaintext h2c, no TLS).
- **Client** — this repo, `src/grpc/client/pdf/client.go`. A thin wrapper that owns the
  connection, the per-call deadline, and the raised receive limit; a **nil client
  is the "disabled" sentinel**. App code (`src/handlers/`, `src/jobs/queues/`)
  depends on a narrow interface, never on gRPC directly.

Canonical references to diff against:
- Server: `pdf-service/proto/pdf/v1/pdf.proto`, `pdf-service/cmd/pdf-service/main.go`,
  `pdf-service/internal/server/server.go`, `pdf-service/buf.gen.yaml`,
  `pdf-service/proto/buf.yaml`, `pdf-service/Dockerfile`, `pdf-service/.github/workflows/ci.yml`.
- Client: `src/grpc/client/pdf/client.go`; wired in `src/server/server.go`
  (`pdfclient.New(...)` + `OnShutdown` close); config in `src/config/config.go`
  (`PDF_SERVICE_ADDR/TIMEOUT/MAX_RECV_BYTES`).

## Architecture decisions (made in CON-103 — read first)

- **Separate repo, not a monorepo package.** The service is its own deployable
  with its own toolchain (pdf-service needs CGO + native `libpdfium`; keeping it
  out of the API lets the API stay a `CGO_ENABLED=0` static binary). The proto
  repo is the **canonical home of the contract**; the API consumes a **pinned,
  versioned** generated client.
- **Private network only.** API↔service traffic never leaves Railway's private
  network, so the channel is **plaintext h2c** — `insecure.NewCredentials()` on
  the client, a plain `grpc.NewServer()` (no TLS) on the server. The service
  exposes **no public port**. Address is `*.railway.internal:50051` in prod /
  `pdf-service:50051` in compose, via `PDF_SERVICE_ADDR`.
- **Graceful degradation everywhere.** An absent/disabled client (`PDF_SERVICE_ADDR`
  empty → `pdfclient.New` returns `(nil, nil)`) must not break callers — they
  no-op / leave work pending. See the error-classification section.

## Step 0: Clarify scope

1. **Repo** — new separate service repo, or an RPC added to an existing one? New
   service ⇒ scaffold proto + buf + server + Dockerfile + CI (Steps 1–7).
2. **RPCs & framing** — unary, or **client-streaming**? Stream when a request (or
   response) payload can exceed gRPC's **4 MiB default message cap** — e.g. file
   bytes. pdf-service streams uploads (Parse, Render); see Step 3.
3. **Contract package** — `<name>.v1` (always version the proto package).
4. **Who's the client**, and is it optional (nil-disabled) or required?
5. **Errors** — which failures are **terminal** (client must not retry: bad
   input) vs **transient** (retry: service down)? This drives the status-code
   mapping (Step 4) and the client classifier (Step 6).

---

## Step 1: Proto contract (`proto/<name>/v1/<name>.proto`)

`proto3`, a versioned package, one `service`. Client-streaming RPCs put the
options in a `oneof` first frame and raw bytes in subsequent frames:

```proto
syntax = "proto3";
package pdf.v1;

service PdfService {
  // First frame = options; every later frame = raw bytes. Server replies once.
  rpc Parse(stream ParseRequest) returns (ParseResponse);
}

message ParseRequest {
  oneof payload {
    ParseOptions options = 1; // first frame only
    bytes chunk = 2;          // subsequent frames: payload bytes
  }
}
message ParseOptions {
  string filename = 1;
  bool render_thumbnail = 2;  // server example reads opts.GetRenderThumbnail()
  int32 thumbnail_dpi = 3;    // 0 -> service default
}
message ParseResponse {
  int32 page_count = 1;
  repeated Chunk chunks = 2;
  bytes thumbnail_png = 3;    // empty if not requested
}
message Chunk {               // referenced by ParseResponse — must be defined
  int32 index = 1;
  string text = 2;
  int32 page_start = 3;
  int32 page_end = 4;
}
```

Conventions: zero-value scalar = "service default" (document it inline); keep the
contract **backward-compatible** (never renumber/reuse field tags).

`proto/buf.yaml`:
```yaml
version: v1
breaking:
  use: [FILE]   # buf breaking guards backward-compat in CI
lint:
  use: [DEFAULT]
```

---

## Step 2: Codegen (`buf.gen.yaml`) — managed mode, commit `gen/`

```yaml
version: v1
managed:
  enabled: true
  go_package_prefix:
    default: github.com/ogen-app/<repo>/gen   # managed mode sets go_package for you
plugins:
  - { plugin: go,      out: gen, opt: paths=source_relative }
  - { plugin: go-grpc, out: gen, opt: paths=source_relative }
```

- Run `buf generate proto`. **Commit the generated `gen/<name>/v1/*.pb.go` +
  `*_grpc.pb.go`** so consumers (and `go build`) don't need buf.
- `buf lint proto` and `buf breaking` run in CI (Step 7).
- **Gotcha:** a stale `protoc-gen-go` can crash with
  `runtime: bsdthread_register error` (binary built by an old Go runtime).
  Fix: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` and
  `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`.

---

## Step 3: Server — streaming reassembly + error mapping (`internal/server/`)

Embed `Unimplemented<Svc>Server` (forward-compat: new RPCs won't break the
build). Reassemble streamed frames with an in-memory **size cap**, then
`SendAndClose` once. Map engine/domain errors to gRPC status codes.

```go
type Server struct {
	pdfv1.UnimplementedPdfServiceServer
	engine *pdfengine.Engine
}

const maxPDFBytes = 110 << 20 // bound reassembled bytes in memory

func (s *Server) Parse(stream pdfv1.PdfService_ParseServer) error {
	var opts *pdfv1.ParseOptions
	var data []byte
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) { break }
		if err != nil { return err }
		switch p := req.Payload.(type) {
		case *pdfv1.ParseRequest_Options:
			opts = p.Options
		case *pdfv1.ParseRequest_Chunk:
			if len(data)+len(p.Chunk) > maxPDFBytes {
				return status.Errorf(codes.InvalidArgument, "pdf exceeds %d bytes", maxPDFBytes)
			}
			data = append(data, p.Chunk...)
		}
	}
	if len(data) == 0 { return status.Error(codes.InvalidArgument, "empty pdf") }
	res, err := s.engine.Extract(data, opts.GetRenderThumbnail(), int(opts.GetThumbnailDpi()))
	if err != nil { return mapEngineErr(err) }
	return stream.SendAndClose(&pdfv1.ParseResponse{ /* ... */ })
}

// Terminal vs transient is the whole game: a bad PDF is InvalidArgument (client
// must NOT retry); anything else is Internal (transient, client may retry).
func mapEngineErr(err error) error {
	if errors.Is(err, pdfengine.ErrInvalidPDF) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
```

Always use `opts.GetX()` accessors (nil-safe). Use a sentinel domain error
(`ErrInvalidPDF`) in the engine and translate it at the gRPC boundary.

---

## Step 4: Server bootstrap (`cmd/<name>/main.go`)

`grpc.NewServer()` (no TLS — private net), register the service **and** a health
server, graceful shutdown on SIGINT/SIGTERM. Config via **envconfig** (see the
`add-config`/envconfig convention; addr defaults to `:50051`).

```go
srv := grpc.NewServer()
pdfv1.RegisterPdfServiceServer(srv, server.New(engine))

// Standard grpc.health.v1 — set BOTH the service name and "" (overall),
// because probes default to checking the empty service.
hs := health.NewServer()
healthpb.RegisterHealthServer(srv, hs)
hs.SetServingStatus("pdf.v1.PdfService", healthpb.HealthCheckResponse_SERVING)
hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

go func() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	srv.GracefulStop() // drains in-flight RPCs
}()

lis, _ := net.Listen("tcp", cfg.Listen)
if err := srv.Serve(lis); err != nil { log.Fatalf("serve: %v", err) }
```

---

## Step 5: The client (`src/grpc/client/pdf/client.go` in the consumer repo)

A thin wrapper owning the conn, per-call timeout, and raised recv limit. **A nil
`*Client` is the disabled sentinel** — `New` returns `(nil, nil)` when the addr
is empty, and every method fails cleanly. `grpc.NewClient` connects **lazily**,
so `New` never blocks on the service being up.

```go
var ErrDisabled = errors.New("pdfclient: disabled (PDF_SERVICE_ADDR not set)")

func New(cfg Config) (*Client, error) {
	if cfg.Addr == "" { return nil, nil } // disabled sentinel
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())} // h2c, private net
	if cfg.MaxRecvBytes > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(cfg.MaxRecvBytes)))
	}
	conn, err := grpc.NewClient(cfg.Addr, opts...) // lazy; does not dial yet
	if err != nil { return nil, fmt.Errorf("pdfclient: dial %q: %w", cfg.Addr, err) }
	return &Client{conn: conn, rpc: pdfv1.NewPdfServiceClient(conn), timeout: cfg.Timeout}, nil
}

func (c *Client) Parse(ctx context.Context, r io.Reader, opts Options) (*Result, error) {
	if c == nil { return nil, ErrDisabled }
	ctx, cancel := context.WithTimeout(ctx, c.timeout); defer cancel()
	stream, err := c.rpc.Parse(ctx)
	if err != nil { return nil, fmt.Errorf("pdfclient: open parse stream: %w", err) }
	// 1) options frame, then 2) byte frames < 4 MiB each.
	_ = stream.Send(&pdfv1.ParseRequest{Payload: &pdfv1.ParseRequest_Options{Options: &pdfv1.ParseOptions{
		Filename: opts.Filename, RenderThumbnail: opts.RenderThumbnail, ThumbnailDpi: int32(opts.ThumbnailDPI),
	}}})
	buf := make([]byte, 1<<20) // 1 MiB frame, well under gRPC's 4 MiB message cap
	for {
		n, rerr := r.Read(buf)
		if n > 0 { _ = stream.Send(&pdfv1.ParseRequest{Payload: &pdfv1.ParseRequest_Chunk{Chunk: buf[:n]}}) }
		if rerr == io.EOF { break }
		if rerr != nil { return nil, fmt.Errorf("pdfclient: read: %w", rerr) }
	}
	resp, err := stream.CloseAndRecv() // ParseResponse
	if err != nil { return nil, fmt.Errorf("pdfclient: parse: %w", err) }
	// Map resp.GetChunks() into Result.Chunks too; elided here for brevity.
	return &Result{PageCount: int(resp.GetPageCount()), ThumbnailPNG: resp.GetThumbnailPng()}, nil
}
```

Wire it in `server.go` and close it on shutdown:
```go
pdfClient, err := pdfclient.New(pdfclient.Config{Addr: cfg.PDFServiceAddr, Timeout: cfg.PDFServiceTimeout, MaxRecvBytes: cfg.PDFServiceMaxRecvBytes})
if err != nil { return nil, err }
if pdfClient != nil { app.Hooks().OnShutdown(func() error { return pdfClient.Close() }) }
```

---

## Step 6: Error classification at the client boundary (the important part)

Don't leak gRPC codes to callers — expose a **typed predicate** so handlers/jobs
decide retry-vs-reject-vs-degrade. `status.FromError` walks the error chain with
`errors.As`, so a status wrapped via `fmt.Errorf("...: %w", err)` is still
recognized.

```go
// IsInvalidPDF is the terminal "not a usable PDF" verdict (gRPC InvalidArgument):
// corrupt/encrypted/not-a-PDF, will never parse → do NOT retry. Transport/internal
// failures (Unavailable, DeadlineExceeded, Internal) return false → transient.
func IsInvalidPDF(err error) bool {
	st, ok := grpcstatus.FromError(err)
	return ok && st.Code() == codes.InvalidArgument
}
```

Three reactions a caller picks from:
- **Terminal (`InvalidArgument`)** → reject the request (e.g. HTTP 400) or mark
  the job failed without retry.
- **Transient (Unavailable/DeadlineExceeded/Internal)** → retry (return the error
  from a River job) or **degrade gracefully** (keep the record, skip the derived
  data).
- **Disabled (nil client / `ErrDisabled`)** → no-op / leave pending.

For an async **River job** consuming the service, classify which codes are
terminal so you don't retry forever (pdf ingestion treats
`InvalidArgument/FailedPrecondition/Unimplemented` as terminal → mark failed,
return nil; everything else → return the error to retry). See `add-river-job`.

---

## Step 7: Docker + CI

**Dockerfile** — multi-stage; runtime ships a `grpc_health_probe` binary and a
`HEALTHCHECK` (no HTTP endpoint exists), runs as non-root, `EXPOSE 50051`:
```dockerfile
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=3 \
  CMD ["/usr/local/bin/grpc_health_probe", "-addr=:50051"]
ENTRYPOINT ["/usr/local/bin/pdf-service"]
```

**GitHub Actions** (`.github/workflows/ci.yml`) — one workflow: a `go` job
(gofmt + vet + `go test -race`), a `proto` job (`buf lint`), and a `build-push`
job that **`needs: [go, proto]`** so the image is only built/pushed to the
registry after tests pass (and only on `push` to main/tags, never PRs). Gate
push with `if: github.event_name == 'push' && github.repository_owner == '...'`.

---

## Step 8: Tests

- **Server**: drive the real implementation. Unit-test the engine behind the
  service directly; for an end-to-end gRPC test, start the server on a real
  listener (or `bufconn`) and call via the generated client. Assert status codes
  with `status.Code(err)`.
- **Client / handler / job**: define a **narrow interface** in the consumer
  package (e.g. `queues.pdfParser`, `handlers.PDFRenderer`) and inject a fake —
  never the real gRPC client. To exercise the terminal path, the fake returns a
  real gRPC status error so the production classifier recognizes it:

```go
func (fakeParser) Parse(_ context.Context, r io.Reader, _ pdfclient.Options) (*pdfclient.Result, error) {
	if /* unparseable */ {
		return nil, status.Error(codes.InvalidArgument, "fake: not a parseable PDF")
	}
	return &pdfclient.Result{ /* ... */ }, nil
}
```

---

## Cross-repo contract sync

The proto lives in the service repo (canonical); the consumer needs the same
contract. Options, cheapest→cleanest:

1. **Copy + CI drift guard** (interim) — consumer keeps a copy of the `.proto`;
   CI fails if its checksum differs from the canonical file. No cross-repo auth;
   still two generated copies.
2. **Consume the generated Go module** (recommended for two Go repos) — consumer
   deletes its copy and imports `github.com/ogen-app/<repo>/gen/<name>/v1`, pinned
   in `go.mod` (one import swap). Needs `GOPRIVATE` + git-token auth in the
   consumer's CI/Docker build if the service repo is private.
3. **Buf Schema Registry** — publish `buf.build/<org>/<name>`; both repos
   generate from it with `buf breaking`. Best when a non-Go consumer appears.

Evolve the contract backward-compatibly (`buf breaking`) and **release the image
and the client together**.

---

## Gotchas (CON-103 lessons)

1. **Stream when a payload can exceed ~4 MiB.** gRPC's default max message size
   is 4 MiB; file bytes blow past it. Client-stream in <4 MiB frames (1 MiB is
   safe); cap the reassembled size on the server. Don't put large bytes in a
   River job's args either — re-read from object storage.
2. **Health: set both the service name AND `""`.** `grpc_health_probe -addr=:port`
   with no `-service` checks the empty service; forgetting `SetServingStatus("", …)`
   makes the container look unhealthy.
3. **`status.FromError` unwraps `%w`.** You can wrap a gRPC error with context and
   still classify it by code downstream — but only via `status`/`errors.As`, not
   string matching.
4. **Nil client = disabled, not an error.** `New("")` returns `(nil, nil)`; every
   method guards `if c == nil { return ErrDisabled }`. Lets the whole feature be
   optional via one empty env var, and `grpc.NewClient` connects lazily so a down
   service never blocks startup.
5. **Plaintext h2c on purpose.** `insecure.NewCredentials()` is correct **only**
   because traffic stays on the private network. Don't "add TLS" without changing
   the deployment assumption; don't expose a public port.
6. **Terminal vs transient must be deliberate.** Mis-mapping a bad-input failure
   as transient = infinite retries; mapping a transient outage as terminal = lost
   work. Pick the code on the server, classify it on the client.
7. **Embed `Unimplemented<Svc>Server`.** Adding an RPC to the proto then won't
   break the build until you implement it.

---

## Checklist

- Proto: versioned package, `oneof` options-first framing for streaming RPCs,
  `proto/buf.yaml` (lint DEFAULT + breaking FILE).
- Codegen: `buf.gen.yaml` managed mode + `go_package_prefix`; `gen/` committed.
- Server: `Unimplemented…Server` embedded, stream reassembly with size cap,
  `mapEngineErr` (terminal `InvalidArgument` vs transient `Internal`),
  `SendAndClose`.
- Bootstrap: `grpc.NewServer`, register service + health (`""` and the service
  name), `GracefulStop` on SIGINT/SIGTERM, envconfig.
- Client: nil-disabled sentinel, `insecure` h2c, lazy `grpc.NewClient`, per-call
  timeout, raised `MaxCallRecvMsgSize`, <4 MiB frames, `CloseAndRecv`.
- Error boundary: typed predicate (`IsInvalidPDF`-style); callers
  reject/retry/degrade accordingly.
- Docker: `grpc_health_probe` HEALTHCHECK, non-root, no public port.
- CI: `go` + `proto(buf lint)` jobs; `build-push` `needs:` both, push-only on main/tags.
- Tests: engine/e2e on the server; narrow-interface fakes returning real
  `status.Error(codes.…)` on the consumer.
- Contract sync plan chosen (copy+drift / Go-module / BSR); `buf breaking` in CI.
- `go test ./...` green on both sides; `buf lint` clean.
```
