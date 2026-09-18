package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

const (
	dekSize       = 32     // AES-256
	nonceSize     = 12     // GCM standard nonce
	frameSize     = 65536  // plaintext bytes per GCM frame
	frameSizeLen  = 4      // bytes used to encode frame length on wire
)

var ErrInvalidMEK = errors.New("encryptor: MEK must be exactly 32 bytes")
var ErrDecryptFailed = errors.New("encryptor: decryption failed")

// AES256GCMEncryptor implements Encryptor using AES-256-GCM with per-frame
// authentication. Each frame is: 4-byte big-endian ciphertext length ||
// 12-byte nonce || GCM ciphertext+tag. The DEK is wrapped with the MEK using
// the same AES-256-GCM scheme: 12-byte nonce || ciphertext+tag.
type AES256GCMEncryptor struct{}

func NewAES256GCM() Encryptor { return AES256GCMEncryptor{} }

func (AES256GCMEncryptor) Algorithm() manifest.EncryptionType {
	return manifest.EncryptionAES256
}

func (AES256GCMEncryptor) Encrypt(r io.Reader, mek []byte) (io.Reader, []byte, error) {
	if len(mek) != dekSize {
		return nil, nil, ErrInvalidMEK
	}

	dek := make([]byte, dekSize)
	if _, err := rand.Read(dek); err != nil {
		return nil, nil, fmt.Errorf("encryptor: generate DEK: %w", err)
	}

	encryptedDEK, err := sealKey(dek, mek)
	if err != nil {
		return nil, nil, fmt.Errorf("encryptor: seal DEK: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(encryptStream(pw, r, dek))
	}()

	return pr, encryptedDEK, nil
}

func (AES256GCMEncryptor) Decrypt(r io.Reader, encryptedDEK []byte, mek []byte) (io.Reader, error) {
	if len(mek) != dekSize {
		return nil, ErrInvalidMEK
	}

	dek, err := openKey(encryptedDEK, mek)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(decryptStream(pw, r, dek))
	}()

	return pr, nil
}

// sealKey encrypts key with mek: nonce || AES-256-GCM(nonce, key, mek).
func sealKey(key, mek []byte) ([]byte, error) {
	block, err := aes.NewCipher(mek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, key, nil)
	return append(nonce, ct...), nil
}

// openKey decrypts an encrypted key produced by sealKey.
func openKey(encryptedKey, mek []byte) ([]byte, error) {
	if len(encryptedKey) < nonceSize {
		return nil, errors.New("encryptedKey too short")
	}
	block, err := aes.NewCipher(mek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := encryptedKey[:nonceSize]
	ct := encryptedKey[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

// encryptStream reads from r in frameSize chunks, encrypts each frame with a
// fresh nonce, and writes length-prefixed frames to w.
func encryptStream(w io.Writer, r io.Reader, dek []byte) error {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	buf := make([]byte, frameSize)
	nonce := make([]byte, nonceSize)

	for {
		n, readErr := io.ReadFull(r, buf)
		if n == 0 && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return readErr
		}

		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		ct := gcm.Seal(nil, nonce, buf[:n], nil)

		// wire format: 4-byte frame length || nonce || ciphertext+tag
		var lenBuf [frameSizeLen]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ct)))
		if _, err := w.Write(lenBuf[:]); err != nil {
			return err
		}
		if _, err := w.Write(nonce); err != nil {
			return err
		}
		if _, err := w.Write(ct); err != nil {
			return err
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	return nil
}

// decryptStream reads length-prefixed frames from r, decrypts each, and
// writes plaintext to w.
func decryptStream(w io.Writer, r io.Reader, dek []byte) error {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	var lenBuf [frameSizeLen]byte
	nonce := make([]byte, nonceSize)

	for {
		_, err := io.ReadFull(r, lenBuf[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: read frame length: %v", ErrDecryptFailed, err)
		}

		ctLen := binary.BigEndian.Uint32(lenBuf[:])
		if _, err := io.ReadFull(r, nonce); err != nil {
			return fmt.Errorf("%w: read nonce: %v", ErrDecryptFailed, err)
		}

		ct := make([]byte, ctLen)
		if _, err := io.ReadFull(r, ct); err != nil {
			return fmt.Errorf("%w: read ciphertext: %v", ErrDecryptFailed, err)
		}

		pt, err := gcm.Open(nil, nonce, ct, nil)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrDecryptFailed, err)
		}
		if _, err := w.Write(pt); err != nil {
			return err
		}
	}
	return nil
}
