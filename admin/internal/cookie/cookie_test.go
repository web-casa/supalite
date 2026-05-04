package cookie

import (
	"strings"
	"testing"
	"time"
)

const validKey = "0123456789abcdef0123456789abcdef" // 32 bytes

func TestNewKeyLengthEnforced(t *testing.T) {
	if _, err := New("short", 0); err == nil {
		t.Fatal("expected error for short key, got nil")
	}
	if _, err := New(validKey, 0); err != nil {
		t.Fatalf("32-byte key should be accepted: %v", err)
	}
}

func TestIssueVerifyRoundtrip(t *testing.T) {
	s, _ := New(validKey, time.Hour)
	tok, err := s.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := s.Verify(tok); err != nil {
		t.Fatalf("freshly issued cookie should verify: %v", err)
	}
}

func TestVerifyTamperedSignature(t *testing.T) {
	s, _ := New(validKey, time.Hour)
	tok, _ := s.Issue()

	// Flip the last byte of the MAC.
	tampered := tok[:len(tok)-1]
	if tok[len(tok)-1] == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}
	if err := s.Verify(tampered); err == nil {
		t.Fatal("tampered MAC must be rejected")
	}
}

func TestVerifyTamperedPayload(t *testing.T) {
	s, _ := New(validKey, time.Hour)
	tok, _ := s.Issue()

	// Flip a char in the encoded payload (before the dot).
	dot := strings.LastIndexByte(tok, '.')
	if dot <= 0 {
		t.Fatal("malformed test cookie")
	}
	tampered := "X" + tok[1:dot] + tok[dot:]
	if err := s.Verify(tampered); err == nil {
		t.Fatal("tampered payload must be rejected")
	}
}

func TestVerifyMalformed(t *testing.T) {
	s, _ := New(validKey, time.Hour)
	cases := []string{
		"",
		"no-dot",
		".only-mac-no-payload",
		"only-payload-no-mac.",
	}
	for _, c := range cases {
		if err := s.Verify(c); err == nil {
			t.Errorf("%q should be rejected", c)
		}
	}
}

func TestVerifyExpired(t *testing.T) {
	// Negative TTL → cookie expires immediately. We have to construct
	// it via Issue() then check. Negative TTL gets normalized to default
	// in New(), so build a Signer with a tiny positive TTL and sleep.
	s, _ := New(validKey, time.Millisecond)
	tok, _ := s.Issue()
	time.Sleep(2 * time.Second) // crosses the 1-sec resolution Unix() deadline
	if err := s.Verify(tok); err == nil {
		t.Fatal("expired cookie must be rejected")
	}
}

func TestVerifyDifferentKey(t *testing.T) {
	a, _ := New(validKey, time.Hour)
	b, _ := New("ffffffffffffffffffffffffffffffff", time.Hour)
	tok, _ := a.Issue()
	if err := b.Verify(tok); err == nil {
		t.Fatal("cookie signed with key A must not verify under key B")
	}
}

func TestDefaultTTLApplied(t *testing.T) {
	s, _ := New(validKey, 0)
	if s.ttl != DefaultTTL {
		t.Fatalf("ttl=0 should normalize to DefaultTTL, got %v", s.ttl)
	}
}
