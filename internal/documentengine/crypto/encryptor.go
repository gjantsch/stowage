package crypto

import (
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

// Encryptor encrypts and decrypts streams using envelope encryption.
// A random DEK is generated per Encrypt call; the DEK is itself encrypted
// with the caller-provided MEK and returned as EncryptedDEK for storage
// in the Manifest.
type Encryptor interface {
	// Encrypt wraps r with an encrypting reader. Reading from the returned
	// reader yields ciphertext. The EncryptedDEK must be stored in the
	// Manifest so Decrypt can recover the DEK.
	// mek must be exactly 32 bytes (AES-256).
	Encrypt(r io.Reader, mek []byte) (ciphertext io.Reader, encryptedDEK []byte, err error)

	// Decrypt wraps r with a decrypting reader. Reading from the returned
	// reader yields plaintext. encryptedDEK is the value stored in the Manifest.
	// mek must be exactly 32 bytes and must match the key used during Encrypt.
	Decrypt(r io.Reader, encryptedDEK []byte, mek []byte) (io.Reader, error)

	// Algorithm returns the EncryptionType constant for the Manifest.
	Algorithm() manifest.EncryptionType
}
