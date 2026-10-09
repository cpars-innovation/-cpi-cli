package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestVerifyNoSensitiveContent(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()
	viper.Set("tmn-userid", "cpi")
	viper.Set("oauth-clientid", "mock")
	viper.Set("tmn-password", "S3cret-Passw0rd")
	viper.Set("oauth-clientsecret", "")

	for _, path := range []string{".cpi/snapshot-state.json", "mock-landscapes/packages", "packages"} {
		ok, err := verifyNoSensitiveContent(path)
		assert.True(t, ok, path)
		assert.NoError(t, err, path)
	}
	ok, err := verifyNoSensitiveContent("/tmp/S3cret-Passw0rd/work")
	assert.False(t, ok)
	assert.ErrorContains(t, err, "tmn-password")

	viper.Set("tmn-password", "mock") // too short to be looked for
	ok, _ = verifyNoSensitiveContent("mock-landscapes")
	assert.True(t, ok)
}
