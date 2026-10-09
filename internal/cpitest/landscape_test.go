package cpitest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func landscapeTenant(t *testing.T, l *cpitest.Landscape, tier string) *cpitest.Tenant {
	t.Helper()
	m := cpitest.NewTenant(t, nil)
	require.NoError(t, cpitest.SeedLandscape(m, l, tier, time.Now()))
	return m
}

// The demo landscape file seeds the same content as the Go demo seed.
func TestDemoLandscapeMatchesDemoSeed(t *testing.T) {
	l, err := cpitest.BuiltinLandscape("demo")
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "test", "prod"}, l.TierNames())
	for _, tier := range l.TierNames() {
		t.Run(tier, func(t *testing.T) {
			want, got := demo(t, tier), landscapeTenant(t, l, tier)
			assert.ElementsMatch(t, want.Packages, got.Packages)
			require.Equal(t, len(want.Artifacts), len(got.Artifacts))
			for id, w := range want.Artifacts {
				g := got.Artifacts[id]
				require.NotNil(t, g, id)
				assert.Equal(t, w.DesignVersion, g.DesignVersion, id)
				assert.Equal(t, w.Package, g.Package, id)
				assert.Equal(t, w.Name, g.Name, id)
				assert.Equal(t, w.Parameters, g.Parameters, id)
				assert.Equal(t, w.ModifiedBy, g.ModifiedBy, id)
				assert.Equal(t, w.Runtime.Version, g.Runtime.Version, id)
				assert.Equal(t, w.EndpointURL != "", g.EndpointURL != "", id)
				assert.Equal(t, len(w.Resources), len(g.Resources), id)
			}
			assert.Equal(t, len(want.Credentials["UserCredentials"]), len(got.Credentials["UserCredentials"]))
			assert.Equal(t, want.Credentials["OAuth2ClientCredentials"]["Finance_OAuth"]["TokenServiceUrl"],
				got.Credentials["OAuth2ClientCredentials"]["Finance_OAuth"]["TokenServiceUrl"])
			require.Len(t, got.Keystore, 2)
			assert.WithinDuration(t, want.Keystore[0].NotAfter, got.Keystore[0].NotAfter, time.Minute)
			assert.Equal(t, want.PDStrings, got.PDStrings)
		})
	}
}

func TestLandscapeValidation(t *testing.T) {
	write := func(t *testing.T, yaml string) string {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "packages"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "landscape.yaml"), []byte(yaml), 0o644))
		return dir
	}
	for name, tc := range map[string]struct{ yaml, err string }{
		"no tiers":        {"name: x\n", "at least one tier"},
		"unknown field":   {"name: x\ntiers: [{name: dev}]\nflows: []\n", "field flows not found"},
		"unknown draft":   {"name: x\ntiers: [{name: dev, drafts: [Nope]}]\n", "unknown artifact Nope"},
		"bad tier name":   {"name: x\ntiers: [{name: DEV}]\n", "lowercase"},
		"unknown system":  {"name: x\ntiers: [{name: dev, systems: {s4: {failRate: 0.1}}}]\n", "unknown system s4"},
		"bad duration":    {"name: x\nhistory: 2 days\ntiers: [{name: dev}]\n", "duration"},
		"credential kind": {"name: x\ntiers: [{name: dev}]\ncredentials: [{name: A, kind: password}]\n", "kind basic, oauth2 or secure"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cpitest.LoadLandscapeDir(write(t, tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.err)
		})
	}
	m := cpitest.NewTenant(t, nil)
	l, err := cpitest.LoadLandscapeDir(write(t, "name: x\ntiers: [{name: dev}]\n"))
	require.NoError(t, err)
	assert.ErrorContains(t, cpitest.SeedLandscape(m, l, "prod", time.Now()), `no tier "prod" (dev)`)
}
