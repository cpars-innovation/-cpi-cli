package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/ops"
)

// An autonomous build loop needs limits the model cannot talk its way past.
// The ledger counts the tenant calls of an open loop on the server and refuses
// further changes once a limit is reached; read tools keep working, so the
// agent can still report.

// LoopLimits are the stop conditions of a loop.
type LoopLimits struct {
	MaxIterations    int `json:"maxIterations"`
	SameErrorLimit   int `json:"sameErrorLimit"`
	WallClockMinutes int `json:"wallClockMinutes"`
	MaxDeploys       int `json:"maxDeploys"`
}

// DefaultLoopLimits apply to omitted limits.
var DefaultLoopLimits = LoopLimits{MaxIterations: 5, SameErrorLimit: 2, WallClockMinutes: 60, MaxDeploys: 15}

// LoopCall is one tool call recorded in a loop.
type LoopCall struct {
	Time     time.Time `json:"time"`
	Tool     string    `json:"tool"`
	OK       bool      `json:"ok"`
	Category string    `json:"errorCategory,omitempty"`
	Summary  string    `json:"summary,omitempty"`
}

// LoopStatus is the state of a loop.
type LoopStatus struct {
	ID         string     `json:"loopId"`
	Goal       string     `json:"goal"`
	Limits     LoopLimits `json:"limits"`
	Started    time.Time  `json:"started"`
	Steps      int        `json:"steps"`
	Iterations int        `json:"iterations"`
	Deploys    int        `json:"deploys"`
	// Errors counts each error fingerprint seen in failed test results.
	Errors  map[string]int `json:"errors"`
	Stopped string         `json:"stopped,omitempty"`
	Ended   bool           `json:"ended"`
	Outcome string         `json:"outcome,omitempty"`
}

type loop struct {
	LoopStatus
	calls []LoopCall
	// failedSinceDeploy is set by a failed test result; the next deploy is
	// then a new iteration.
	failedSinceDeploy bool
}

// StoppedError refuses a tenant call because a loop limit was reached.
type StoppedError struct{ Reason string }

func (e *StoppedError) Error() string {
	return "loop stopped: " + e.Reason + " (read tools still work; call loop_end and report to the user)"
}

// ExitCode implements output.ExitCoder.
func (e *StoppedError) ExitCode() int { return exitcode.Stopped }

// Ledger tracks the open loop of a server (at most one at a time).
type Ledger struct {
	mu    sync.Mutex
	root  string
	now   func() time.Time
	loops map[string]*loop
	open  *loop
}

// NewLedger returns a ledger writing to <root>/.cpi/loops.
func NewLedger(root string) *Ledger {
	return &Ledger{root: root, now: time.Now, loops: map[string]*loop{}}
}

func (l *Ledger) dir() string { return filepath.Join(l.root, ".cpi", "loops") }

func (l *Ledger) start(goal string, limits LoopLimits) (*LoopStatus, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.open != nil {
		return nil, output.Usagef("loop %s is still open: call loop_end first", l.open.ID)
	}
	if strings.TrimSpace(goal) == "" {
		return nil, output.Usagef("goal is required")
	}
	d := DefaultLoopLimits
	if limits.MaxIterations > 0 {
		d.MaxIterations = limits.MaxIterations
	}
	if limits.SameErrorLimit > 0 {
		d.SameErrorLimit = limits.SameErrorLimit
	}
	if limits.WallClockMinutes > 0 {
		d.WallClockMinutes = limits.WallClockMinutes
	}
	if limits.MaxDeploys > 0 {
		d.MaxDeploys = limits.MaxDeploys
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	now := l.now().UTC()
	lp := &loop{LoopStatus: LoopStatus{ID: now.Format("20060102-150405") + "-" + hex.EncodeToString(b), Goal: goal, Limits: d, Started: now, Errors: map[string]int{}}}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return nil, err
	}
	l.loops[lp.ID], l.open = lp, lp
	st := lp.LoopStatus
	return &st, nil
}

func (l *Ledger) status(id string) (*LoopStatus, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lp, ok := l.loops[id]
	if !ok {
		return nil, output.Usagef("unknown loop %q", id)
	}
	l.checkClock(lp)
	st := lp.LoopStatus
	st.Errors = copyMap(lp.Errors)
	return &st, nil
}

func copyMap(m map[string]int) map[string]int {
	c := make(map[string]int, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func (l *Ledger) checkClock(lp *loop) {
	if lp.Stopped == "" && !lp.Ended && l.now().Sub(lp.Started) > time.Duration(lp.Limits.WallClockMinutes)*time.Minute {
		lp.Stopped = fmt.Sprintf("wall clock limit of %d minutes reached", lp.Limits.WallClockMinutes)
	}
}

// before is called before a tool runs; it refuses tenant calls of a stopped
// loop and deployments over the limits.
func (l *Ledger) before(tool string, args json.RawMessage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	lp := l.open
	if lp == nil || EffectOf(tool) != EffectTenant {
		return nil
	}
	l.checkClock(lp)
	if lp.Stopped != "" {
		return &StoppedError{Reason: lp.Stopped}
	}
	if isDeployCall(tool, args) {
		if lp.Deploys >= lp.Limits.MaxDeploys {
			lp.Stopped = fmt.Sprintf("max_deploys (%d) reached", lp.Limits.MaxDeploys)
			return &StoppedError{Reason: lp.Stopped}
		}
		if lp.failedSinceDeploy && lp.Iterations >= lp.Limits.MaxIterations {
			lp.Stopped = fmt.Sprintf("max_iterations (%d) reached", lp.Limits.MaxIterations)
			return &StoppedError{Reason: lp.Stopped}
		}
	}
	return nil
}

// isDeployCall reports deployments that count against the limits:
// deploy, and pd_deploy unless it is a dry run.
func isDeployCall(tool string, args json.RawMessage) bool {
	switch tool {
	case "deploy":
		return true
	case "pd_deploy":
		var a struct {
			DryRun *bool `json:"dry_run"`
		}
		_ = json.Unmarshal(args, &a)
		return a.DryRun != nil && !*a.DryRun
	}
	return false
}

// after records a tool call of the open loop.
func (l *Ledger) after(tool string, args json.RawMessage, value any, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lp := l.open
	if lp == nil || strings.HasPrefix(tool, "loop_") {
		return
	}
	var stopped *StoppedError
	refused := false
	if e, ok := err.(*StoppedError); ok {
		stopped, refused = e, true
	}
	call := LoopCall{Time: l.now().UTC(), Tool: tool, OK: err == nil}
	if err != nil {
		call.Category = Category(output.ExitCode(err))
		call.Summary = firstLine(err.Error())
	}
	if !refused && EffectOf(tool) == EffectTenant {
		lp.Steps++
		if isDeployCall(tool, args) {
			lp.Deploys++
			if lp.failedSinceDeploy {
				lp.Iterations++
				lp.failedSinceDeploy = false
			}
		}
	}
	if fp, failed := failureFingerprint(tool, value, err); failed && stopped == nil {
		lp.failedSinceDeploy = true
		if fp != "" {
			lp.Errors[fp]++
			if lp.Errors[fp] >= lp.Limits.SameErrorLimit && lp.Stopped == "" {
				lp.Stopped = fmt.Sprintf("the same error occurred %d times (same_error_limit)", lp.Errors[fp])
			}
		}
	}
	lp.calls = append(lp.calls, call)
	l.appendJSONL(lp, call)
}

func (l *Ledger) appendJSONL(lp *loop, call LoopCall) {
	f, err := os.OpenFile(filepath.Join(l.dir(), lp.ID+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(call)
}

var (
	reGUIDLike   = regexp.MustCompile(`[A-Za-z0-9_-]{28}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	reTimestamp  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?`)
	reLongNumber = regexp.MustCompile(`\d{6,}`)
)

// normalizeError removes what differs between two runs of the same error.
func normalizeError(s string) string {
	s = reTimestamp.ReplaceAllString(s, "<time>")
	s = reGUIDLike.ReplaceAllString(s, "<id>")
	s = reLongNumber.ReplaceAllString(s, "<n>")
	return strings.Join(strings.Fields(s), " ")
}

func fingerprint(artifactID, modelStepID, errorText string) string {
	sum := sha256.Sum256([]byte(artifactID + "|" + modelStepID + "|" + normalizeError(errorText)))
	return hex.EncodeToString(sum[:8])
}

// failureFingerprint reports whether a test or diagnosis result shows a
// failed message, and its fingerprint.
func failureFingerprint(tool string, value any, err error) (string, bool) {
	switch v := value.(type) {
	case *ops.SentMessage:
		if err == nil || v == nil {
			return "", false
		}
		text := err.Error()
		if v.Log != nil && v.Log.ErrorText != "" {
			text = v.Log.ErrorText
		}
		return fingerprint(v.ArtifactID, "", text), true
	case *ops.TraceTree:
		if v == nil || v.FirstFailure == nil {
			return "", false
		}
		return fingerprint(v.FirstFailure.ArtifactID, "", v.FirstFailure.ErrorText), true
	case *ops.MessageLogList:
		if v == nil {
			return "", false
		}
		for _, m := range v.Logs {
			if m.Status == "FAILED" || m.Status == "ESCALATED" {
				return fingerprint(m.ArtifactID, "", m.ErrorText), true
			}
		}
	}
	return "", false
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func (l *Ledger) end(id, outcome string) (map[string]any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lp, ok := l.loops[id]
	if !ok {
		return nil, output.Usagef("unknown loop %q", id)
	}
	lp.Ended, lp.Outcome = true, outcome
	if l.open == lp {
		l.open = nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Loop %s\n\n**Goal:** %s\n\n**Outcome:** %s\n\n", lp.ID, lp.Goal, outcome)
	if lp.Stopped != "" {
		fmt.Fprintf(&b, "**Stopped:** %s\n\n", lp.Stopped)
	}
	fmt.Fprintf(&b, "| Counter | Value | Limit |\n|---|---|---|\n| Steps | %d | |\n| Iterations | %d | %d |\n| Deploys | %d | %d |\n| Duration | %s | %d min |\n\n",
		lp.Steps, lp.Iterations, lp.Limits.MaxIterations, lp.Deploys, lp.Limits.MaxDeploys,
		l.now().Sub(lp.Started).Round(time.Second), lp.Limits.WallClockMinutes)
	b.WriteString("| Time | Tool | Result | Detail |\n|---|---|---|---|\n")
	for _, c := range lp.calls {
		result := "ok"
		if !c.OK {
			result = c.Category
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Time.Format(time.TimeOnly), c.Tool, result, strings.ReplaceAll(c.Summary, "|", "\\|"))
	}
	file := filepath.Join(l.dir(), lp.ID+".md")
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(l.root, file)
	st := lp.LoopStatus
	return map[string]any{"status": st, "summaryFile": filepath.ToSlash(rel)}, nil
}

// Wrap returns the tools with ledger accounting and adds the loop tools.
func (l *Ledger) Wrap(tools []Tool) []Tool {
	out := make([]Tool, 0, len(tools)+3)
	for _, t := range tools {
		t := t
		inner := t.Handler
		t.Handler = func(ctx context.Context, raw json.RawMessage) (any, error) {
			if err := l.before(t.Name, raw); err != nil {
				l.after(t.Name, raw, nil, err)
				return nil, err
			}
			v, err := inner(ctx, raw)
			l.after(t.Name, raw, v, err)
			return v, err
		}
		out = append(out, t)
	}
	return append(out, l.tools()...)
}

func (l *Ledger) tools() []Tool {
	return []Tool{
		{
			Name: "loop_start", Title: "Start a build loop with limits",
			Description: "Open a build/test/fix loop before changing anything on the tenant. While it is open, the server counts tenant calls and refuses further changes " +
				"(errorCategory stopped) when max_iterations (deploys after a failed test), same_error_limit (the same failure seen again), wall_clock_minutes or max_deploys is reached. " +
				"Read tools keep working. Returns loop_id; finish with loop_end.",
			InputSchema: object(props{
				"goal":               str("What the loop should achieve"),
				"max_iterations":     map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "Deploys after a failed test, default 5"},
				"same_error_limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "description": "Stop when the same error was seen this often, default 2"},
				"wall_clock_minutes": map[string]any{"type": "integer", "minimum": 1, "maximum": 480, "description": "Maximum duration, default 60"},
				"max_deploys":        map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Maximum deploy / pd_deploy calls, default 15"},
			}, "goal"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					Goal             string `json:"goal"`
					MaxIterations    int    `json:"max_iterations"`
					SameErrorLimit   int    `json:"same_error_limit"`
					WallClockMinutes int    `json:"wall_clock_minutes"`
					MaxDeploys       int    `json:"max_deploys"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return l.start(a.Goal, LoopLimits{MaxIterations: a.MaxIterations, SameErrorLimit: a.SameErrorLimit, WallClockMinutes: a.WallClockMinutes, MaxDeploys: a.MaxDeploys})
			},
		},
		{
			Name: "loop_status", Title: "Get the counters of a loop",
			Description: "Counters and limits of a loop: steps, iterations, deploys, error fingerprints and why it stopped, if it did.",
			InputSchema: object(props{"loop_id": str("Loop ID from loop_start")}, "loop_id"),
			Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": false},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					LoopID string `json:"loop_id"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return l.status(a.LoopID)
			},
		},
		{
			Name: "loop_end", Title: "End a loop and write its summary",
			Description: "Close a loop (always allowed, also after a stop) and write .cpi/loops/<loop_id>.md: goal, outcome, counters and every tool call with its result. Give the summary to the user.",
			InputSchema: object(props{
				"loop_id": str("Loop ID from loop_start"),
				"outcome": str("What was achieved, or why the loop ended"),
			}, "loop_id", "outcome"),
			Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var a struct {
					LoopID  string `json:"loop_id"`
					Outcome string `json:"outcome"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return l.end(a.LoopID, a.Outcome)
			},
		},
	}
}
