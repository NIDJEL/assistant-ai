package stt

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroqSOCKS5RemoteDNSAndIsolation(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("lost Groq authentication")
		}
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credentials reached origin")
		}
		io.WriteString(w, `{"text":"Проверить смету"}`)
	}))
	defer origin.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		check := func() error {
			header := make([]byte, 2)
			if _, err := io.ReadFull(conn, header); err != nil {
				return err
			}
			methods := make([]byte, int(header[1]))
			if _, err := io.ReadFull(conn, methods); err != nil {
				return err
			}
			if header[0] != 5 {
				return errors.New("not SOCKS5")
			}
			conn.Write([]byte{5, 2})
			if _, err := io.ReadFull(conn, header); err != nil {
				return err
			}
			username := make([]byte, int(header[1]))
			if _, err := io.ReadFull(conn, username); err != nil {
				return err
			}
			length := make([]byte, 1)
			if _, err := io.ReadFull(conn, length); err != nil {
				return err
			}
			password := make([]byte, int(length[0]))
			if _, err := io.ReadFull(conn, password); err != nil {
				return err
			}
			if string(username) != "test-user" || string(password) != "test-password" {
				return errors.New("SOCKS5 authentication missing")
			}
			conn.Write([]byte{1, 0})
			request := make([]byte, 4)
			if _, err := io.ReadFull(conn, request); err != nil {
				return err
			}
			if request[0] != 5 || request[1] != 1 || request[3] != 3 {
				return errors.New("SOCKS5 must resolve domain remotely")
			}
			if _, err := io.ReadFull(conn, length); err != nil {
				return err
			}
			host := make([]byte, int(length[0]))
			if _, err := io.ReadFull(conn, host); err != nil {
				return err
			}
			port := make([]byte, 2)
			if _, err := io.ReadFull(conn, port); err != nil {
				return err
			}
			if string(host) != "audio.groq.invalid" || binary.BigEndian.Uint16(port) != 443 {
				return errors.New("wrong SOCKS5 target")
			}
			upstream, err := net.Dial("tcp", strings.TrimPrefix(origin.URL, "https://"))
			if err != nil {
				return err
			}
			defer upstream.Close()
			conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
			go io.Copy(upstream, conn)
			_, err = io.Copy(conn, upstream)
			return err
		}
		result <- check()
	}()
	before := http.DefaultTransport
	g, err := NewGroqWithProxy("test-key", "", "socks5h://test-user:test-password@"+listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	g.client.Transport.(*http.Transport).TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	// The test certificate is for example.com; keep TLS verification enabled.
	g.client.Transport.(*http.Transport).TLSClientConfig.ServerName = "example.com"
	g.endpoint = "https://audio.groq.invalid/transcriptions"
	text, err := g.Transcribe(t.Context(), "audio.wav", "audio/wav", []byte("test audio"))
	g.Close()
	if err != nil || text != "Проверить смету" {
		t.Fatalf("transcribe=%q error=%v", text, err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if http.DefaultTransport != before {
		t.Fatal("global transport changed")
	}
	// An unrelated client still connects directly and never enters this proxy.
	req, _ := http.NewRequestWithContext(t.Context(), "GET", origin.URL, nil)
	req.Header.Set("Authorization", "Bearer test-key")
	response, err := origin.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestGroqProxyValidationAndFailure(t *testing.T) {
	for _, value := range []string{"socks5h://test-user:test-password@localhost:wrong", "ftp://test-user:test-password@localhost:1080", "socks5://", "socks5://localhost:99999", "http://localhost:1080/path?test-password"} {
		_, err := NewGroqWithProxy("test-key", "", value)
		if err == nil || strings.Contains(err.Error(), "test-password") {
			t.Fatal("unsafe proxy configuration error")
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	g, err := NewGroqWithProxy("test-key", "", "socks5h://test-user:test-password@"+address)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if _, err = g.Transcribe(context.Background(), "audio.wav", "audio/wav", []byte("audio")); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "test-password") {
		t.Fatal("proxy error leaked or direct fallback occurred")
	}
}

func TestGroqDirectModeIgnoresGlobalProxySettings(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	g, err := NewGroqWithProxy("test-key", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if g.client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("Groq routing inherited process proxy")
	}
}
