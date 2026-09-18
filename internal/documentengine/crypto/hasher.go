package crypto

import (
	"hash"

	"github.com/gjantsch/stowage/internal/manifest"
)

type Hasher interface {
	New() hash.Hash
	Algorithm() manifest.HashAlgorithm
}
