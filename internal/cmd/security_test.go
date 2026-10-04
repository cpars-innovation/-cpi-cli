package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "Pa55-w0rd-NEVER-LOGGED"

func TestCredentialsCommandsNeverLeakSecrets(t *testing.T) {
	t.Setenv("ERP_PASSWORD", testSecret)
	mock := cpitest.NewTenant(t, nil)

	// --debug on purpose: request bodies must not be logged
	res := runMain(t, append([]string{"credentials", "set-user", "--name", "ERP", "--user", "svc", "--password-env", "ERP_PASSWORD",
		"--debug", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"action": "CREATED"`)
	assert.Equal(t, testSecret, mock.Credentials["UserCredentials"]["ERP"]["Password"])
	assert.NotContains(t, res.stdout+res.stderr, testSecret)

	res = runMain(t, append([]string{"credentials", "list", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"user": "svc"`)
	assert.NotContains(t, res.stdout, testSecret)

	// no secret source -> usage error, nothing written
	before := len(mock.Requests())
	res = runMain(t, append([]string{"credentials", "set-user", "--name", "X", "--user", "u", "--output", "json"}, basicAuth(mock)...)...)
	assert.Equal(t, 2, res.code)
	assert.Equal(t, before, len(mock.Requests()))

	res = runMain(t, append([]string{"credentials", "delete", "--kind", "user", "--name", "ERP"}, basicAuth(mock)...)...)
	assert.Equal(t, 2, res.code, "delete needs --confirm")
	res = runMain(t, append([]string{"credentials", "delete", "--kind", "user", "--name", "ERP", "--confirm"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Empty(t, mock.Credentials["UserCredentials"])
}

func TestCredentialsApplyCommand(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "graph.secret"), []byte(testSecret+"\n"), 0600))
	file := filepath.Join(dir, "creds.yml")
	require.NoError(t, os.WriteFile(file, []byte(`oauth2Credentials:
  - name: Graph
    tokenServiceUrl: https://login/token
    clientId: app
    clientSecret: {file: graph.secret}
`), 0600))
	mock := cpitest.NewTenant(t, nil)

	res := runMain(t, append([]string{"credentials", "apply", "--file", file, "--dry-run", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "WOULD_DEPLOY")
	assert.Empty(t, mock.Credentials["OAuth2ClientCredentials"])

	res = runMain(t, append([]string{"credentials", "apply", "--file", file, "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, testSecret, mock.Credentials["OAuth2ClientCredentials"]["Graph"]["ClientSecret"])
	assert.NotContains(t, res.stdout+res.stderr, testSecret)
}

func TestKeystoreCommands(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	mock.Keystore = []cpitest.KeystoreEntry{{Alias: "partner", NotAfter: time.Now().Add(5 * 24 * time.Hour)}}

	res := runMain(t, append([]string{"keystore", "list", "--expiring-within", "30d", "--output", "json"}, basicAuth(mock)...)...)
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, `"expiringSoon": 1`)

	res = runMain(t, append([]string{"keystore", "list", "--expiring-within", "30d", "--fail-on-expiry"}, basicAuth(mock)...)...)
	assert.Equal(t, 5, res.code)

	res = runMain(t, append([]string{"keystore", "list", "--expiring-within", "soon"}, basicAuth(mock)...)...)
	assert.Equal(t, 2, res.code)
}
