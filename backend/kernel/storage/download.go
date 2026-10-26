package storage

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope"
)

// Download serves the URLs Signer makes: `GET /files/{key...}?expires&sig`.
// The browser following an attachment link has no bearer token, so the
// signature is the whole authorisation - it was minted by a request that
// held the permission, it names one key, and it expires. The key's first
// segment is the tenant, and the handler enters that scope so the backend's
// own ownership check (Owned) agrees with the signature rather than
// trusting it twice.
//
// Always `Content-Disposition: attachment` and `nosniff`: a stored file is
// a download, never a page the API origin executes.
type Download struct {
	Storage Storage
	Signer  *Signer
}

// Mount registers the route on mux.
func (d Download) Mount(mux *http.ServeMux) {
	mux.Handle("GET "+DownloadPath+"{key...}", d)
}

func (d Download) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	q := r.URL.Query()
	if d.Signer == nil || d.Signer.Verify(key, q.Get("expires"), q.Get("sig")) != nil {
		httpx.WriteError(w, r, httpx.Forbidden(httpx.KeyForbidden))
		return
	}
	ctx := r.Context()
	if tenant, _, ok := strings.Cut(key, "/"); ok && tenant != "global" {
		ctx = scope.WithTenant(ctx, tenant)
	}
	info, err := d.Storage.Stat(ctx, key)
	if err == nil {
		var f io.ReadCloser
		if f, err = d.Storage.Open(ctx, key); err == nil {
			defer f.Close()
			h := w.Header()
			h.Set("Content-Type", info.ContentType)
			h.Set("Content-Length", strconv.FormatInt(info.Size, 10))
			h.Set("Content-Disposition", `attachment; filename="`+downloadName(key)+`"`)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Cache-Control", "private, no-store")
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, f)
			return
		}
	}
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, httpx.NotFound(httpx.KeyNotFound))
		return
	}
	httpx.WriteError(w, r, httpx.Internal(err))
}

// downloadName is the filename the key was made from: the last segment
// without the random prefix NewKey put before it.
func downloadName(key string) string {
	base := path.Base(key)
	if _, name, ok := strings.Cut(base, "-"); ok && name != "" {
		return name
	}
	return base
}
