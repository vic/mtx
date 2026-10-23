// Package matrix is the Matrix backend: env config, the SQLite store, and the sync loop.
package matrix

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Config is read from env only. Nothing here is persisted except via the store.
type Config struct {
	Homeserver  string
	UserID      string
	AccessToken string
	DeviceID    string
	PickleKey   string
	DBPath      string
}

// LoadConfig reads MTX_* from getenv. dataHome is $XDG_DATA_HOME, or ~/.local/share when unset.
func LoadConfig(getenv func(string) string, dataHome string) (Config, error) {
	var c Config
	req := map[string]*string{
		"MTX_HOMESERVER":   &c.Homeserver,
		"MTX_USER_ID":      &c.UserID,
		"MTX_ACCESS_TOKEN": &c.AccessToken,
		"MTX_DEVICE_ID":    &c.DeviceID,
		"MTX_PICKLE_KEY":   &c.PickleKey,
	}
	for _, name := range []string{"MTX_HOMESERVER", "MTX_USER_ID", "MTX_ACCESS_TOKEN", "MTX_DEVICE_ID", "MTX_PICKLE_KEY"} {
		v := getenv(name)
		if v == "" {
			return Config{}, fmt.Errorf("missing required env %s", name)
		}
		*req[name] = v
	}
	if u, err := url.Parse(c.Homeserver); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return Config{}, errors.New("MTX_HOMESERVER must be an http(s) URL")
	}
	if !strings.HasPrefix(c.UserID, "@") || !strings.Contains(c.UserID, ":") {
		return Config{}, errors.New("MTX_USER_ID must look like @user:server")
	}
	c.DBPath = getenv("MTX_DB")
	if c.DBPath == "" {
		if dataHome == "" {
			return Config{}, errors.New("no data dir: set MTX_DB or XDG_DATA_HOME/HOME")
		}
		c.DBPath = filepath.Join(dataHome, "mtx", "mtx.db")
	}
	return c, nil
}
