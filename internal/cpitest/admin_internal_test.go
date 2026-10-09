package cpitest

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdminOnlyOnLoopbackOrTLS(t *testing.T) {
	r := httptest.NewRequest("GET", "/_mock/state", nil)
	r.RemoteAddr = "10.0.0.5:4711"
	assert.False(t, adminAllowed(r))
	r.RemoteAddr = "127.0.0.1:4711"
	assert.True(t, adminAllowed(r))
	r.RemoteAddr = "[::1]:4711"
	assert.True(t, adminAllowed(r))
	r.RemoteAddr, r.TLS = "10.0.0.5:4711", &tls.ConnectionState{}
	assert.True(t, adminAllowed(r))
}
