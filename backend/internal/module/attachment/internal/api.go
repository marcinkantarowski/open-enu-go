package internal

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
)

const (
	keyFileRequired i18n.Key = "attachment.error.file_required"
	keyTooLarge     i18n.Key = "attachment.error.too_large"
)

// api implements the StrictServerInterface genapi derives from
// ../openapi.json. The multipart body is read here - the one place the
// request's shape is known - and the file reaches the handler as bytes with
// a content type sniffed from them, never the client's claim.
type api struct {
	bus   *command.Bus
	repo  repository
	guard auth.Guard
}

var _ StrictServerInterface = api{}

func (a api) PostAttachmentUpload(ctx context.Context, req PostAttachmentUploadRequestObject) (PostAttachmentUploadResponseObject, error) {
	if _, err := auth.Require(ctx, "attachment.manage"); err != nil {
		return nil, err
	}
	// A file in the tenant's storage under the person's name is the
	// person's act, not support's (ADR-0020).
	if err := a.guard.DenyImpersonated(ctx, "attachment.uploaded"); err != nil {
		return nil, err
	}
	cmd, err := readUpload(req.Body)
	if err != nil {
		return nil, err
	}
	at, err := command.Dispatch[attachment](ctx, a.bus, cmd)
	if err != nil {
		return nil, err
	}
	url, err := a.repo.url(ctx, at)
	if err != nil {
		return nil, err
	}
	return PostAttachmentUpload201JSONResponse(at.view(url)), nil
}

func (a api) GetAttachmentShow(ctx context.Context, req GetAttachmentShowRequestObject) (GetAttachmentShowResponseObject, error) {
	if _, err := auth.Require(ctx, "attachment.view"); err != nil {
		return nil, err
	}
	at, err := a.repo.find(ctx, req.Id)
	if err != nil {
		return nil, notFound(err)
	}
	url, err := a.repo.url(ctx, at)
	if err != nil {
		return nil, err
	}
	return GetAttachmentShow200JSONResponse(at.view(url)), nil
}

// readUpload walks the parts: `file` (required, at most MaxBytes),
// `ownerType` and `ownerId` (optional, short).
func readUpload(r *multipart.Reader) (uploadAttachment, error) {
	var cmd uploadAttachment
	var seen bool
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return cmd, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err)
		}
		switch part.FormName() {
		case "file":
			data, err := io.ReadAll(io.LimitReader(part, MaxBytes+1))
			if err != nil {
				return cmd, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err)
			}
			if int64(len(data)) > MaxBytes {
				return cmd, httpx.BadRequest(keyTooLarge).WithDetails(map[string]any{"maxBytes": MaxBytes})
			}
			name := strings.TrimSpace(part.FileName())
			if name == "" {
				name = "file"
			}
			cmd.Filename, cmd.Data, cmd.ContentType, seen = name, data, sniff(data), true
		case "ownerType":
			cmd.OwnerType = short(part)
		case "ownerId":
			cmd.OwnerID = short(part)
		}
		part.Close()
	}
	if !seen || len(cmd.Data) == 0 {
		return cmd, httpx.BadRequest(keyFileRequired)
	}
	return cmd, nil
}

// sniff is the content type the bytes say they are; a client's claim is
// how an HTML page gets stored as an image.
func sniff(data []byte) string {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	return http.DetectContentType(head)
}

func short(part *multipart.Part) *string {
	// 64 characters, as the spec says; read enough bytes for 65 of the
	// widest, so a longer value is seen as too long rather than cut short.
	b, _ := io.ReadAll(io.LimitReader(part, 65*utf8.UTFMax))
	s := strings.TrimSpace(string(b))
	if s == "" || utf8.RuneCountInString(s) > 64 {
		return nil
	}
	return &s
}
