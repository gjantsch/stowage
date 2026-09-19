package crypto

import (
	"fmt"
	"hash"

	"github.com/gjantsch/stowage/internal/manifest"
)

type Hasher interface {
	New() hash.Hash
	Algorithm() manifest.HashAlgorithm
}

func NewHasher(t manifest.HashAlgorithm) (Hasher, error) {
	switch t {
	case manifest.HashSHA256:
		return NewSHA256(), nil
	case manifest.HashBLAKE3:
		return NewBLAKE3(), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %v", t)
	}
}
