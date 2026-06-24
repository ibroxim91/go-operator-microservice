package utils

import (
	"crypto/sha1"
	"testing"
)

func TestBuildTourSlugStripsOAuthToken(t *testing.T) {
	tourID := "0x430A00000023000000073D"
	urlWithToken := "https://api.example.com/search?ADULT=2&STATEINC=30&oauth_token=secret-token"
	urlWithoutToken := "https://api.example.com/search?ADULT=2&STATEINC=30"

	slugWithToken := BuildTourSlug(urlWithToken, tourID)
	slugWithoutToken := BuildTourSlug(urlWithoutToken, tourID)

	if slugWithToken == "" {
		t.Fatal("expected non-empty slug")
	}
	if slugWithToken != slugWithoutToken {
		t.Fatalf("oauth_token must not affect slug: %q != %q", slugWithToken, slugWithoutToken)
	}
}

func TestBuildTourSlugUsesTourOperatorID(t *testing.T) {
	requestURL := "https://api.example.com/search?ADULT=2&STATEINC=30"
	slugA := BuildTourSlug(requestURL, "tour-a")
	slugB := BuildTourSlug(requestURL, "tour-b")

	if slugA == slugB {
		t.Fatal("different tour operator IDs must produce different slugs")
	}
}

func TestBuildTourSlugBase64RoundTrip(t *testing.T) {
	slug := BuildTourSlug("https://api.example.com/search?ADULT=2", "tour-id")
	if slug == "" {
		t.Fatal("expected non-empty slug")
	}

	hashBytes, err := DecodeTourSlugHash(slug)
	if err != nil {
		t.Fatalf("failed to decode slug: %v", err)
	}
	if len(hashBytes) != sha1.Size {
		t.Fatalf("expected %d bytes, got %d", sha1.Size, len(hashBytes))
	}
}
