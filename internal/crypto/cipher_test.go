package crypto_test

import (
	"bytes"
	"testing"

	"sqlite-p2p/internal/crypto"
)

func TestGenerateKey(t *testing.T) {
	key1, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("unexpected error generating key: %v", err)
	}
	if len(key1) != crypto.KeySize {
		t.Fatalf("expected key length %d, got %d", crypto.KeySize, len(key1))
	}

	key2, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("unexpected error generating second key: %v", err)
	}
	if bytes.Equal(key1, key2) {
		t.Fatal("two independently generated keys should not be identical")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("Sensitive customer PII: John Doe, +1-555-0199")
	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext must not match plaintext")
	}

	decrypted, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("expected %s, got %s", string(plaintext), string(decrypted))
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("")
	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	decrypted, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Fatalf("expected empty decrypted plaintext, got %s", string(decrypted))
	}
}

func TestEncrypt_UniqueNonces(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("Identical message encrypted twice")
	c1, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("first encryption failed: %v", err)
	}
	c2, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("second encryption failed: %v", err)
	}

	if bytes.Equal(c1, c2) {
		t.Fatal("ciphertexts for identical plaintext must differ due to random nonce")
	}
}

func TestDecrypt_TamperDetection(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("Confidential transaction data")
	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Tamper with the ciphertext
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = crypto.Decrypt(key, tampered)
	if err == nil {
		t.Fatal("expected error decrypting tampered ciphertext, got nil")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()

	plaintext := []byte("Message to protect")
	ciphertext, err := crypto.Encrypt(key1, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	_, err = crypto.Decrypt(key2, ciphertext)
	if err == nil {
		t.Fatal("expected authentication error when decrypting with wrong key, got nil")
	}
}

func TestInvalidKeySize(t *testing.T) {
	shortKey := []byte("short-key")
	plaintext := []byte("test")

	if _, err := crypto.Encrypt(shortKey, plaintext); err == nil {
		t.Fatal("expected error encrypting with invalid key size")
	}

	if _, err := crypto.Decrypt(shortKey, []byte("short-ciphertext-12345678901234567890")); err == nil {
		t.Fatal("expected error decrypting with invalid key size")
	}
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key, _ := crypto.GenerateKey()
	shortCiphertext := []byte("short")

	if _, err := crypto.Decrypt(key, shortCiphertext); err == nil {
		t.Fatal("expected error when ciphertext is shorter than nonce")
	}
}
