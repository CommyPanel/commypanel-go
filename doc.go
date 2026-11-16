// Package commypanel is a small client for the HTTP API of a CommyPanel server: list apps, start and
// follow deployments, start, stop and restart an app, read its logs and manage its environment
// variables.
//
// It uses only the Go standard library and is built to be hard to misuse:
//
//   - the panel address must be https (plain http only for localhost, and only when asked for);
//   - redirects are never followed, so the token cannot be sent to another host;
//   - the token is kept out of errors, logs and formatting;
//   - calls have a timeout, bounded retries and a limit on the size of an answer;
//   - environment variable values can only be written, because the API never returns them.
//
// Create a token in the panel under Settings > API tokens, give it only the abilities it needs, limit
// it to one app and let it expire.
//
//	client, err := commypanel.New("https://panel.example.com", os.Getenv("COMMY_TOKEN"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	deployment, err := client.StartDeployment(ctx, "shop")
package commypanel
