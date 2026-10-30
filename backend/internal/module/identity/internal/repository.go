package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the only file with SQL. None of the four tables is under
// row-level security: a person, their memberships and their tokens span
// tenants, and a login has no tenant yet. Isolation here is by user id,
// and every query says so.
type repository struct {
	db     *scope.Pool
	crypto *crypto.Encryptor
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, crypto: svc.Crypto}
}

var (
	errNotFound     = errors.New("identity: not found")
	errInvalidToken = errors.New("identity: token unknown, used or expired")
)

const userColumns = `id, email, email_hash, password, display_name, status, locale, avatar_key, version, created_at, last_login_at`

// ── users ───────────────────────────────────────────────────────────────

// emailHash is the lookup hash of an address, keyed under the platform's
// master key (crypto.Encryptor.HashForLookup), over its one spelling.
func (r repository) emailHash(email string) string {
	return r.crypto.HashForLookup(normalizeEmail(email))
}

func (r repository) insertUser(ctx context.Context, tx *scope.Tx, u user) error {
	u.EmailHash = r.emailHash(u.Email)
	email, err := r.crypto.Encrypt(u.Email, "")
	if err != nil {
		return err
	}
	name, err := r.encryptOpt(u.DisplayName)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO identity_user (`+userColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		u.ID, email, u.EmailHash, u.Password, name, u.Status, u.Locale, u.AvatarKey, u.Version, u.CreatedAt, u.LastLoginAt)
	return err
}

func (r repository) updateUser(ctx context.Context, tx *scope.Tx, u user) error {
	name, err := r.encryptOpt(u.DisplayName)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE identity_user SET password = $2, display_name = $3, status = $4, locale = $5, avatar_key = $6, version = $7 WHERE id = $1`,
		u.ID, u.Password, name, u.Status, u.Locale, u.AvatarKey, u.Version)
	return err
}

func (r repository) getUser(ctx context.Context, tx *scope.Tx, id string) (user, error) {
	if _, err := uuid.Parse(id); err != nil {
		return user{}, errNotFound
	}
	return r.oneUser(ctx, tx, `WHERE id = $1`, id)
}

func (r repository) userByEmailHashTx(ctx context.Context, tx *scope.Tx, hash string) (user, error) {
	return r.oneUser(ctx, tx, `WHERE email_hash = $1`, hash)
}

func (r repository) oneUser(ctx context.Context, tx *scope.Tx, where string, arg any) (user, error) {
	rows, err := tx.Query(ctx, `SELECT `+userColumns+` FROM identity_user `+where, arg)
	if err != nil {
		return user{}, err
	}
	users, err := r.scanUsers(rows)
	if err != nil {
		return user{}, err
	}
	if len(users) == 0 {
		return user{}, errNotFound
	}
	return users[0], nil
}

// findUser and userByEmailHash are the controller's and the session's reads,
// each in its own transaction.
func (r repository) findUser(ctx context.Context, id string) (u user, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		u, err = r.getUser(ctx, tx, id)
		return err
	})
	return u, err
}

func (r repository) userByEmailHash(ctx context.Context, hash string) (u user, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		u, err = r.userByEmailHashTx(ctx, tx, hash)
		return err
	})
	return u, err
}

// localeOf is the locale preference's read: one indexed lookup, decrypting
// nothing.
func (r repository) localeOf(ctx context.Context, id string) (locale string, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		var l *string
		if err := tx.QueryRow(ctx, `SELECT locale FROM identity_user WHERE id = $1`, id).Scan(&l); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		} else if l != nil {
			locale = *l
		}
		return nil
	})
	return locale, err
}

// recordLogin stamps last_login_at outside the bus, with the session.
// recordLogin stamps the login and, given one, replaces the password's
// verifier with a current one. Neither is a change of anything the person
// owns - the version is not bumped and nothing is audited; the password is
// the same password.
func (r repository) recordLogin(ctx context.Context, id, verifier string) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE identity_user SET last_login_at = now(), password = COALESCE(NULLIF($2, ''), password) WHERE id = $1`, id, verifier)
		return err
	})
}

func (r repository) deleteUser(ctx context.Context, tx *scope.Tx, id string) (int, error) {
	var memberships, tokens int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM identity_membership WHERE user_id = $1) + (SELECT count(*) FROM identity_security_token WHERE user_id = $1),
		(SELECT count(*) FROM identity_refresh_token WHERE user_id = $1)`, id).Scan(&memberships, &tokens); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM identity_user WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()) + memberships + tokens, nil
}

// ── memberships ─────────────────────────────────────────────────────────

const membershipColumns = `id, user_id, tenant_id, role, status, created_at`

func (r repository) insertMembership(ctx context.Context, tx *scope.Tx, m membership) error {
	_, err := tx.Exec(ctx, `INSERT INTO identity_membership (`+membershipColumns+`) VALUES ($1, $2, $3, $4, $5, $6)`,
		m.ID, m.UserID, m.TenantID, m.Role, m.Status, m.CreatedAt)
	return err
}

func (r repository) membershipsOfTx(ctx context.Context, tx *scope.Tx, userID string) ([]membership, error) {
	rows, err := tx.Query(ctx, `SELECT `+membershipColumns+` FROM identity_membership WHERE user_id = $1 ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []membership
	for rows.Next() {
		var m membership
		if err := rows.Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r repository) membershipsOf(ctx context.Context, userID string) (ms []membership, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		ms, err = r.membershipsOfTx(ctx, tx, userID)
		return err
	})
	return ms, err
}

// membersOf is the operator console's list: every membership of a
// workspace, with the person behind each.
func (r repository) membersOf(ctx context.Context, tenantID string) (out []Member, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+membershipColumns+` FROM identity_membership WHERE tenant_id = $1 ORDER BY created_at, id`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		var ms []membership
		for rows.Next() {
			var m membership
			if err := rows.Scan(&m.ID, &m.UserID, &m.TenantID, &m.Role, &m.Status, &m.CreatedAt); err != nil {
				return err
			}
			ms = append(ms, m)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		ids := make([]string, 0, len(ms))
		for _, m := range ms {
			ids = append(ids, m.UserID)
		}
		urows, err := tx.Query(ctx, `SELECT `+userColumns+` FROM identity_user WHERE id = ANY($1)`, ids)
		if err != nil {
			return err
		}
		users, err := r.scanUsers(urows)
		if err != nil {
			return err
		}
		byID := map[string]user{}
		for _, u := range users {
			byID[u.ID] = u
		}
		out = make([]Member, 0, len(ms))
		for _, m := range ms {
			u, ok := byID[m.UserID]
			if !ok {
				continue
			}
			out = append(out, Member{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Role: m.Role, Status: m.Status, Locale: u.Locale})
		}
		return nil
	})
	return out, err
}

func (r repository) countMembers(ctx context.Context, tenantID string) (n int, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM identity_membership WHERE tenant_id = $1`, tenantID).Scan(&n)
	})
	return n, err
}

func (r repository) deleteTenantMemberships(ctx context.Context, tx *scope.Tx, tenantID string) (int, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM identity_membership WHERE tenant_id = $1`, tenantID)
	return int(tag.RowsAffected()), err
}

// ── refresh tokens (session.go's writes, each in its own transaction) ────

const refreshColumns = `id, token_hash, user_id, family, tenant_id, expires_at, used_at, revoked, created_at`

// issueRefresh stores a new token in the family and returns the raw value.
func (r repository) issueRefresh(ctx context.Context, userID string, tenantID *string, family string) (string, error) {
	raw := newRawToken()
	t := refreshToken{ID: uuid.Must(uuid.NewV7()).String(), TokenHash: hashToken(raw), UserID: userID, Family: family, TenantID: tenantID,
		ExpiresAt: time.Now().Add(refreshTTL), CreatedAt: time.Now()}
	err := r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO identity_refresh_token (`+refreshColumns+`) VALUES ($1, $2, $3, $4, $5, $6, NULL, false, $7)`,
			t.ID, t.TokenHash, t.UserID, t.Family, t.TenantID, t.ExpiresAt, t.CreatedAt)
		return err
	})
	return raw, err
}

// consumeRefresh marks the token used and returns it. Unknown: invalid.
// Already used or revoked: the family is revoked - a second presentation is
// a replay - and that revocation must COMMIT, so it happens in the
// transaction and the refusal is decided after it. Expired: invalid.
func (r repository) consumeRefresh(ctx context.Context, raw string) (t refreshToken, err error) {
	var invalid bool
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+refreshColumns+` FROM identity_refresh_token WHERE token_hash = $1 FOR UPDATE`, hashToken(raw))
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			invalid = true
			return rows.Err()
		}
		if err := rows.Scan(&t.ID, &t.TokenHash, &t.UserID, &t.Family, &t.TenantID, &t.ExpiresAt, &t.UsedAt, &t.Revoked, &t.CreatedAt); err != nil {
			return err
		}
		rows.Close()
		switch {
		case t.Revoked || t.UsedAt != nil:
			invalid = true
			_, err = tx.Exec(ctx, `UPDATE identity_refresh_token SET revoked = true WHERE family = $1`, t.Family)
			return err
		case !time.Now().Before(t.ExpiresAt):
			invalid = true
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE identity_refresh_token SET used_at = now() WHERE id = $1`, t.ID)
		return err
	})
	if err == nil && invalid {
		err = errInvalidToken
	}
	return t, err
}

func (r repository) revokeAllRefresh(ctx context.Context, tx *scope.Tx, userID string) error {
	_, err := tx.Exec(ctx, `UPDATE identity_refresh_token SET revoked = true WHERE user_id = $1 AND NOT revoked`, userID)
	return err
}

// revokeTenantRefresh revokes every live refresh token whose workspace is
// tenantID; its families die with it, as on a replay.
func (r repository) revokeTenantRefresh(ctx context.Context, tx *scope.Tx, tenantID string) (int, error) {
	tag, err := tx.Exec(ctx, `UPDATE identity_refresh_token SET revoked = true WHERE family IN
		(SELECT family FROM identity_refresh_token WHERE tenant_id = $1 AND NOT revoked AND used_at IS NULL) AND NOT revoked`, tenantID)
	return int(tag.RowsAffected()), err
}

func (r repository) revokeAllRefreshOf(ctx context.Context, userID string) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error { return r.revokeAllRefresh(ctx, tx, userID) })
}

// refreshTokensOf lists a person's sessions for the GDPR export.
func (r repository) refreshTokensOf(ctx context.Context, userID string) (out []refreshToken, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+refreshColumns+` FROM identity_refresh_token WHERE user_id = $1 ORDER BY created_at`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t refreshToken
			if err := rows.Scan(&t.ID, &t.TokenHash, &t.UserID, &t.Family, &t.TenantID, &t.ExpiresAt, &t.UsedAt, &t.Revoked, &t.CreatedAt); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// ── security tokens ─────────────────────────────────────────────────────

// issueSecurityToken stores the hash and returns the raw value, once.
func (r repository) issueSecurityToken(ctx context.Context, tx *scope.Tx, purpose string, userID, tenantID *string, payload map[string]any, ttl time.Duration) (string, error) {
	raw := newRawToken()
	if payload == nil {
		payload = map[string]any{}
	}
	_, err := tx.Exec(ctx, `INSERT INTO identity_security_token (id, token_hash, purpose, user_id, tenant_id, payload, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())`, uuid.Must(uuid.NewV7()).String(), hashToken(raw), purpose, userID, tenantID, payload, time.Now().Add(ttl))
	return raw, err
}

// redeemSecurityToken consumes a live token of the purpose, or fails with
// one error for unknown, wrong purpose, used and expired alike.
func (r repository) redeemSecurityToken(ctx context.Context, tx *scope.Tx, raw, purpose string) (securityToken, error) {
	var t securityToken
	err := tx.QueryRow(ctx, `UPDATE identity_security_token SET consumed_at = now()
		WHERE token_hash = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > now()
		RETURNING id, token_hash, purpose, user_id, tenant_id, payload, expires_at, consumed_at, created_at`, hashToken(raw), purpose).
		Scan(&t.ID, &t.TokenHash, &t.Purpose, &t.UserID, &t.TenantID, &t.Payload, &t.ExpiresAt, &t.ConsumedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return securityToken{}, errInvalidToken
	}
	return t, err
}

// ── helpers ─────────────────────────────────────────────────────────────

func (r repository) encryptOpt(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	ct, err := r.crypto.Encrypt(*v, "")
	if err != nil {
		return nil, fmt.Errorf("identity: encrypt: %w", err)
	}
	return &ct, nil
}

func (r repository) scanUsers(rows pgx.Rows) ([]user, error) {
	defer rows.Close()
	var out []user
	for rows.Next() {
		var u user
		var email string
		var name *string
		if err := rows.Scan(&u.ID, &email, &u.EmailHash, &u.Password, &name, &u.Status, &u.Locale, &u.AvatarKey, &u.Version, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, err
		}
		plain, err := r.crypto.Decrypt(email, "")
		if err != nil {
			return nil, fmt.Errorf("identity: decrypt email of %s: %w", u.ID, err)
		}
		u.Email = plain
		if name != nil {
			n, err := r.crypto.Decrypt(*name, "")
			if err != nil {
				return nil, fmt.Errorf("identity: decrypt display name of %s: %w", u.ID, err)
			}
			u.DisplayName = &n
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// notFound maps the repository's miss to the wire's 404; anything else passes.
func notFound(err error) error {
	if errors.Is(err, errNotFound) {
		return httpx.NotFound(httpx.KeyNotFound)
	}
	return err
}
