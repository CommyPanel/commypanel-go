package commypanel

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the version of this library, sent in the User-Agent.
const Version = "0.1.0"

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxBody    = 16 << 20 // 16 MiB: far more than 5000 log lines
	defaultRetries    = 2
	maxRetryWait      = 30 * time.Second
	basePath          = "/api/v1"
	jsonContentType   = "application/json"
	maxErrorBodyBytes = 1 << 20
)

// Client talks to one CommyPanel server with one token. It is safe for concurrent use.
type Client struct {
	base    string
	token   string
	http    *http.Client
	retries int
	maxBody int64
	agent   string
	sleep   func(ctx context.Context, d time.Duration) error
}

// Option changes how a Client behaves.
type Option func(*options) error

type options struct {
	allowLoopbackHTTP bool
	httpClient        *http.Client
	caPEM             []byte
	timeout           time.Duration
	retries           int
	maxBody           int64
	userAgent         string
}

// WithAllowHTTPLoopback allows a plain http address, but only for localhost, 127.0.0.1 or ::1 (for
// development against a panel on the same machine). Any other http address is still refused.
func WithAllowHTTPLoopback() Option {
	return func(o *options) error { o.allowLoopbackHTTP = true; return nil }
}

// WithCACertPEM trusts one more certificate authority, in PEM, besides the system's. Use it for a panel
// whose certificate comes from a private CA. There is deliberately no option to skip verification.
func WithCACertPEM(pem []byte) Option {
	return func(o *options) error { o.caPEM = pem; return nil }
}

// WithTimeout sets the time one request may take (default 30 seconds).
func WithTimeout(d time.Duration) Option {
	return func(o *options) error {
		if d <= 0 {
			return errors.New("commypanel: the timeout must be positive")
		}

		o.timeout = d

		return nil
	}
}

// WithRetries sets how many times a failed request may be tried again (default 2, at most 5).
// Only requests that are safe to repeat are retried: any request the panel refused with 429, and
// reading calls that met a 502, 503 or 504.
func WithRetries(n int) Option {
	return func(o *options) error {
		if n < 0 || n > 5 {
			return errors.New("commypanel: retries must be between 0 and 5")
		}

		o.retries = n

		return nil
	}
}

// WithMaxResponseBytes limits the size of an answer the client will read (default 16 MiB).
func WithMaxResponseBytes(n int64) Option {
	return func(o *options) error {
		if n <= 0 {
			return errors.New("commypanel: the limit must be positive")
		}

		o.maxBody = n

		return nil
	}
}

// WithUserAgent adds a name (your tool) in front of the library's User-Agent.
func WithUserAgent(name string) Option {
	return func(o *options) error { o.userAgent = strings.TrimSpace(name); return nil }
}

// WithHTTPClient uses your own http.Client (for a proxy, or to set transport details). Its
// redirects are still never followed, and the address and token are still checked.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) error { o.httpClient = c; return nil }
}

// New makes a client for the panel at baseURL (https://panel.example.com) with a token.
func New(baseURL, token string, opts ...Option) (*Client, error) {
	o := options{timeout: defaultTimeout, retries: defaultRetries, maxBody: defaultMaxBody}

	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}

	base, err := checkBaseURL(baseURL, o.allowLoopbackHTTP)
	if err != nil {
		return nil, err
	}

	if err := checkToken(token); err != nil {
		return nil, err
	}

	hc, err := o.buildHTTPClient()
	if err != nil {
		return nil, err
	}

	agent := "commypanel-go/" + Version

	if o.userAgent != "" {
		agent = o.userAgent + " " + agent
	}

	return &Client{base: base, token: token, http: hc, retries: o.retries, maxBody: o.maxBody, agent: agent, sleep: sleepContext}, nil
}

func (o options) buildHTTPClient() (*http.Client, error) {
	var hc http.Client

	if o.httpClient != nil {
		hc = *o.httpClient
	} else {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		hc = http.Client{Transport: transport}
	}

	if o.httpClient == nil || hc.Timeout == 0 {
		hc.Timeout = o.timeout
	}

	// The token must never go to another address: a redirect is an error, not something to follow.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	if len(o.caPEM) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}

		if !pool.AppendCertsFromPEM(o.caPEM) {
			return nil, errors.New("commypanel: the CA certificate is not valid PEM")
		}

		transport, ok := hc.Transport.(*http.Transport)
		if !ok {
			return nil, errors.New("commypanel: a CA certificate needs the default transport")
		}

		transport = transport.Clone()
		cfg := transport.TLSClientConfig

		if cfg == nil {
			cfg = &tls.Config{MinVersion: tls.VersionTLS12}
		} else {
			cfg = cfg.Clone()
		}

		cfg.RootCAs = pool
		transport.TLSClientConfig = cfg
		hc.Transport = transport
	}

	return &hc, nil
}

func checkBaseURL(raw string, allowLoopbackHTTP bool) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", ErrInvalidURL
	}

	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/" && strings.TrimRight(u.Path, "/") != basePath) {
		return "", ErrInvalidURL
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !allowLoopbackHTTP || !isLoopback(u.Hostname()) {
			return "", ErrInsecureURL
		}
	default:
		return "", ErrInsecureURL
	}

	return u.Scheme + "://" + u.Host, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func checkToken(token string) error {
	if token == "" || len(token) > 512 {
		return ErrInvalidToken
	}

	for _, r := range token {
		if r <= ' ' || r == 0x7f {
			return ErrInvalidToken
		}
	}

	return nil
}

// String describes the client without the token.
func (c *Client) String() string { return "commypanel.Client{" + c.base + ", token: redacted}" }

// GoString makes %#v safe too.
func (c *Client) GoString() string { return c.String() }

// BaseURL is the panel address the client uses.
func (c *Client) BaseURL() string { return c.base }

// do makes one call and decodes a JSON answer of the form {"data": ...} into out (if not nil). It
// returns the HTTP status, which the caller needs to tell 200 from 201.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) (int, error) {
	var payload []byte

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("commypanel: encoding the request: %w", err)
		}

		payload = encoded
	}

	target := c.base + basePath + path

	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		status, retryAfter, err := c.once(ctx, method, target, payload, out)

		if err == nil || attempt >= c.retries || !c.retryable(method, status) {
			return status, err
		}

		wait := retryAfter

		if wait <= 0 {
			wait = time.Duration(500<<attempt) * time.Millisecond
		}

		if wait > maxRetryWait {
			wait = maxRetryWait
		}

		if err := c.sleep(ctx, wait); err != nil {
			return status, err
		}
	}
}

// retryable: a 429 means the panel did not do the work, so any call may be repeated; a gateway error
// is only repeated for calls that read.
func (c *Client) retryable(method string, status int) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return method == http.MethodGet
	}

	return false
}

func (c *Client) once(ctx context.Context, method, target string, payload []byte, out any) (int, time.Duration, error) {
	var reader io.Reader

	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, 0, fmt.Errorf("commypanel: building the request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", jsonContentType)
	req.Header.Set("User-Agent", c.agent)

	if payload != nil {
		req.Header.Set("Content-Type", jsonContentType)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// The URL has no secret in it, but a transport error can quote it: keep only the cause.
		return 0, 0, fmt.Errorf("commypanel: %s %s: %w", method, req.URL.Path, scrub(err, c.token))
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return resp.StatusCode, 0, ErrRedirect
	}

	limit := c.maxBody

	if resp.StatusCode >= 400 && limit > maxErrorBodyBytes {
		limit = maxErrorBodyBytes
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, 0, fmt.Errorf("commypanel: reading the answer: %w", scrub(err, c.token))
	}

	if int64(len(raw)) > limit {
		return resp.StatusCode, 0, ErrResponseTooLarge
	}

	if resp.StatusCode >= 400 {
		return resp.StatusCode, retryAfterOf(resp), parseAPIError(resp.StatusCode, raw, resp)
	}

	if out != nil {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Data) == 0 {
			return resp.StatusCode, 0, fmt.Errorf("commypanel: the answer is not the JSON the API gives (is %s a CommyPanel?)", c.base)
		}

		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return resp.StatusCode, 0, fmt.Errorf("commypanel: reading the answer: %w", err)
		}
	}

	return resp.StatusCode, 0, nil
}

func retryAfterOf(resp *http.Response) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After")))
	if err != nil || seconds < 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

func parseAPIError(status int, raw []byte, resp *http.Response) error {
	apiErr := &APIError{Status: status, RetryAfterSeconds: int(retryAfterOf(resp) / time.Second)}

	var body struct {
		Error      string              `json:"error"`
		Message    string              `json:"message"`
		Errors     map[string][]string `json:"errors"`
		Deployment *Deployment         `json:"deployment"`
	}

	if json.Unmarshal(raw, &body) == nil && (body.Error != "" || body.Message != "") {
		apiErr.Message = body.Error

		if apiErr.Message == "" {
			apiErr.Message = body.Message
		}

		apiErr.Fields = body.Errors
		apiErr.Deployment = body.Deployment

		return apiErr
	}

	// A proxy or a crash answered with something that is not the API's JSON: say what it was, not what it said.
	apiErr.Message = http.StatusText(status) + " (not an answer from the CommyPanel API)"

	return apiErr
}

// scrub makes sure an error from the transport cannot repeat the token.
func scrub(err error, token string) error {
	if err == nil || token == "" || !strings.Contains(err.Error(), token) {
		return err
	}

	return errors.New(strings.ReplaceAll(err.Error(), token, "[redacted]"))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
