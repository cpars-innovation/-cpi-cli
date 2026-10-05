package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
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
	require.NoError(t, os.WriteFile(filepath.Join(home, "cpictl.yaml"), []byte(cfg), 0600))

	root := NewCLI("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"undeploy", "--artifact-ids", "A"})
	require.NoError(t, root.Execute())
	assert.Equal(t, 1, mock.Count("GET /api/v1/IntegrationRuntimeArtifacts('A')"))
}

func TestDeployArtifactIDsFromConfigFile(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{
		"A": {Type: "Integration", DesignVersion: "1", Runtime: &cpitest.Runtime{Version: "1", Status: "STARTED"}},
	})
	host, port := mock.HostPort()
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)
	cfg := fmt.Sprintf("tmn-host: http://%s:%d\ntmn-userid: user\ntmn-password: secret\ndeploy:\n  artifactIds: [A]\n", host, port)
	require.NoError(t, os.WriteFile(filepath.Join(home, "cpictl.yaml"), []byte(cfg), 0600))

	root := NewCLI("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"deploy"})
	require.NoError(t, root.Execute())
	assert.Equal(t, 1, mock.Count("GET /api/v1/IntegrationDesigntimeArtifacts(Id='A',Version='active')"))
}

// An extension-less $HOME/cpictl (typically the binary itself) is not a config file.
func TestHomeBinaryIsNotReadAsConfig(t *testing.T) {
	mock := cpitest.NewTenant(t, map[string]*cpitest.Artifact{"A": {}})
	host, port := mock.HostPort()
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)
	require.NoError(t, os.WriteFile(filepath.Join(home, "cpictl"), []byte("\x7fELF\x02\x01\x01\x00"), 0700))

	root := NewCLI("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"undeploy", "--artifact-ids", "A",
		"--tmn-host", fmt.Sprintf("http://%s:%d", host, port), "--tmn-userid", "user", "--tmn-password", "secret"})
	require.NoError(t, root.Execute())
}
