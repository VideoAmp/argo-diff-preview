package extract

import (
	"sort"
	"strings"
)

// FailurePhase classifies how a single application failed to render.
type FailurePhase string

const (
	// PhaseNeverStarted: the global render budget expired before the
	// application's worker acquired a concurrency slot.
	PhaseNeverStarted FailurePhase = "never-started"
	// PhaseTimeout: the application started rendering but its per-app
	// wait exceeded the render budget; the error carries the last seen
	// error from the poll loop.
	PhaseTimeout FailurePhase = "timeout"
	// PhaseError: a deterministic application error — fast-abort
	// errorMessages match, manifest generation failure, apply error, etc.
	PhaseError FailurePhase = "error"
)

// Error-message prefixes that identify the failure phases above. They are
// the single source of truth for both the synthesized errors (the fmt.Errorf
// sites in extract.go) and the classification in this file: keep them
// byte-identical to the log lines consumers already parse.
const (
	errNeverStartedPrefix = "timeout reached before starting to render application"
	errAppTimeoutPrefix   = "timed out waiting for application"
)

// AppFailure is one failed application in a RenderReport.
type AppFailure struct {
	Name   string       `json:"name"`
	Branch string       `json:"branch"` // "base" | "target"
	AppSet string       `json:"appset"` // the ApplicationSet file the app was generated from
	Phase  FailurePhase `json:"phase"`
	Error  string       `json:"error"`
}

// RenderReport is the structured outcome of a server-api render: the
// budget/tolerance header plus one record per failed application, so
// consumers (--failures-report) never have to regex-parse the log.
type RenderReport struct {
	Version        int          `json:"version"`
	TimeoutSeconds uint64       `json:"timeout_seconds"`
	Tolerant       bool         `json:"tolerant"`
	TotalApps      int          `json:"total_apps"`
	Rendered       int          `json:"rendered"`        // workers that completed successfully, pre-drop
	RenderedBase   int          `json:"rendered_base"`   // post-drop: failed apps lose their good side too
	RenderedTarget int          `json:"rendered_target"` // post-drop
	Failures       []AppFailure `json:"failures"`
}

// renderReportVersion is the report schema version; bump it on any
// breaking change so consumers can refuse unknown shapes.
const renderReportVersion = 1

// classifyFailurePhase maps an extraction error to its failure phase using
// the canonical error prefixes defined above.
func classifyFailurePhase(err error) FailurePhase {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, errNeverStartedPrefix):
		return PhaseNeverStarted
	case strings.HasPrefix(msg, errAppTimeoutPrefix):
		return PhaseTimeout
	default:
		return PhaseError
	}
}

// buildRenderReport folds the collected failures and render counters into a
// deterministic RenderReport. Failures are sorted (branch, appset, name) so
// two runs over the same inputs produce byte-identical JSON.
func buildRenderReport(
	failed []appResult,
	timeout uint64,
	tolerant bool,
	totalApps, rendered, renderedBase, renderedTarget int,
) *RenderReport {
	failures := make([]AppFailure, 0, len(failed))
	for _, failure := range failed {
		failures = append(failures, AppFailure{
			Name:   failure.source.Name,
			Branch: string(failure.source.Branch),
			AppSet: failure.source.FileName,
			Phase:  classifyFailurePhase(failure.err),
			Error:  failure.err.Error(),
		})
	}
	sort.Slice(failures, func(i, j int) bool {
		a, b := failures[i], failures[j]
		if a.Branch != b.Branch {
			return a.Branch < b.Branch
		}
		if a.AppSet != b.AppSet {
			return a.AppSet < b.AppSet
		}
		return a.Name < b.Name
	})
	return &RenderReport{
		Version:        renderReportVersion,
		TimeoutSeconds: timeout,
		Tolerant:       tolerant,
		TotalApps:      totalApps,
		Rendered:       rendered,
		RenderedBase:   renderedBase,
		RenderedTarget: renderedTarget,
		Failures:       failures,
	}
}
