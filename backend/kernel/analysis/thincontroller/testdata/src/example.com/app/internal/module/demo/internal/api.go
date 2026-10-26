package internal

import (
	"database/sql" // want `api.go imports database/sql`
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool" // want `api.go imports github.com/jackc/pgx/v5/pgxpool - a controller imports the kernel's HTTP surface`
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope" // want `api.go imports github.com/open-enu/kernel/scope - a controller may import only httpx, i18n, command, platform, flags and auth`

	"example.com/app/internal/module/demo/contract"
)

var _ = sql.ErrNoRows
var _ pgxpool.Pool
var _ scope.Tx
var _ contract.ProjectID

func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/demo", func(w http.ResponseWriter, r *http.Request) {
		_ = CreateProject{Name: "fine - a command type"}
		_ = findAll()         // fine - a controller reads through the repository
		_ = handleCreate("x") // want `api.go uses handleCreate, declared in handler.go`
		httpx.WriteError(w, r, nil)
	})
}
