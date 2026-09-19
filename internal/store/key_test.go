package store_test

import (
	"strings"
	"testing"

	"crm-sqlite-pear-p2p/internal/store"
)

func TestFormatCustomerKey(t *testing.T) {
	phone := "+91 98765-43210"
	key, err := store.FormatCustomerKey(phone)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prefix := store.CustomerNamespace + ":"
	if !strings.HasPrefix(key, prefix) {
		t.Errorf("expected key to have prefix %s, got %s", prefix, key)
	}

	// Deterministic test: same normalized number yields exact same key
	phoneVariant := "9876543210"
	key2, err := store.FormatCustomerKey(phoneVariant)
	if err != nil {
		t.Fatalf("unexpected error for variant: %v", err)
	}
	if key != key2 {
		t.Errorf("expected deterministic keys to match for %s and %s, got %s vs %s", phone, phoneVariant, key, key2)
	}

	// Base32 part should have no padding '='
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts separated by ':', got %v", parts)
	}
	if strings.Contains(parts[1], "=") {
		t.Errorf("base32 encoded hash should not contain padding '=', got %s", parts[1])
	}
}

func TestFormatCustomerKeyInvalid(t *testing.T) {
	invalidPhones := []string{"", "   ", "abc", "+-()"}
	for _, p := range invalidPhones {
		_, err := store.FormatCustomerKey(p)
		if err == nil {
			t.Errorf("expected error for invalid phone '%s', got nil", p)
		}
	}
}
