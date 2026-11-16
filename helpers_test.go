package commypanel

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const testToken = "7|cpt_secretsecretsecretsecretsecretsecret"

// recorded is one request a test server saw.
type recorded struct {
	Method string
	Path   string
	Query  string
	Body   string
	Header http.Header
}

// panelServer is a fake panel: it records every request and answers with whatever the test sets.
type panelServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
	handler  func(w http.ResponseWriter, r *http.Request, n int)
}

func newPanel(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, n int)) *panelServer {
	t.Helper()

	p := &panelServer{handler: handler}
	p.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0)
		if r.Body != nil {
			buf := make([]byte, 4096)
			for {
				n, err := r.Body.Read(buf)
				body = append(body, buf[:n]...)
				if err != nil {
					break
				}
			}
		}

		p.mu.Lock()
		p.requests = append(p.requests, recorded{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Clone()})
		n := len(p.requests)
		p.mu.Unlock()

		p.handler(w, r, n)
	}))
	t.Cleanup(p.Close)

	return p
}

func (p *panelServer) seen() []recorded {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]recorded(nil), p.requests...)
}

func (p *panelServer) last(t *testing.T) recorded {
	t.Helper()

	all := p.seen()
	if len(all) == 0 {
		t.Fatal("the panel saw no request")
	}

	return all[len(all)-1]
}

// client returns a client that trusts the fake panel's certificate and does not really sleep.
func (p *panelServer) client(t *testing.T, opts ...Option) *Client {
	t.Helper()

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.Certificate().Raw})
	c, err := New(p.URL, testToken, append([]Option{WithCACertPEM(certPEM)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}

	c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }

	return c
}

func jsonReply(status int, body string) func(http.ResponseWriter, *http.Request, int) {
	return func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}
