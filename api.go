package commypanel

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func appPath(slug string, rest ...string) (string, error) {
	if err := checkSlug(slug); err != nil {
		return "", err
	}

	path := "/apps/" + slug

	for _, part := range rest {
		path += "/" + part
	}

	return path, nil
}

// Token describes the token this client uses: its name, abilities, the apps it is limited to and when it
// expires. Any valid token may ask. It is a good way to check a token when it is saved.
func (c *Client) Token(ctx context.Context) (*Token, error) {
	var token Token

	if _, err := c.do(ctx, http.MethodGet, "/token", nil, nil, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// ListApps lists the apps the token may see (ability apps:read).
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App

	if _, err := c.do(ctx, http.MethodGet, "/apps", nil, nil, &apps); err != nil {
		return nil, err
	}

	return apps, nil
}

// GetApp returns one app (ability apps:read).
func (c *Client) GetApp(ctx context.Context, slug string) (*App, error) {
	path, err := appPath(slug)
	if err != nil {
		return nil, err
	}

	var app App

	if _, err := c.do(ctx, http.MethodGet, path, nil, nil, &app); err != nil {
		return nil, err
	}

	return &app, nil
}

// ListDeployments returns the newest deployments of an app, newest first (ability apps:read). A limit
// of 0 uses the panel's default (20); the panel allows at most 50.
func (c *Client) ListDeployments(ctx context.Context, slug string, limit int) ([]Deployment, error) {
	path, err := appPath(slug, "deployments")
	if err != nil {
		return nil, err
	}

	query := url.Values{}

	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	var deployments []Deployment

	if _, err := c.do(ctx, http.MethodGet, path, query, nil, &deployments); err != nil {
		return nil, err
	}

	return deployments, nil
}

// GetDeployment returns one deployment (ability apps:read), with the build log when withLog is true.
func (c *Client) GetDeployment(ctx context.Context, slug string, number int, withLog bool) (*Deployment, error) {
	if number < 1 {
		return nil, ErrInvalidInput
	}

	path, err := appPath(slug, "deployments", strconv.Itoa(number))
	if err != nil {
		return nil, err
	}

	query := url.Values{}

	if withLog {
		query.Set("log", "1")
	}

	var deployment Deployment

	if _, err := c.do(ctx, http.MethodGet, path, query, nil, &deployment); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// StartDeployment queues a deployment (ability apps:deploy) and returns it at once; follow it with
// WaitForDeployment. If one is already running the error is a conflict (IsConflict) whose
// APIError.Deployment is the running one.
func (c *Client) StartDeployment(ctx context.Context, slug string) (*Deployment, error) {
	return c.queue(ctx, slug, "deployments")
}

// ApplyChanges makes changed variables, volumes and links take effect from the version that is already
// running, without a build (ability apps:deploy). It returns the queued deployment.
func (c *Client) ApplyChanges(ctx context.Context, slug string) (*Deployment, error) {
	return c.queue(ctx, slug, "apply")
}

func (c *Client) queue(ctx context.Context, slug, what string) (*Deployment, error) {
	path, err := appPath(slug, what)
	if err != nil {
		return nil, err
	}

	var deployment Deployment

	if _, err := c.do(ctx, http.MethodPost, path, nil, nil, &deployment); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// StartApp starts a stopped app (ability apps:control).
func (c *Client) StartApp(ctx context.Context, slug string) (*App, error) {
	return c.control(ctx, slug, "start")
}

// StopApp stops a running app; visitors see an error until it is started again (ability apps:control).
func (c *Client) StopApp(ctx context.Context, slug string) (*App, error) {
	return c.control(ctx, slug, "stop")
}

// RestartApp restarts a running app (ability apps:control).
func (c *Client) RestartApp(ctx context.Context, slug string) (*App, error) {
	return c.control(ctx, slug, "restart")
}

func (c *Client) control(ctx context.Context, slug, action string) (*App, error) {
	path, err := appPath(slug, action)
	if err != nil {
		return nil, err
	}

	var app App

	if _, err := c.do(ctx, http.MethodPost, path, nil, nil, &app); err != nil {
		return nil, err
	}

	return &app, nil
}

// Logs reads the last lines of the running container's output (ability apps:logs). tail is clamped by
// the panel to 10..5000; 0 uses the default (200). Logs often hold secrets, and every read is recorded
// in the panel's audit log.
func (c *Client) Logs(ctx context.Context, slug string, tail int) (*Logs, error) {
	path, err := appPath(slug, "logs")
	if err != nil {
		return nil, err
	}

	query := url.Values{}

	if tail > 0 {
		query.Set("tail", strconv.Itoa(tail))
	}

	var logs Logs

	if _, err := c.do(ctx, http.MethodGet, path, query, nil, &logs); err != nil {
		return nil, err
	}

	return &logs, nil
}

// ListEnv lists the names of an app's environment variables (ability apps:env). The API never returns values.
func (c *Client) ListEnv(ctx context.Context, slug string) (*EnvKeys, error) {
	path, err := appPath(slug, "env")
	if err != nil {
		return nil, err
	}

	var keys EnvKeys

	if _, err := c.do(ctx, http.MethodGet, path, nil, nil, &keys); err != nil {
		return nil, err
	}

	return &keys, nil
}

// SetEnv sets an environment variable (ability apps:env). created is true when the variable is new. The
// change takes effect with ApplyChanges or the next deployment. The value is sent, never read back.
func (c *Client) SetEnv(ctx context.Context, slug, key, value string) (change *EnvChange, created bool, err error) {
	if err := checkEnvKey(key); err != nil {
		return nil, false, err
	}

	path, err := appPath(slug, "env", key)
	if err != nil {
		return nil, false, err
	}

	var result EnvChange

	status, err := c.do(ctx, http.MethodPut, path, nil, map[string]string{"value": value}, &result)
	if err != nil {
		return nil, false, err
	}

	return &result, status == http.StatusCreated, nil
}

// UnsetEnv removes an environment variable (ability apps:env). A variable that is not there is a 404 (IsNotFound).
func (c *Client) UnsetEnv(ctx context.Context, slug, key string) (*EnvChange, error) {
	if err := checkEnvKey(key); err != nil {
		return nil, err
	}

	path, err := appPath(slug, "env", key)
	if err != nil {
		return nil, err
	}

	var result EnvChange

	if _, err := c.do(ctx, http.MethodDelete, path, nil, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// WaitForDeployment polls a deployment until it is finished and returns the final state. poll is how
// often to look (default 3 seconds, at least 1); onUpdate, when not nil, is called with every look.
// Cancel the context to stop waiting; the deployment itself carries on in the panel. Check Succeeded
// (and Status) on the result: a deployment can finish as failed or rolled back.
func (c *Client) WaitForDeployment(ctx context.Context, slug string, number int, poll time.Duration, onUpdate func(*Deployment)) (*Deployment, error) {
	if poll < time.Second {
		poll = 3 * time.Second
	}

	for {
		deployment, err := c.GetDeployment(ctx, slug, number, false)
		if err != nil {
			return nil, err
		}

		if onUpdate != nil {
			onUpdate(deployment)
		}

		if deployment.Finished {
			return deployment, nil
		}

		if err := c.sleep(ctx, poll); err != nil {
			return nil, err
		}
	}
}
