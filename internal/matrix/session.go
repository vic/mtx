package matrix

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// savedSession is the device's login, kept in the store so start needs no access token.
type savedSession struct {
	Homeserver  string `json:"homeserver"`
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
}

// sealKey derives the AES-256 key for the session from the pickle key. The pickle key never leaves
// the caller; the derived key is used only here.
func sealKey(pickle string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(pickle), nil, "mtx session v1", 32)
}

func sealAEAD(pickle string) (cipher.AEAD, error) {
	k, err := sealKey(pickle)
	if err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// saveSession replaces the stored session with s, sealed under pickle.
func saveSession(ctx context.Context, db *sql.DB, pickle string, s savedSession) error {
	g, err := sealAEAD(pickle)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(s)
	if err != nil {
		return err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	blob := g.Seal(nonce, nonce, plain, nil)
	_, err = db.ExecContext(ctx, `INSERT INTO session (id, sealed) VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET sealed = excluded.sealed`, blob)
	return err
}

// loadSession returns the stored session. ok is false when none is stored. A wrong pickle key
// or a damaged row is an error.
func loadSession(ctx context.Context, db *sql.DB, pickle string) (savedSession, bool, error) {
	var blob []byte
	err := db.QueryRowContext(ctx, `SELECT sealed FROM session WHERE id = 1`).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return savedSession{}, false, nil
	}
	if err != nil {
		return savedSession{}, false, err
	}
	g, err := sealAEAD(pickle)
	if err != nil {
		return savedSession{}, false, err
	}
	if len(blob) < g.NonceSize() {
		return savedSession{}, false, errors.New("session row damaged")
	}
	plain, err := g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], nil)
	if err != nil {
		return savedSession{}, false, fmt.Errorf("session: wrong MTX_PICKLE_KEY or damaged store: %w", err)
	}
	var s savedSession
	if err := json.Unmarshal(plain, &s); err != nil {
		return savedSession{}, false, err
	}
	return s, true, nil
}
