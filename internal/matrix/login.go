package matrix

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// Session is the result of a login. The access token is secret: mtx shows it once and does not store it.
type Session struct {
	Homeserver  string
	UserID      string
	DeviceID    string
	AccessToken string
}

// NewPickleKey returns 32 random bytes, base64. It encrypts the store's E2EE keys.
func NewPickleKey() (string, error) {
	b, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// Authenticate runs OIDC (MAS, MSC2965) with PKCE and a loopback redirect. show gets the
// authorization URL; the caller opens it or shows it. The device ID is random and is requested
// through the device scope; whoami must echo it back.
func Authenticate(ctx context.Context, hs string, show func(authURL string)) (Session, error) {
	hc := &http.Client{Timeout: 30 * time.Second}
	base := strings.TrimSuffix(hs, "/")
	var meta struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		RegistrationEndpoint  string `json:"registration_endpoint"`
	}
	if err := getJSON(ctx, hc, base+"/_matrix/client/v1/auth_metadata", &meta); err != nil {
		return Session{}, fmt.Errorf("auth metadata: %w (homeserver may not support SSO)", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Session{}, fmt.Errorf("callback listener: %w", err)
	}
	redirect := fmt.Sprintf("http://%s/callback", ln.Addr())
	clientID, err := register(ctx, hc, meta.RegistrationEndpoint, redirect)
	if err != nil {
		_ = ln.Close()
		return Session{}, err
	}
	verifier, err := randomString(32)
	if err != nil {
		_ = ln.Close()
		return Session{}, err
	}
	state, err := randomString(16)
	if err != nil {
		_ = ln.Close()
		return Session{}, err
	}
	device, err := randomDeviceID()
	if err != nil {
		_ = ln.Close()
		return Session{}, err
	}
	got := make(chan callback, 1)
	srv := &http.Server{Handler: callbackHandler(state, got), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	scope := "openid urn:matrix:client:api:* urn:matrix:client:device:" + device
	auth := meta.AuthorizationEndpoint + "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"scope":                 {scope},
		"state":                 {state},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode()
	show(auth)

	var cb callback
	select {
	case cb = <-got:
	case <-ctx.Done():
		return Session{}, ctx.Err()
	}
	if cb.err != nil {
		return Session{}, cb.err
	}
	tok, err := exchange(ctx, hc, meta.TokenEndpoint, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {cb.code},
		"redirect_uri":  {redirect},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
	if err != nil {
		return Session{}, err
	}
	cli, err := mautrix.NewClient(hs, "", tok)
	if err != nil {
		return Session{}, fmt.Errorf("client: %w", err)
	}
	who, err := cli.Whoami(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("whoami: %w", err)
	}
	if who.DeviceID != id.DeviceID(device) {
		return Session{}, fmt.Errorf("server gave device %q, asked for %q", who.DeviceID, device)
	}
	return Session{Homeserver: hs, UserID: string(who.UserID), DeviceID: device, AccessToken: tok}, nil
}

type callback struct {
	code string
	err  error
}

// callbackHandler takes the one redirect. A wrong state or an error from the server is a failed login.
func callbackHandler(state string, got chan<- callback) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var cb callback
		switch {
		case q.Get("state") != state:
			cb.err = errors.New("login: state mismatch, try again")
		case q.Get("error") != "":
			cb.err = fmt.Errorf("login refused: %s", q.Get("error"))
		default:
			cb.code = q.Get("code")
		}
		select {
		case got <- cb:
		default: // a second redirect is ignored
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if cb.err != nil {
			http.Error(w, "mtx: login failed. You can close this tab.", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "mtx: logged in. You can close this tab.\n")
	})
	return mux
}

func register(ctx context.Context, hc *http.Client, endpoint, redirect string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"client_name":                "mtx",
		"client_uri":                 "https://github.com/vic/mtx",
		"application_type":           "native",
		"redirect_uris":              []string{redirect},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := do(hc, req, &out); err != nil {
		return "", fmt.Errorf("register client: %w", err)
	}
	if out.ClientID == "" {
		return "", errors.New("register client: no client_id")
	}
	return out.ClientID, nil
}

func exchange(ctx context.Context, hc *http.Client, endpoint string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := do(hc, req, &out); err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("token exchange: no access_token")
	}
	return out.AccessToken, nil
}

func getJSON(ctx context.Context, hc *http.Client, endpoint string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return do(hc, req, v)
}

func do(hc *http.Client, req *http.Request, v any) error {
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// pkceChallenge is the S256 code challenge for verifier (RFC 7636).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randomDeviceID is "mtx-" and 8 random upper-case letters or digits.
func randomDeviceID() (string, error) {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b, err := randomBytes(8)
	if err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alpha[int(b[i])%len(alpha)]
	}
	return "mtx-" + string(b), nil
}

func randomString(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("random: %w", err)
	}
	return b, nil
}
