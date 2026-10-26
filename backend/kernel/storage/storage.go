// Package storage is where files go.
//
// Modules never touch the filesystem (the platformservice analyzer refuses
// os.WriteFile and friends outside the kernel). Three reasons, and the first
// is the one that matters:
//
//  1. Paths are tenant-scoped here, once. A key is `<tenant>/<module>/<id>-
//     <name>` and a read under another tenant's context is a not-found, so
//     one tenant reading another's uploads cannot be written.
//  2. The backend is configuration, not a rewrite: Local is the one shipped,
//     in dev and in production; another store would implement Storage and
//     change no caller.
//  3. Downloads are served through signed, expiring URLs rather than by
//     streaming bytes through a handler that has to re-check everything.
package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/open-enu/kernel/scope"
)

// ErrNotFound is returned for a key that does not exist - or exists under a
// tenant the context does not act for. The two are deliberately the same
// answer.
var ErrNotFound = errors.New("storage: not found")

// Info is what Stat returns.
type Info struct {
	Size        int64
	ContentType string
}

// Storage is the contract a module holds.
type Storage interface {
	// Write stores the content and returns the key to persist on the owning
	// record. The key carries the tenant from the context.
	Write(ctx context.Context, module, filename string, r io.Reader, contentType string) (string, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (Info, error)
	Delete(ctx context.Context, key string) error
	// TemporaryURL is a time-limited URL the browser can fetch directly.
	// Short-lived on purpose: a key that grants permanent access is a
	// credential, and it will end up in a chat message.
	TemporaryURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

var (
	moduleShape = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	keyShape    = regexp.MustCompile(`^[A-Za-z0-9_-]+/[a-z][a-z0-9_]*/[A-Za-z0-9._-]+$`)
	unsafeChar  = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// NewKey builds `<tenant|global>/<module>/<random>-<filename>` with the
// filename reduced to safe characters. The module must have a module name's
// shape; it is how a key says who owns it.
func NewKey(ctx context.Context, module, filename string) (string, error) {
	if !moduleShape.MatchString(module) {
		return "", fmt.Errorf("storage: module %q is not a module name", module)
	}
	name := strings.Trim(unsafeChar.ReplaceAllString(filename, "_"), "._-")
	if name == "" {
		name = "file"
	}
	if len(name) > 100 {
		name = name[len(name)-100:]
	}
	return scope.TenantOrGlobal(ctx) + "/" + module + "/" + strings.ToLower(rand.Text()[:16]) + "-" + name, nil
}

// Owned reports whether a key is well-formed and belongs to the context's
// tenant (or is global under an unscoped context). Every read goes through
// it; a malformed key is also a traversal attempt, and gets the same answer.
func Owned(ctx context.Context, key string) bool {
	if !keyShape.MatchString(key) {
		return false
	}
	tenant, _, _ := strings.Cut(key, "/")
	return tenant == scope.TenantOrGlobal(ctx)
}

// Memory is a Storage in memory, for tests. It enforces the same ownership
// rule as the real backends.
type Memory struct {
	Signer *Signer
	files  map[string]memFile
}

type memFile struct {
	data        []byte
	contentType string
}

// Write stores a copy.
func (m *Memory) Write(ctx context.Context, module, filename string, r io.Reader, contentType string) (string, error) {
	key, err := NewKey(ctx, module, filename)
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if m.files == nil {
		m.files = map[string]memFile{}
	}
	m.files[key] = memFile{data: data, contentType: contentType}
	return key, nil
}

// Open returns the content, or ErrNotFound for another tenant's key.
func (m *Memory) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	f, ok := m.files[key]
	if !ok || !Owned(ctx, key) {
		return nil, ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(f.data))), nil
}

// Stat returns size and content type.
func (m *Memory) Stat(ctx context.Context, key string) (Info, error) {
	f, ok := m.files[key]
	if !ok || !Owned(ctx, key) {
		return Info{}, ErrNotFound
	}
	return Info{Size: int64(len(f.data)), ContentType: f.contentType}, nil
}

// Delete removes; deleting what is not yours is a not-found.
func (m *Memory) Delete(ctx context.Context, key string) error {
	if _, ok := m.files[key]; !ok || !Owned(ctx, key) {
		return ErrNotFound
	}
	delete(m.files, key)
	return nil
}

// TemporaryURL signs a download URL.
func (m *Memory) TemporaryURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if _, err := m.Stat(ctx, key); err != nil {
		return "", err
	}
	if m.Signer == nil {
		return "", errors.New("storage: no signer configured")
	}
	return m.Signer.Sign(key, ttl), nil
}
