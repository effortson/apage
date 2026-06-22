package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apage/apage/internal/config"
)

func testServer() *Server {
	return &Server{cfg: &config.Config{SessionSecret: "unit-test-secret", RenderDomain: "render.example.com"}}
}

func TestRenderGrantRoundTrip(t *testing.T) {
	s := testServer()
	g := s.issueRenderGrant("plink_abc")
	if g == "" {
		t.Fatal("grant must be non-empty")
	}
	if !s.validRenderGrant("plink_abc", g) {
		t.Fatal("a freshly issued grant must validate")
	}
	if s.validRenderGrant("plink_abc", "") {
		t.Fatal("empty grant must be rejected")
	}
	if s.validRenderGrant("plink_abc", "deadbeef") {
		t.Fatal("forged grant must be rejected")
	}
	if s.validRenderGrant("plink_other", g) {
		t.Fatal("grant must be bound to its link id")
	}
}

func TestRenderGrantDependsOnSecret(t *testing.T) {
	s1 := testServer()
	s2 := &Server{cfg: &config.Config{SessionSecret: "different", RenderDomain: "render.example.com"}}
	g := s1.issueRenderGrant("plink_abc")
	if s2.validRenderGrant("plink_abc", g) {
		t.Fatal("grant must not validate under a different SessionSecret")
	}
}

func TestOnRenderDomain(t *testing.T) {
	s := testServer()
	cases := map[string]bool{
		"render.example.com":        true,
		"render.example.com:8080":   true,
		"alice.preview.example.com": false,
		"console.example.com":       false,
	}
	for host, want := range cases {
		r := httptest.NewRequest("GET", "http://"+host+"/p/x/y", nil)
		r.Host = host
		if got := s.onRenderDomain(r); got != want {
			t.Errorf("onRenderDomain(%q)=%v want %v", host, got, want)
		}
	}
	// Unset render domain disables isolation (treated as on-render).
	s.cfg.RenderDomain = ""
	r := httptest.NewRequest("GET", "http://anything/p/x/y", nil)
	if !s.onRenderDomain(r) {
		t.Fatal("unset RenderDomain must disable isolation")
	}
}

func TestWrapperHTMLSandboxing(t *testing.T) {
	htmlOut := renderWrapperHTML("plink_1", "aps_sec", "html", "report.html", true, "grant123")
	if !strings.Contains(htmlOut, "<iframe") || !strings.Contains(htmlOut, "sandbox") {
		t.Error("html must render in a sandboxed iframe")
	}
	if strings.Contains(htmlOut, "allow-scripts") || strings.Contains(htmlOut, "allow-same-origin") {
		t.Error("sandbox must not relax scripts or same-origin")
	}
	if !strings.Contains(htmlOut, "/p/plink_1/aps_sec/raw?g=grant123") {
		t.Error("wrapper must point the iframe at the grant-bearing raw URL")
	}

	svgOut := renderWrapperHTML("plink_2", "aps_s2", "svg", "logo.svg", false, "g2")
	if !strings.Contains(svgOut, "<img") {
		t.Error("svg must be downgraded to an image element")
	}
	if strings.Contains(svgOut, "<iframe") {
		t.Error("svg must not be framed as a document")
	}
	if strings.Contains(svgOut, "dl=1") {
		t.Error("download link must be absent when allowDownload is false")
	}
}
