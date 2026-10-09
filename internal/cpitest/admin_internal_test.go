package cpitest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// adminCall sends a request to the admin API as a client at remote, with an
// optional bearer token, without going through the network.
func adminCall(m *Tenant, remote, token string) int {
	r := httptest.NewRequest(http.MethodGet, "/_mock/state", nil)
	r.RemoteAddr = remote
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	m.handle(w, r)
	return w.Code
}

func TestAdminWithoutTokenOnlyOnLoopback(t *testing.T) {
	m := Start(nil)
	defer m.Close()
	assert.Equal(t, http.StatusOK, adminCall(m, "127.0.0.1:4711", ""))
	assert.Equal(t, http.StatusOK, adminCall(m, "[::1]:4711", ""))
	assert.Equal(t, http.StatusForbidden, adminCall(m, "10.0.0.5:4711", ""))
	assert.Equal(t, http.StatusForbidden, adminCall(m, "10.0.0.5:4711", "guess"), "a token the mock does not have")
	assert.Equal(t, http.StatusForbidden, adminCall(m, "no-address", ""))
}

func TestAdminTokenIsRequiredEverywhere(t *testing.T) {
	m := Start(nil)
	defer m.Close()
	m.AdminToken = "s3cret"
	for _, remote := range []string{"10.0.0.5:4711", "127.0.0.1:4711"} {
		assert.Equal(t, http.StatusForbidden, adminCall(m, remote, ""), remote+" without token")
		assert.Equal(t, http.StatusForbidden, adminCall(m, remote, "wrong"), remote+" wrong token")
		assert.Equal(t, http.StatusOK, adminCall(m, remote, "s3cret"), remote+" right token")
	}
	r := httptest.NewRequest(http.MethodGet, "/_mock/state", nil)
	r.RemoteAddr = "10.0.0.5:4711"
	r.Header.Set("Authorization", "s3cret") // no Bearer scheme
	w := httptest.NewRecorder()
	m.handle(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
