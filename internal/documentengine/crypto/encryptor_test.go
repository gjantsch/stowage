package crypto_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gjantsch/stowage/internal/documentengine/crypto"
	"github.com/gjantsch/stowage/internal/manifest"
)

var validMEK = bytes.Repeat([]byte{0x01}, 32)

func TestEncryptor(t *testing.T) {
	enc := crypto.NewAES256GCM()

	tests := []struct {
		name        string
		plaintext   []byte
		mek         []byte
		decryptMEK  []byte
		wantErr     error
	}{
		{
			name:       "empty plaintext",
			plaintext:  []byte{},
			mek:        validMEK,
			decryptMEK: validMEK,
		},
		{
			name:       "short plaintext",
			plaintext:  []byte("hello encryption"),
			mek:        validMEK,
			decryptMEK: validMEK,
		},
		{
			name:       "multi-frame plaintext",
			plaintext:  bytes.Repeat([]byte("stowage "), 65536/8+1), // > one frame
			mek:        validMEK,
			decryptMEK: validMEK,
		},
		{
			name:       "exact frame boundary",
			plaintext:  bytes.Repeat([]byte("x"), 65536),
			mek:        validMEK,
			decryptMEK: validMEK,
		},
		{
			name:        "wrong MEK on decrypt",
			plaintext:   []byte("secret"),
			mek:         validMEK,
			decryptMEK:  bytes.Repeat([]byte{0x02}, 32),
			wantErr:     crypto.ErrDecryptFailed,
		},
		{
			name:      "MEK too short on encrypt",
			plaintext: []byte("data"),
			mek:       []byte("tooshort"),
			wantErr:   crypto.ErrInvalidMEK,
		},
		{
			name:        "MEK too short on decrypt",
			plaintext:   []byte("data"),
			mek:         validMEK,
			decryptMEK:  []byte("tooshort"),
			wantErr:     crypto.ErrInvalidMEK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt.
			cipherReader, encryptedDEK, err := enc.Encrypt(
				bytes.NewReader(tt.plaintext), tt.mek,
			)
			if tt.wantErr != nil && err != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Encrypt: got err %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Encrypt: unexpected error: %v", err)
			}

			// Drain ciphertext.
			ciphertext, err := io.ReadAll(cipherReader)
			if err != nil {
				t.Fatalf("ReadAll ciphertext: %v", err)
			}

			// Decrypt.
			plainReader, err := enc.Decrypt(
				bytes.NewReader(ciphertext), encryptedDEK, tt.decryptMEK,
			)
			if tt.wantErr != nil && err != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Decrypt: got err %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decrypt: unexpected error: %v", err)
			}

			got, err := io.ReadAll(plainReader)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ReadAll: expected error wrapping %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReadAll: got err %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadAll plaintext: %v", err)
			}

			if !bytes.Equal(got, tt.plaintext) {
				t.Errorf("plaintext mismatch: got %d bytes, want %d bytes",
					len(got), len(tt.plaintext))
			}
		})
	}
}

func TestEncryptor_Algorithm(t *testing.T) {
	enc := crypto.NewAES256GCM()
	if got := enc.Algorithm(); got != manifest.EncryptionAES256 {
		t.Errorf("Algorithm() = %q, want %q", got, manifest.EncryptionAES256)
	}
}

func TestEncryptor_CiphertextDiffersFromPlaintext(t *testing.T) {
	enc := crypto.NewAES256GCM()
	plaintext := []byte("sensitive data that must not appear in ciphertext")

	cr, _, err := enc.Encrypt(bytes.NewReader(plaintext), validMEK)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	ct, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if bytes.Equal(ct, plaintext) {
		t.Error("ciphertext equals plaintext — encryption did nothing")
	}
	if bytes.Contains(ct, plaintext) {
		t.Error("plaintext appears verbatim inside ciphertext")
	}
}

func TestEncryptor_TwoEncryptsProduceDifferentCiphertext(t *testing.T) {
	enc := crypto.NewAES256GCM()
	plaintext := []byte("same input")

	cr1, _, err := enc.Encrypt(bytes.NewReader(plaintext), validMEK)
	if err != nil {
		t.Fatalf("first Encrypt: %v", err)
	}
	ct1, _ := io.ReadAll(cr1)

	cr2, _, err := enc.Encrypt(bytes.NewReader(plaintext), validMEK)
	if err != nil {
		t.Fatalf("second Encrypt: %v", err)
	}
	ct2, _ := io.ReadAll(cr2)

	if bytes.Equal(ct1, ct2) {
		t.Error("two encrypts of the same plaintext produced identical ciphertext — nonce not random")
	}
}

func TestEncryptor_EncryptedDEKDiffersFromMEK(t *testing.T) {
	enc := crypto.NewAES256GCM()
	_, encDEK, err := enc.Encrypt(strings.NewReader("data"), validMEK)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(encDEK, validMEK) {
		t.Error("EncryptedDEK equals the MEK — DEK was not encrypted")
	}
	if len(encDEK) == 0 {
		t.Error("EncryptedDEK is empty")
	}
}
