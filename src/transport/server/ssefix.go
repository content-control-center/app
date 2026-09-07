package server

// This file patches the openai-go SSE decoder to handle Anthropic's keepalive
// ping events. Anthropic's API sends comment-only SSE events (": ping\n\n") to
// keep long-running streaming connections alive. The default openai-go decoder
// (eventStreamDecoder) dispatches an event on every empty line — even when no
// data was accumulated — which causes json.Unmarshal to fail with
// "unexpected end of JSON input" and terminates the stream prematurely.
//
// The fix replaces the decoder for "text/event-stream" with one that skips
// events that have no data (comment-only blocks).

import (
	"bufio"
	"bytes"
	"io"

	"github.com/openai/openai-go/packages/ssestream"
)

func init() {
	ssestream.RegisterDecoder("text/event-stream", newSkipEmptySSEDecoder)
}

type skipEmptySSEDecoder struct {
	rc  io.ReadCloser
	scn *bufio.Scanner
	evt ssestream.Event
	err error
}

func newSkipEmptySSEDecoder(rc io.ReadCloser) ssestream.Decoder {
	scn := bufio.NewScanner(rc)
	scn.Buffer(nil, bufio.MaxScanTokenSize<<9) // 32 MiB — same as the default decoder
	return &skipEmptySSEDecoder{rc: rc, scn: scn}
}

// Next advances to the next non-empty SSE event. Comment-only event blocks
// such as ": ping\n\n" are silently skipped.
func (s *skipEmptySSEDecoder) Next() bool {
	if s.err != nil {
		return false
	}

outerLoop:
	for {
		event := ""
		data := bytes.NewBuffer(nil)

		for s.scn.Scan() {
			txt := s.scn.Bytes()

			if len(txt) == 0 {
				// Empty line = event boundary.
				if data.Len() > 0 {
					s.evt = ssestream.Event{Type: event, Data: data.Bytes()}
					return true
				}
				// No data accumulated (keepalive comment block such as ": ping").
				// Jump back to the outer loop to reset state and read the next event.
				continue outerLoop
			}

			name, value, _ := bytes.Cut(txt, []byte(":"))
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}

			switch string(name) {
			case "":
				// Comment line (": something") — ignore.
				continue
			case "event":
				event = string(value)
			case "data":
				_, s.err = data.Write(value)
				if s.err != nil {
					return false
				}
				_, s.err = data.WriteRune('\n')
				if s.err != nil {
					return false
				}
			}
		}

		// Scanner stopped: EOF or error.
		if s.scn.Err() != nil {
			s.err = s.scn.Err()
		}
		return false

		// (unreachable — kept for outer-loop structure clarity)
	}
}

func (s *skipEmptySSEDecoder) Event() ssestream.Event { return s.evt }
func (s *skipEmptySSEDecoder) Err() error             { return s.err }
func (s *skipEmptySSEDecoder) Close() error           { return s.rc.Close() }
