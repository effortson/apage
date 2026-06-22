package api

import (
	"testing"
	"time"
)

func TestPlanMaxLinkTTL(t *testing.T) {
	if planMaxLinkTTL("lite") != 24*time.Hour {
		t.Error("lite must cap link TTL at 24h")
	}
	if planMaxLinkTTL("pro") != 0 {
		t.Error("non-lite plans must not cap beyond the backing file")
	}
	if planMaxLinkTTL("") != 0 {
		t.Error("unknown plan must not impose a cap")
	}
}

func TestPlanUpgrades(t *testing.T) {
	if got := planUpgrades("lite"); len(got) != 3 || got[0] != "starter" {
		t.Errorf("lite should upgrade to starter/pro/team, got %v", got)
	}
	if got := planUpgrades("pro"); len(got) != 1 || got[0] != "team" {
		t.Errorf("pro should upgrade to team, got %v", got)
	}
	if got := planUpgrades("team"); len(got) != 0 {
		t.Errorf("team has no upgrades, got %v", got)
	}
	if got := planUpgrades("mystery"); got != nil {
		t.Errorf("unknown plan yields nil, got %v", got)
	}
}

func TestBodyHashStableAndDistinct(t *testing.T) {
	a := bodyHash(map[string]any{"x": 1, "y": "z"})
	b := bodyHash(map[string]any{"x": 1, "y": "z"})
	if a == "" || a != b {
		t.Error("identical payloads must hash identically")
	}
	if a == bodyHash(map[string]any{"x": 2, "y": "z"}) {
		t.Error("different payloads must hash differently")
	}
}

func TestLinkCreateCapByTrust(t *testing.T) {
	if linkCreateCap("new") != 20 {
		t.Errorf("new tenants must get the conservative cap")
	}
	if linkCreateCap("basic") <= linkCreateCap("new") {
		t.Errorf("basic must exceed new")
	}
	if linkCreateCap("trusted") <= linkCreateCap("basic") {
		t.Errorf("trusted must exceed basic")
	}
	if linkCreateCap("") != linkCreateCap("new") {
		t.Errorf("unknown trust must default to the new-tenant cap")
	}
}

func TestIsActiveContent(t *testing.T) {
	for _, c := range []string{"html", "svg"} {
		if !isActiveContent(c) {
			t.Errorf("%q must be treated as active content", c)
		}
	}
	for _, c := range []string{"pdf", "image", "text", "binary"} {
		if isActiveContent(c) {
			t.Errorf("%q must not be treated as active content", c)
		}
	}
}

func TestEtagMatches(t *testing.T) {
	if !etagMatches(`"abc123"`, "abc123") {
		t.Error("quotes must be ignored")
	}
	if !etagMatches("ABC123", "abc123") {
		t.Error("case must be ignored")
	}
	if etagMatches("abc", "def") {
		t.Error("different etags must not match")
	}
}
