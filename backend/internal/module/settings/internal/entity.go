package internal

import (
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/i18n"
)

const (
	keyUnknown         i18n.Key = "settings.error.unknown"
	keyPlatformManaged i18n.Key = "settings.error.platform_managed"
	keyWrongScope      i18n.Key = "settings.error.wrong_scope"
)

// override is one tenant's answer to one declared flag. Absence means
// "inherit": removing the row restores the default rather than pinning
// yesterday's value. The value is stored boxed - `{"value": v}` - so a
// deliberate `false` or `null` is never mistaken for "unset" on the way
// through the JSON column.
type override struct {
	ID         string
	TenantID   string
	Identifier string
	Value      any
	UpdatedAt  time.Time
}

func (o override) Snapshot() map[string]any {
	return map[string]any{"identifier": o.Identifier, "value": o.Value}
}

func (o override) view() Override {
	return Override{Id: o.ID, Identifier: o.Identifier, Value: o.Value, UpdatedAt: o.UpdatedAt}
}

func newOverride(tenantID, identifier string, value any) override {
	return override{ID: uuid.Must(uuid.NewV7()).String(), TenantID: tenantID, Identifier: identifier, Value: value, UpdatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

// boxed is the stored and cached form: Found distinguishes "no override"
// from "overridden to null".
type boxed struct {
	Found bool `json:"found"`
	Value any  `json:"value"`
}

// settingView is the wire shape of a declaration. The identifier is its
// id: a flag is declared in code and has no row.
func settingView(d flags.Definition) Setting {
	typ := d.Type
	if typ == "" {
		typ = flags.TypeBool
	}
	return Setting{Id: d.Identifier, Identifier: d.Identifier, Name: d.Name, Description: nilIfEmpty(d.Description), Type: SettingType(typ),
		DefaultValue: d.Default, TenantEditable: d.TenantEditable, Category: nilIfEmpty(d.Category), Version: 1}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
