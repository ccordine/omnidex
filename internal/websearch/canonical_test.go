package websearch

import "testing"

func TestCanonicalizeURLRemovesTrackingAndNormalizesIdentity(t *testing.T) {
	got, err := CanonicalizeURL("HTTPS://Example.COM:443/a/../docs/?utm_medium=email&b=2&a=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/docs/?a=1&b=2" {
		t.Fatalf("canonical URL=%q", got)
	}
	first := candidateID(got)
	second, err := CanonicalizeURL("https://example.com/docs/?b=2&a=1&gclid=ignored")
	if err != nil {
		t.Fatal(err)
	}
	if first != candidateID(second) {
		t.Fatalf("stable IDs differ: %q != %q", first, candidateID(second))
	}
}

func TestCanonicalizeURLRejectsCredentialsFragmentsAndLocalLiterals(t *testing.T) {
	for _, rawURL := range []string{
		"https://user:secret@example.com/",
		"https://example.com/#fragment",
		"http://localhost/",
		"http://127.0.0.1/",
		"http://[::1]/",
		"http://10.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
	} {
		if _, err := CanonicalizeURL(rawURL); err == nil {
			t.Fatalf("CanonicalizeURL(%q) unexpectedly succeeded", rawURL)
		}
	}
}
