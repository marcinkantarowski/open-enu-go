package internal

import (
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
)

const keyMissing i18n.Key = "demo.project.missing"
const keyWrong i18n.Key = "No such project" // want `"No such project" is not a translation key`

var fromVar = someKey()

func someKey() i18n.Key { return keyMissing }

func handlers() {
	_ = httpx.NotFound(keyMissing)
	_ = httpx.NotFound("demo.project.gone")
	_ = httpx.NotFound(fromVar)
	_ = httpx.NotFound("Project not found.") // want `"Project not found." is not a translation key`
	_ = i18n.Key("Please try again")         // want `"Please try again" is not a translation key`
	_ = httpx.ValidationFailed(map[string]i18n.Key{"name": "demo.validation.name_required"})
	_ = httpx.ValidationFailed(map[string]i18n.Key{"name": i18n.Key("Name is required")}) // want `"Name is required" is not a translation key`
}
