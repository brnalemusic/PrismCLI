package main

import (
	"bytes"
	"testing"
)

func TestDPAPIEncryption(t *testing.T) {
	originalData := []byte("Gemini-Super-Secret-API-Key-12345")

	// Encrypt
	encrypted, err := encrypt(originalData)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if len(encrypted) == 0 {
		t.Fatalf("Encrypted data is empty")
	}

	if bytes.Equal(originalData, encrypted) {
		t.Fatalf("Encrypted data is identical to plain text")
	}

	// Decrypt
	decrypted, err := decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if !bytes.Equal(originalData, decrypted) {
		t.Errorf("Decrypted data %s does not match original %s", string(decrypted), string(originalData))
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DefaultModel != "gemini-3.5-flash" {
		t.Errorf("Expected default model gemini-3.5-flash, got %s", cfg.DefaultModel)
	}
	if cfg.LauncherShortcut != "Ctrl+Space" {
		t.Errorf("Expected shortcut Ctrl+Space, got %s", cfg.LauncherShortcut)
	}
}
