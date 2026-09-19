package crypto

import (
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

type NoneEncryptor struct{}

func NewNone() Encryptor { return NoneEncryptor{} }

func (NoneEncryptor) Algorithm() manifest.EncryptionType { return manifest.EncryptionNone }

func (NoneEncryptor) Encrypt(r io.Reader, mek []byte) (ciphertext io.Reader, encryptedDEK []byte, err error) {
	return r, nil, nil
}

func (NoneEncryptor) Decrypt(r io.Reader, encryptedDEK []byte, mek []byte) (io.Reader, error) {
	return r, nil
}
