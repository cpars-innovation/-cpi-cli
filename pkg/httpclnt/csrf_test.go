package httpclnt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// csrfServer is a tenant that enforces CSRF like SAP Cloud Integration:
// modifying requests need the token and the session cookie it is bound to.
type csrfServer struct {
	mu        sync.Mutex
	token     string
	serial    int
	fetches   int
	writes    int
	rejected  int
	bodies    []string
	oauth     bool // expect a bearer token instead of basic auth
	failFetch int  // status for token fetches (0: 200)
	forbidden bool // answer every write with a plain 403 (no CSRF header)
}

func (s *csrfServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URL.Path == "/oauth/token" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "bearer", "expires_in": 3600})
		return
	}
	if s.oauth && r.Header.Get("Authorization") != "Bearer at" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == CSRFFetchPath {
		if !strings.EqualFold(r.Header.Get("X-CSRF-Token"), "fetch") {
			return
		}
		s.fetches++
		if s.failFetch != 0 {
			w.WriteHeader(s.failFetch)
			return
		}
		if s.token == "" {
			s.serial++
			s.token = fmt.Sprintf("t%d", s.serial)
		}
		w.Header().Set("X-CSRF-Token", s.token)
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "s-" + s.token})
		return
	}
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte("ok"))
		return
	}
	if s.forbidden {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	cookie, err := r.Cookie("JSESSIONID")
	if s.token == "" || r.Header.Get("X-CSRF-Token") != s.token || err != nil || cookie.Value != "s-"+s.token {
		s.rejected++
		w.Header().Set("X-CSRF-Token", "Required")
		w.WriteHeader(http.StatusForbidden)
		return
	}
	body, _ := io.ReadAll(r.Body)
	s.bodies = append(s.bodies, string(body))
	s.writes++
	w.WriteHeader(http.StatusCreated)
}

func (s *csrfServer) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = ""
}

func newCSRFServer(t *testing.T, s *csrfServer) *HTTPExecuter {
	t.Helper()
	svr := httptest.NewServer(http.HandlerFunc(s.handler))
	t.Cleanup(svr.Close)
	host, port := GetHostPort(svr.URL)
	if s.oauth {
		return New(host, "/oauth/token", "id", "secret", "", "", host, "http", port, false)
	}
	return New("", "", "", "", "user", "pw", host, "http", port, false)
}

func post(t *testing.T, e *HTTPExecuter, body string) *http.Response {
	t.Helper()
	resp, err := e.Exec(http.MethodPost, "/api/v1/Things", strings.NewReader(body), map[string]string{"Content-Type": "text/plain"})
	require.NoError(t, err)
	_, _ = e.ReadRespBody(resp)
	return resp
}

func TestCSRFTokenIsFetchedOnceAndReused(t *testing.T) {
	s := &csrfServer{}
	e := newCSRFServer(t, s)
	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusCreated, post(t, e, fmt.Sprint(i)).StatusCode)
	}
	assert.Equal(t, 1, s.fetches)
	assert.Equal(t, 5, s.writes)
	assert.Zero(t, s.rejected)
	assert.Equal(t, 1, e.CSRFFetches())
}

func TestCSRFReadsNeverFetchAToken(t *testing.T) {
	s := &csrfServer{}
	e := newCSRFServer(t, s)
	resp, err := e.ExecGetRequest("/api/v1/Things", nil)
	require.NoError(t, err)
	_, _ = e.ReadRespBody(resp)
	assert.Zero(t, s.fetches)
}

func TestCSRFExpiredTokenIsRefreshedAndRequestReplayed(t *testing.T) {
	s := &csrfServer{}
	e := newCSRFServer(t, s)
	post(t, e, "first")
	s.expire() // session ended on the tenant side

	resp := post(t, e, "second")
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, 2, s.fetches)
	assert.Equal(t, 1, s.rejected)
	assert.Equal(t, []string{"first", "second"}, s.bodies, "the body is sent again on retry")
}

func TestCSRFOtherForbiddenIsNotRetried(t *testing.T) {
	s := &csrfServer{forbidden: true}
	e := newCSRFServer(t, s)
	resp := post(t, e, "x")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, 1, s.fetches, "only the initial fetch, no retry loop")
}

func TestCSRFFetchFailureIsReported(t *testing.T) {
	s := &csrfServer{failFetch: http.StatusUnauthorized}
	e := newCSRFServer(t, s)
	_, err := e.Exec(http.MethodPost, "/api/v1/Things", strings.NewReader("x"), nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, StatusCode(err), "fetch errors keep the HTTP status for exit-code classification")
	assert.Zero(t, s.writes)
}

func TestCSRFWithOAuthOnlyWhenRequired(t *testing.T) {
	s := &csrfServer{oauth: true}
	e := newCSRFServer(t, s)
	// This server enforces CSRF even for OAuth: the first write is rejected,
	// then the token is fetched and the write retried.
	assert.Equal(t, http.StatusCreated, post(t, e, "a").StatusCode)
	assert.Equal(t, 1, s.fetches)
	assert.Equal(t, 1, s.rejected)
	// Later writes reuse the token
	assert.Equal(t, http.StatusCreated, post(t, e, "b").StatusCode)
	assert.Equal(t, 1, s.fetches)
}

func TestCSRFConcurrentWritesShareOneRefresh(t *testing.T) {
	s := &csrfServer{}
	e := newCSRFServer(t, s)
	post(t, e, "warm-up")
	s.expire()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			resp, err := e.Exec(http.MethodPut, "/api/v1/Things", strings.NewReader(fmt.Sprint(i)), nil)
			if assert.NoError(t, err) {
				_, _ = e.ReadRespBody(resp)
				assert.Equal(t, http.StatusCreated, resp.StatusCode)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 21, s.writes)
	assert.LessOrEqual(t, s.fetches, 3, "expired token is refreshed once, not once per request")
}
