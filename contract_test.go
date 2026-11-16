package commypanel

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

// The API description in testdata/openapi.yaml is a copy of the one in the panel repository
// (scripts/sync-spec.sh). These tests fail when the API gains a call this library does not have, or has
// an ability the library does not know, so the library cannot quietly fall behind.

// methodOf names the Client method that implements each operation of the spec.
var methodOf = map[string]string{
	"getToken":        "Token",
	"listApps":        "ListApps",
	"getApp":          "GetApp",
	"listDeployments": "ListDeployments",
	"startDeployment": "StartDeployment",
	"getDeployment":   "GetDeployment",
	"applyChanges":    "ApplyChanges",
	"startApp":        "StartApp",
	"stopApp":         "StopApp",
	"restartApp":      "RestartApp",
	"getLogs":         "Logs",
	"listEnv":         "ListEnv",
	"setEnv":          "SetEnv",
	"unsetEnv":        "UnsetEnv",
}

func specText(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile("testdata/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func TestEveryOperationOfTheSpecHasAMethod(t *testing.T) {
	ids := regexp.MustCompile(`(?m)^\s+operationId:\s*(\w+)\s*$`).FindAllStringSubmatch(specText(t), -1)
	if len(ids) < 14 {
		t.Fatalf("found only %d operations in the spec", len(ids))
	}

	clientType := reflect.TypeOf(&Client{})
	seen := map[string]bool{}

	for _, match := range ids {
		id := match[1]
		seen[id] = true

		name, ok := methodOf[id]
		if !ok {
			t.Errorf("the spec has the operation %q and this library has no method for it", id)

			continue
		}

		if _, ok := clientType.MethodByName(name); !ok {
			t.Errorf("operation %q should be Client.%s, which does not exist", id, name)
		}
	}

	for id := range methodOf {
		if !seen[id] {
			t.Errorf("the library maps %q, which is no longer in the spec", id)
		}
	}
}

func TestTheSpecsAbilitiesAreTheLibrarys(t *testing.T) {
	known := map[string]bool{string(AbilityRead): true, string(AbilityDeploy): true, string(AbilityControl): true, string(AbilityLogs): true, string(AbilityEnv): true}
	used := map[string]bool{}

	for _, match := range regexp.MustCompile(`(?m)x-ability:\s*['"]?([\w:]+)['"]?`).FindAllStringSubmatch(specText(t), -1) {
		used[match[1]] = true

		if !known[match[1]] {
			t.Errorf("the spec uses the ability %q, which the library does not define", match[1])
		}
	}

	var names []string
	for name := range known {
		if !used[name] {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	if len(names) > 0 {
		t.Errorf("the library defines abilities the spec does not use: %v", names)
	}
}

func TestTheSpecsDeployModesAreTheLibrarys(t *testing.T) {
	text := specText(t)

	for _, mode := range []DeployStrategy{StrategyRolling, StrategyStopFirst} {
		if !regexp.MustCompile(`(?m)^\s+-\s*` + regexp.QuoteMeta(string(mode)) + `\s*$`).MatchString(text) {
			t.Errorf("deploy mode %q is not in the spec", mode)
		}
	}
}
