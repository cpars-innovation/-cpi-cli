package ops

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// Doctor check statuses.
const (
	CheckOK        = "ok"
	CheckWarn      = "warn"      // works partly or is optional
	CheckForbidden = "forbidden" // 403: the credentials lack a role
	CheckMissing   = "missing"   // 404: not available on this tenant
	CheckFailed    = "failed"
)

// Check is the outcome of one doctor check.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	// Needed lists what does not work without it.
	Needed string `json:"needed,omitempty"`
	Ms     int64  `json:"ms,omitempty"`
}

// DoctorResult lists the checks; OK is false when the tenant cannot be used
// at all (no connection, authentication or designtime access).
type DoctorResult struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
	// Err says why the tenant cannot be used (exit code: auth, tenant HTTP
	// or failed); nil when OK.
	Err error `json:"-"`
}

// apiProbe is one API area: a cheap read that needs the area's role.
type apiProbe struct {
	name, path, needed string
	core               bool
}

var apiProbes = []apiProbe{
	{"designtime", "/api/v1/IntegrationPackages?$top=1", "packages, artifacts, upload, snapshot, orchestrator", true},
	{"runtime", "/api/v1/IntegrationRuntimeArtifacts?$top=1", "deploy, undeploy, status, plan", false},
	{"message logs", "/api/v1/MessageProcessingLogs?$top=1", "logs, send (wait), test loops", false},
	{"security material", "/api/v1/UserCredentials?$top=1", "credentials list/apply", false},
	{"keystore", "/api/v1/KeystoreEntries?$top=1", "keystore list, expiring certificates", false},
	{"partner directory", "/api/v1/StringParameters?$top=1", "pd-snapshot, pd-deploy, pd_diff", false},
	{"data stores", "/api/v1/DataStores?$top=1", "datastore commands", false},
	{"log files", "/api/v1/LogFiles?$top=1", "log-files", false},
}

// Doctor checks the connection and which API areas the credentials can
// read. Only GET requests with $top=1 are sent.
func Doctor(ctx context.Context, exe *httpclnt.HTTPExecuter) *DoctorResult {
	res := &DoctorResult{Checks: make([]Check, len(apiProbes))}

	// the first probe also proves authentication and connectivity; run it
	// alone so that a token problem is reported once
	var err error
	res.Checks[0], err = probe(exe, apiProbes[0])
	if res.Checks[0].Status == CheckFailed {
		res.Checks = res.Checks[:1]
		res.Err = err
		return res
	}
	var wg sync.WaitGroup
	for i := 1; i < len(apiProbes); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ctx.Err() == nil {
				res.Checks[i], _ = probe(exe, apiProbes[i])
			}
		}()
	}
	wg.Wait()
	res.OK = res.Checks[0].Status == CheckOK
	return res
}

func probe(exe *httpclnt.HTTPExecuter, p apiProbe) (c Check, err error) {
	c = Check{Name: p.name, Needed: p.needed}
	started := time.Now()
	resp, err := exe.ExecGetRequest(p.path, map[string]string{"Accept": "application/json"})
	c.Ms = time.Since(started).Milliseconds()
	if err != nil {
		c.Status = CheckFailed
		switch output.TransportExitCode(err, 0) {
		case exitcode.Auth:
			c.Detail = "authentication failed: " + err.Error()
			return c, output.WithExitCode(err, exitcode.Auth)
		default:
			c.Detail = "no connection: " + err.Error()
		}
		return c, output.WithExitCode(err, exitcode.TenantHTTP)
	}
	_, _ = exe.ReadRespBody(resp)
	switch {
	case resp.StatusCode < 300:
		c.Status = CheckOK
		c.Needed = ""
	case resp.StatusCode == http.StatusUnauthorized:
		c.Status, c.Detail = CheckFailed, "401: the credentials are not accepted"
	case resp.StatusCode == http.StatusForbidden:
		c.Status, c.Detail = CheckForbidden, "403: the credentials lack the role for this area"
	case resp.StatusCode == http.StatusNotFound:
		c.Status, c.Detail = CheckMissing, "404: this API is not available on the tenant"
	default:
		c.Status, c.Detail = CheckWarn, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if p.core && c.Status != CheckOK {
		c.Status = CheckFailed
		code := exitcode.DeployFailed
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			code = exitcode.Auth
		}
		return c, output.WithExitCode(fmt.Errorf("%s: %s", p.name, c.Detail), code)
	}
	return c, nil
}
