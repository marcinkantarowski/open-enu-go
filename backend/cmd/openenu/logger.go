package main

import (
	"io"
	"log/slog"

	"github.com/open-enu/kernel/logctx"
)

// newLogger is every process's logger: JSON lines to w, each one logged with
// a context carrying what that context knows - the kernel's request, tenant,
// caller and command, then the modules' own fields (kernel/logctx). A
// process builds it once without the modules, to report a registry that
// refused to start, and again with them.
func newLogger(w io.Writer, opts *slog.HandlerOptions, fields ...logctx.Field) *slog.Logger {
	return slog.New(logctx.Handler(slog.NewJSONHandler(w, opts), append(logctx.Standard(), fields...)...))
}
