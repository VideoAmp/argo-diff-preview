package extract

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dag-andersen/argocd-diff-preview/pkg/argoapplication"
	"github.com/dag-andersen/argocd-diff-preview/pkg/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func appResource(branch git.BranchType, fileName, name string) argoapplication.ArgoResource {
	return *argoapplication.NewArgoResource(&unstructured.Unstructured{}, argoapplication.Application, name, name, fileName, branch)
}

func extracted(branch git.BranchType, fileName, name string) ExtractedApp {
	return CreateExtractedApp(name, name, fileName, nil, branch)
}

func baseNames(apps []ExtractedApp) []string {
	return names(apps, git.Base)
}

func targetNames(apps []ExtractedApp) []string {
	return names(apps, git.Target)
}

func names(apps []ExtractedApp, want git.BranchType) []string {
	out := []string{}
	for _, app := range apps {
		if app.Branch == want {
			out = append(out, app.Name)
		}
	}
	return out
}

func TestCollectAppResultsStrictModeDiscardsEverythingOnFirstFailure(t *testing.T) {
	results := []appResult{
		{app: extracted(git.Base, "apps.yaml", "ok-app"), source: appResource(git.Base, "apps.yaml", "ok-app")},
		{app: ExtractedApp{}, source: appResource(git.Base, "apps.yaml", "broken-app"), err: fmt.Errorf("failed to get manifests for application broken-app [b|apps.yaml]: open values/missing.yaml: no such file or directory")},
		{app: extracted(git.Target, "apps.yaml", "ok-app"), source: appResource(git.Target, "apps.yaml", "ok-app")},
	}

	base, target, failed, firstErr := collectAppResults(results, false)

	require.Error(t, firstErr)
	assert.Contains(t, firstErr.Error(), "no such file or directory")
	assert.Empty(t, base)
	assert.Empty(t, target)
	assert.Empty(t, failed)
}

func TestCollectAppResultsStrictModeKeepsFirstError(t *testing.T) {
	first := errors.New("first failure")
	second := errors.New("second failure")
	results := []appResult{
		{app: ExtractedApp{}, source: appResource(git.Base, "a.yaml", "app-a"), err: first},
		{app: ExtractedApp{}, source: appResource(git.Target, "a.yaml", "app-a"), err: second},
	}

	_, _, _, err := collectAppResults(results, false)

	require.Same(t, first, err)
}

func TestCollectAppResultsTolerantKeepsUnrelatedApps(t *testing.T) {
	results := []appResult{
		// ok-app renders on both branches
		{app: extracted(git.Base, "central_staging.yaml", "ok-app"), source: appResource(git.Base, "central_staging.yaml", "ok-app")},
		{app: extracted(git.Target, "central_staging.yaml", "ok-app"), source: appResource(git.Target, "central_staging.yaml", "ok-app")},
		// ghost-app fails on BOTH branches (base-side render debt)
		{app: ExtractedApp{}, source: appResource(git.Base, "central_staging.yaml", "ghost-app"), err: fmt.Errorf("failed to get manifests for application ghost-app [b|central_staging.yaml]: open values/strawman/ghost.yaml: no such file or directory")},
		{app: ExtractedApp{}, source: appResource(git.Target, "central_staging.yaml", "ghost-app"), err: fmt.Errorf("failed to get manifests for application ghost-app [t|central_staging.yaml]: open values/strawman/ghost.yaml: no such file or directory")},
	}

	base, target, failed, firstErr := collectAppResults(results, true)

	require.NoError(t, firstErr)
	assert.Equal(t, []string{"ok-app"}, baseNames(base))
	assert.Equal(t, []string{"ok-app"}, targetNames(target))
	require.Len(t, failed, 2)
	assert.Equal(t, "ghost-app", failed[0].source.Name)
	assert.Equal(t, git.Base, failed[0].source.Branch)
	assert.Equal(t, git.Target, failed[1].source.Branch)
}

func TestCollectAppResultsTolerantDropsSuccessfulSideOfFailedApp(t *testing.T) {
	results := []appResult{
		// pr-broken-app renders on base but fails on target (PR-caused)
		{app: extracted(git.Base, "central_staging.yaml", "pr-broken-app"), source: appResource(git.Base, "central_staging.yaml", "pr-broken-app")},
		{app: ExtractedApp{}, source: appResource(git.Target, "central_staging.yaml", "pr-broken-app"), err: fmt.Errorf("failed to get manifests for application pr-broken-app [t|central_staging.yaml]: values violate chart guard")},
		// unrelated app untouched on both sides
		{app: extracted(git.Base, "central_staging.yaml", "fine-app"), source: appResource(git.Base, "central_staging.yaml", "fine-app")},
		{app: extracted(git.Target, "central_staging.yaml", "fine-app"), source: appResource(git.Target, "central_staging.yaml", "fine-app")},
	}

	base, target, failed, firstErr := collectAppResults(results, true)

	require.NoError(t, firstErr)
	// all-or-nothing: the app's successful BASE extraction is dropped too, so
	// the diff cannot fabricate a "PR deleted fine-app's resources" change.
	assert.Equal(t, []string{"fine-app"}, baseNames(base))
	assert.Equal(t, []string{"fine-app"}, targetNames(target))
	require.Len(t, failed, 1)
	assert.Equal(t, "pr-broken-app", failed[0].source.Name)
}

func TestCollectAppResultsTolerantScopesFailureByAppSetFile(t *testing.T) {
	// Same Application name generated by two different ApplicationSets: only
	// the failing appset's instance must be dropped.
	results := []appResult{
		{app: extracted(git.Base, "central_staging.yaml", "shared-name"), source: appResource(git.Base, "central_staging.yaml", "shared-name")},
		{app: extracted(git.Target, "central_staging.yaml", "shared-name"), source: appResource(git.Target, "central_staging.yaml", "shared-name")},
		{app: extracted(git.Base, "infrastructure_monitors.yaml", "shared-name"), source: appResource(git.Base, "infrastructure_monitors.yaml", "shared-name")},
		{app: ExtractedApp{}, source: appResource(git.Target, "infrastructure_monitors.yaml", "shared-name"), err: errors.New("render failed")},
	}

	base, target, failed, firstErr := collectAppResults(results, true)

	require.NoError(t, firstErr)
	assert.Equal(t, []string{"shared-name"}, baseNames(base))
	assert.Equal(t, []string{"shared-name"}, targetNames(target))
	require.Len(t, failed, 1)
	assert.Equal(t, "infrastructure_monitors.yaml", failed[0].source.FileName)
}

func TestCollectAppResultsTolerantNoFailuresKeepsEverything(t *testing.T) {
	results := []appResult{
		{app: extracted(git.Base, "apps.yaml", "app-a"), source: appResource(git.Base, "apps.yaml", "app-a")},
		{app: extracted(git.Target, "apps.yaml", "app-a"), source: appResource(git.Target, "apps.yaml", "app-a")},
		{app: extracted(git.Target, "apps.yaml", "app-b"), source: appResource(git.Target, "apps.yaml", "app-b")},
	}

	base, target, failed, firstErr := collectAppResults(results, true)

	require.NoError(t, firstErr)
	assert.Empty(t, failed)
	assert.Equal(t, []string{"app-a"}, baseNames(base))
	assert.ElementsMatch(t, []string{"app-a", "app-b"}, targetNames(target))
}

func TestCollectAppResultsUnknownBranchTypeErrors(t *testing.T) {
	results := []appResult{
		{app: ExtractedApp{Branch: "neither"}, source: appResource("neither", "apps.yaml", "app-a")},
	}

	for _, tolerate := range []bool{false, true} {
		_, _, _, err := collectAppResults(results, tolerate)
		require.Error(t, err, "tolerate=%t", tolerate)
		assert.Contains(t, err.Error(), "unknown branch type")
	}
}
