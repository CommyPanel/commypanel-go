package commypanel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewChecksTheAddress(t *testing.T) {
	good := []struct {
		url  string
		opts []Option
	}{
		{"https://panel.example.com", nil},
		{"https://panel.example.com/", nil},
		{"https://panel.example.com:8443", nil},
		{"https://panel.example.com/api/v1", nil},
		{"http://localhost:8000", []Option{WithAllowHTTPLoopback()}},
		{"http://127.0.0.1:8000", []Option{WithAllowHTTPLoopback()}},
		{"http://[::1]:8000", []Option{WithAllowHTTPLoopback()}},
	}

	for _, tc := range good {
		if _, err := New(tc.url, testToken, tc.opts...); err != nil {
			t.Errorf("%s should be accepted: %v", tc.url, err)
		}
	}

	insecure := []struct {
		url  string
		opts []Option
	}{
		{"http://panel.example.com", nil},
		{"http://localhost:8000", nil},                                  // loopback http needs the option
		{"http://panel.example.com", []Option{WithAllowHTTPLoopback()}}, // and only loopback
		{"http://192.168.1.10", []Option{WithAllowHTTPLoopback()}},
		{"ftp://panel.example.com", nil},
	}

	for _, tc := range insecure {
		if _, err := New(tc.url, testToken, tc.opts...); !errors.Is(err, ErrInsecureURL) {
			t.Errorf("%s should be refused as insecure, got %v", tc.url, err)
		}
	}

	invalid := []string{"", "panel.example.com", "https://", "https://user:pw@panel.example.com", "https://panel.example.com/admin", "https://panel.example.com?x=1", "https://panel.example.com#top", "https://panel.example.com/api/v2"}

	for _, raw := range invalid {
		if _, err := New(raw, testToken); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q should be an invalid address, got %v", raw, err)
		}
	}
}

func TestNewChecksTheToken(t *testing.T) {
	for _, token := range []string{"", " ", "has space", "new\nline", "tab\t", "ctl\x01", strings.Repeat("a", 513)} {
		if _, err := New("https://panel.example.com", token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("token %q should be refused, got %v", token, err)
		}
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	for _, opt := range []Option{WithTimeout(0), WithRetries(-1), WithRetries(6), WithMaxResponseBytes(0), WithCACertPEM([]byte("not pem"))} {
		if _, err := New("https://panel.example.com", testToken, opt); err == nil {
			t.Error("a bad option should be refused")
		}
	}
}

func TestEveryRequestCarriesTheTokenOnlyInTheHeader(t *testing.T) {
	panel := newPanel(t, jsonReply(200, `{"data":{"name":"ci","abilities":["apps:read"],"apps":null,"expires_at":null,"last_used_at":null,"owner":"Sam"}}`))
	c := panel.client(t, WithUserAgent("mytool/1.0"))

	token, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	req := panel.last(t)
	if req.Header.Get("Authorization") != "Bearer "+testToken {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}

	if req.Header.Get("Accept") != "application/json" || !strings.HasPrefix(req.Header.Get("User-Agent"), "mytool/1.0 commypanel-go/") {
		t.Errorf("headers = %v", req.Header)
	}

	if strings.Contains(req.Path+req.Query+req.Body, "secretsecret") {
		t.Error("the token must not be in the path, query or body")
	}

	if token.Name != "ci" || token.Owner != "Sam" || token.Apps != nil || len(token.Abilities) != 1 || token.Abilities[0] != AbilityRead {
		t.Errorf("token = %+v", token)
	}
}

func TestCallsUseTheRightMethodPathAndBody(t *testing.T) {
	deployment := `{"data":{"number":4,"status":"queued","stage":null,"finished":false,"succeeded":false,"commit":null,"commit_message":null,"strategy":"rolling","is_rollback":false,"error":null,"created_at":null,"finished_at":null,"url":"https://x/api/v1/apps/shop/deployments/4"}}`
	app := `{"data":{"slug":"shop","name":"Shop","status":"running","stack":"go","deploy_strategy":"rolling","domains":["shop.example.com"],"live_deployment":3,"latest_deployment":null,"created_at":"2026-11-16T10:00:00+00:00"}}`

	tests := []struct {
		name   string
		reply  string
		status int
		call   func(c *Client) error
		method string
		path   string
		query  string
		body   string
	}{
		{"list apps", `{"data":[]}`, 200, func(c *Client) error { _, err := c.ListApps(context.Background()); return err }, "GET", "/api/v1/apps", "", ""},
		{"get app", app, 200, func(c *Client) error { _, err := c.GetApp(context.Background(), "shop"); return err }, "GET", "/api/v1/apps/shop", "", ""},
		{"list deployments", `{"data":[]}`, 200, func(c *Client) error { _, err := c.ListDeployments(context.Background(), "shop", 5); return err }, "GET", "/api/v1/apps/shop/deployments", "limit=5", ""},
		{"get deployment with log", deployment, 200, func(c *Client) error { _, err := c.GetDeployment(context.Background(), "shop", 4, true); return err }, "GET", "/api/v1/apps/shop/deployments/4", "log=1", ""},
		{"start deployment", deployment, 202, func(c *Client) error { _, err := c.StartDeployment(context.Background(), "shop"); return err }, "POST", "/api/v1/apps/shop/deployments", "", ""},
		{"apply", deployment, 202, func(c *Client) error { _, err := c.ApplyChanges(context.Background(), "shop"); return err }, "POST", "/api/v1/apps/shop/apply", "", ""},
		{"start app", app, 200, func(c *Client) error { _, err := c.StartApp(context.Background(), "shop"); return err }, "POST", "/api/v1/apps/shop/start", "", ""},
		{"stop app", app, 200, func(c *Client) error { _, err := c.StopApp(context.Background(), "shop"); return err }, "POST", "/api/v1/apps/shop/stop", "", ""},
		{"restart app", app, 200, func(c *Client) error { _, err := c.RestartApp(context.Background(), "shop"); return err }, "POST", "/api/v1/apps/shop/restart", "", ""},
		{"logs", `{"data":{"live":true,"lines":50,"logs":"hi"}}`, 200, func(c *Client) error { _, err := c.Logs(context.Background(), "shop", 50); return err }, "GET", "/api/v1/apps/shop/logs", "tail=50", ""},
		{"list env", `{"data":{"keys":["A"],"pending_changes":false}}`, 200, func(c *Client) error { _, err := c.ListEnv(context.Background(), "shop"); return err }, "GET", "/api/v1/apps/shop/env", "", ""},
		{"set env", `{"data":{"key":"A_B","pending_changes":true}}`, 201, func(c *Client) error {
			_, _, err := c.SetEnv(context.Background(), "shop", "A_B", `va"lue`)
			return err
		}, "PUT", "/api/v1/apps/shop/env/A_B", "", `{"value":"va\"lue"}`},
		{"unset env", `{"data":{"key":"A_B","pending_changes":true}}`, 200, func(c *Client) error { _, err := c.UnsetEnv(context.Background(), "shop", "A_B"); return err }, "DELETE", "/api/v1/apps/shop/env/A_B", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			panel := newPanel(t, jsonReply(tc.status, tc.reply))

			if err := tc.call(panel.client(t)); err != nil {
				t.Fatal(err)
			}

			req := panel.last(t)
			if req.Method != tc.method || req.Path != tc.path || req.Query != tc.query || req.Body != tc.body {
				t.Errorf("got %s %s ?%s body %q; want %s %s ?%s body %q", req.Method, req.Path, req.Query, req.Body, tc.method, tc.path, tc.query, tc.body)
			}
		})
	}
}

func TestDecodesTheObjects(t *testing.T) {
	panel := newPanel(t, jsonReply(200, `{"data":{"slug":"shop","name":"Shop","status":"running","stack":"go","deploy_strategy":"stop_first","domains":["a.example.com"],"live_deployment":3,"latest_deployment":{"number":3,"status":"active","stage":"done","finished":true,"succeeded":true,"commit":"abc123","commit_message":"Fix","strategy":"stop_first","is_rollback":false,"error":null,"created_at":"2026-11-16T10:00:00+00:00","finished_at":"2026-11-16T10:01:00+00:00","url":"https://x"},"created_at":"2026-11-16T09:00:00+00:00"}}`))

	app, err := panel.client(t).GetApp(context.Background(), "shop")
	if err != nil {
		t.Fatal(err)
	}

	if app.DeployStrategy != StrategyStopFirst || *app.Stack != "go" || *app.LiveDeployment != 3 || len(app.Domains) != 1 {
		t.Errorf("app = %+v", app)
	}

	d := app.LatestDeployment
	if d == nil || !d.Finished || !d.Succeeded || *d.Commit != "abc123" || d.Strategy != StrategyStopFirst || d.FinishedAt == nil || d.FinishedAt.Sub(*d.CreatedAt) != time.Minute {
		t.Errorf("deployment = %+v", d)
	}
}

func TestSetEnvTellsNewFromReplaced(t *testing.T) {
	for status, wantCreated := range map[int]bool{201: true, 200: false} {
		panel := newPanel(t, jsonReply(status, `{"data":{"key":"A","pending_changes":true}}`))

		change, created, err := panel.client(t).SetEnv(context.Background(), "shop", "A", "v")
		if err != nil || created != wantCreated || change.Key != "A" || !change.PendingChanges {
			t.Errorf("status %d: change %+v created %v err %v", status, change, created, err)
		}
	}
}

func TestErrorsCarryThePanelsSentence(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"not found", 404, `{"error":"Not Found","message":"Not Found"}`, IsNotFound},
		{"unauthorized", 401, `{"error":"Unauthenticated."}`, IsUnauthorized},
		{"forbidden", 403, `{"error":"Invalid ability provided."}`, IsForbidden},
		{"conflict", 409, `{"error":"A deployment is already in progress.","deployment":{"number":7,"status":"building","finished":false}}`, IsConflict},
		{"rate limited", 429, `{"error":"Too Many Attempts."}`, IsRateLimited},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel := newPanel(t, jsonReply(tc.status, tc.body))

			_, err := panel.client(t, WithRetries(0)).GetApp(context.Background(), "shop")
			if err == nil || !tc.check(err) {
				t.Fatalf("err = %v", err)
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tc.status || apiErr.Message == "" {
				t.Errorf("apiErr = %+v", apiErr)
			}
		})
	}
}

func TestAConflictNamesTheRunningDeploymentAndAValidationErrorTheFields(t *testing.T) {
	panel := newPanel(t, jsonReply(409, `{"error":"A deployment is already in progress.","deployment":{"number":7,"status":"building","finished":false}}`))

	_, err := panel.client(t).StartDeployment(context.Background(), "shop")

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Deployment == nil || apiErr.Deployment.Number != 7 {
		t.Fatalf("err = %v", err)
	}

	panel = newPanel(t, jsonReply(422, `{"error":"PORT is set by CommyPanel.","errors":{"key":["PORT is set by CommyPanel."]}}`))

	_, _, err = panel.client(t).SetEnv(context.Background(), "shop", "PORT", "1")
	if !errors.As(err, &apiErr) || len(apiErr.Fields["key"]) != 1 || !strings.Contains(err.Error(), "key: PORT is set by CommyPanel.") {
		t.Errorf("err = %v / %+v", err, apiErr)
	}
}

func TestAnAnswerThatIsNotTheAPIsIsSaidPlainly(t *testing.T) {
	for _, status := range []int{200, 502} {
		panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, _ int) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("<html>nginx says hi</html>"))
		})

		_, err := panel.client(t, WithRetries(0)).ListApps(context.Background())
		if err == nil || strings.Contains(err.Error(), "nginx says hi") {
			t.Errorf("status %d: err = %v", status, err)
		}
	}
}

func TestRedirectsAreNeverFollowedSoTheTokenStaysHome(t *testing.T) {
	elsewhere := newPanel(t, jsonReply(200, `{"data":[]}`))
	panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		http.Redirect(w, r, elsewhere.URL+"/api/v1/apps", http.StatusFound)
	})

	_, err := panel.client(t).ListApps(context.Background())
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("err = %v", err)
	}

	if n := len(elsewhere.seen()); n != 0 {
		t.Errorf("the other address received %d request(s); the token must never be sent there", n)
	}
}

func TestRetries(t *testing.T) {
	t.Run("a read is retried after a gateway error", func(t *testing.T) {
		panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n < 3 {
				jsonReply(503, `{"error":"down"}`)(w, r, n)

				return
			}

			jsonReply(200, `{"data":[]}`)(w, r, n)
		})

		if _, err := panel.client(t).ListApps(context.Background()); err != nil {
			t.Fatal(err)
		}

		if n := len(panel.seen()); n != 3 {
			t.Errorf("attempts = %d, want 3", n)
		}
	})

	t.Run("a write is not retried after a gateway error, it may have happened", func(t *testing.T) {
		panel := newPanel(t, jsonReply(503, `{"error":"down"}`))

		if _, err := panel.client(t).StartDeployment(context.Background(), "shop"); err == nil {
			t.Fatal("expected an error")
		}

		if n := len(panel.seen()); n != 1 {
			t.Errorf("attempts = %d, want 1", n)
		}
	})

	t.Run("a refusal for too many calls is retried for writes too, after the wait it asks for", func(t *testing.T) {
		var waits []time.Duration

		panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Retry-After", "7")
				jsonReply(429, `{"error":"slow down"}`)(w, r, n)

				return
			}

			jsonReply(202, `{"data":{"number":1,"status":"queued","finished":false,"succeeded":false,"strategy":"rolling","is_rollback":false,"url":"u"}}`)(w, r, n)
		})
		c := panel.client(t)
		c.sleep = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return nil }

		if _, err := c.StartDeployment(context.Background(), "shop"); err != nil {
			t.Fatal(err)
		}

		if len(waits) != 1 || waits[0] != 7*time.Second {
			t.Errorf("waits = %v", waits)
		}
	})

	t.Run("an enormous Retry-After is capped", func(t *testing.T) {
		var waits []time.Duration

		panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, n int) {
			w.Header().Set("Retry-After", "86400")
			jsonReply(429, `{"error":"slow down"}`)(w, r, n)
		})
		c := panel.client(t, WithRetries(1))
		c.sleep = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return nil }

		_, err := c.ListApps(context.Background())
		if !IsRateLimited(err) || len(waits) != 1 || waits[0] != maxRetryWait {
			t.Errorf("err %v waits %v", err, waits)
		}
	})

	t.Run("retries stop at the limit and a cancelled context stops the waiting", func(t *testing.T) {
		panel := newPanel(t, jsonReply(503, `{"error":"down"}`))

		if _, err := panel.client(t, WithRetries(1)).ListApps(context.Background()); err == nil || len(panel.seen()) != 2 {
			t.Errorf("err %v attempts %d", err, len(panel.seen()))
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := panel.client(t).ListApps(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestAnAnswerThatIsTooLargeIsRefused(t *testing.T) {
	panel := newPanel(t, jsonReply(200, `{"data":{"live":true,"lines":10,"logs":"`+strings.Repeat("x", 5000)+`"}}`))

	_, err := panel.client(t, WithMaxResponseBytes(1000)).Logs(context.Background(), "shop", 10)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("err = %v", err)
	}
}

func TestTheTokenNeverShowsUp(t *testing.T) {
	panel := newPanel(t, jsonReply(200, `{"data":[]}`))
	c := panel.client(t)

	for _, shown := range []string{fmt.Sprint(c), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%s", c)} {
		if strings.Contains(shown, "secretsecret") {
			t.Errorf("the token is in %q", shown)
		}
	}

	// A failed connection must not repeat it either.
	panel.Close()

	_, err := c.ListApps(context.Background())
	if err == nil || strings.Contains(err.Error(), "secretsecret") {
		t.Errorf("err = %v", err)
	}

	if got := scrub(errors.New("failed with "+testToken), testToken); strings.Contains(got.Error(), "secretsecret") {
		t.Errorf("scrub left the token in %q", got)
	}
}

func TestNamesThatCannotBeRightNeverBecomeRequests(t *testing.T) {
	panel := newPanel(t, jsonReply(200, `{"data":{}}`))
	c := panel.client(t)

	for _, slug := range []string{"", "../etc", "a/b", "A", "a b", "-x", "a?x=1", "a#b", "a%2Fb", strings.Repeat("a", 64)} {
		if _, err := c.GetApp(context.Background(), slug); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("slug %q: err = %v", slug, err)
		}
	}

	for _, key := range []string{"", "1BAD", "A-B", "A B", "A/B", "A?", strings.Repeat("A", 129)} {
		if _, _, err := c.SetEnv(context.Background(), "shop", key, "v"); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("key %q: err = %v", key, err)
		}

		if _, err := c.UnsetEnv(context.Background(), "shop", key); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("unset key %q: err = %v", key, err)
		}
	}

	if _, err := c.GetDeployment(context.Background(), "shop", 0, false); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("deployment 0: err = %v", err)
	}

	if n := len(panel.seen()); n != 0 {
		t.Errorf("%d request(s) were made for invalid names", n)
	}
}

func TestWaitForDeploymentPollsUntilItIsOver(t *testing.T) {
	panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, n int) {
		finished := n >= 3
		status := "building"

		if finished {
			status = "active"
		}

		jsonReply(200, fmt.Sprintf(`{"data":{"number":4,"status":%q,"finished":%v,"succeeded":%v,"strategy":"rolling","is_rollback":false,"url":"u"}}`, status, finished, finished))(w, r, n)
	})
	c := panel.client(t)

	var looks int

	deployment, err := c.WaitForDeployment(context.Background(), "shop", 4, time.Second, func(*Deployment) { looks++ })
	if err != nil || !deployment.Finished || !deployment.Succeeded || looks != 3 {
		t.Errorf("deployment %+v err %v looks %d", deployment, err, looks)
	}

	// The context stops the waiting.
	panel2 := newPanel(t, jsonReply(200, `{"data":{"number":4,"status":"building","finished":false,"succeeded":false,"strategy":"rolling","is_rollback":false,"url":"u"}}`))
	ctx, cancel := context.WithCancel(context.Background())
	c2 := panel2.client(t)
	c2.sleep = func(ctx context.Context, d time.Duration) error { cancel(); return ctx.Err() }

	if _, err := c2.WaitForDeployment(ctx, "shop", 4, time.Second, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestACustomHTTPClientStillNeverFollowsRedirects(t *testing.T) {
	elsewhere := newPanel(t, jsonReply(200, `{"data":[]}`))
	panel := newPanel(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	})

	hc := panel.Client()
	hc.CheckRedirect = nil // the default would follow

	c, err := New(panel.URL, testToken, WithHTTPClient(hc))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.ListApps(context.Background()); !errors.Is(err, ErrRedirect) || len(elsewhere.seen()) != 0 {
		t.Errorf("err %v, other address saw %d", err, len(elsewhere.seen()))
	}
}

func TestAServerThatEchoesTheTokenInItsErrorDoesNotGetItIntoOurs(t *testing.T) {
	echo := "Invalid header: Authorization: Bearer " + testToken
	panel := newPanel(t, jsonReply(422, `{"error":"`+echo+`","errors":{"value":["`+echo+`"]},"deployment":{"number":1,"status":"building","error":"`+echo+`","commit_message":"`+echo+`"}}`))

	_, _, err := panel.client(t, WithRetries(0)).SetEnv(context.Background(), "shop", "KEY", "v")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}

	shown := fmt.Sprintf("%v %+v %#v %s %v", err, apiErr, apiErr.Fields, apiErr.Message, apiErr.Fields)
	if apiErr.Deployment != nil {
		shown += *apiErr.Deployment.Error + *apiErr.Deployment.CommitMessage
	}

	if strings.Contains(shown, "secretsecret") {
		t.Errorf("the token is in what the panel said, and so in our error: %s", shown)
	}

	if !strings.Contains(apiErr.Message, "[redacted]") {
		t.Errorf("message = %q", apiErr.Message)
	}
}
