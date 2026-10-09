package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockTenantCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ca := filepath.Join(t.TempDir(), "ca.pem")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"mock-tenant", "--addr", "127.0.0.1:0", "--tier", "prod", "--tls", "--ca-out", ca}, &stdout, &stderr, "test", "test")
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "CPICTL_TMN_HOST=https://127.0.0.1:")
	assert.Contains(t, stdout.String(), "SSL_CERT_FILE="+ca)
	pemData, err := os.ReadFile(ca)
	require.NoError(t, err)
	assert.Contains(t, string(pemData), "BEGIN CERTIFICATE")

	assert.Equal(t, 2, runMain(t, "mock-tenant", "--tls").code, "--tls needs --ca-out")
	assert.Equal(t, 2, runMain(t, "mock-tenant", "--seed", "full").code)
	assert.Equal(t, 2, runMain(t, "mock-tenant", "--addr", "127.0.0.1:0", "--tier", "qa").code)
}

func TestMockTenantSeedDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "packages"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "landscape.yaml"), []byte("name: customer-b\ntiers: [{name: qa}, {name: prod-eu}]\n"), 0o644))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"mock-tenant", "--addr", "127.0.0.1:0", "--seed-dir", dir, "--tier", "prod-eu"}, &stdout, &stderr, "test", "test")
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "Mock CPI tenant (prod-eu, seed customer-b)")

	assert.Equal(t, 2, runMain(t, "mock-tenant", "--addr", "127.0.0.1:0", "--seed-dir", dir, "--tier", "dev").code, "not a tier of the landscape")
	assert.Equal(t, 2, runMain(t, "mock-tenant", "--addr", "127.0.0.1:0", "--seed-dir", t.TempDir()).code, "no landscape.yaml")
}
