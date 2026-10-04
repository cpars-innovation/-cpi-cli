// Package deployer is the single implementation of "deploy designtime
// artifacts to runtime and wait for the outcome" (and its inverse, undeploy).
// It is used by the deploy, undeploy, configure and orchestrator commands.
package deployer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cpars-innovation/-cpi-cli/internal/api"
	"github.com/cpars-innovation/-cpi-cli/internal/httpclnt"
	"github.com/rs/zerolog/log"
)

// Status is the final outcome for one artifact.
type Status string

const (
	StatusDeployed    Status = "DEPLOYED"     // deployment confirmed on runtime
	StatusSkipped     Status = "SKIPPED"      // same version already STARTED (compare-versions)
	StatusUndeployed  Status = "UNDEPLOYED"   // runtime artifact removed
	StatusNotDeployed Status = "NOT_DEPLOYED" // undeploy: nothing was deployed
	StatusFailed      Status = "FAILED"
	StatusTimeout     Status = "TIMEOUT"
)

// Succeeded reports whether s is a successful outcome.
func (s Status) Succeeded() bool {
	switch s {
	case StatusDeployed, StatusSkipped, StatusUndeployed, StatusNotDeployed:
		return true
	}
	return false
}

// Artifact identifies a designtime artifact to deploy or a runtime artifact
// to undeploy.
type Artifact struct {
	ID        string
	Type      string // Integration, MessageMapping, ScriptCollection, ValueMapping
	PackageID string // informational only
}

// Result is the structured outcome for one artifact.
type Result struct {
	ID        string `json:"id"`
	Type      string `json:"type,omitempty"`
	PackageID string `json:"packageId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	Status    Status `json:"status"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`

	// Err is the underlying error for FAILED/TIMEOUT results (not serialised).
	Err error `json:"-"`
}

// Options control polling and concurrency.
type Options struct {
	// Interval between two status checks.
	Interval time.Duration
	// MaxChecks is the maximum number of status checks per artifact.
	MaxChecks int
	// CompareVersions skips deployment when the same version is already STARTED.
	CompareVersions bool
	// Parallelism is the number of artifacts processed concurrently
	// (<= 0 means all at once).
	Parallelism int
	// ClockSkew is the tolerance applied when comparing the tenant's DeployedOn
	// with the local trigger time (only used when no previous deployment
	// timestamp is known). Defaults to 2 minutes.
	ClockSkew time.Duration
}

// Tenant is the subset of the CPI API the deployer needs.
type Tenant interface {
	DesigntimeVersion(artifactType, id string) (version string, exists bool, err error)
	TriggerDeploy(artifactType, id string) (taskID string, err error)
	BuildAndDeployStatus(taskID string) (string, error)
	RuntimeArtifact(id string) (*api.RuntimeArtifact, error) // nil if not deployed
	RuntimeErrorInfo(id string) (string, error)
	Undeploy(id string) error
}

// NewTenant returns a Tenant backed by the CPI OData API.
func NewTenant(exe *httpclnt.HTTPExecuter) Tenant {
	return &apiTenant{exe: exe, rt: api.NewRuntime(exe)}
}

type apiTenant struct {
	exe *httpclnt.HTTPExecuter
	rt  *api.Runtime
}

func (t *apiTenant) DesigntimeVersion(artifactType, id string) (string, bool, error) {
	dt := api.NewDesigntimeArtifact(artifactType, t.exe)
	if dt == nil {
		return "", false, fmt.Errorf("unsupported artifact type %q (valid types: %s)", artifactType, strings.Join(api.ArtifactTypes, ", "))
	}
	version, _, exists, err := dt.Get(id, "active")
	return version, exists, err
}

func (t *apiTenant) TriggerDeploy(artifactType, id string) (string, error) {
	return api.TriggerDeploy(id, artifactType, t.exe)
}

func (t *apiTenant) BuildAndDeployStatus(taskID string) (string, error) {
	return t.rt.GetBuildAndDeployStatus(taskID)
}

func (t *apiTenant) RuntimeArtifact(id string) (*api.RuntimeArtifact, error) {
	return t.rt.GetArtifact(id)
}

func (t *apiTenant) RuntimeErrorInfo(id string) (string, error) {
	return t.rt.GetErrorInfo(id)
}

func (t *apiTenant) Undeploy(id string) error {
	return t.rt.UnDeploy(id)
}

// Deploy deploys all artifacts and waits for each outcome. Results are
// returned in input order; an individual failure never stops the others.
func Deploy(ctx context.Context, tenant Tenant, artifacts []Artifact, opts Options) []Result {
	return forEach(ctx, artifacts, opts.Parallelism, func(a Artifact) Result {
		return deployOne(ctx, tenant, a, opts)
	})
}

// Undeploy removes all artifacts from runtime and waits until each is gone.
func Undeploy(ctx context.Context, tenant Tenant, artifacts []Artifact, opts Options) []Result {
	return forEach(ctx, artifacts, opts.Parallelism, func(a Artifact) Result {
		return undeployOne(ctx, tenant, a, opts)
	})
}

func forEach(ctx context.Context, artifacts []Artifact, parallelism int, fn func(Artifact) Result) []Result {
	results := make([]Result, len(artifacts))
	if parallelism <= 0 || parallelism > len(artifacts) {
		parallelism = len(artifacts)
	}
	sem := make(chan struct{}, max(parallelism, 1))
	var wg sync.WaitGroup
	for i, a := range artifacts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := fn(a)
			r.ID, r.Type, r.PackageID = a.ID, a.Type, a.PackageID
			results[i] = r
		}()
	}
	wg.Wait()
	return results
}

func fail(r Result, err error) Result {
	r.Status, r.Err, r.Error = StatusFailed, err, err.Error()
	return r
}

func timeout(r Result, err error) Result {
	r.Status, r.Err, r.Error = StatusTimeout, err, err.Error()
	return r
}

func deployOne(ctx context.Context, tenant Tenant, a Artifact, opts Options) Result {
	var r Result
	logger := log.With().Str("artifact", a.ID).Logger()

	if !api.IsValidArtifactType(a.Type) {
		return fail(r, fmt.Errorf("unsupported artifact type %q (valid types: %s)", a.Type, strings.Join(api.ArtifactTypes, ", ")))
	}
	version, exists, err := tenant.DesigntimeVersion(a.Type, a.ID)
	if err != nil {
		return fail(r, err)
	}
	if !exists {
		return fail(r, fmt.Errorf("designtime artifact %v does not exist", a.ID))
	}
	r.Version = version

	// The runtime state before the trigger is the baseline used to tell a
	// fresh deployment from the previous one that is still visible.
	before, err := tenant.RuntimeArtifact(a.ID)
	if err != nil {
		return fail(r, err)
	}
	if opts.CompareVersions && before != nil && before.Status == "STARTED" && before.Version == version {
		logger.Info().Msgf("Artifact %v with version %v already deployed. Skipping runtime deployment", a.ID, version)
		r.Status = StatusSkipped
		return r
	}

	logger.Info().Msgf("🚀 Deploying artifact %v with version %v", a.ID, version)
	triggeredAt := time.Now()
	r.TaskID, err = tenant.TriggerDeploy(a.Type, a.ID)
	if err != nil {
		return fail(r, err)
	}
	logger.Info().Str("taskId", r.TaskID).Msgf("Artifact %v deployment triggered", a.ID)

	return waitForDeployment(ctx, tenant, a, r, before, triggeredAt, opts)
}

func waitForDeployment(ctx context.Context, tenant Tenant, a Artifact, r Result, before *api.RuntimeArtifact,
	triggeredAt time.Time, opts Options) Result {

	logger := log.With().Str("artifact", a.ID).Logger()
	skew := opts.ClockSkew
	if skew == 0 {
		skew = 2 * time.Minute
	}

	taskDone := r.TaskID == "" // nothing to poll without a task ID
	taskConfirmed := false
	taskStatus, runtimeStatus := "", ""
	var lastErr error

	for check := 1; check <= opts.MaxChecks; check++ {
		if err := sleep(ctx, opts.Interval); err != nil {
			return fail(r, fmt.Errorf("deployment of %v not confirmed: %w", a.ID, err))
		}

		// 1) Build and deploy task status
		if !taskDone {
			status, err := tenant.BuildAndDeployStatus(r.TaskID)
			switch {
			case err != nil && httpclnt.IsAuthError(err):
				return fail(r, err)
			case err != nil && httpclnt.StatusCode(err) == 404:
				// Task unknown (or endpoint unavailable): rely on runtime status only
				logger.Debug().Msgf("Build and deploy status of task %v not available, using runtime status", r.TaskID)
				taskDone = true
			case err != nil:
				lastErr = err
				logger.Warn().Msgf("Check %d/%d - failed to get build and deploy status: %v", check, opts.MaxChecks, err)
				continue
			default:
				taskStatus = status
				logger.Info().Msgf("Check %d/%d - build and deploy status of %v = %s", check, opts.MaxChecks, a.ID, status)
				switch strings.ToUpper(status) {
				case "SUCCESS":
					taskDone, taskConfirmed = true, true
				case "FAIL", "FAILED", "ERROR":
					return fail(r, fmt.Errorf("artifact %v build/deploy task %v ended with status %s%s",
						a.ID, r.TaskID, status, errorInfoSuffix(ctx, tenant, a.ID, opts.Interval)))
				default:
					continue
				}
			}
		}

		// 2) Runtime status
		rt, err := tenant.RuntimeArtifact(a.ID)
		if err != nil {
			if httpclnt.IsAuthError(err) {
				return fail(r, err)
			}
			lastErr = err
			logger.Warn().Msgf("Check %d/%d - failed to get runtime status: %v", check, opts.MaxChecks, err)
			continue
		}
		if rt == nil {
			runtimeStatus = "NOT_DEPLOYED"
			logger.Info().Msgf("Check %d/%d - artifact %v not yet visible on runtime", check, opts.MaxChecks, a.ID)
			continue
		}
		runtimeStatus = rt.Status
		if !isFresh(rt, before, triggeredAt, skew, taskConfirmed) {
			logger.Info().Msgf("Check %d/%d - runtime still shows the previous deployment of %v (status %s)", check, opts.MaxChecks, a.ID, rt.Status)
			continue
		}
		logger.Info().Msgf("Check %d/%d - runtime status of %v = %s", check, opts.MaxChecks, a.ID, rt.Status)
		switch rt.Status {
		case "STARTED":
			if rt.Version != "" {
				r.Version = rt.Version
			}
			r.Status = StatusDeployed
			logger.Info().Msgf("Artifact %v deployed successfully", a.ID)
			return r
		case "STARTING":
			continue
		default:
			return fail(r, fmt.Errorf("artifact %v deployment unsuccessful, ended with status %s%s",
				a.ID, rt.Status, errorInfoSuffix(ctx, tenant, a.ID, opts.Interval)))
		}
	}

	err := fmt.Errorf("deployment status of %v remained unfinished after %d checks (task status %q, runtime status %q)",
		a.ID, opts.MaxChecks, taskStatus, runtimeStatus)
	if lastErr != nil {
		err = fmt.Errorf("%w; last error: %v", err, lastErr)
	}
	return timeout(r, err)
}

// isFresh reports whether the runtime artifact reflects the deployment that
// was triggered at triggeredAt, rather than the one that was running before.
// This prevents reporting success for a redeploy of the same version while
// the old STARTED artifact is still visible.
func isFresh(rt, before *api.RuntimeArtifact, triggeredAt time.Time, skew time.Duration, taskConfirmed bool) bool {
	if before == nil {
		return true // nothing was deployed before the trigger
	}
	if !rt.DeployedOn.IsZero() {
		if !before.DeployedOn.IsZero() {
			// Both timestamps come from the tenant clock: no skew involved
			return rt.DeployedOn.After(before.DeployedOn)
		}
		return !rt.DeployedOn.Before(triggeredAt.Add(-skew))
	}
	// No timestamp reported: only accept evidence that something changed
	return taskConfirmed || rt.Version != before.Version
}

// errorInfoSuffix fetches the runtime error information (best effort). The
// tenant sometimes needs a moment before it is available.
func errorInfoSuffix(ctx context.Context, tenant Tenant, id string, wait time.Duration) string {
	if sleep(ctx, wait) != nil {
		return ""
	}
	msg, err := tenant.RuntimeErrorInfo(id)
	if err != nil || msg == "" {
		return ""
	}
	return ". Error message = " + msg
}

func undeployOne(ctx context.Context, tenant Tenant, a Artifact, opts Options) Result {
	var r Result
	logger := log.With().Str("artifact", a.ID).Logger()

	current, err := tenant.RuntimeArtifact(a.ID)
	if err != nil {
		return fail(r, err)
	}
	if current == nil {
		logger.Info().Msgf("Artifact %v is not deployed. Nothing to undeploy", a.ID)
		r.Status = StatusNotDeployed
		return r
	}
	r.Version = current.Version

	if err = tenant.Undeploy(a.ID); err != nil {
		if httpclnt.StatusCode(err) == 404 {
			r.Status = StatusNotDeployed
			return r
		}
		return fail(r, err)
	}
	logger.Info().Msgf("Artifact %v undeployment triggered", a.ID)

	status := current.Status
	var lastErr error
	for check := 1; check <= opts.MaxChecks; check++ {
		if err := sleep(ctx, opts.Interval); err != nil {
			return fail(r, fmt.Errorf("undeployment of %v not confirmed: %w", a.ID, err))
		}
		rt, err := tenant.RuntimeArtifact(a.ID)
		if err != nil {
			if httpclnt.IsAuthError(err) {
				return fail(r, err)
			}
			lastErr = err
			logger.Warn().Msgf("Check %d/%d - failed to get runtime status: %v", check, opts.MaxChecks, err)
			continue
		}
		if rt == nil {
			logger.Info().Msgf("Artifact %v undeployed successfully", a.ID)
			r.Status = StatusUndeployed
			return r
		}
		status = rt.Status
		logger.Info().Msgf("Check %d/%d - artifact %v still on runtime (status %s)", check, opts.MaxChecks, a.ID, rt.Status)
	}
	err = fmt.Errorf("artifact %v still present on runtime after %d checks (status %q)", a.ID, opts.MaxChecks, status)
	if lastErr != nil {
		err = fmt.Errorf("%w; last error: %v", err, lastErr)
	}
	return timeout(r, err)
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Error summarises failed results; it is nil when every result succeeded.
type Error struct {
	Results []Result
}

// Err returns an *Error if any result did not succeed.
func Err(results []Result) error {
	for _, r := range results {
		if !r.Status.Succeeded() {
			return &Error{Results: results}
		}
	}
	return nil
}

// Failed returns the results that did not succeed.
func (e *Error) Failed() []Result {
	var failed []Result
	for _, r := range e.Results {
		if !r.Status.Succeeded() {
			failed = append(failed, r)
		}
	}
	return failed
}

func (e *Error) Error() string {
	failed := e.Failed()
	parts := make([]string, 0, len(failed))
	for _, r := range failed {
		parts = append(parts, fmt.Sprintf("%s: %s (%s)", r.ID, r.Status, r.Error))
	}
	return fmt.Sprintf("%d of %d artifact(s) failed - %s", len(failed), len(e.Results), strings.Join(parts, "; "))
}
