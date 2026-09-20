package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// resolveKey returns the 32-byte MEK for the given encryption type.
// Resolution order: -key flag → STOWAGE_KEY env var → interactive terminal prompt.
// Returns nil without error when encryptionType is "none".
func resolveKey(flagKey, encryptionType string) ([]byte, error) {
	if encryptionType == "none" {
		return nil, nil
	}

	if flagKey != "" {
		return decodeKey(flagKey, "-key flag")
	}

	if env := os.Getenv("STOWAGE_KEY"); env != "" {
		return decodeKey(env, "STOWAGE_KEY")
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Enter encryption key (hex, 32 bytes): ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}
		return decodeKey(string(raw), "prompt")
	}

	return nil, fmt.Errorf("encryption is %q but no key provided: set -key, STOWAGE_KEY, or run interactively", encryptionType)
}

func decodeKey(hexKey, source string) ([]byte, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("key from %s is not valid hex: %w", source, err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("key from %s must decode to 32 bytes, got %d", source, len(b))
	}
	return b, nil
}

// encryptFilename encrypts the file base name with the MEK using AES-256-GCM.
// Wire format: 12-byte nonce || GCM ciphertext+tag.
func encryptFilename(name string, mek []byte) ([]byte, error) {
	block, err := aes.NewCipher(mek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, []byte(name), nil)
	return append(nonce, ct...), nil
}

// decryptFilename decrypts a filename previously encrypted by encryptFilename.
func decryptFilename(ciphertext, mek []byte) (string, error) {
	block, err := aes.NewCipher(mek)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return "", fmt.Errorf("encrypted filename too short")
	}
	plain, err := gcm.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt filename: %w", err)
	}
	return string(plain), nil
}
