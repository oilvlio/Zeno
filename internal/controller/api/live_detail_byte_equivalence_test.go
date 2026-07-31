package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestLiveDetailInitialPayloadMatchesV191Bytes(t *testing.T) {
	stateWindow, ok := resolveStateWindow("1d")
	if !ok {
		t.Fatal("resolve state window")
	}
	latencyWindow, ok := resolveLatencyWindow("1h")
	if !ok {
		t.Fatal("resolve latency window")
	}

	tests := []struct {
		name       string
		path       string
		wantBytes  int
		wantSHA256 string
		handle     func(*handler, http.ResponseWriter, *http.Request)
	}{
		{name: "node-state", path: "/node-state", wantBytes: 468859, wantSHA256: "e9b01e10276ba317d02db5f4f230fc66fe460dfa3995952c6e73ca695eb9998b", handle: func(h *handler, w http.ResponseWriter, r *http.Request) {
			h.handleNodeStateWebSocket(w, r, "example-node-a", stateWindow)
		}},
		{name: "node-latency", path: "/node-latency", wantBytes: 4012, wantSHA256: "4cc415dd224a5ad71776e1f6a435874067ad19ba05d8a81de24f7a32d11baecb", handle: func(h *handler, w http.ResponseWriter, r *http.Request) {
			h.handleNodeLatencyWebSocket(w, r, "example-node-a", latencyWindow)
		}},
		{name: "service-latency", path: "/service-latency", wantBytes: 3363, wantSHA256: "dcf481720a46f9fe5e6ab61bf90d307b0148c3b85f131f64f4b021ac56413839", handle: func(h *handler, w http.ResponseWriter, r *http.Request) {
			h.handleServiceLatencyWebSocket(w, r, "google", latencyWindow)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := &handler{
				store:          mockStore{},
				liveHub:        newLiveUpdateHub(),
				publicWSGate:   newWebSocketGateWithPerKey(8, 2),
				detailCache:    newDetailJSONCache(),
				trustedProxies: TrustedProxySet{},
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				test.handle(h, w, r)
			}))
			defer server.Close()

			conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+test.path, nil)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			if response != nil && response.Body != nil {
				defer response.Body.Close()
			}
			defer conn.Close()

			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read initial payload: %v", err)
			}
			gotSHA256 := fmt.Sprintf("%x", sha256.Sum256(payload))
			if len(payload) != test.wantBytes || gotSHA256 != test.wantSHA256 {
				t.Fatalf("initial payload bytes=%d sha256=%s, want bytes=%d sha256=%s", len(payload), gotSHA256, test.wantBytes, test.wantSHA256)
			}
		})
	}
}
