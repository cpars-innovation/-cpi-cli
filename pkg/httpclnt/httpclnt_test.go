package httpclnt

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockOauth(t *testing.T) {
	// Set credentials details
	const clientId = "dummyid"
	const clientSecret = "dummysecret"
	const token = "token123"

	// Set up local server with mock HTTP responses
	mux := http.NewServeMux()
	// Handler for OAuth token
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		encoded := base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%v:%v", clientId, clientSecret))
		if auth != fmt.Sprintf("Basic %v", encoded) {
			http.Error(w, "Invalid credentials for token URL authorization", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fmt.Appendf(nil, `{ "access_token": "%v" }`, token))
	})
	// Handler for OData endpoint using OAuth token
	mux.HandleFunc("/api/v1/IntegrationDesigntimeArtifacts(Id='Dummy',Version='Active')", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != fmt.Sprintf("Bearer %v", token) {
			http.Error(w, "Invalid token for endpoint authorization", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{ "d": { "Id": "Dummy" } }`))
	})
	svr := httptest.NewServer(mux)

	defer svr.Close()

	// Initialise HTTP executer
	host, port := GetHostPort(svr.URL)
	exe := New(host, "/oauth/token", clientId, clientSecret, "", "", host, "http", port, true)

	headers := map[string]string{
		"Accept": "application/json",
	}
	// Execute HTTP request
	resp, err := exe.ExecGetRequest("/api/v1/IntegrationDesigntimeArtifacts(Id='Dummy',Version='Active')", headers)

	// Verify HTTP response
	if err != nil {
		t.Fatalf("HTTP call failed with error - %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP call failed with response code - %v", resp.StatusCode)
	}
}

func TestMockBasicAuth(t *testing.T) {
	// Set credentials details
	const userId = "dummyuser"
	const password = "dummypassword"

	// Set up local server with mock HTTP responses
	mux := http.NewServeMux()
	// Handler for OData endpoint using basic authentication
	mux.HandleFunc("/api/v1/IntegrationDesigntimeArtifacts(Id='Dummy',Version='Active')", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		encoded := base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%v:%v", userId, password))
		if auth != fmt.Sprintf("Basic %v", encoded) {
			http.Error(w, "Invalid credentials for basic authentication", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{ "d": { "Id": "Dummy" } }`))
	})
	svr := httptest.NewServer(mux)

	defer svr.Close()

	// Initialise HTTP executer
	host, port := GetHostPort(svr.URL)
	exe := New("", "", "", "", userId, password, host, "http", port, true)

	headers := map[string]string{
		"Accept": "application/json",
	}
	// Execute HTTP request
	resp, err := exe.ExecGetRequest("/api/v1/IntegrationDesigntimeArtifacts(Id='Dummy',Version='Active')", headers)
	if err != nil {
		t.Fatalf("HTTP call failed with error - %v", err)
	}
	// Verify HTTP response
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP call failed with response code - %v", resp.StatusCode)
	}
}

func TestMockBasicAuthIDNotFound(t *testing.T) {
	// Set credentials details
	const userId = "dummyuser"
	const password = "dummypassword"

	// Set up local server with mock HTTP responses
	mux := http.NewServeMux()
	// Handler for OData endpoint using basic authentication
	mux.HandleFunc("/api/v1/IntegrationDesigntimeArtifacts(Id='Dummy',Version='Active')", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		encoded := base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%v:%v", userId, password))
		if auth != fmt.Sprintf("Basic %v", encoded) {
			http.Error(w, "Invalid credentials for basic authentication", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{ "error": { "code": "Not Found" } }`))
	})
	svr := httptest.NewServer(mux)

	defer svr.Close()

	// Initialise HTTP executer
	host, port := GetHostPort(svr.URL)
	exe := New("", "", "", "", userId, password, host, "http", port, true)

	headers := map[string]string{
		"Accept": "application/json",
	}
	// Execute HTTP request
	resp, err := exe.ExecGetRequest("/api/v1/IntegrationDesigntimeArtifacts(Id='Dummy',Version='Active')", headers)
	if err != nil {
		t.Fatalf("HTTP call failed with error - %v", err)
	}
	// Verify HTTP response
	if resp.StatusCode == http.StatusNotFound {
		_, err = exe.LogError(resp, "Get Integration designtime")
		errMsg := err.Error()
		if errMsg != "Get Integration designtime call failed with response code = 404" {
			t.Fatalf("Actual error returned = %s", errMsg)
		}
	} else {
		t.Fatalf("HTTP call failed with response code - %v", resp.StatusCode)
	}
}

// RetryReads retries throttled reads, never writes, and only when enabled.
func TestRetryReads(t *testing.T) {
	calls := map[string]int{}
	var mu sync.Mutex
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.Method+" "+r.URL.Path]++
		n := calls[r.Method+" "+r.URL.Path]
		mu.Unlock()
		switch {
		case r.URL.Path == "/flaky" && n <= 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case r.URL.Path == "/down":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer svr.Close()
	host, port := GetHostPort(svr.URL)

	plain := New("", "", "", "", "user", "pw", host, "http", port, false)
	resp, err := plain.ExecGetRequest("/flaky", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "no retries by default")

	exe := New("", "", "", "", "user", "pw", host, "http", port, false).RetryReads(3, time.Millisecond)
	calls["GET /flaky"] = 0
	resp, err = exe.ExecGetRequest("/flaky", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, calls["GET /flaky"])

	resp, err = exe.ExecGetRequest("/down", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "gives up after the retries")
	assert.Equal(t, 4, calls["GET /down"])
}
