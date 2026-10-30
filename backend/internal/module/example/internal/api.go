package internal

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
)

const keyNameRequired i18n.Key = "example.error.name_required"

// api implements the StrictServerInterface genapi derives from
// ../openapi.json; a method that disagrees with the spec does not compile.
// It turns a request into a command and a result into a response - it holds
// no transaction, runs no SQL and never calls a handler (thincontroller,
// writeboundary, rawsql). Reads go through the repository.
type api struct {
	bus   *command.Bus
	repo  repository
	flags flags.Flags
	guard auth.Guard
}

var _ StrictServerInterface = api{}

func (a api) GetExampleProjectIndex(ctx context.Context, req GetExampleProjectIndexRequestObject) (GetExampleProjectIndexResponseObject, error) {
	if _, err := auth.Require(ctx, "example.view"); err != nil {
		return nil, err
	}
	page := httpx.Paginate(req.Params.Page, req.Params.Size, httpx.Deref(req.Params.Sort), httpx.Deref(req.Params.Direction))
	items, total, err := a.repo.list(ctx, page)
	if err != nil {
		return nil, err
	}
	meta := page.Meta(total)
	out := ProjectList{Items: make([]Project, 0, len(items)), Meta: ListMeta{Total: meta.Total, Page: meta.Page, Size: meta.Size, Pages: meta.Pages}}
	for _, p := range items {
		out.Items = append(out.Items, p.view())
	}
	return GetExampleProjectIndex200JSONResponse(out), nil
}

func (a api) PostExampleProjectCreate(ctx context.Context, req PostExampleProjectCreateRequestObject) (PostExampleProjectCreateResponseObject, error) {
	if _, err := auth.Require(ctx, "example.manage"); err != nil {
		return nil, err
	}
	name, ok := cleanName(req.Body.Name)
	if !ok {
		return nil, httpx.BadRequest(keyNameRequired)
	}
	if err := a.guard.DenyImpersonated(ctx, "example.project.created"); err != nil {
		return nil, err
	}
	cmd := createProject{Name: name, Description: req.Body.Description, ClientReference: req.Body.ClientReference}
	if req.Body.Attributes != nil {
		cmd.Attributes = *req.Body.Attributes
	}
	p, err := command.Dispatch[project](ctx, a.bus, cmd)
	if err != nil {
		return nil, err
	}
	return PostExampleProjectCreate201JSONResponse(p.view()), nil
}

func (a api) GetExampleProjectShow(ctx context.Context, req GetExampleProjectShowRequestObject) (GetExampleProjectShowResponseObject, error) {
	if _, err := auth.Require(ctx, "example.view"); err != nil {
		return nil, err
	}
	p, err := a.repo.find(ctx, req.Id)
	if err != nil {
		return nil, notFound(err)
	}
	// The version the client now holds; it comes back as If-Match on the
	// next write, and a mismatch is what makes a concurrent edit visible.
	etag := httpx.ETag(p.Version)
	return GetExampleProjectShow200JSONResponse{Body: p.detail(), Headers: GetExampleProjectShow200ResponseHeaders{ETag: &etag}}, nil
}

func (a api) PatchExampleProjectRename(ctx context.Context, req PatchExampleProjectRenameRequestObject) (PatchExampleProjectRenameResponseObject, error) {
	if _, err := auth.Require(ctx, "example.manage"); err != nil {
		return nil, err
	}
	// Support may look; support may not rename things while wearing someone
	// else's face (ADR-0020).
	if err := a.guard.DenyImpersonated(ctx, "example.project.renamed"); err != nil {
		return nil, err
	}
	name, ok := cleanName(req.Body.Name)
	if !ok {
		return nil, httpx.BadRequest(keyNameRequired)
	}
	cmd := renameProject{ID: req.Id, Name: name}
	if req.Params.IfMatch != nil {
		cmd.IfMatch, cmd.Locked = httpx.IfMatch(*req.Params.IfMatch)
	}
	p, err := command.Dispatch[project](ctx, a.bus, cmd)
	if err != nil {
		return nil, err
	}
	return PatchExampleProjectRename200JSONResponse(p.view()), nil
}

func (a api) PostExampleProjectArchive(ctx context.Context, _ PostExampleProjectArchiveRequestObject) (PostExampleProjectArchiveResponseObject, error) {
	if _, err := auth.Require(ctx, "example.manage"); err != nil {
		return nil, err
	}
	// 404, not 403: "not built yet" and "switched off during an incident"
	// must look identical from outside.
	if !a.flags.Enabled(ctx, "example.archive", false) {
		return nil, httpx.NotFound(httpx.KeyNotFound)
	}
	if err := a.guard.DenyImpersonated(ctx, "example.projects.archive_requested"); err != nil {
		return nil, err
	}
	accepted, err := command.Dispatch[ArchiveAccepted](ctx, a.bus, archiveProjects{})
	if err != nil {
		return nil, err
	}
	return PostExampleProjectArchive202JSONResponse(accepted), nil
}

func cleanName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, s != "" && utf8.RuneCountInString(s) <= 200
}
