package mcp

import (
	"sync"
	"time"

	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
)

// Tracing exposes payloads, so set_log_level is time-boxed: the server sets
// the flow back to INFO on the first tool call after the deadline and when it
// shuts down.

// LogLevelReverter remembers pending log level reverts.
type LogLevelReverter struct {
	mu      sync.Mutex
	exe     *httpclnt.HTTPExecuter
	now     func() time.Time
	pending map[string]pendingRevert
}

type pendingRevert struct {
	at  time.Time
	req ops.LogLevelRequest
}

// NewLogLevelReverter returns a reverter that uses exe.
func NewLogLevelReverter(exe *httpclnt.HTTPExecuter) *LogLevelReverter {
	return &LogLevelReverter{exe: exe, now: time.Now, pending: map[string]pendingRevert{}}
}

// schedule reverts req.ArtifactID to INFO at at (replacing an earlier one);
// a zero at cancels a pending revert.
func (r *LogLevelReverter) schedule(req ops.LogLevelRequest, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if at.IsZero() {
		delete(r.pending, req.ArtifactID)
		return
	}
	req.Level = "INFO"
	r.pending[req.ArtifactID] = pendingRevert{at: at, req: req}
}

// RunDue reverts every flow whose deadline has passed.
func (r *LogLevelReverter) RunDue() { r.run(false) }

// RevertAll reverts every pending flow now (server shutdown).
func (r *LogLevelReverter) RevertAll() { r.run(true) }

func (r *LogLevelReverter) run(all bool) {
	r.mu.Lock()
	var due []ops.LogLevelRequest
	now := r.now()
	for id, p := range r.pending {
		if all || !now.Before(p.at) {
			due = append(due, p.req)
			delete(r.pending, id)
		}
	}
	r.mu.Unlock()
	for _, req := range due {
		if _, err := ops.SetLogLevel(r.exe, req); err != nil {
			log.Warn().Str("artifact", req.ArtifactID).Msgf("Could not set the log level back to INFO: %v", err)
		} else {
			log.Info().Str("artifact", req.ArtifactID).Msg("Log level set back to INFO")
		}
	}
}
