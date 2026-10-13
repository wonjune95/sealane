package auth

import (
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
