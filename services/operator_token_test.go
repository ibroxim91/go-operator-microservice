package services

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestSignURLAddsAndReplacesToken(t *testing.T) {
	t.Setenv("SAMO_TOUR_BASE_URL", "https://samo.example/export")
	t.Setenv("SAMO_TOUR_OAUTH_TOKEN", "new-secret")

	signed, err := SignURL("samo_tour", "https://samo.example/export?STATEINC=7&oauth_token=old-secret")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(signed)
	if got := u.Query().Get("oauth_token"); got != "new-secret" {
		t.Fatalf("token = %q, want new-secret", got)
	}
	if u.Query().Get("STATEINC") != "7" {
		t.Fatalf("other params lost: %s", signed)
	}
}

func TestSignURLRejectsForeignHost(t *testing.T) {
	t.Setenv("SAMO_TOUR_BASE_URL", "https://samo.example/export")
	t.Setenv("SAMO_TOUR_OAUTH_TOKEN", "secret")

	for _, raw := range []string{
		"https://evil.example/export?a=1",
		"http://samo.example/export?a=1",
		"https://samo.example.evil.example/export?a=1",
		"https://samo.example/other?a=1",
		"https://user@samo.example/export?a=1",
		"//evil.example/export",
	} {
		if signed, err := SignURL("samo_tour", raw); err == nil {
			t.Errorf("SignURL(%q) = %q, want error", raw, signed)
		}
	}
}

func TestSignURLUnknownOperator(t *testing.T) {
	if _, err := SignURL("no_such_operator", "https://x/?a=1"); err == nil {
		t.Fatal("expected error for operator without token")
	}
}

func TestStripToken(t *testing.T) {
	got := StripToken("https://samo.example/export?STATEINC=7&oauth_token=secret")
	if strings.Contains(got, "secret") || !strings.Contains(got, "STATEINC=7") {
		t.Fatalf("StripToken = %q", got)
	}
	plain := "https://samo.example/export?STATEINC=7"
	if StripToken(plain) != plain {
		t.Fatal("URL without token must stay unchanged")
	}
}

func TestRedactError(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://samo.example/export?oauth_token=secret&a=1", Err: errors.New("timeout")}
	red := RedactError(err)
	if strings.Contains(red.Error(), "secret") {
		t.Fatalf("token leaked: %s", red)
	}
	if !strings.Contains(err.Error(), "secret") {
		t.Fatal("original error must not be mutated")
	}
	if RedactToken(`url="https://x/?oauth_token=abc123"`) != `url="https://x/?oauth_token=REDACTED"` {
		t.Fatal("RedactToken did not redact")
	}
}

func TestSameEndpoint(t *testing.T) {
	base := "https://samo.example/export"
	if !sameEndpoint("https://samo.example/export?a=1&oauth_token=x", base) {
		t.Fatal("same endpoint must match")
	}
	if sameEndpoint("https://old-samo.example/export?a=1", base) {
		t.Fatal("cached URL on an old host must not be reused")
	}
}
