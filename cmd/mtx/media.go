package main

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// isMediaType reports whether a message of this type carries a file that /open can save.
func isMediaType(t event.MessageType) bool {
	switch t {
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		return true
	}
	return false
}

// recordMedia remembers a media message by its event ID, for /open.
func (r *runner) recordMedia(ev id.EventID, m *event.MessageEventContent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.media == nil {
		r.media = map[id.EventID]*event.MessageEventContent{}
	}
	r.media[ev] = m
}

// fetchMedia downloads m's file and decrypts it when the room is encrypted.
func (r *runner) fetchMedia(ctx context.Context, m *event.MessageEventContent) ([]byte, error) {
	raw := m.URL
	if m.File != nil {
		raw = m.File.URL
	}
	uri, err := id.ParseContentURI(string(raw))
	if err != nil {
		return nil, fmt.Errorf("media url: %w", err)
	}
	data, err := r.mx.DownloadBytes(ctx, uri)
	if err != nil {
		return nil, err
	}
	if m.File != nil {
		if err := m.File.PrepareForDecryption(); err != nil {
			return nil, err
		}
		if err := m.File.DecryptInPlace(data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// mediaDir is where /open saves files: $XDG_RUNTIME_DIR/mtx, else the temp dir.
func mediaDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "mtx")
	}
	return filepath.Join(os.TempDir(), "mtx")
}

// open saves event ev's media and opens it with xdg-open.
func (r *runner) open(ev string) error {
	r.mu.Lock()
	m := r.media[id.EventID(ev)]
	r.mu.Unlock()
	if m == nil {
		return fmt.Errorf("open: that line has no media")
	}
	ctx, cancel := callCtx()
	defer cancel()
	data, err := r.fetchMedia(ctx, m)
	if err != nil {
		return err
	}
	dir := mediaDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, filepath.Base(ev)+"-"+filepath.Base(m.Body)) // Base: a sender cannot pick the directory
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return exec.Command("xdg-open", path).Start()
}

// upload sends the file at path to channel ch: an image as m.image, else m.file. In an encrypted room
// the bytes are encrypted first and the key goes in the (megolm-encrypted) message.
func (r *runner) upload(ch, path string) error {
	room, err := r.roomOf(ch)
	if err != nil {
		return err
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, rest)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ctx, cancel := callCtx()
	defer cancel()
	mt := mime.TypeByExtension(filepath.Ext(path))
	if mt == "" {
		mt = http.DetectContentType(data)
	}
	name := filepath.Base(path)
	content := &event.MessageEventContent{MsgType: event.MsgFile, Body: name, Info: &event.FileInfo{MimeType: mt, Size: len(data)}}
	if strings.HasPrefix(mt, "image/") {
		content.MsgType = event.MsgImage
	}
	encrypted := false
	if r.mx.StateStore != nil {
		encrypted, _ = r.mx.StateStore.IsEncrypted(ctx, room)
	}
	upType := mt
	var ef *attachment.EncryptedFile
	if encrypted {
		ef = attachment.NewEncryptedFile()
		ef.EncryptInPlace(data)
		upType = "application/octet-stream"
	}
	resp, err := r.mx.UploadBytesWithName(ctx, data, upType, name)
	if err != nil {
		return err
	}
	if ef != nil {
		content.File = &event.EncryptedFileInfo{EncryptedFile: *ef, URL: resp.ContentURI.CUString()}
	} else {
		content.URL = resp.ContentURI.CUString()
	}
	_, err = r.mx.SendMessageEvent(ctx, room, event.EventMessage, content)
	return err
}

// setAvatar uploads the image at path and makes it my avatar.
func (r *runner) setAvatar(path string) error {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, rest)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mt := http.DetectContentType(data)
	if !strings.HasPrefix(mt, "image/") {
		return fmt.Errorf("avatar: %s is not an image (%s)", filepath.Base(path), mt)
	}
	ctx, cancel := callCtx()
	defer cancel()
	resp, err := r.mx.UploadBytesWithName(ctx, data, mt, filepath.Base(path))
	if err != nil {
		return err
	}
	return r.mx.SetAvatarURL(ctx, resp.ContentURI)
}
