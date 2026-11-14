package utils

import (
	"regexp"
	"testing"
)

func TestSmallAlphaNumericHashMatchesDNSSubdomainRegex(t *testing.T) {
	dnsSubdomain := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

	for _, length := range []int{1, 8, 64, 100} {
		hash, err := SmallAlphaNumericHash("example", length)
		if err != nil {
			t.Fatalf("SmallAlphaNumericHash(%q, %d) returned error: %v", "example", length, err)
		}
		if !dnsSubdomain.MatchString(hash) {
			t.Errorf("SmallAlphaNumericHash(%q, %d) = %q, does not match DNS subdomain regex", "example", length, hash)
		}
		if len(hash) > 64 {
			t.Errorf("SmallAlphaNumericHash(%q, %d) returned %d characters, want at most 64", "example", length, len(hash))
		}
	}
}

func TestSmallAlphaNumericHashRejectsNonPositiveLength(t *testing.T) {
	for _, length := range []int{0, -1} {
		if _, err := SmallAlphaNumericHash("example", length); err == nil {
			t.Errorf("SmallAlphaNumericHash(%q, %d) returned no error", "example", length)
		}
	}
}
