package httpclnt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2/clientcredentials"
)

type HTTPExecuter struct {
	basicUserId   string
	basicPassword string
	host          string
	scheme        string
	port          int
	httpClient    *http.Client
	AuthType      string
	showLogs      bool
	csrf          csrf
}

// New returns an initialised HTTPExecuter instance.
func New(oauthHost string, oauthPath string, clientId string, clientSecret string, userId string, password string, host string, scheme string, port int, showLogs bool) *HTTPExecuter {
	e := new(HTTPExecuter)
	e.host = host
	e.scheme = scheme
	e.port = port
	e.showLogs = showLogs
	if oauthHost != "" {
		if showLogs {
			log.Debug().Msg("Initialising HTTP client with OAuth 2.0")
		}

		tokenURL := fmt.Sprintf("%v://%v:%d%v", scheme, oauthHost, port, oauthPath)
		if showLogs {
			log.Debug().Msgf("Setting up OAuth 2.0 client with token URL %v", tokenURL)
		}

		// Reference https://pkg.go.dev/golang.org/x/oauth2/clientcredentials#pkg-overview
		conf := &clientcredentials.Config{
			ClientID:     clientId,
			ClientSecret: clientSecret,
			TokenURL:     tokenURL,
		}

		ctx := context.Background()
		e.httpClient = conf.Client(ctx)
		e.AuthType = "OAUTH"
	} else {
		if showLogs {
			log.Debug().Msg("Initialising HTTP client with Basic Authentication")
		}
		e.httpClient = &http.Client{Timeout: 30 * time.Second}
		e.basicUserId = userId
		e.basicPassword = password
		e.AuthType = "BASIC"
	}
	return e
}

// Exec sends a request to the tenant. Modifying requests (POST, PUT, PATCH,
// DELETE) get a CSRF token automatically, see csrf.
func (e *HTTPExecuter) Exec(method string, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	var payload []byte
	if body != nil && body != http.NoBody {
		var err error
		if payload, err = io.ReadAll(body); err != nil {
			return nil, err
		}
	}
	if !isModifying(method) {
		return e.send(method, path, payload, headers, nil)
	}

	token, cookies, generation, err := e.csrfCurrent(e.AuthType == "BASIC")
	if err != nil {
		return nil, err
	}
	resp, err := e.send(method, path, payload, withCSRF(headers, token), cookies)
	if err != nil || !csrfRequired(resp) {
		return resp, err
	}

	// Token missing or expired: fetch a new one and retry once
	_, _ = e.ReadRespBody(resp)
	if e.showLogs {
		log.Debug().Msgf("CSRF token required for %v %v, fetching a new token", method, path)
	}
	token, cookies, err = e.csrfRefresh(generation)
	if err != nil {
		return nil, err
	}
	return e.send(method, path, payload, withCSRF(headers, token), cookies)
}

func withCSRF(headers map[string]string, token string) map[string]string {
	if token == "" {
		return headers
	}
	h := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		h[k] = v
	}
	h["X-CSRF-Token"] = token
	return h
}

func (e *HTTPExecuter) send(method string, path string, payload []byte, headers map[string]string, cookies []*http.Cookie) (*http.Response, error) {
	url := fmt.Sprintf("%v://%v:%d%v", e.scheme, e.host, e.port, path)
	if e.showLogs {
		log.Debug().Msgf("Executing HTTP request: %v %v", method, url)
	}

	var body io.Reader = http.NoBody
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if e.basicUserId != "" {
		req.SetBasicAuth(e.basicUserId, e.basicPassword)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return e.httpClient.Do(req)
}

func (e *HTTPExecuter) ExecGetRequest(path string, headers map[string]string) (resp *http.Response, err error) {
	return e.Exec(http.MethodGet, path, http.NoBody, headers)
}

func (e *HTTPExecuter) ReadRespBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// HTTPError is returned for tenant responses with an unexpected status code.
// Its message is kept identical to the historic plain error string because
// some callers still match on it.
type HTTPError struct {
	CallType   string
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%v call failed with response code = %d", e.CallType, e.StatusCode)
}

// StatusCode returns the HTTP status code carried by err (or any error it
// wraps), or 0 if err is not an HTTPError.
func StatusCode(err error) int {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}
	return 0
}

// IsAuthError reports whether err is an HTTP 401/403 from the tenant.
func IsAuthError(err error) bool {
	code := StatusCode(err)
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

func (e *HTTPExecuter) LogError(resp *http.Response, callType string) (resBody []byte, err error) {
	resBody, err = e.ReadRespBody(resp)
	if err != nil {
		return
	}

	if len(resBody) != 0 && e.showLogs {
		log.Warn().Msgf("Response body = %s", resBody)
	}

	return resBody, &HTTPError{CallType: callType, StatusCode: resp.StatusCode, Body: resBody}
}
