//go:build integration

// Tenant integration tests: these create, deploy and delete content on a real
// SAP Integration Suite tenant. Run explicitly with `go test -tags integration`.

package httpclnt

import (
	"os"
	"testing"
)

func TestOauth(t *testing.T) {
	host := os.Getenv("FLASHPIPE_TMN_HOST")
	oauthHost := os.Getenv("FLASHPIPE_OAUTH_HOST")
	oauthPath := os.Getenv("FLASHPIPE_OAUTH_PATH")
	clientId := os.Getenv("FLASHPIPE_OAUTH_CLIENTID")
	clientSecret := os.Getenv("FLASHPIPE_OAUTH_CLIENTSECRET")
	exe := New(oauthHost, oauthPath, clientId, clientSecret, "", "", host, "https", 443, true)

	headers := map[string]string{
		"Accept": "application/json",
	}
	resp, err := exe.ExecGetRequest("/api/v1/", headers)
	if err != nil {
		t.Fatalf("HTTP call failed with error - %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP call failed with response code - %v", resp.StatusCode)
	}
}

func TestBasicAuth(t *testing.T) {
	host := os.Getenv("FLASHPIPE_TMN_HOST")
	userId := os.Getenv("FLASHPIPE_TMN_USERID")
	password := os.Getenv("FLASHPIPE_TMN_PASSWORD")
	exe := New("", "", "", "", userId, password, host, "https", 443, true)

	headers := map[string]string{
		"Accept": "application/json",
	}
	resp, err := exe.ExecGetRequest("/api/v1/", headers)
	if err != nil {
		t.Fatalf("HTTP call failed with error - %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP call failed with response code - %v", resp.StatusCode)
	}
}
