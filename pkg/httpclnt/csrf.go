package httpclnt

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// csrf caches the CSRF token and the session cookies it is bound to.
//
// SAP Cloud Integration requires an X-CSRF-Token on modifying requests when
// Basic Auth is used. The token is fetched once (GET with "X-CSRF-Token:
// Fetch") and reused for all later requests; when the tenant answers 403 with
// "X-CSRF-Token: Required" (token expired or session lost), it is fetched again
// and the request is retried once. With OAuth no token is fetched up front,
// but the same 403 handling applies.
type csrf struct {
	mu      sync.Mutex
	token   string
	cookies []*http.Cookie
	// generation increases on every fetch, so concurrent requests that saw
	// the same expired token trigger only one refetch.
	generation int
	fetches    int
}

// CSRFFetchPath is the path used to fetch the CSRF token.
const CSRFFetchPath = "/api/v1/"

func isModifying(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// csrfRequired reports whether the response asks for a (new) CSRF token.
func csrfRequired(resp *http.Response) bool {
	return resp.StatusCode == http.StatusForbidden && strings.EqualFold(resp.Header.Get("X-CSRF-Token"), "required")
}

// current returns the cached token (fetching it if needed when fetch is set).
func (e *HTTPExecuter) csrfCurrent(fetch bool) (token string, cookies []*http.Cookie, generation int, err error) {
	e.csrf.mu.Lock()
	defer e.csrf.mu.Unlock()
	if e.csrf.token == "" && fetch {
		if err := e.fetchCSRFLocked(); err != nil {
			return "", nil, 0, err
		}
	}
	return e.csrf.token, e.csrf.cookies, e.csrf.generation, nil
}

// csrfRefresh fetches a new token unless another request already did so
// after seenGeneration.
func (e *HTTPExecuter) csrfRefresh(seenGeneration int) (string, []*http.Cookie, error) {
	e.csrf.mu.Lock()
	defer e.csrf.mu.Unlock()
	if e.csrf.generation == seenGeneration {
		if err := e.fetchCSRFLocked(); err != nil {
			return "", nil, err
		}
	}
	return e.csrf.token, e.csrf.cookies, nil
}

func (e *HTTPExecuter) fetchCSRFLocked() error {
	path := CSRFFetchPath
	if e.csrfPath != "" {
		path = e.csrfPath
	}
	resp, err := e.send(http.MethodGet, path, nil, map[string]string{"X-CSRF-Token": "Fetch"}, nil)
	if err != nil {
		return err
	}
	e.csrf.fetches++
	if resp.StatusCode != http.StatusOK {
		_, err = e.LogError(resp, "Get CSRF token")
		return err
	}
	_, _ = e.ReadRespBody(resp)
	token := resp.Header.Get("X-CSRF-Token")
	if token == "" {
		return fmt.Errorf("tenant did not return a CSRF token")
	}
	e.csrf.token, e.csrf.cookies = token, resp.Cookies()
	e.csrf.generation++
	return nil
}

// CSRFFetches returns how many times a CSRF token was fetched (for tests and
// diagnostics).
func (e *HTTPExecuter) CSRFFetches() int {
	e.csrf.mu.Lock()
	defer e.csrf.mu.Unlock()
	return e.csrf.fetches
}
