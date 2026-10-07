package cpi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every executer retries throttled reads, never writes.
func TestInitHTTPExecuterRetriesReads(t *testing.T) {
	old := ReadBackoff
	ReadBackoff = time.Millisecond
	t.Cleanup(func() { ReadBackoff = old })

	var gets, posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "token")
		if r.URL.Path != "/x" {
			return // CSRF token fetch
		}
		n := gets.Load()
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		gets.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	exe := InitHTTPExecuter(&ServiceDetails{Host: srv.URL, Userid: "u", Password: "p"})
	resp, err := exe.ExecGetRequest("/x", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(3), gets.Load())

	resp, err = exe.Exec(http.MethodPost, "/x", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, int32(1), posts.Load(), "writes are not retried")
}
