package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local keeps files under Root, one path per key, with the content type in
// a `.type` sidecar, and serves downloads itself: TemporaryURL signs a key
// with Signer and Download verifies it and streams the file. It is the only
// persistent backend the kernel ships; an object store that hands the object
// straight to the client would be another Storage implementation, not a
// change to any caller.
type Local struct {
	Root   string
	Signer *Signer
}

func (l *Local) path(key string) string { return filepath.Join(l.Root, filepath.FromSlash(key)) }

// Write stores to a temp file and renames, so a reader never sees a partial
// file.
func (l *Local) Write(ctx context.Context, module, filename string, r io.Reader, contentType string) (string, error) {
	key, err := NewKey(ctx, module, filename)
	if err != nil {
		return "", err
	}
	dst := l.path(key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".upload-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.WriteFile(dst+".type", []byte(contentType), 0o640); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		os.Remove(dst + ".type")
		return "", err
	}
	return key, nil
}

// Open returns the file, or ErrNotFound for another tenant's key.
func (l *Local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !Owned(ctx, key) {
		return nil, ErrNotFound
	}
	f, err := os.Open(l.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// Stat returns size and the sidecar's content type.
func (l *Local) Stat(ctx context.Context, key string) (Info, error) {
	if !Owned(ctx, key) {
		return Info{}, ErrNotFound
	}
	st, err := os.Stat(l.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return Info{}, ErrNotFound
	}
	if err != nil {
		return Info{}, err
	}
	ct, _ := os.ReadFile(l.path(key) + ".type")
	return Info{Size: st.Size(), ContentType: strings.TrimSpace(string(ct))}, nil
}

// Delete removes the file and its sidecar.
func (l *Local) Delete(ctx context.Context, key string) error {
	if _, err := l.Stat(ctx, key); err != nil {
		return err
	}
	os.Remove(l.path(key) + ".type")
	return os.Remove(l.path(key))
}

// TemporaryURL signs a download URL for the key.
func (l *Local) TemporaryURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if _, err := l.Stat(ctx, key); err != nil {
		return "", err
	}
	if l.Signer == nil {
		return "", errors.New("storage: no signer configured")
	}
	return l.Signer.Sign(key, ttl), nil
}
