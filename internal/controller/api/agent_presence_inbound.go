package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"time"
)

// agentPresenceConn is the inbound half of a presence WebSocket.
//
// The reader loop is expressed against an interface rather than *websocket.Conn
// so its framing, quota and error-handling contract can be tested without
// standing up a real WebSocket connection.
type agentPresenceConn interface {
	SetReadLimit(limit int64)
	NextReader() (int, io.Reader, error)
}

// readAgentPresenceMessages consumes the Agent's half of a presence WebSocket
// until the peer goes away, and returns when the connection can no longer be
// read.
//
// The only message the controller acts on is the probe-config acknowledgement;
// anything else is ignored rather than treated as an error, so a newer Agent can
// send additional message types without breaking against an older controller.
//
// Every successful read refreshes the read deadline, which is how a silent but
// live connection is distinguished from a dead one: pings alone would keep the
// socket open even if the Agent had stopped reading.
func (h *handler) readAgentPresenceMessages(
	ctx context.Context,
	conn agentPresenceConn,
	store agentStore,
	nodeID string,
	refreshReadDeadline func() error,
) {
	// The inbound frame limit is deliberately small: acknowledgements are tiny,
	// and this bounds what an authenticated Agent can force the controller to
	// buffer.
	conn.SetReadLimit(4 << 10)
	for {
		_, reader, err := conn.NextReader()
		if err != nil {
			return
		}
		if err := refreshReadDeadline(); err != nil {
			return
		}
		var message agentPresenceClientMessage
		if err := json.NewDecoder(reader).Decode(&message); err != nil {
			// A malformed frame is dropped rather than closing the stream, so one
			// bad message does not cost the Agent its presence connection.
			continue
		}
		if message.Type != "config_applied" {
			continue
		}
		if !h.recordAgentPresenceConfigApplied(ctx, store, nodeID, message.Version) {
			return
		}
	}
}

// recordAgentPresenceConfigApplied persists a probe-config acknowledgement and
// reports whether the stream may continue.
//
// It returns false only when the node exhausted its write quota: HTTP 429 is no
// longer available after the upgrade, so an abusive per-node stream is closed
// instead of being allowed to amplify SQLite writes or affect another node.
// Storage errors keep the stream open because they are the controller's problem,
// not the Agent's.
func (h *handler) recordAgentPresenceConfigApplied(
	ctx context.Context,
	store agentStore,
	nodeID string,
	version int64,
) bool {
	ackStore, ok := store.(probeConfigAppliedStore)
	if !ok {
		return true
	}
	releaseWrite, _, accepted := h.agentQuotas.admitWrite(nodeID, 1)
	if !accepted {
		return false
	}
	err := ackStore.RecordProbeConfigApplied(ctx, nodeID, version, time.Now().UTC())
	releaseWrite()
	if err != nil && !errors.Is(err, errProbeConfigAckInvalid) {
		// An invalid acknowledgement is the Agent reporting a version the
		// controller never issued, which is expected during config churn and is
		// not worth logging.
		log.Printf("agent_presence_ack_error endpoint=presence node_id=%s stage=config_applied error=%s",
			safeLogToken(nodeID), sanitizeAgentAPIError(err))
	}
	return true
}
