package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionRoundTripCarriesProvider(t *testing.T) {
	s := NewSessions("k")
	id, err := s.Parse(s.Sign(Identity{Name: "alice@example.com", Role: "editor", Provider: ProviderOIDC}, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if id.Name != "alice@example.com" || id.Role != "editor" || id.Provider != ProviderOIDC {
		t.Fatalf("got %+v", id)
	}
	local, _ := s.Parse(s.Sign(Identity{Name: "bob", Role: "viewer"}, time.Hour))
	if local.Provider != ProviderLocal {
		t.Fatalf("empty provider must default to local, got %q", local.Provider)
	}
}

func TestSessionRejectsLegacyThreeFieldToken(t *testing.T) {
	s := NewSessions("k")
	payload := "Ym9i|viewer|" + strings.TrimSpace("9999999999")
	if _, err := s.Parse(payload + "|" + s.sig(payload)); err == nil {
		t.Fatal("a 0.9-style token without provider must be rejected")
	}
}

func TestStateCookieRoundTripAndTamper(t *testing.T) {
	s := NewSessions("k")
	v := s.SignState("st", "no", "ver", time.Minute)
	st, no, ver, err := s.ParseState(v)
	if err != nil || st != "st" || no != "no" || ver != "ver" {
		t.Fatalf("round trip: %v %q %q %q", err, st, no, ver)
	}
	if _, _, _, err := s.ParseState(v[:len(v)-2] + "xx"); err == nil {
		t.Fatal("tampered signature must fail")
	}
	if _, _, _, err := s.ParseState(s.SignState("a", "b", "c", -time.Second)); err == nil {
		t.Fatal("expired state must fail")
	}
	if _, _, _, err := s.ParseState(s.SignState("", "b", "c", time.Minute)); err == nil {
		t.Fatal("empty state must fail")
	}
	if _, _, _, err := NewSessions("other").ParseState(v); err == nil {
		t.Fatal("different key must fail")
	}
}

func TestMapRolePrecedenceAndDefault(t *testing.T) {
	roles := map[string][]string{"admin": {"platform-admins"}, "editor": {"sre"}, "viewer": {"everyone"}}
	cases := []struct {
		groups []string
		def    string
		want   string
		ok     bool
	}{
		{[]string{"sre", "platform-admins"}, "", "admin", true},
		{[]string{"sre"}, "", "editor", true},
		{[]string{"everyone"}, "", "viewer", true},
		{nil, "viewer", "viewer", true},
		{nil, "", "", false},
		{[]string{"unknown"}, "", "", false},
	}
	for _, c := range cases {
		got, ok := MapRole(c.groups, roles, c.def)
		if got != c.want || ok != c.ok {
			t.Errorf("MapRole(%v, def=%q) = %q,%v want %q,%v", c.groups, c.def, got, ok, c.want, c.ok)
		}
	}
}

// A failing account source keeps the last read for a short while, then falls back to the mounted
// directory, so an API outage never keeps a deleted or demoted account alive for long.
func TestStore_SourceFailureFallsBackToDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alice"), []byte("viewer:h1"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(dir)
	failing, errs := false, 0
	s.SourceErrors = func() { errs++ }
	s.SetSource(func() (map[string]string, error) {
		if failing {
			return nil, errors.New("api down")
		}
		return map[string]string{"alice": "admin:h2"}, nil
	})
	if role, _, _ := s.Lookup("alice"); role != "admin" {
		t.Fatalf("source read: role %q, want admin", role)
	}
	failing = true
	s.Invalidate()
	if role, _, _ := s.Lookup("alice"); role != "admin" || errs != 1 {
		t.Fatalf("fresh cache after one failure: role %q errs %d, want admin 1", role, errs)
	}
	s.srcMu.Lock()
	s.srcAt = time.Now().Add(-sourceMaxStale - time.Second)
	s.srcMu.Unlock()
	if role, _, _ := s.Lookup("alice"); role != "viewer" {
		t.Fatalf("stale cache: role %q, want the mounted directory's viewer", role)
	}
}
