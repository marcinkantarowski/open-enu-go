package httpx

import "github.com/open-enu/kernel/i18n"

type Error struct{ Message i18n.Key }

func NotFound(message i18n.Key) *Error { return &Error{Message: message} }

func ValidationFailed(violations map[string]i18n.Key) *Error { return nil }
