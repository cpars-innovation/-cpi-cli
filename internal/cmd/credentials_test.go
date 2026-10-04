package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cpars-innovation/-cpi-cli/internal/cpitest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Credentials in the global config file are bound to the persistent flags by
// initializeConfig, so every command reads them through api.GetServiceDetails
// (configure and orchestrator used to have their own viper fallback).
func TestCredentialsFromConfigFile(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": {}})
	host, port := mock.HostPort()

	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)
	cfg := fmt.Sprintf("tmn-host: http://%s:%d\ntmn-userid: user\ntmn-password: secret\n", host, port)
	require.NoError(t, os.WriteFile(filepath.Join(home, "flashpipe.yaml"), []byte(cfg), 0600))

	root := NewCLI("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"undeploy", "--artifact-ids", "A"})
	require.NoError(t, root.Execute())
	assert.Equal(t, 1, mock.Count("GET /api/v1/IntegrationRuntimeArtifacts('A')"))
}
