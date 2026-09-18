package crypto

import (
	"crypto/sha256"
	"hash"

	"github.com/gjantsch/stowage/internal/manifest"
)

type SHA256Hasher struct{}

func NewSHA256() SHA256Hasher {
	return SHA256Hasher{}
}

func (SHA256Hasher) New() hash.Hash {
	return sha256.New()
}

func (SHA256Hasher) Algorithm() manifest.HashAlgorithm {
	return manifest.HashSHA256
}
