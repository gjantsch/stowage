package crypto

import (
	"hash"

	"github.com/gjantsch/stowage/internal/manifest"
	"github.com/zeebo/blake3"
)

type BLAKE3Hasher struct{}

func NewBLAKE3() BLAKE3Hasher {
	return BLAKE3Hasher{}
}

func (BLAKE3Hasher) New() hash.Hash {
	return blake3.New()
}

func (BLAKE3Hasher) Algorithm() manifest.HashAlgorithm {
	return manifest.HashBLAKE3
}
