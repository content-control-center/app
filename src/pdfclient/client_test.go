package pdfclient_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	pdfv1 "github.com/ogen-app/ogen/gen/pdf/v1"
	"github.com/ogen-app/ogen/src/pdfclient"
)

// stubServer records the streamed request and returns a canned response.
type stubServer struct {
	pdfv1.UnimplementedPdfServiceServer
	gotOptions *pdfv1.ParseOptions
	gotBytes   []byte
	resp       *pdfv1.ParseResponse
}

func (s *stubServer) Parse(stream grpc.ClientStreamingServer[pdfv1.ParseRequest, pdfv1.ParseResponse]) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch p := req.Payload.(type) {
		case *pdfv1.ParseRequest_Options:
			s.gotOptions = p.Options
		case *pdfv1.ParseRequest_Chunk:
			s.gotBytes = append(s.gotBytes, p.Chunk...)
		}
	}
	return stream.SendAndClose(s.resp)
}

// serveStub starts the stub on a loopback listener and returns its address.
func serveStub(t *testing.T, stub *stubServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pdfv1.RegisterPdfServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestParseStreamsBytesAndReturnsResult(t *testing.T) {
	stub := &stubServer{resp: &pdfv1.ParseResponse{
		PageCount:    3,
		Chunks:       []*pdfv1.Chunk{{Index: 0, Text: "hello", PageStart: 1, PageEnd: 2}},
		ThumbnailPng: []byte{0x89, 'P', 'N', 'G'},
	}}
	addr := serveStub(t, stub)

	client, err := pdfclient.New(pdfclient.Config{Addr: addr, Timeout: 5 * time.Second, MaxRecvBytes: 1 << 20})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.Close()

	// ~1.5 MiB forces multiple stream frames (frame size is 1 MiB).
	pdf := bytes.Repeat([]byte("%PDF-1.7 data "), 110_000)
	res, err := client.Parse(context.Background(), bytes.NewReader(pdf),
		pdfclient.Options{Filename: "doc.pdf", RenderThumbnail: true, ThumbnailDPI: 96})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// All bytes arrived, reassembled in order.
	if !bytes.Equal(stub.gotBytes, pdf) {
		t.Fatalf("server got %d bytes, want %d", len(stub.gotBytes), len(pdf))
	}
	// Options were carried in the first frame.
	if got := stub.gotOptions; got == nil || got.GetFilename() != "doc.pdf" ||
		!got.GetRenderThumbnail() || got.GetThumbnailDpi() != 96 {
		t.Fatalf("options not propagated: %+v", stub.gotOptions)
	}
	// Response was mapped onto the typed Result.
	if res.PageCount != 3 || len(res.Chunks) != 1 || res.Chunks[0].Text != "hello" ||
		res.Chunks[0].PageStart != 1 || res.Chunks[0].PageEnd != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !bytes.Equal(res.ThumbnailPNG, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("thumbnail mismatch: %v", res.ThumbnailPNG)
	}
}

func TestDisabledClientReportsErrDisabled(t *testing.T) {
	c, err := pdfclient.New(pdfclient.Config{Addr: ""})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c != nil {
		t.Fatalf("expected a nil client when Addr is empty")
	}
	if _, err := c.Parse(context.Background(), bytes.NewReader(nil), pdfclient.Options{}); err != pdfclient.ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
}
