package ops

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "s3cr3t-Value!"

func TestSecretSource(t *testing.T) {
	t.Setenv("CPICTL_TEST_SECRET", secret)
	file := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(file, []byte(secret+"\n"), 0600))

	for name, src := range map[string]SecretSource{
		"env":   {Env: "CPICTL_TEST_SECRET"},
		"file":  {File: file},
		"stdin": {Stdin: strings.NewReader(secret + "\r\n")},
	} {
		v, err := src.Resolve("x")
		require.NoError(t, err, name)
		assert.Equal(t, secret, v, name)
	}
	for name, src := range map[string]SecretSource{
		"none":        {},
		"two":         {Env: "CPICTL_TEST_SECRET", File: file},
		"missing env": {Env: "CPICTL_TEST_UNSET"},
		"empty":       {Stdin: strings.NewReader("\n")},
	} {
		_, err := src.Resolve("x")
		assert.Equal(t, exitcode.Usage, output.ExitCode(err), name)
	}
}

func TestDeployUserCredential(t *testing.T) {
	t.Setenv("CPICTL_TEST_SECRET", secret)
	mock := cpitest.NewTenant(t, nil)
	spec := UserCredentialSpec{Name: "ERP User", User: "svc_erp", Password: SecretSource{Env: "CPICTL_TEST_SECRET"}}

	res, err := DeployCredential(mock.Executer(), spec, false)
	require.NoError(t, err)
	assert.Equal(t, "CREATED", res.Action)
	stored := mock.Credentials["UserCredentials"]["ERP User"]
	assert.Equal(t, secret, stored["Password"])
	assert.Equal(t, "default", stored["Kind"])

	res, err = DeployCredential(mock.Executer(), spec, false)
	require.NoError(t, err)
	assert.Equal(t, "UPDATED", res.Action)
	assert.Equal(t, 1, mock.Count("PUT /api/v1/UserCredentials('ERP User')"), "names with spaces are escaped in the path")

	out, _ := json.Marshal(res)
	assert.NotContains(t, string(out), secret)

	list, err := ListCredentials(mock.Executer(), "")
	require.NoError(t, err)
	out, _ = json.Marshal(list)
	assert.Contains(t, string(out), "svc_erp")
	assert.NotContains(t, string(out), secret, "secrets are never part of results")

	// dry run resolves the secret but writes nothing
	before := len(mock.Requests())
	res, err = DeployCredential(mock.Executer(), spec, true)
	require.NoError(t, err)
	assert.Equal(t, "WOULD_DEPLOY", res.Action)
	assert.Equal(t, before, len(mock.Requests()))
}

func TestDeployOAuth2CredentialValidation(t *testing.T) {
	t.Setenv("CPICTL_TEST_SECRET", secret)
	mock := cpitest.NewTenant(t, nil)
	_, err := DeployCredential(mock.Executer(), OAuth2CredentialSpec{Name: "G", TokenServiceURL: "http://insecure", ClientID: "c",
		ClientSecret: SecretSource{Env: "CPICTL_TEST_SECRET"}}, false)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
	_, err = DeployCredential(mock.Executer(), OAuth2CredentialSpec{Name: "G", TokenServiceURL: "https://idp/token", ClientID: "c",
		ClientAuthentication: "query", ClientSecret: SecretSource{Env: "CPICTL_TEST_SECRET"}}, false)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
	assert.Empty(t, mock.Requests())

	res, err := DeployCredential(mock.Executer(), OAuth2CredentialSpec{Name: "G", TokenServiceURL: "https://idp/token", ClientID: "c",
		ClientSecret: SecretSource{Env: "CPICTL_TEST_SECRET"}}, false)
	require.NoError(t, err)
	assert.Equal(t, "CREATED", res.Action)
	assert.Equal(t, "body", mock.Credentials["OAuth2ClientCredentials"]["G"]["ClientAuthentication"])
}

func writeCredentialsFile(t *testing.T, content string) string {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "erp.secret"), []byte("from-file\n"), 0600))
	p := filepath.Join(dir, "credentials.yml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0600))
	return p
}

func TestApplyCredentials(t *testing.T) {
	t.Setenv("CPICTL_TEST_SECRET", secret)
	file := writeCredentialsFile(t, `
userCredentials:
  - name: ERP
    user: svc
    password: {file: erp.secret}
oauth2Credentials:
  - name: Graph
    tokenServiceUrl: https://login/token
    clientId: app
    clientSecret: {env: CPICTL_TEST_SECRET}
secureParameters:
  - name: ApiKey
    value: {env: CPICTL_TEST_SECRET}
`)
	f, err := LoadCredentialsFile(file)
	require.NoError(t, err)

	t.Run("all ok", func(t *testing.T) {
		mock := cpitest.NewTenant(t, nil)
		res, err := ApplyCredentials(mock.Executer(), f, false)
		require.NoError(t, err)
		require.Len(t, res.Results, 3)
		assert.Equal(t, "from-file", mock.Credentials["UserCredentials"]["ERP"]["Password"], "relative secret files resolve next to the credentials file")
		assert.Equal(t, secret, mock.Credentials["SecureParameters"]["ApiKey"]["SecureParam"])
	})

	t.Run("partial failure", func(t *testing.T) {
		mock := cpitest.NewTenant(t, nil)
		mock.SecureParametersUnavailable = true // Cloud Foundry tenant
		res, err := ApplyCredentials(mock.Executer(), f, false)
		assert.Equal(t, exitcode.Partial, output.ExitCode(err))
		assert.Equal(t, "FAILED", res.Results[2].Action)
	})

	t.Run("missing secret writes nothing", func(t *testing.T) {
		missing := writeCredentialsFile(t, "userCredentials:\n  - {name: A, user: u, password: {env: CPICTL_TEST_SECRET}}\n  - {name: B, user: u, password: {env: CPICTL_TEST_UNSET}}\n")
		f, err := LoadCredentialsFile(missing)
		require.NoError(t, err)
		mock := cpitest.NewTenant(t, nil)
		_, err = ApplyCredentials(mock.Executer(), f, false)
		assert.Equal(t, exitcode.Usage, output.ExitCode(err))
		assert.Empty(t, mock.Requests())
	})

	t.Run("inline secrets are rejected", func(t *testing.T) {
		inline := writeCredentialsFile(t, "userCredentials:\n  - name: A\n    user: u\n    password: plaintext\n")
		_, err := LoadCredentialsFile(inline)
		assert.Equal(t, exitcode.Usage, output.ExitCode(err))
		inline = writeCredentialsFile(t, "userCredentials:\n  - name: A\n    user: u\n    passwordValue: plaintext\n")
		_, err = LoadCredentialsFile(inline)
		assert.Equal(t, exitcode.Usage, output.ExitCode(err))
	})
}

func TestDeleteCredentialAndSecureParamWarning(t *testing.T) {
	mock := cpitest.NewTenant(t, nil)
	mock.Credentials = map[string]map[string]map[string]any{"UserCredentials": {"A": {"Name": "A", "User": "u", "Password": "p"}}}
	mock.SecureParametersUnavailable = true

	list, err := ListCredentials(mock.Executer(), "")
	require.NoError(t, err)
	assert.Len(t, list.UserCredentials, 1)
	assert.Len(t, list.Warnings, 1, "secure parameters are Neo-only")

	_, err = DeleteCredential(mock.Executer(), cpi.KindUser, "missing")
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
	res, err := DeleteCredential(mock.Executer(), cpi.KindUser, "A")
	require.NoError(t, err)
	assert.Equal(t, "DELETED", res.Action)
	assert.Empty(t, mock.Credentials["UserCredentials"])
}

func testCertificate(t *testing.T, cn string, notAfter time.Time) []byte {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return der
}

func TestKeystoreAndCertificates(t *testing.T) {
	now := time.Now()
	soon := testCertificate(t, "soon", now.Add(10*24*time.Hour))
	mock := cpitest.NewTenant(t, nil)
	mock.Keystore = []cpitest.KeystoreEntry{
		{Alias: "partner_soon", NotAfter: now.Add(10 * 24 * time.Hour), DER: soon},
		{Alias: "old", NotAfter: now.Add(-24 * time.Hour)},
		{Alias: "fine", NotAfter: now.Add(400 * 24 * time.Hour)},
	}

	report, err := ListKeystore(mock.Executer(), "", 30*24*time.Hour, false, now)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Expired)
	assert.Equal(t, 1, report.ExpiringSoon)
	assert.Equal(t, "old", report.Entries[0].Alias, "sorted by expiry")
	assert.Equal(t, 9, report.Entries[1].DaysLeft)

	_, err = ListKeystore(mock.Executer(), "", 30*24*time.Hour, true, now)
	assert.Equal(t, exitcode.DeployFailed, output.ExitCode(err))
	_, err = ListKeystore(mock.Executer(), "nope", 0, false, now)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))

	info, err := ExportCertificate(mock.Executer(), "partner_soon", "")
	require.NoError(t, err)
	assert.Equal(t, "CN=soon", info.Subject)
	assert.True(t, strings.HasPrefix(info.PEM, "-----BEGIN CERTIFICATE-----"))
	assert.Equal(t, 1, mock.Count("GET /api/v1/KeystoreEntries('"+cpi.HexAlias("partner_soon")+"')/Certificate/$value"))

	// import: PEM accepted, existing alias needs update, keys rejected
	newCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: testCertificate(t, "new", now.Add(365*24*time.Hour))})
	imported, err := ImportCertificate(mock.Executer(), "partner_new", newCert, false)
	require.NoError(t, err)
	assert.Equal(t, "CN=new", imported.Subject)
	_, err = ImportCertificate(mock.Executer(), "partner_new", newCert, false)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
	_, err = ImportCertificate(mock.Executer(), "partner_new", newCert, true)
	require.NoError(t, err)
	_, err = ImportCertificate(mock.Executer(), "k", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")}), true)
	assert.Equal(t, exitcode.Usage, output.ExitCode(err))
}

func TestHexAlias(t *testing.T) {
	assert.Equal(t, "313233", cpi.HexAlias("123"))
	assert.Equal(t, "69645F727361", cpi.HexAlias("id_rsa"))
}
