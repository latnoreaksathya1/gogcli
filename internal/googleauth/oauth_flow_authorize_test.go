package googleauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"golang.org/x/oauth2"
)

func newTokenServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			http.Error(w, "bad grant_type", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code") == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at",
			"refresh_token": "rt",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
}

func TestAuthorize_MissingScopes(t *testing.T) {
	_, err := Authorize(context.Background(), AuthorizeOptions{})
	if err == nil || !strings.Contains(err.Error(), "missing scopes") {
		t.Fatalf("expected missing scopes error, got: %v", err)
	}
}

func TestAuthorize_Manual_Success(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn
	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})

	readClientCredentials = func() (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	origStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = origStdin })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	_, _ = w.WriteString("http://localhost:1/?code=abc&state=state123\n")
	_ = w.Close()

	rt, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Manual:  true,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if rt != "rt" {
		t.Fatalf("unexpected refresh token: %q", rt)
	}
}

func TestAuthorize_Manual_StateMismatch(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn
	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})

	readClientCredentials = func() (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	origStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = origStdin })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	_, _ = w.WriteString("http://localhost:1/?code=abc&state=DIFFERENT\n")
	_ = w.Close()

	_, err = Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Manual:  true,
		Timeout: 2 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got: %v", err)
	}
}

func TestAuthorize_ServerFlow_Success(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origOpen := openBrowserFn
	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		openBrowserFn = origOpen
	})

	readClientCredentials = func() (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	openBrowserFn = func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := u.Query()
		redirect := q.Get("redirect_uri")
		state := q.Get("state")
		if redirect == "" || state == "" {
			return errors.New("missing redirect/state")
		}
		cb := redirect + "?code=abc&state=" + url.QueryEscape(state)
		resp, err := http.Get(cb)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}

	rt, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if rt != "rt" {
		t.Fatalf("unexpected refresh token: %q", rt)
	}
}

func TestAuthorize_ServerFlow_CallbackErrors(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantText string
	}{
		{name: "missing_code", query: "state=%s", wantText: "missing code"},
		{name: "state_mismatch", query: "code=abc&state=WRONG", wantText: "state mismatch"},
		{name: "oauth_error", query: "error=access_denied&state=%s", wantText: "authorization error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origRead := readClientCredentials
			origEndpoint := oauthEndpoint
			origOpen := openBrowserFn
			t.Cleanup(func() {
				readClientCredentials = origRead
				oauthEndpoint = origEndpoint
				openBrowserFn = origOpen
			})

			readClientCredentials = func() (config.ClientCredentials, error) {
				return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
			}

			tokenSrv := newTokenServer(t)
			defer tokenSrv.Close()
			oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

			openBrowserFn = func(authURL string) error {
				u, err := url.Parse(authURL)
				if err != nil {
					return err
				}
				q := u.Query()
				redirect := q.Get("redirect_uri")
				state := q.Get("state")
				if redirect == "" || state == "" {
					return errors.New("missing redirect/state")
				}
				query := tt.query
				if strings.Contains(query, "%s") {
					query = fmtSprintf(query, url.QueryEscape(state))
				}
				cb := redirect + "?" + query
				resp, err := http.Get(cb)
				if err != nil {
					return err
				}
				_ = resp.Body.Close()
				return nil
			}

			_, err := Authorize(context.Background(), AuthorizeOptions{
				Scopes:  []string{"s1"},
				Timeout: 2 * time.Second,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("expected %q error, got: %v", tt.wantText, err)
			}
		})
	}
}

// oauth2.Endpoint is a plain struct; keep construction centralized.
func oauth2EndpointForTest(base string) oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  base + "/auth",
		TokenURL: base + "/token",
	}
}

// Minimal sprintf to avoid importing fmt just for one small helper in tests.
func fmtSprintf(format string, v string) string {
	return strings.ReplaceAll(format, "%s", v)
}
