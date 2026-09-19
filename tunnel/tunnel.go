package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxLocalResponse = 10 << 20
	tunnelReadLimit  = 16 << 20
	pongWait         = 90 * time.Second
)

// localHTTPClient is shared across all forwarded requests so localhost
// connections are reused (keep-alive) instead of re-dialed per request.
var localHTTPClient = &http.Client{
	Timeout: 55 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // do not follow redirects
	},
}

type TunnelRequest struct {
	ID      string              `json:"id"`
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body,omitempty"`
}

type TunnelResponse struct {
	ID      string              `json:"id"`
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body,omitempty"`
	Error   string              `json:"error,omitempty"`
}

type Tunnel struct {
	ServerURL string
	ClientID  string
	LocalPort int
	IsTCP     bool
	IsE2E     bool
	IsUDP     bool
	IsPTY     bool

	// OnRequest is an optional callback called after each request is forwarded.
	OnRequest func(method, path string, status int, durationMs int64)

	conn   *websocket.Conn
	mu     sync.Mutex
	binary atomic.Bool
}

func (t *Tunnel) wsURL() (string, error) {
	serverURL, err := url.Parse(t.ServerURL)
	if err != nil {
		return "", err
	}
	if serverURL.Host == "" {
		return "", fmt.Errorf("invalid server URL: %s", t.ServerURL)
	}
	scheme := "ws"
	if serverURL.Scheme == "https" {
		scheme = "wss"
	}
	// proto=2 requests raw binary framing; older servers ignore it and reply
	// with legacy JSON, which the read loop auto-detects.
	return fmt.Sprintf("%s://%s/tunnel/connect?clientId=%s&proto=2", scheme, serverURL.Host, url.QueryEscape(t.ClientID)), nil
}

// Start connects and forwards until ctx is cancelled, reconnecting with
// backoff on drops. It returns nil on clean shutdown.
func (t *Tunnel) Start(ctx context.Context) error {
	SetReplayTarget(t.LocalPort, localHTTPClient.Do)
	go StartInspector(4040)

	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		connectedAt := time.Now()
		err := t.startOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			fmt.Printf("Tunnel disconnected (%v). Reconnecting in %s...\n", err, backoff.Round(time.Second))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		// Reset backoff after a healthy long-lived connection.
		if time.Since(connectedAt) > 30*time.Second {
			backoff = time.Second
		} else if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (t *Tunnel) startOnce(ctx context.Context) error {
	wsURL, err := t.wsURL()
	if err != nil {
		return err
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to tunnel: %w", err)
	}
	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()
	defer func() {
		_ = conn.Close()
		t.mu.Lock()
		t.conn = nil
		t.mu.Unlock()
	}()

	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	conn.SetReadLimit(tunnelReadLimit)

	fmt.Printf("Tunnel connected to %s\n", t.ServerURL)
	fmt.Printf("Forwarding requests to http://localhost:%d\n", t.LocalPort)

	errCh := make(chan error, 1)
	go func() {
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			var req TunnelRequest
			if messageType == websocket.BinaryMessage {
				t.binary.Store(true)
				meta, body, decodeErr := decodeTunnelFrame(data)
				if decodeErr != nil {
					errCh <- decodeErr
					return
				}
				var m tunnelMeta
				if jsonErr := json.Unmarshal(meta, &m); jsonErr != nil {
					errCh <- jsonErr
					return
				}
				req = TunnelRequest{ID: m.ID, Method: m.Method, Path: m.Path, Headers: m.Headers, Body: body}
			} else {
				// Legacy JSON protocol (server ignored proto=2).
				if jsonErr := json.Unmarshal(data, &req); jsonErr != nil {
					errCh <- jsonErr
					return
				}
			}
			go t.handleRequest(req)
		}
	}()

	select {
	case <-ctx.Done():
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(5*time.Second))
		return nil
	case err := <-errCh:
		return fmt.Errorf("tunnel connection closed: %w", err)
	}
}

func (t *Tunnel) handleRequest(req TunnelRequest) {
	resp := TunnelResponse{
		ID:      req.ID,
		Headers: make(map[string][]string),
	}

	start := time.Now()

	var reqBody io.Reader
	if len(req.Body) > 0 {
		reqBody = bytes.NewReader(req.Body)
	}

	// 127.0.0.1 (not "localhost") avoids IPv6 ::1 resolution mismatches when
	// the local dev server only binds IPv4.
	localURL := fmt.Sprintf("http://127.0.0.1:%d%s", t.LocalPort, req.Path)
	httpReq, err := http.NewRequest(req.Method, localURL, reqBody)
	if err != nil {
		resp.Error = fmt.Sprintf("failed to create local request: %v", err)
		t.sendResponse(resp)
		if t.OnRequest != nil {
			t.OnRequest(req.Method, req.Path, 0, time.Since(start).Milliseconds())
		}
		return
	}

	for k, v := range req.Headers {
		// Avoid passing hop-by-hop headers or host if we don't want to
		if strings.ToLower(k) != "host" {
			httpReq.Header[k] = v
		}
	}

	httpResp, err := Intercept(httpReq, localHTTPClient.Do)
	if err != nil {
		resp.Error = fmt.Sprintf("local server error: %v", err)
		t.sendResponse(resp)
		if t.OnRequest != nil {
			t.OnRequest(req.Method, req.Path, 0, time.Since(start).Milliseconds())
		}
		return
	}
	defer httpResp.Body.Close()

	resp.Status = httpResp.StatusCode
	for k, v := range httpResp.Header {
		resp.Headers[k] = v
	}

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, maxLocalResponse+1))
	if err != nil {
		resp.Error = fmt.Sprintf("failed to read local response: %v", err)
		t.sendResponse(resp)
		if t.OnRequest != nil {
			t.OnRequest(req.Method, req.Path, httpResp.StatusCode, time.Since(start).Milliseconds())
		}
		return
	}
	if len(respBody) > maxLocalResponse {
		resp.Error = "local response too large (10 MB max)"
		t.sendResponse(resp)
		if t.OnRequest != nil {
			t.OnRequest(req.Method, req.Path, httpResp.StatusCode, time.Since(start).Milliseconds())
		}
		return
	}

	if len(respBody) > 0 {
		resp.Body = respBody
	}

	t.sendResponse(resp)
	if t.OnRequest != nil {
		t.OnRequest(req.Method, req.Path, httpResp.StatusCode, time.Since(start).Milliseconds())
	}
}

func (t *Tunnel) sendResponse(resp TunnelResponse) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return
	}
	var err error
	if t.binary.Load() {
		meta, marshalErr := json.Marshal(tunnelMeta{
			ID: resp.ID, Status: resp.Status, Headers: resp.Headers, Error: resp.Error,
		})
		if marshalErr != nil {
			return
		}
		err = t.conn.WriteMessage(websocket.BinaryMessage, encodeTunnelFrame(meta, resp.Body))
	} else {
		err = t.conn.WriteJSON(resp)
	}
	if err != nil {
		fmt.Printf("failed to send response: %v\n", err)
	}
}
