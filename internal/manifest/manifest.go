package manifest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gjantsch/stowage/pkg/blobstore"
	"gopkg.in/yaml.v3"
)

type HashAlgorithm string

const (
	HashSHA256 HashAlgorithm = "sha256"
	HashBLAKE3 HashAlgorithm = "blake3"
)

type CompressionType string

const (
	CompressionNone CompressionType = "none"
	CompressionGZIP CompressionType = "gzip"
	CompressionZSTD CompressionType = "zstd"
	CompressionLZ4  CompressionType = "lz4"
)

type EncryptionType string

const (
	EncryptionNone   EncryptionType = "none"
	EncryptionAES256 EncryptionType = "aes256gcm"
)

// Base64Bytes is a []byte that serialises as a base64 string in both JSON and
// YAML, keeping manifests human-readable and compact.
type Base64Bytes []byte

func (b Base64Bytes) MarshalJSON() ([]byte, error) {
	if b == nil {
		return []byte("null"), nil
	}
	return json.Marshal(base64.StdEncoding.EncodeToString(b))
}

func (b *Base64Bytes) UnmarshalJSON(data []byte) error {
	var s *string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == nil {
		*b = nil
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(*s)
	if err != nil {
		return fmt.Errorf("Base64Bytes: %w", err)
	}
	*b = decoded
	return nil
}

func (b Base64Bytes) MarshalYAML() (any, error) {
	if b == nil {
		return nil, nil
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (b *Base64Bytes) UnmarshalYAML(value *yaml.Node) error {
	if value.Tag == "!!null" {
		*b = nil
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value.Value)
	if err != nil {
		return fmt.Errorf("Base64Bytes: %w", err)
	}
	*b = decoded
	return nil
}

// Chunk is a single stored piece of the original file. Order is explicit so
// the manifest remains correct across serialisation boundaries.
// Hash is computed from the final transformed bytes (compressed + encrypted)
// — the exact bytes stored by BlobStore.
type Chunk struct {
	Order int              `json:"order" yaml:"order"`
	Hash  blobstore.Hash32 `json:"hash"  yaml:"hash"`
}

// Manifest describes how to reconstruct a stored file.
type Manifest struct {
	HashAlgorithm HashAlgorithm   `json:"hash_algorithm" yaml:"hash_algorithm"`
	Compression   CompressionType `json:"compression"    yaml:"compression"`
	Encryption    EncryptionType  `json:"encryption"     yaml:"encryption"`
	// EncryptedDEK holds the Data Encryption Key encrypted with the caller's MEK.
	// Empty when Encryption is EncryptionNone.
	EncryptedDEK Base64Bytes `json:"encrypted_dek,omitempty" yaml:"encrypted_dek,omitempty"`
	// EncryptedFilename is the original file base name encrypted with the MEK
	// using AES-256-GCM (nonce || ciphertext+tag, base64-encoded).
	// Nil when Encryption is EncryptionNone or when no filename was provided.
	EncryptedFilename Base64Bytes `json:"encrypted_filename,omitempty" yaml:"encrypted_filename,omitempty"`
	// IntegrityHash is the hash of the original plaintext stream, computed
	// before compression and encryption. Verified on full retrieval.
	IntegrityHash blobstore.Hash32 `json:"integrity_hash" yaml:"integrity_hash"`
	// Chunks are ordered by Chunk.Order.
	Chunks []Chunk `json:"chunks" yaml:"chunks"`
}

// ToJSON serialises the manifest to indented JSON.
func (m *Manifest) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserialises a manifest from JSON.
func FromJSON(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ToYAML serialises the manifest to YAML.
func (m *Manifest) ToYAML() ([]byte, error) {
	return yaml.Marshal(m)
}

// FromYAML deserialises a manifest from YAML.
func FromYAML(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
