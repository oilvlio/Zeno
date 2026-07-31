package api

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// scriptedPresenceConn replays a fixed sequence of inbound frames and then
// reports the connection as closed, so the reader loop's exit and continue
// conditions can be driven deterministically.
type scriptedPresenceConn struct {
	frames    []string
	index     int
	readLimit int64
}

func (c *scriptedPresenceConn) SetReadLimit(limit int64) { c.readLimit = limit }

func (c *scriptedPresenceConn) NextReader() (int, io.Reader, error) {
	if c.index >= len(c.frames) {
		return 0, nil, io.EOF
	}
	frame := c.frames[c.index]
	c.index++
	return websocketTextMessage, strings.NewReader(frame), nil
}

// The reader loop must ignore frames it does not understand instead of dropping
// the connection: a newer Agent may send message types an older controller does
// not know, and one malformed frame must not cost the Agent its presence.
func TestReadAgentPresenceMessagesIgnoresUnknownAndMalformedFrames(t *testing.T) {
	applied := mustMarshal(t, agentPresenceClientMessage{Type: "config_applied", Version: 7})
	conn := &scriptedPresenceConn{frames: []string{
		"{not json",
		`{"type":"something_new"}`,
		applied,
		`{"type":"config_applied"`,
	}}

	handler := &handler{agentQuotas: newAgentQuotaManager()}
	refreshed := 0
	handler.readAgentPresenceMessages(t.Context(), conn, nil, "node-a", func() error {
		refreshed++
		return nil
	})

	// All four frames must be consumed: the loop only exits on read failure.
	if conn.index != len(conn.frames) {
		t.Fatalf("consumed %d frames, want %d", conn.index, len(conn.frames))
	}
	if refreshed != len(conn.frames) {
		t.Fatalf("read deadline refreshed %d times, want %d", refreshed, len(conn.frames))
	}
	// The inbound frame limit bounds what an authenticated Agent can make the
	// controller buffer.
	if conn.readLimit != 4<<10 {
		t.Fatalf("read limit = %d, want %d", conn.readLimit, 4<<10)
	}
}

// A failing read-deadline refresh means the connection is no longer usable, so
// the loop must stop rather than spin.
func TestReadAgentPresenceMessagesStopsWhenDeadlineRefreshFails(t *testing.T) {
	conn := &scriptedPresenceConn{frames: []string{`{"type":"config_applied"}`, `{"type":"config_applied"}`}}
	handler := &handler{agentQuotas: newAgentQuotaManager()}

	handler.readAgentPresenceMessages(t.Context(), conn, nil, "node-a", func() error {
		return io.ErrClosedPipe
	})

	if conn.index != 1 {
		t.Fatalf("consumed %d frames, want 1 before bailing out", conn.index)
	}
}

// A store that cannot record acknowledgements must not end the stream: presence
// itself still works, so the connection stays useful.
func TestRecordAgentPresenceConfigAppliedKeepsStreamWithoutAckStore(t *testing.T) {
	handler := &handler{agentQuotas: newAgentQuotaManager()}
	if !handler.recordAgentPresenceConfigApplied(t.Context(), nil, "node-a", 3) {
		t.Fatal("a store without ack support must not close the stream")
	}
}

func mustMarshal(t *testing.T, value any) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(value); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return buffer.String()
}
