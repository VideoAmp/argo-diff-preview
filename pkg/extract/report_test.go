package extract

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/dag-andersen/argocd-diff-preview/pkg/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyFailurePhase(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want FailurePhase
	}{
		{
			name: "never-started",
			err:  fmt.Errorf("%s: %s", errNeverStartedPrefix, "app [t|set.yaml]"),
			want: PhaseNeverStarted,
		},
		{
			name: "per-app timeout",
			err:  fmt.Errorf("%s '%s': last seen error: boom", errAppTimeoutPrefix, "app [t|set.yaml]"),
			want: PhaseTimeout,
		},
		{
			name: "timeout phrase mid-string stays an error",
			err:  errors.New("helm template .: something timed out waiting for application inside"),
			want: PhaseError,
		},
		{
			name: "deterministic error",
			err:  errors.New("failed to set app instance tracking info: contains non-string value"),
			want: PhaseError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyFailurePhase(tt.err))
		})
	}
}

func TestBuildRenderReport(t *testing.T) {
	failed := []appResult{
		{source: appResource(git.Target, "central_prod.yaml", "zeta-app"), err: errors.New("ArgoCD API error (code 2): Manifest generation error (cached): helm template .")},
		{source: appResource(git.Base, "central_prod.yaml", "alpha-app"), err: fmt.Errorf("%s: %s", errNeverStartedPrefix, "alpha-app [b|central_prod.yaml]")},
		{source: appResource(git.Base, "agent_sandbox.yaml", "mid-app"), err: fmt.Errorf("%s '%s': last seen error: Client.Timeout exceeded while awaiting headers", errAppTimeoutPrefix, "mid-app [b|agent_sandbox.yaml]")},
	}

	report := buildRenderReport(failed, 5400, true, 100, 94, 40, 54)

	require.NotNil(t, report)
	assert.Equal(t, renderReportVersion, report.Version)
	assert.Equal(t, uint64(5400), report.TimeoutSeconds)
	assert.True(t, report.Tolerant)
	assert.Equal(t, 100, report.TotalApps)
	assert.Equal(t, 94, report.Rendered)
	assert.Equal(t, 40, report.RenderedBase)
	assert.Equal(t, 54, report.RenderedTarget)
	require.Len(t, report.Failures, 3)
	// Sorted: base before target; within a branch by appset, then name.
	assert.Equal(t, AppFailure{
		Name:   "mid-app",
		Branch: "base",
		AppSet: "agent_sandbox.yaml",
		Phase:  PhaseTimeout,
		Error:  fmt.Sprintf("%s '%s': last seen error: Client.Timeout exceeded while awaiting headers", errAppTimeoutPrefix, "mid-app [b|agent_sandbox.yaml]"),
	}, report.Failures[0])
	assert.Equal(t, AppFailure{
		Name:   "alpha-app",
		Branch: "base",
		AppSet: "central_prod.yaml",
		Phase:  PhaseNeverStarted,
		Error:  fmt.Sprintf("%s: %s", errNeverStartedPrefix, "alpha-app [b|central_prod.yaml]"),
	}, report.Failures[1])
	assert.Equal(t, AppFailure{
		Name:   "zeta-app",
		Branch: "target",
		AppSet: "central_prod.yaml",
		Phase:  PhaseError,
		Error:  "ArgoCD API error (code 2): Manifest generation error (cached): helm template .",
	}, report.Failures[2])
}

func TestBuildRenderReportNoFailures(t *testing.T) {
	report := buildRenderReport(nil, 2400, false, 10, 10, 5, 5)

	require.NotNil(t, report)
	require.NotNil(t, report.Failures)
	assert.Empty(t, report.Failures)
	assert.False(t, report.Tolerant)
	assert.Equal(t, 10, report.Rendered)
}

func TestRenderReportJSONShape(t *testing.T) {
	failed := []appResult{
		{source: appResource(git.Base, "set.yaml", "app"), err: errors.New("boom")},
	}
	report := buildRenderReport(failed, 5400, true, 2, 1, 1, 0)

	data, err := json.Marshal(report)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"phase":"error"`)
	assert.Contains(t, string(data), `"branch":"base"`)
	assert.Contains(t, string(data), `"version":1`)
	assert.Contains(t, string(data), `"failures":[`)
	assert.Contains(t, string(data), `"timeout_seconds":5400`)
}
