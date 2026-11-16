package commypanel

import "time"

// Ability is what a token may do.
type Ability string

// The abilities a token can carry.
const (
	AbilityRead    Ability = "apps:read"
	AbilityDeploy  Ability = "apps:deploy"
	AbilityControl Ability = "apps:control"
	AbilityLogs    Ability = "apps:logs"
	AbilityEnv     Ability = "apps:env"
)

// Token describes the token the client is using.
type Token struct {
	Name      string    `json:"name"`
	Abilities []Ability `json:"abilities"`
	// Apps lists the apps the token is limited to; nil means every app its owner may use.
	Apps       []string   `json:"apps"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	// Owner is the owner's name.
	Owner string `json:"owner"`
}

// DeployStrategy is how a new release replaces the running one.
type DeployStrategy string

// The deploy modes.
const (
	// StrategyRolling starts the new version beside the old one (no downtime).
	StrategyRolling DeployStrategy = "rolling"
	// StrategyStopFirst stops the old version first, for software that cannot run twice on the same data.
	StrategyStopFirst DeployStrategy = "stop_first"
)

// App is one app, as the API describes it. It never holds secrets.
type App struct {
	Slug             string         `json:"slug"`
	Name             string         `json:"name"`
	Status           string         `json:"status"`
	Stack            *string        `json:"stack"`
	DeployStrategy   DeployStrategy `json:"deploy_strategy"`
	Domains          []string       `json:"domains"`
	LiveDeployment   *int           `json:"live_deployment"`
	LatestDeployment *Deployment    `json:"latest_deployment"`
	CreatedAt        time.Time      `json:"created_at"`
}

// Deployment is one deployment, rollback or "apply changes".
type Deployment struct {
	Number int     `json:"number"`
	Status string  `json:"status"`
	Stage  *string `json:"stage"`
	// Finished is true once it is over, whatever the result. Poll until it is.
	Finished bool `json:"finished"`
	// Succeeded is only meaningful once Finished. A watched release can still be rolled back later
	// (status "rolled_back").
	Succeeded     bool           `json:"succeeded"`
	Commit        *string        `json:"commit"`
	CommitMessage *string        `json:"commit_message"`
	Strategy      DeployStrategy `json:"strategy"`
	IsRollback    bool           `json:"is_rollback"`
	Error         *string        `json:"error"`
	CreatedAt     *time.Time     `json:"created_at"`
	FinishedAt    *time.Time     `json:"finished_at"`
	URL           string         `json:"url"`
	// Log is the build log, only when asked for.
	Log string `json:"log,omitempty"`
}

// Logs is the output of an app's running container.
type Logs struct {
	// Live is false when the app has no running container.
	Live  bool   `json:"live"`
	Lines int    `json:"lines"`
	Logs  string `json:"logs"`
}

// EnvKeys lists the names of an app's environment variables. Values are never returned by the API.
type EnvKeys struct {
	Keys []string `json:"keys"`
	// PendingChanges is true when variables, volumes or links changed after the running container was
	// made: call ApplyChanges.
	PendingChanges bool `json:"pending_changes"`
}

// EnvChange is the answer to setting or removing a variable. The value is not part of it.
type EnvChange struct {
	Key            string `json:"key"`
	PendingChanges bool   `json:"pending_changes"`
}
