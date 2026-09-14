package tunnel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestTunnelBinaryProtocolEndToEnd drives a real Tunnel against a mock tunnel
// server and a local HTTP app, asserting the v2 binary framing both ways.
func TestTunnelBinaryProtocolEndToEnd(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "pong")
	}))
	defer local.Close()
	localURL, _ := url.Parse(local.URL)
	localPort, err := strconv.Atoi(localURL.Port())
	if err != nil {
		t.Fatalf("local port: %v", err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	type captured struct {
		messageType int
		resp        TunnelResponse
	}
	capturedCh := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tunnel/connect" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("proto") != "2" {
			t.Errorf("client did not request proto=2, got %q", r.URL.Query().Get("proto"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		meta, _ := json.Marshal(tunnelMeta{ID: "req-1", Method: "GET", Path: "/hello"})
		if err := conn.WriteMessage(websocket.BinaryMessage, encodeTunnelFrame(meta, nil)); err != nil {
			return
		}
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		metaBytes, body, err := decodeTunnelFrame(data)
		if err != nil {
			return
		}
		var m tunnelMeta
		_ = json.Unmarshal(metaBytes, &m)
		capturedCh <- captured{messageType: messageType, resp: TunnelResponse{ID: m.ID, Status: m.Status, Body: body}}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tun := &Tunnel{ServerURL: srv.URL, ClientID: "test-client", LocalPort: localPort}
	go func() { _ = tun.Start(ctx) }()

	select {
	case got := <-capturedCh:
		if got.messageType != websocket.BinaryMessage {
			t.Errorf("response message type = %d, want binary", got.messageType)
		}
		if got.resp.ID != "req-1" {
			t.Errorf("response id = %q, want req-1", got.resp.ID)
		}
		if got.resp.Status != http.StatusOK {
			t.Errorf("status = %d, want 200", got.resp.Status)
		}
		if string(got.resp.Body) != "pong" {
			t.Errorf("body = %q, want pong", got.resp.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tunneled response")
	}
}

// TestTunnelLegacyFallback verifies a client connected to an old JSON server
// still works: first inbound Text frame flips it out of binary mode.
func TestTunnelLegacyFallback(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "legacy-pong")
	}))
	defer local.Close()
	localURL, _ := url.Parse(local.URL)
	localPort, _ := strconv.Atoi(localURL.Port())

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	type captured struct {
		messageType int
		resp        TunnelResponse
		raw         string
	}
	capturedCh := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Server ignores proto=2 and writes legacy JSON.
		legacy, _ := json.Marshal(TunnelRequest{ID: "old-1", Method: "GET", Path: "/"})
		_ = conn.WriteMessage(websocket.TextMessage, legacy)

		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var resp TunnelResponse
		_ = json.Unmarshal(data, &resp)
		capturedCh <- captured{messageType: messageType, resp: resp, raw: string(data)}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tun := &Tunnel{ServerURL: srv.URL, ClientID: "legacy-client", LocalPort: localPort}
	go func() { _ = tun.Start(ctx) }()

	select {
	case got := <-capturedCh:
		if got.messageType != websocket.TextMessage {
			t.Errorf("response should be legacy text, got type %d (%s)", got.messageType, got.raw)
		}
		if got.resp.Status != http.StatusOK || string(got.resp.Body) != "legacy-pong" {
			t.Errorf("resp = %+v body=%q", got.resp, got.resp.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for legacy response")
	}
}
