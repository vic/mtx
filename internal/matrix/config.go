// Package matrix is the Matrix backend: env config, the SQLite store, and the sync loop.
package matrix

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Required is the one env name every start needs: the pickle key. The rest is read from the store
// when not in env. MTX_DB is optional.
var Required = []string{"MTX_PICKLE_KEY"}

// envNames are every MTX_* name that config reads.
var envNames = []string{"MTX_HOMESERVER", "MTX_USER_ID", "MTX_ACCESS_TOKEN", "MTX_DEVICE_ID", "MTX_PICKLE_KEY"}

// ErrNoEnv means no MTX_* env is set. The caller decides whether to onboard.
var ErrNoEnv = errors.New("no MTX_* env set")

// Config is read from env, then from the stored session. Nothing here is persisted except via the store.
type Config struct {
	Homeserver  string
	UserID      string
	AccessToken string
	DeviceID    string
	PickleKey   string
	DBPath      string
}

// StorePath is MTX_DB, or dataHome/mtx/mtx.db. dataHome is $XDG_DATA_HOME or ~/.local/share.
func StorePath(getenv func(string) string, dataHome string) (string, error) {
	if p := getenv("MTX_DB"); p != "" {
		return p, nil
	}
	if dataHome == "" {
		return "", errors.New("no data dir: set MTX_DB or XDG_DATA_HOME/HOME")
	}
	return filepath.Join(dataHome, "mtx", "mtx.db"), nil
}

// LoadConfig reads MTX_* from getenv. It returns ErrNoEnv when no MTX_* env is set. Otherwise
// MTX_PICKLE_KEY must be set; the other fields may be empty and New fills them from the store.
func LoadConfig(getenv func(string) string, dataHome string) (Config, error) {
	set := 0
	for _, name := range envNames {
		if getenv(name) != "" {
			set++
		}
	}
	if set == 0 {
		return Config{}, ErrNoEnv
	}
	c := Config{
		Homeserver:  getenv("MTX_HOMESERVER"),
		UserID:      getenv("MTX_USER_ID"),
		AccessToken: getenv("MTX_ACCESS_TOKEN"),
		DeviceID:    getenv("MTX_DEVICE_ID"),
		PickleKey:   getenv("MTX_PICKLE_KEY"),
	}
	if c.PickleKey == "" {
		return Config{}, fmt.Errorf("missing required env %s", Required[0])
	}
	if c.Homeserver != "" {
		if err := ValidateHomeserver(c.Homeserver); err != nil {
			return Config{}, err
		}
	}
	if c.UserID != "" {
		if err := ValidateUserID(c.UserID); err != nil {
			return Config{}, err
		}
	}
	var err error
	c.DBPath, err = StorePath(getenv, dataHome)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}

// ValidateHomeserver accepts an http(s) base URL with a host.
func ValidateHomeserver(s string) error {
	if u, err := url.Parse(s); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("homeserver must be an http(s) URL")
	}
	return nil
}

// ValidateUserID accepts @localpart:server.
func ValidateUserID(s string) error {
	if !strings.HasPrefix(s, "@") || !strings.Contains(s, ":") {
		return errors.New("user ID must look like @user:server")
	}
	return nil
}
