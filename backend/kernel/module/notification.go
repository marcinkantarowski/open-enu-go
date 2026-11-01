package module

import (
	"fmt"
	"strings"

	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/platform"
)

// Notifications returns every declared notification type, in module order:
// the catalogue the Notification module stores against and the list a
// preferences screen would show.
func (r *Registry) Notifications() []notification.Definition {
	var out []notification.Definition
	for _, m := range r.ordered {
		out = append(out, m.Notifications...)
	}
	return out
}

// Notifier returns the one module's notifier, or nil when no module
// provides one - the process then keeps notification.Log.
func (r *Registry) Notifier(svc *platform.Services) (notification.Notifier, error) {
	var out notification.Notifier
	owner := ""
	for _, m := range r.ordered {
		if m.Notifier == nil {
			continue
		}
		if owner != "" {
			return nil, fmt.Errorf("modules %q and %q both deliver notifications - exactly one may", owner, m.Manifest.Name)
		}
		owner, out = m.Manifest.Name, m.Notifier(svc)
	}
	return out, nil
}

// MailJournal returns the one module's journal, or nil when none keeps one.
func (r *Registry) MailJournal(svc *platform.Services) (mail.Journal, error) {
	var out mail.Journal
	owner := ""
	for _, m := range r.ordered {
		if m.MailJournal == nil {
			continue
		}
		if owner != "" {
			return nil, fmt.Errorf("modules %q and %q both keep a mail journal - exactly one may", owner, m.Manifest.Name)
		}
		owner, out = m.Manifest.Name, m.MailJournal(svc)
	}
	return out, nil
}

// Forecasts returns what every module expects to raise, in module order.
func (r *Registry) Forecasts(svc *platform.Services) []notification.Forecast {
	out := []notification.Forecast{}
	for _, m := range r.ordered {
		if m.Upcoming != nil {
			out = append(out, m.Upcoming(svc))
		}
	}
	return out
}

// declareNotifications checks each type's shape and owner, and that the
// module's own catalogue can translate it: a row whose keys nobody holds
// is permanently unreadable, and that is found here rather than in a feed.
func (r *Registry) declareNotifications(m Module) error {
	n := m.Manifest.Name
	if len(m.Notifications) == 0 {
		return nil
	}
	var catalogue map[string]map[string]string
	if m.Locales != nil {
		var err error
		if catalogue, err = i18n.CheckParity(m.Locales); err != nil {
			return fmt.Errorf("module %q: %w", n, err)
		}
	}
	for _, d := range m.Notifications {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("module %q: %w", n, err)
		}
		if !strings.HasPrefix(d.Type, n+".") {
			return fmt.Errorf("module %q declares notification %q - a type is prefixed with its owner: %q", n, d.Type, n+".<what>")
		}
		if d.Email && !d.Reactive && m.Upcoming == nil {
			return fmt.Errorf("module %q: notification %q sends mail, but the module has no Upcoming - add one, or mark it Reactive if nobody can know it ahead", n, d.Type)
		}
		if other, taken := r.notifications[d.Type]; taken {
			return fmt.Errorf("notification type %q is declared by both %q and %q", d.Type, other, n)
		}
		for _, key := range []i18n.Key{d.TitleKey, d.BodyKey} {
			for locale, keys := range catalogue {
				if _, ok := keys[string(key)]; !ok {
					return fmt.Errorf("module %q: notification %q names %q, which its %s catalogue does not hold - a feed row nobody can translate is unreadable forever", n, d.Type, key, locale)
				}
			}
			if catalogue == nil {
				return fmt.Errorf("module %q: notification %q names %q but the module has no locales", n, d.Type, key)
			}
		}
		r.notifications[d.Type] = n
	}
	return nil
}
