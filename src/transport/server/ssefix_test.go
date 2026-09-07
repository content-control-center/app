package server

import (
	"io"
	"strings"
	"testing"
)

// mockReadCloser wraps a strings.Reader so it implements io.ReadCloser.
type mockReadCloser struct{ r io.Reader }

func (m *mockReadCloser) Read(p []byte) (int, error) { return m.r.Read(p) }
func (m *mockReadCloser) Close() error               { return nil }

func newRC(s string) io.ReadCloser {
	return &mockReadCloser{strings.NewReader(s)}
}

func TestSkipEmptySSEDecoder(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantEvents []string // data fields of dispatched events
	}{
		{
			name:       "single data event",
			input:      "data: {\"a\":1}\n\n",
			wantEvents: []string{"{\"a\":1}\n"},
		},
		{
			name:       "two data events",
			input:      "data: {\"a\":1}\n\ndata: {\"b\":2}\n\n",
			wantEvents: []string{"{\"a\":1}\n", "{\"b\":2}\n"},
		},
		{
			name: "keepalive between real events is skipped",
			input: ": ping\n\n" +
				"data: {\"a\":1}\n\n" +
				": ping\n\n" +
				"data: {\"b\":2}\n\n",
			wantEvents: []string{"{\"a\":1}\n", "{\"b\":2}\n"},
		},
		{
			name: "multiple keepalives before first event",
			input: ": ping\n\n: ping\n\n: ping\n\n" +
				"data: {\"a\":1}\n\n",
			wantEvents: []string{"{\"a\":1}\n"},
		},
		{
			name:       "empty stream",
			input:      "",
			wantEvents: nil,
		},
		{
			name:       "only keepalives — no events dispatched",
			input:      ": ping\n\n: ping\n\n",
			wantEvents: nil,
		},
		{
			name:       "event with type field",
			input:      "event: content\ndata: hello\n\n",
			wantEvents: []string{"hello\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := newSkipEmptySSEDecoder(newRC(tt.input))
			var got []string
			for dec.Next() {
				got = append(got, string(dec.Event().Data))
			}
			if dec.Err() != nil {
				t.Fatalf("decoder error: %v", dec.Err())
			}
			if len(got) != len(tt.wantEvents) {
				t.Fatalf("got %d events, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantEvents), got, tt.wantEvents)
			}
			for i := range tt.wantEvents {
				if got[i] != tt.wantEvents[i] {
					t.Errorf("event[%d] data: got %q, want %q", i, got[i], tt.wantEvents[i])
				}
			}
		})
	}
}
