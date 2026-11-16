package commypanel

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// APIError is an error answer from the panel. Message is the panel's own sentence.
type APIError struct {
	// Status is the HTTP status code.
	Status int
	// Message is the `error` sentence of the answer.
	Message string
	// Fields holds the problems of a 422, by field name.
	Fields map[string][]string
	// Deployment is the deployment that is already running, on a 409 that says so.
	Deployment *Deployment
	// RetryAfter is how long the panel asked to wait, when it did (429).
	RetryAfterSeconds int
}

func (e *APIError) Error() string {
	var b strings.Builder

	fmt.Fprintf(&b, "commypanel: %d %s", e.Status, e.Message)

	if len(e.Fields) > 0 {
		names := make([]string, 0, len(e.Fields))
		for name := range e.Fields {
			names = append(names, name)
		}

		sort.Strings(names)

		for _, name := range names {
			fmt.Fprintf(&b, " (%s: %s)", name, strings.Join(e.Fields[name], " "))
		}
	}

	return b.String()
}

// IsNotFound reports whether err is a 404: no such app, deployment or variable, or an app the token may not use.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsUnauthorized reports whether err is a 401: no token, or an invalid, revoked or expired one.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

// IsForbidden reports whether err is a 403: the token lacks the ability, or its owner lacks the permission.
func IsForbidden(err error) bool { return hasStatus(err, http.StatusForbidden) }

// IsConflict reports whether err is a 409: the state does not allow it, for example a deployment is
// already running (see APIError.Deployment).
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

// IsRateLimited reports whether err is a 429.
func IsRateLimited(err error) bool { return hasStatus(err, http.StatusTooManyRequests) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError

	return errors.As(err, &apiErr) && apiErr.Status == status
}

var (
	// ErrInsecureURL is returned by New for an address that is not https (or loopback http, when allowed).
	ErrInsecureURL = errors.New("commypanel: the panel address must be https")
	// ErrInvalidURL is returned by New for an address that is not a plain scheme://host[:port].
	ErrInvalidURL = errors.New("commypanel: the panel address must look like https://panel.example.com")
	// ErrInvalidToken is returned by New for an empty token or one with spaces or control characters.
	ErrInvalidToken = errors.New("commypanel: the token is empty or has characters a token cannot have")
	// ErrInvalidInput is returned for an app name or variable name that cannot be valid, before any request is made.
	ErrInvalidInput = errors.New("commypanel: invalid app or variable name")
	// ErrResponseTooLarge is returned when an answer is bigger than the client accepts.
	ErrResponseTooLarge = errors.New("commypanel: the answer is too large")
	// ErrRedirect is returned when the panel answers with a redirect. Redirects are never followed, so a
	// token cannot be handed to another address.
	ErrRedirect = errors.New("commypanel: the panel answered with a redirect, which is not followed (check the address, https and www)")
)
