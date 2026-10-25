package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// RFC 7636 appendix B test vector.
func TestPKCEChallengeRFC7636(t *testing.T) {
	require.Equal(t, "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		pkceChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"))
}

func TestNewPickleKeyRandom(t *testing.T) {
	a, err := NewPickleKey()
	require.NoError(t, err)
	b, err := NewPickleKey()
	require.NoError(t, err)
	require.Len(t, a, 44) // 32 bytes, std base64
	require.NotEqual(t, a, b)
}

func TestRandomDeviceID(t *testing.T) {
	d, err := randomDeviceID()
	require.NoError(t, err)
	require.Len(t, d, 12)
	require.True(t, strings.HasPrefix(d, "mtx-"))
	require.Equal(t, strings.ToUpper(d[4:]), d[4:])
}

// fakeMAS serves OIDC metadata, client registration, token exchange and whoami. It checks PKCE
// and echoes the device from the requested scope, like MAS does.
type fakeMAS struct {
	mu        sync.Mutex
	challenge string
	device    string
}

func (f *fakeMAS) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/_matrix/client/v1/auth_metadata", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		fmt.Fprintf(w, `{"authorization_endpoint":%q,"token_endpoint":%q,"registration_endpoint":%q}`,
			base+"/authorize", base+"/token", base+"/register")
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "none", body["token_endpoint_auth_method"])
		_, _ = io.WriteString(w, `{"client_id":"CID"}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		f.mu.Lock()
		defer f.mu.Unlock()
		if pkceChallenge(r.PostForm.Get("code_verifier")) != f.challenge {
			http.Error(w, `{"errcode":"M_FORBIDDEN"}`, http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"tok-1"}`)
	})
	mux.HandleFunc("/_matrix/client/v3/account/whoami", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer tok-1", r.Header.Get("Authorization"))
		f.mu.Lock()
		defer f.mu.Unlock()
		fmt.Fprintf(w, `{"user_id":"@alice:h.test","device_id":%q}`, f.device)
	})
	return mux
}

// browser stands in for the user: it reads the authorization URL and follows the redirect.
func browser(t *testing.T, f *fakeMAS, state string) func(string) {
	return func(authURL string) {
		u, err := url.Parse(authURL)
		require.NoError(t, err)
		q := u.Query()
		f.mu.Lock()
		f.challenge = q.Get("code_challenge")
		for _, s := range strings.Fields(q.Get("scope")) {
			if d, ok := strings.CutPrefix(s, "urn:matrix:client:device:"); ok {
				f.device = d
			}
		}
		f.mu.Unlock()
		require.Equal(t, "S256", q.Get("code_challenge_method"))
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?code=C1&state=" + url.QueryEscape(state))
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}
}

func TestAuthenticateOIDC(t *testing.T) {
	f := &fakeMAS{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	var state string
	show := func(authURL string) {
		u, _ := url.Parse(authURL)
		state = u.Query().Get("state")
		browser(t, f, state)(authURL)
	}
	s, err := Authenticate(context.Background(), srv.URL, show)
	require.NoError(t, err)
	require.Equal(t, Session{Homeserver: srv.URL, UserID: "@alice:h.test", DeviceID: f.device, AccessToken: "tok-1"}, s)
	require.Len(t, s.DeviceID, 12)
}

func TestAuthenticateStateMismatch(t *testing.T) {
	f := &fakeMAS{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	show := func(authURL string) {
		u, _ := url.Parse(authURL)
		f.mu.Lock()
		f.challenge = u.Query().Get("code_challenge")
		f.mu.Unlock()
		go func() {
			resp, err := http.Get(u.Query().Get("redirect_uri") + "?code=C1&state=forged")
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}
	_, err := Authenticate(context.Background(), srv.URL, show)
	require.ErrorContains(t, err, "state mismatch")
}

func TestAuthenticateNoSSO(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, err := Authenticate(context.Background(), srv.URL, func(string) { t.Fatal("no browser without SSO") })
	require.ErrorContains(t, err, "homeserver may not support SSO")
}
