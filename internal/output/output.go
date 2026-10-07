// Package output implements the agent output contract: error classification
// into exit codes, the JSON result envelope and logger setup.
package output

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"

	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"golang.org/x/oauth2"
)

// Formats accepted by --output.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// UsageError marks invalid usage or configuration (exit code 2).
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// Usagef returns a UsageError.
func Usagef(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// Usage wraps err as a UsageError (nil stays nil).
func Usage(err error) error {
	if err == nil {
		return nil
	}
	return &UsageError{Err: err}
}

// PartialError marks an operation where some items failed (exit code 7).
type PartialError struct{ Err error }

func (e *PartialError) Error() string { return e.Err.Error() }
func (e *PartialError) Unwrap() error { return e.Err }

// Partial wraps err as a PartialError (nil stays nil).
func Partial(err error) error {
	if err == nil {
		return nil
	}
	return &PartialError{Err: err}
}

// FailedError marks an operation that failed on the tenant without any
// successful item (exit code 5).
type FailedError struct{ Err error }

func (e *FailedError) Error() string { return e.Err.Error() }
func (e *FailedError) Unwrap() error { return e.Err }

// Failed wraps err as a FailedError (nil stays nil).
func Failed(err error) error {
	if err == nil {
		return nil
	}
	return &FailedError{Err: err}
}

// ExitCoder is implemented by errors that know their exit code.
type ExitCoder interface{ ExitCode() int }

type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }
func (e *codedError) ExitCode() int { return e.code }

// WithExitCode wraps err with an explicit exit code (nil stays nil).
func WithExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, code: code}
}

// ExitCode classifies err into the exit code contract.
func ExitCode(err error) int {
	if err == nil {
		return exitcode.OK
	}
	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return exitcode.Usage
	}
	var partialErr *PartialError
	if errors.As(err, &partialErr) {
		return exitcode.Partial
	}
	var failedErr *FailedError
	if errors.As(err, &failedErr) {
		return exitcode.DeployFailed
	}
	return TransportExitCode(err, exitcode.Error)
}

// TransportExitCode classifies authentication and tenant communication
// errors, returning fallback for anything else.
func TransportExitCode(err error, fallback int) int {
	var retrieveErr *oauth2.RetrieveError
	if httpclnt.IsAuthError(err) || errors.As(err, &retrieveErr) {
		return exitcode.Auth
	}
	if httpclnt.StatusCode(err) != 0 {
		return exitcode.TenantHTTP
	}
	var urlErr *url.Error
	var netErr net.Error
	if errors.As(err, &urlErr) || errors.As(err, &netErr) {
		return exitcode.TenantHTTP
	}
	return fallback
}

// Envelope is the single JSON document written to stdout with --output json.
type Envelope struct {
	Command  string `json:"command"`
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
	// DurationMs is the time the command took, in milliseconds.
	DurationMs int64 `json:"durationMs"`
	Result     any   `json:"result"`
}

// WriteEnvelope writes env as one JSON document.
func WriteEnvelope(w io.Writer, env Envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}

type resultKey struct{}

type resultHolder struct {
	mu    sync.Mutex
	value any
}

// WithResultHolder returns a context in which commands can publish their
// structured result with SetResult.
func WithResultHolder(ctx context.Context) context.Context {
	return context.WithValue(ctx, resultKey{}, &resultHolder{})
}

// SetResult publishes the structured result of the running command.
func SetResult(ctx context.Context, v any) {
	if ctx == nil {
		return
	}
	if h, ok := ctx.Value(resultKey{}).(*resultHolder); ok {
		h.mu.Lock()
		h.value = v
		h.mu.Unlock()
	}
}

// Result returns the value published with SetResult (nil if none).
func Result(ctx context.Context) any {
	if h, ok := ctx.Value(resultKey{}).(*resultHolder); ok {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.value
	}
	return nil
}
