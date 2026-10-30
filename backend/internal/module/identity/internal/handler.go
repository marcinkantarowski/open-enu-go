package internal

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/scope"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

const (
	keyTokenInvalid i18n.Key = "identity.error.token_invalid"
	keyRoleInvalid  i18n.Key = "identity.error.role_invalid"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct {
	repo    repository
	tenants tenantc.Provisioner
	hasher  crypto.PasswordHasher
	mail    mailer
}

// register answers the same whether or not the address has an account
// (ADR-0023): a new address gets an account, a pending workspace and a
// verification link; a known one gets a mail and nothing else changes. The
// password is hashed either way, so the two also take the same time.
func (h handlers) register(ctx context.Context, tx *scope.Tx, c registerUser) (registered, error) {
	email := normalizeEmail(c.Email)
	if existing, err := h.repo.userByEmailHashTx(ctx, tx, h.repo.emailHash(email)); err == nil {
		h.hasher.Spend(c.Password)
		return registered{}, h.registeredAgain(ctx, tx, existing)
	} else if !errors.Is(err, errNotFound) {
		return registered{}, err
	}
	verifier, err := h.hasher.Hash(c.Password)
	if err != nil {
		return registered{}, err
	}
	// The workspace, through Tenant's own command: audited as a creation in
	// its own right, with every module's defaults in place before the
	// person can log in. The bus runs it as a savepoint of this transaction,
	// so a failure below takes the workspace with it: a registration is one
	// transaction.
	locale := ""
	if c.Locale != nil {
		locale = *c.Locale
	}
	t, err := h.tenants.Create(ctx, tenantc.Create{Name: c.TenantName, Slug: uniqueSlug(c.TenantName), DefaultLocale: locale})
	if err != nil {
		return registered{}, err
	}
	u := newUser(email, verifier, c.DisplayName, c.Locale, userUnverified)
	if err := h.repo.insertUser(ctx, tx, u); err != nil {
		return registered{}, err
	}
	if err := h.repo.insertMembership(ctx, tx, newMembership(u.ID, t.ID, roleOwner)); err != nil {
		return registered{}, err
	}
	raw, err := h.repo.issueSecurityToken(ctx, tx, purposeVerifyEmail, &u.ID, &t.ID, map[string]any{"tenantId": t.ID}, verifyTTL)
	if err != nil {
		return registered{}, err
	}
	tx.Snapshot(nil, u.Snapshot())
	// After the commit: a link to a row that a rollback removed would be a
	// mail that cannot be acted on.
	tx.AfterCommit(func(ctx context.Context) error { h.mail.verification(ctx, u, raw); return nil })
	return registered{}, nil
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// uniqueSlug is the name's slug, cut to 40 characters, plus six hex digits
// so two "Acme"s never collide.
func uniqueSlug(name string) string {
	base := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if r := []rune(base); len(r) > 40 {
		base = strings.TrimRight(string(r[:40]), "-")
	}
	if base == "" {
		base = "workspace"
	}
	return base + "-" + uuid.NewString()[:6]
}

// registeredAgain is the mail for an address that has an account. Someone
// who never verified most likely lost the first link, and "sign in" would be
// a door that does not open: they get a new link to the workspace they
// started. Everyone else is told they already have an account.
func (h handlers) registeredAgain(ctx context.Context, tx *scope.Tx, u user) error {
	// On the trail as what it was: an attempt on an existing account.
	tx.Snapshot(nil, map[string]any{"userId": u.ID, "accountExisted": true})
	if u.Status == userUnverified {
		ms, err := h.repo.membershipsOfTx(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		for _, m := range ms {
			if m.Role != roleOwner {
				continue
			}
			raw, err := h.repo.issueSecurityToken(ctx, tx, purposeVerifyEmail, &u.ID, &m.TenantID, map[string]any{"tenantId": m.TenantID}, verifyTTL)
			if err != nil {
				return err
			}
			tx.AfterCommit(func(ctx context.Context) error { h.mail.verification(ctx, u, raw); return nil })
			return nil
		}
	}
	tx.AfterCommit(func(ctx context.Context) error { h.mail.accountExists(ctx, u); return nil })
	return nil
}

func (h handlers) verify(ctx context.Context, tx *scope.Tx, c verifyEmail) (user, error) {
	t, err := h.repo.redeemSecurityToken(ctx, tx, c.Token, purposeVerifyEmail)
	if err != nil {
		return user{}, invalidToken(err)
	}
	if t.UserID == nil {
		return user{}, httpx.BadRequest(keyTokenInvalid)
	}
	old, err := h.repo.getUser(ctx, tx, *t.UserID)
	if err != nil {
		return user{}, invalidToken(err)
	}
	updated := old
	if old.Status == userUnverified {
		updated.Status, updated.Version = userActive, old.Version+1
		if err := h.repo.updateUser(ctx, tx, updated); err != nil {
			return user{}, err
		}
	}
	if t.TenantID != nil {
		if err := h.tenants.Activate(ctx, *t.TenantID); err != nil {
			return user{}, err
		}
	}
	tx.Update(old, updated)
	return updated, nil
}

func (h handlers) requestReset(ctx context.Context, tx *scope.Tx, c requestPasswordReset) (string, error) {
	u, err := h.repo.userByEmailHashTx(ctx, tx, h.repo.emailHash(c.Email))
	if errors.Is(err, errNotFound) {
		return "", nil // the same answer leaves the controller either way
	}
	if err != nil {
		return "", err
	}
	raw, err := h.repo.issueSecurityToken(ctx, tx, purposeResetPassword, &u.ID, nil, nil, resetTTL)
	if err != nil {
		return "", err
	}
	tx.Snapshot(nil, map[string]any{"userId": u.ID})
	tx.AfterCommit(func(ctx context.Context) error { h.mail.reset(ctx, u, raw); return nil })
	return u.ID, nil
}

func (h handlers) reset(ctx context.Context, tx *scope.Tx, c resetPassword) (user, error) {
	t, err := h.repo.redeemSecurityToken(ctx, tx, c.Token, purposeResetPassword)
	if err != nil || t.UserID == nil {
		return user{}, invalidToken(errInvalidToken)
	}
	old, err := h.repo.getUser(ctx, tx, *t.UserID)
	if err != nil {
		return user{}, invalidToken(err)
	}
	verifier, err := h.hasher.Hash(c.Password)
	if err != nil {
		return user{}, err
	}
	updated := old
	updated.Password, updated.Version = verifier, old.Version+1
	if err := h.repo.updateUser(ctx, tx, updated); err != nil {
		return user{}, err
	}
	// Every session goes: whoever held one may be why the password changed.
	if err := h.repo.revokeAllRefresh(ctx, tx, old.ID); err != nil {
		return user{}, err
	}
	tx.Update(old, updated)
	return updated, nil
}

func (h handlers) updateProfile(ctx context.Context, tx *scope.Tx, c updateProfile) (user, error) {
	old, err := h.repo.getUser(ctx, tx, c.UserID)
	if err != nil {
		return user{}, notFound(err)
	}
	updated := old
	if c.SetDisplayName {
		updated.DisplayName = c.DisplayName
	}
	if c.Locale != nil {
		updated.Locale = c.Locale
	}
	updated.Version = old.Version + 1
	if err := h.repo.updateUser(ctx, tx, updated); err != nil {
		return user{}, err
	}
	tx.Update(old, updated)
	return updated, nil
}

func (h handlers) removeTenantMemberships(ctx context.Context, tx *scope.Tx, c removeTenantMemberships) (int, error) {
	n, err := h.repo.deleteTenantMemberships(ctx, tx, c.TenantID)
	if err != nil {
		return 0, err
	}
	// Without this, a refresh in the deleted workspace would quietly land
	// the person in another one (open's fallback) rather than end.
	ended, err := h.repo.revokeTenantRefresh(ctx, tx, c.TenantID)
	tx.Snapshot(nil, map[string]any{"tenantId": c.TenantID, "removed": n, "sessionsEnded": ended})
	return n, err
}

func (h handlers) endTenantSessions(ctx context.Context, tx *scope.Tx, c EndTenantSessions) (int, error) {
	n, err := h.repo.revokeTenantRefresh(ctx, tx, c.TenantID)
	tx.Snapshot(nil, map[string]any{"tenantId": c.TenantID, "sessionsEnded": n})
	return n, err
}

// createUser is idempotent by identity: an existing account keeps its
// verifier, a missing membership is added, an existing one is left alone.
func (h handlers) createUser(ctx context.Context, tx *scope.Tx, c createUser) (user, error) {
	if !validRole(c.Role) {
		return user{}, httpx.BadRequest(keyRoleInvalid)
	}
	email := normalizeEmail(c.Email)
	u, err := h.repo.userByEmailHashTx(ctx, tx, h.repo.emailHash(email))
	switch {
	case errors.Is(err, errNotFound):
		verifier, err := h.hasher.Hash(c.Password)
		if err != nil {
			return user{}, err
		}
		u = newUser(email, verifier, c.DisplayName, nil, userActive)
		if err := h.repo.insertUser(ctx, tx, u); err != nil {
			return user{}, err
		}
		tx.Snapshot(nil, u.Snapshot())
	case err != nil:
		return user{}, err
	}
	ms, err := h.repo.membershipsOfTx(ctx, tx, u.ID)
	if err != nil {
		return user{}, err
	}
	for _, m := range ms {
		if m.TenantID == c.TenantID {
			return u, nil
		}
	}
	return u, h.repo.insertMembership(ctx, tx, newMembership(u.ID, c.TenantID, c.Role))
}

func (h handlers) erase(ctx context.Context, tx *scope.Tx, c eraseUser) (int, error) {
	old, err := h.repo.getUser(ctx, tx, c.UserID)
	if errors.Is(err, errNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := h.repo.deleteUser(ctx, tx, old.ID)
	tx.Delete(old)
	return n, err
}

// invalidToken is the one answer for unknown, used, expired and wrong-purpose
// tokens: 400, one message.
func invalidToken(err error) error {
	if errors.Is(err, errInvalidToken) || errors.Is(err, errNotFound) {
		return httpx.BadRequest(keyTokenInvalid)
	}
	return err
}
