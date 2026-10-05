package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/deploy"
	"github.com/cpars-innovation/cpicli/internal/repo"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const examplesDir = "../../docs/examples"

// TestDocsExamples loads every file in docs/examples with the same code the
// commands use, so the examples cannot silently go stale.
func TestDocsExamples(t *testing.T) {
	t.Run("cpictl.yaml", func(t *testing.T) {
		v := viper.New()
		v.SetConfigFile(filepath.Join(examplesDir, "cpictl.yaml"))
		require.NoError(t, v.ReadInConfig())
		assert.NotEmpty(t, v.GetString("tmn-host"))
		assert.NotContains(t, v.AllSettings(), "oauth-clientsecret", "secrets belong in the environment")
		for _, key := range []string{"deploy.delayLength", "undeploy.maxCheckLimit", "configure.configPath",
			"configure.pull.outputDir", "orchestrator.deployConfig", "pd-deploy.resources-path", "pd-snapshot.replace"} {
			assert.True(t, v.IsSet(key), key)
		}
		assert.True(t, cpi.IsValidArtifactType(v.GetString("deploy.artifactType")))
	})

	t.Run("deploy-config.yml", func(t *testing.T) {
		loader := deploy.NewConfigLoader()
		require.NoError(t, loader.DetectSource(filepath.Join(examplesDir, "deploy-config.yml")))
		files, err := loader.LoadConfigs()
		require.NoError(t, err)
		cfg := files[0].Config
		require.Len(t, cfg.Packages, 2)
		assert.True(t, cfg.Packages[1].Sync, "omitted sync defaults to true")
		assert.False(t, cfg.Packages[1].Deploy)
		for _, p := range cfg.Packages {
			for _, a := range p.Artifacts {
				assert.True(t, cpi.IsValidArtifactType(mapArtifactTypeForSync(a.Type)), a.Type)
			}
		}
	})

	t.Run("configure.yml", func(t *testing.T) {
		files, err := ops.LoadConfigureFiles(filepath.Join(examplesDir, "configure.yml"))
		require.NoError(t, err)
		pkg := files[0].Config.Packages[0]
		require.Len(t, pkg.Artifacts, 2)
		for _, a := range pkg.Artifacts {
			assert.True(t, cpi.IsValidArtifactType(a.Type), a.Type)
			assert.Equal(t, "active", a.Version)
			assert.NotEmpty(t, a.Parameters)
		}
		assert.False(t, pkg.Artifacts[1].Batch.Enabled)
	})

	for _, name := range []string{"mcp.json", "claude-code.mcp.json"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(examplesDir, name))
			require.NoError(t, err)
			var cfg struct {
				MCPServers map[string]struct {
					Command string            `json:"command"`
					Args    []string          `json:"args"`
					Env     map[string]string `json:"env"`
				} `json:"mcpServers"`
			}
			require.NoError(t, json.Unmarshal(data, &cfg))
			require.Contains(t, cfg.MCPServers, "cpi-dev")
			mcpFlags := NewMCPCommand("test").Flags()
			for id, server := range cfg.MCPServers {
				require.NotEmpty(t, server.Args, id)
				assert.Equal(t, "mcp", server.Args[0], id)
				for _, arg := range server.Args[1:] {
					if flag, ok := strings.CutPrefix(arg, "--"); ok {
						assert.NotNil(t, mcpFlags.Lookup(flag), "%s: unknown flag %s", id, arg)
					}
				}
				assert.Contains(t, server.Env, "CPICTL_TMN_HOST", id)
				for k, v := range server.Env {
					assert.True(t, strings.HasPrefix(k, "CPICTL_"), "%s: %s", id, k)
					if name == "claude-code.mcp.json" {
						// a committed project file must not contain values, only references
						assert.Regexp(t, `^\$\{[A-Z0-9_]+\}$`, v, "%s: %s", id, k)
					}
				}
			}
		})
	}

	t.Run("partner-directory", func(t *testing.T) {
		pd := repo.NewPartnerDirectory(filepath.Join(examplesDir, "partner-directory"))
		pids, err := pd.GetLocalPIDs()
		require.NoError(t, err)
		assert.Equal(t, []string{"SAP_SYSTEM_001"}, pids)
		strs, err := pd.ReadStringParameters("SAP_SYSTEM_001")
		require.NoError(t, err)
		assert.Len(t, strs, 2)
		bins, err := pd.ReadBinaryParameters("SAP_SYSTEM_001")
		require.NoError(t, err)
		require.Len(t, bins, 1)
		assert.Equal(t, "Identity", bins[0].ID)
		assert.Equal(t, "xsl", bins[0].ContentType)
	})
}
