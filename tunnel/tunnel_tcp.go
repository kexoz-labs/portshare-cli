package tunnel

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

var e2eTLSConfig *tls.Config

func generateSelfSignedCert() (*tls.Config, error) {
	if e2eTLSConfig != nil {
		return e2eTLSConfig, nil
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"PortShare E2E"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	e2eTLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	return e2eTLSConfig, nil
}

// StartMultiplexedWS connects to the server's WS port and multiplexes using Yamux.
func (t *Tunnel) StartMultiplexedWS(ctx context.Context) error {
	if !t.IsTCP {
		SetReplayTarget(t.LocalPort, localHTTPClient.Do)
		go StartInspector(4040)
	}

	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		connectedAt := time.Now()
		err := t.startOnceWS(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			fmt.Printf("Multiplexed Tunnel disconnected (%v). Reconnecting in %s...\n", err, backoff.Round(time.Second))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
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

func (t *Tunnel) startOnceWS(ctx context.Context) error {
	serverURL, err := url.Parse(t.ServerURL)
	if err != nil {
		return err
	}
	scheme := "ws"
	if serverURL.Scheme == "https" {
		scheme = "wss"
	}
	
	wsURL := fmt.Sprintf("%s://%s/tunnel/connect?clientId=%s&proto=3", scheme, serverURL.Host, url.QueryEscape(t.ClientID))

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial failed: %w", err)
	}
	defer conn.Close()

	if t.IsE2E {
		fmt.Printf("E2E Tunnel connected to %s\n", t.ServerURL)
	} else if t.IsTCP {
		fmt.Printf("Raw TCP Tunnel connected to %s\n", t.ServerURL)
	}
	fmt.Printf("Forwarding requests to http://localhost:%d\n", t.LocalPort)

	// We are the Yamux Server, the Gateway is the Yamux Client
	wsconn := &WSNetConn{Conn: conn}
	session, err := yamux.Server(wsconn, yamux.DefaultConfig())
	if err != nil {
		return fmt.Errorf("yamux setup failed: %w", err)
	}
	defer session.Close()

	errCh := make(chan error, 1)

	go func() {
		for {
			stream, err := session.AcceptStream()
			if err != nil {
				errCh <- err
				return
			}
			go t.handleStream(stream)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// StartTCP connects to the server's raw TCP port and multiplexes using Yamux.
func (t *Tunnel) StartTCP(ctx context.Context, tcpAddr string) error {
	if !t.IsTCP && !t.IsUDP && !t.IsPTY {
		SetReplayTarget(t.LocalPort, localHTTPClient.Do)
		go StartInspector(4040)
	}

	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		connectedAt := time.Now()
		err := t.startOnceTCP(ctx, tcpAddr)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			fmt.Printf("TCP Tunnel disconnected (%v). Reconnecting in %s...\n", err, backoff.Round(time.Second))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
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

func (t *Tunnel) startOnceTCP(ctx context.Context, tcpAddr string) error {
	// 1. Dial the server (try TLS, fallback to TCP for local dev if TLS fails or not configured)
	// For simplicity, we dial plain TCP if it's localhost, else TLS. Or just try TLS first.
	// Since we might not have a valid cert locally, let's use plain TCP if insecure.
	
	serverURL, _ := url.Parse(t.ServerURL)
	
	var conn net.Conn
	var err error
	if serverURL.Scheme == "https" {
		conn, err = tls.Dial("tcp", tcpAddr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = net.Dial("tcp", tcpAddr)
	}
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Handshake
	authMsg := fmt.Sprintf("AUTH %s\n", t.ClientID)
	if _, err := conn.Write([]byte(authMsg)); err != nil {
		return fmt.Errorf("auth write failed: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("auth read failed: %w", err)
	}
	if resp != "OK\n" {
		return fmt.Errorf("server rejected auth: %s", resp)
	}

	fmt.Printf("Raw TCP Tunnel connected to %s\n", tcpAddr)
	fmt.Printf("Forwarding requests to http://localhost:%d\n", t.LocalPort)

	// Setup Yamux server side (since server opens streams to us)
	session, err := yamux.Server(conn, yamux.DefaultConfig())
	if err != nil {
		return fmt.Errorf("yamux setup failed: %w", err)
	}
	defer session.Close()

	errCh := make(chan error, 1)

	go func() {
		for {
			stream, err := session.AcceptStream()
			if err != nil {
				errCh <- err
				return
			}
			go t.handleStream(stream)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (t *Tunnel) handleStream(stream net.Conn) {
	defer stream.Close()

	if t.IsTCP {
		localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", t.LocalPort))
		if err != nil {
			return
		}
		defer localConn.Close()

		errc := make(chan error, 2)
		go func() {
			_, err := io.Copy(stream, localConn)
			errc <- err
		}()
		go func() {
			_, err := io.Copy(localConn, stream)
			errc <- err
		}()
		<-errc
		return
	}

	if t.IsPTY {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("powershell.exe")
		} else {
			cmd = exec.Command("bash")
		}
		
		cmd.Stdin = stream
		cmd.Stdout = stream
		cmd.Stderr = stream

		if err := cmd.Start(); err != nil {
			stream.Write([]byte(fmt.Sprintf("Error starting shell: %v\r\n", err)))
			return
		}
		
		cmd.Wait()
		return
	}

	if t.IsUDP {
		localConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", t.LocalPort))
		if err != nil {
			return
		}
		defer localConn.Close()

		errc := make(chan error, 2)

		// Read framed packets from yamux stream and write to local UDP
		go func() {
			for {
				var length uint16
				if err := binary.Read(stream, binary.BigEndian, &length); err != nil {
					errc <- err
					return
				}
				packet := make([]byte, length)
				if _, err := io.ReadFull(stream, packet); err != nil {
					errc <- err
					return
				}
				if _, err := localConn.Write(packet); err != nil {
					errc <- err
					return
				}
			}
		}()

		// Read responses from local UDP and write framed packets to yamux stream
		go func() {
			buf := make([]byte, 65535)
			for {
				n, err := localConn.Read(buf)
				if err != nil {
					errc <- err
					return
				}
				lenBuf := make([]byte, 2)
				binary.BigEndian.PutUint16(lenBuf, uint16(n))
				if _, err := stream.Write(lenBuf); err != nil {
					errc <- err
					return
				}
				if _, err := stream.Write(buf[:n]); err != nil {
					errc <- err
					return
				}
			}
		}()

		<-errc
		return
	}
	
	var connToRead net.Conn = stream
	if t.IsE2E {
		tlsConfig, err := generateSelfSignedCert()
		if err != nil {
			return
		}
		tlsConn := tls.Server(stream, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		connToRead = tlsConn
	}
	
	// Read HTTP request from the stream
	req, err := http.ReadRequest(bufio.NewReader(connToRead))
	if err != nil {
		return
	}

	// Update request to target localhost
	req.URL.Scheme = "http"
	req.URL.Host = fmt.Sprintf("127.0.0.1:%d", t.LocalPort)
	req.RequestURI = ""
	
	// Execute local request via Interceptor
	resp, err := Intercept(req, localHTTPClient.Do)
	if err != nil {
		// Send 502 Bad Gateway
		errorResp := "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"
		connToRead.Write([]byte(errorResp))
		return
	}
	defer resp.Body.Close()

	// Write response back to the stream
	resp.Write(connToRead)
}

// WSNetConn wraps gorilla/websocket to implement net.Conn
type WSNetConn struct {
	Conn   *websocket.Conn
	Reader io.Reader
}

func (c *WSNetConn) Read(b []byte) (int, error) {
	for {
		if c.Reader == nil {
			msgType, reader, err := c.Conn.NextReader()
			if err != nil {
				return 0, err
			}
			if msgType != websocket.BinaryMessage {
				continue // ignore non-binary messages
			}
			c.Reader = reader
		}
		n, err := c.Reader.Read(b)
		if err == io.EOF {
			c.Reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (c *WSNetConn) Write(b []byte) (int, error) {
	err := c.Conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *WSNetConn) Close() error {
	return c.Conn.Close()
}

func (c *WSNetConn) LocalAddr() net.Addr                { return c.Conn.LocalAddr() }
func (c *WSNetConn) RemoteAddr() net.Addr               { return c.Conn.RemoteAddr() }
func (c *WSNetConn) SetDeadline(t time.Time) error      { return c.Conn.SetReadDeadline(t) }
func (c *WSNetConn) SetReadDeadline(t time.Time) error  { return c.Conn.SetReadDeadline(t) }
func (c *WSNetConn) SetWriteDeadline(t time.Time) error { return c.Conn.SetWriteDeadline(t) }
