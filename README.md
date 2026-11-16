# commypanel-go

A small Go client for the HTTP API of a [CommyPanel](https://github.com/commypanel/panel) server: list
apps, start and follow deployments, start, stop and restart an app, read its logs and manage its
environment variables. It is also what the `commy` command-line tool is built on.

- **Standard library only.** No dependencies.
- **Hard to misuse.** https only, no redirects, bounded retries and answers, and the token kept out of
  errors and logs. See [Security](#security).
- **Matches the API.** `testdata/openapi.yaml` is a copy of the panel's API description, and a test fails
  when the API gains a call this library does not have.

```bash
go get github.com/commypanel/commypanel-go
```

## Use

Create a token in the panel under **Settings > API tokens**. Give it only what it needs (for a deploy
job: `apps:deploy` and `apps:read`), limit it to one app, and let it expire.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	commypanel "github.com/commypanel/commypanel-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := commypanel.New("https://panel.example.com", os.Getenv("COMMY_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	deployment, err := client.StartDeployment(ctx, "shop")
	if err != nil {
		log.Fatal(err)
	}

	deployment, err = client.WaitForDeployment(ctx, "shop", deployment.Number, 3*time.Second, func(d *commypanel.Deployment) {
		fmt.Println(d.Status)
	})
	if err != nil {
		log.Fatal(err)
	}

	if !deployment.Succeeded {
		log.Fatalf("the deployment ended as %s", deployment.Status)
	}
}
```

Errors from the panel are `*commypanel.APIError`; test them with `commypanel.IsNotFound(err)`,
`IsForbidden`, `IsConflict` (a deployment is already running: `APIError.Deployment` is that one),
`IsUnauthorized` and `IsRateLimited`, or with `errors.As`.

| Method | API call | Ability |
|---|---|---|
| `Token` | `GET /token` | none |
| `ListApps`, `GetApp` | `GET /apps`, `GET /apps/{slug}` | `apps:read` |
| `ListDeployments`, `GetDeployment` | `GET /apps/{slug}/deployments[/{n}]` | `apps:read` |
| `StartDeployment`, `ApplyChanges` | `POST /apps/{slug}/deployments`, `/apply` | `apps:deploy` |
| `StartApp`, `StopApp`, `RestartApp` | `POST /apps/{slug}/start`, `/stop`, `/restart` | `apps:control` |
| `Logs` | `GET /apps/{slug}/logs` | `apps:logs` |
| `ListEnv`, `SetEnv`, `UnsetEnv` | `GET`, `PUT`, `DELETE /apps/{slug}/env[/{key}]` | `apps:env` |
| `WaitForDeployment` | polls `GET .../deployments/{n}` | `apps:read` |

Environment variable **values are write-only**: the API never returns them, so `ListEnv` gives names.

## Security

- **https only.** `New` refuses any other address. `WithAllowHTTPLoopback()` allows plain http for
  `localhost`, `127.0.0.1` and `::1` only, for development.
- **TLS is always verified.** There is no option to skip it. For a panel with a private CA use
  `WithCACertPEM`.
- **Redirects are never followed**, so the token cannot be handed to another address; a redirect is an
  error (`ErrRedirect`).
- **The token stays out of sight.** It is only ever in the `Authorization` header: not in URLs, errors,
  `String()` or `%v`/`%#v` output.
- **Bounded.** A timeout (30 s), at most 2 retries (never for a write that may have happened: only a
  429, and gateway errors for reads), `Retry-After` capped at 30 s, answers capped at 16 MiB.
- **Names are checked before any request**, so an app or variable name cannot change the path.

Report a security problem privately, as described in [SECURITY.md](SECURITY.md).

## Compatibility

This library speaks API `v1` (`/api/v1`) of CommyPanel 0.10 and later. It uses the Go standard library
of Go 1.24 or newer.

## Development

```bash
scripts/sync-spec.sh ../../commypanel   # copy the API description from the panel repository
go vet ./... && go test -race ./...
```

MIT licensed.
