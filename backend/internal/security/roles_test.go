package security_test

import (
	"strings"
	"testing"
)

// Identity's role resolver, seen from the session it issues: owner holds
// everything, admin everything but what is the owner's alone, member the
// everyday verbs. A permission no module declares exists for nobody.

func TestAMemberReadsAndDoesTheEverydayWorkAndNothingElse(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	sess := s.member("member@example.test", "acme", "member")
	if sess.str("role") != "member" {
		t.Fatalf("role: %s", sess.raw)
	}
	p := perms(sess)
	for _, want := range []string{"example.view", "audit.view", "progress.view", "attachment.view", "api_key.view", "tenant.view", "identity.member.view"} {
		if !has(p, want) {
			t.Fatalf("a member lacks %s: %v", want, p)
		}
	}
	for _, refused := range []string{"example.manage", "api_key.manage", "tenant.manage", "tenant.delete", "identity.member.manage", "attachment.manage"} {
		if has(p, refused) {
			t.Fatalf("a member holds %s: %v", refused, p)
		}
	}
	if r := s.do("POST", "/api/projects", sess.str("token"), map[string]any{"name": "x"}); r.code != 403 {
		t.Fatalf("a member creating a project: %d", r.code)
	}
	if r := s.do("GET", "/api/projects", sess.str("token"), nil); r.code != 200 {
		t.Fatalf("a member listing projects: %d", r.code)
	}
}

func TestAnAdminHoldsEverythingButWhatIsTheOwnersAlone(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	owner, admin := perms(s.login(ownerEmail)), perms(s.member("admin@example.test", "acme", "admin"))
	for _, p := range s.svc.Permissions {
		if !has(owner, p) {
			t.Fatalf("the owner lacks %s", p)
		}
	}
	for _, ownersAlone := range []string{"tenant.delete", "api_key.manage"} {
		if has(admin, ownersAlone) {
			t.Fatalf("an admin holds %s", ownersAlone)
		}
	}
	if len(admin) != len(owner)-2 {
		t.Fatalf("admin %d vs owner %d: %v", len(admin), len(owner), admin)
	}
	tok := s.member("admin@example.test", "acme", "admin").str("token")
	if r := s.do("POST", "/api/api-keys", tok, map[string]any{"name": "x", "permissions": []string{"example.view"}}); r.code != 403 {
		t.Fatalf("an admin minting a key: %d", r.code)
	}
	if r := s.do("POST", "/api/projects", tok, map[string]any{"name": "x"}); r.code != 201 {
		t.Fatalf("an admin creating a project: %d %s", r.code, r.raw)
	}
}

func TestAnUndeclaredPermissionIsRefusedToEveryone(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	sess := s.login(ownerEmail)
	for _, p := range perms(sess) {
		if !has(s.svc.Permissions, p) {
			t.Fatalf("the session holds %s, which no module declares", p)
		}
	}
	// Not even by asking for it on a key.
	if r := s.do("POST", "/api/api-keys", sess.str("token"), map[string]any{"name": "x", "permissions": []string{"billing.refund"}}); r.code != 400 || !strings.Contains(string(r.raw), "billing.refund") {
		t.Fatalf("an undeclared permission on a key: %d %s", r.code, r.raw)
	}
}

func TestAConsultantHoldsWhatAnAdminHolds(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	admin, consultant := perms(s.member("admin@example.test", "acme", "admin")), perms(s.member("iod@example.test", "acme", "consultant"))
	if len(consultant) != len(admin) {
		t.Fatalf("consultant %d vs admin %d: %v", len(consultant), len(admin), consultant)
	}
	for _, ownersAlone := range []string{"tenant.delete", "api_key.manage"} {
		if has(consultant, ownersAlone) {
			t.Fatalf("a consultant holds %s", ownersAlone)
		}
	}
}
