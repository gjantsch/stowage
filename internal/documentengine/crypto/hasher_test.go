package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/gjantsch/stowage/internal/manifest"
)

type expected struct {
	hash      string
	algorithm manifest.HashAlgorithm
}

func TestHashers(t *testing.T) {
	tests := []struct {
		name     string
		hasher   Hasher
		content  []byte
		expected expected
	}{
		{
			name:    "empty content sha256",
			hasher:  NewSHA256(),
			content: []byte{},
			expected: expected{
				hash:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				algorithm: manifest.HashSHA256,
			},
		},
		{
			name:    "empty content blake3",
			hasher:  NewBLAKE3(),
			content: []byte{},
			expected: expected{
				hash:      "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
				algorithm: manifest.HashBLAKE3,
			},
		},
		{
			name:    "short content sha256",
			hasher:  NewSHA256(),
			content: []byte("123"),
			expected: expected{
				hash:      "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
				algorithm: manifest.HashSHA256,
			},
		},
		{
			name:    "short content blake3",
			hasher:  NewBLAKE3(),
			content: []byte("123"),
			expected: expected{
				hash:      "b3d4f8803f7e24b8f389b072e75477cdbcfbe074080fb5e500e53e26e054158e",
				algorithm: manifest.HashBLAKE3,
			},
		},
		{
			name:    "long content sha256",
			hasher:  NewSHA256(),
			content: []byte("the quick brown fox"),
			expected: expected{
				hash:      "9ecb36561341d18eb65484e833efea61edc74b84cf5e6ae1b81c63533e25fc8f",
				algorithm: manifest.HashSHA256,
			},
		},
		{
			name:    "long content blake3",
			hasher:  NewBLAKE3(),
			content: []byte("the quick brown fox"),
			expected: expected{
				hash:      "6589539add86d80ad66e9a12104dc414cdeaf3f2e20486ca63cba42324af4856",
				algorithm: manifest.HashBLAKE3,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.hasher.Algorithm(); got != test.expected.algorithm {
				t.Fatalf("Algorithm() = %q, want %q", got, test.expected.algorithm)
			}

			hash := test.hasher.New()
			if _, err := hash.Write(test.content); err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			computedHash := hash.Sum(nil)
			hashString := hex.EncodeToString(computedHash)
			if len(computedHash) != 32 {
				t.Errorf("invalid hash length %d", len(computedHash))
			}
			if hashString != test.expected.hash {
				t.Errorf("digest = %q, want %q", hashString, test.expected.hash)
			}
		})
	}
}
