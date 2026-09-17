package manifest_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/gjantsch/stowage/internal/manifest"
	"github.com/gjantsch/stowage/pkg/blobstore"
)

func makeHash(b byte) blobstore.Hash32 {
	var h blobstore.Hash32
	for i := range h {
		h[i] = b
	}
	return h
}

func sampleManifest() *manifest.Manifest {
	return &manifest.Manifest{
		HashAlgorithm: manifest.HashSHA256,
		Compression:   manifest.CompressionZSTD,
		Encryption:    manifest.EncryptionAES256,
		EncryptedDEK:  manifest.Base64Bytes{0x01, 0x02, 0x03},
		IntegrityHash: makeHash(0xab),
		Chunks: []manifest.Chunk{
			{Order: 0, Hash: makeHash(0x11)},
			{Order: 1, Hash: makeHash(0x22)},
		},
	}
}

func manifestsEqual(a, b *manifest.Manifest) bool {
	if a.HashAlgorithm != b.HashAlgorithm ||
		a.Compression != b.Compression ||
		a.Encryption != b.Encryption ||
		a.IntegrityHash != b.IntegrityHash ||
		len(a.EncryptedDEK) != len(b.EncryptedDEK) ||
		len(a.Chunks) != len(b.Chunks) {
		return false
	}
	for i := range a.EncryptedDEK {
		if a.EncryptedDEK[i] != b.EncryptedDEK[i] {
			return false
		}
	}
	for i := range a.Chunks {
		if a.Chunks[i].Order != b.Chunks[i].Order ||
			a.Chunks[i].Hash != b.Chunks[i].Hash {
			return false
		}
	}
	return true
}

func TestJSON_RoundTrip(t *testing.T) {
	orig := sampleManifest()

	data, err := orig.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	got, err := manifest.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}

	if !manifestsEqual(orig, got) {
		t.Errorf("JSON round-trip mismatch\norig: %+v\ngot:  %+v", orig, got)
	}
}

func TestYAML_RoundTrip(t *testing.T) {
	orig := sampleManifest()

	data, err := orig.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}

	got, err := manifest.FromYAML(data)
	if err != nil {
		t.Fatalf("FromYAML: %v", err)
	}

	if !manifestsEqual(orig, got) {
		t.Errorf("YAML round-trip mismatch\norig: %+v\ngot:  %+v", orig, got)
	}
}

func TestJSON_NilEncryptedDEK(t *testing.T) {
	orig := &manifest.Manifest{
		HashAlgorithm: manifest.HashBLAKE3,
		Compression:   manifest.CompressionNone,
		Encryption:    manifest.EncryptionNone,
		IntegrityHash: makeHash(0xff),
		Chunks:        []manifest.Chunk{{Order: 0, Hash: makeHash(0x01)}},
	}

	data, err := orig.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	got, err := manifest.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if !manifestsEqual(orig, got) {
		t.Errorf("JSON nil-DEK round-trip mismatch")
	}
}

func TestYAML_NilEncryptedDEK(t *testing.T) {
	orig := &manifest.Manifest{
		HashAlgorithm: manifest.HashBLAKE3,
		Compression:   manifest.CompressionGZIP,
		Encryption:    manifest.EncryptionNone,
		IntegrityHash: makeHash(0xcc),
		Chunks:        []manifest.Chunk{{Order: 0, Hash: makeHash(0xdd)}},
	}

	data, err := orig.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}
	got, err := manifest.FromYAML(data)
	if err != nil {
		t.Fatalf("FromYAML: %v", err)
	}
	if !manifestsEqual(orig, got) {
		t.Errorf("YAML nil-DEK round-trip mismatch")
	}
}

func TestJSON_HashEncoding(t *testing.T) {
	orig := sampleManifest()
	data, err := orig.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	h := makeHash(0xab)
	want := hex.EncodeToString(h[:])
	if !bytes.Contains(data, []byte(want)) {
		t.Errorf("JSON does not contain hex integrity hash %q\nJSON: %s", want, data)
	}
}

func TestYAML_HashEncoding(t *testing.T) {
	orig := sampleManifest()
	data, err := orig.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}
	h := makeHash(0xab)
	want := hex.EncodeToString(h[:])
	if !bytes.Contains(data, []byte(want)) {
		t.Errorf("YAML does not contain hex integrity hash %q\nYAML: %s", want, data)
	}
}

func TestJSON_EncryptedDEK_IsBase64(t *testing.T) {
	orig := sampleManifest()
	data, err := orig.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	// {0x01, 0x02, 0x03} base64-encodes to "AQID"
	if !bytes.Contains(data, []byte("AQID")) {
		t.Errorf("JSON EncryptedDEK is not base64-encoded\nJSON: %s", data)
	}
}

func TestYAML_EncryptedDEK_IsBase64(t *testing.T) {
	orig := sampleManifest()
	data, err := orig.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}
	if !bytes.Contains(data, []byte("AQID")) {
		t.Errorf("YAML EncryptedDEK is not base64-encoded\nYAML: %s", data)
	}
}

func TestJSON_MultipleChunks_OrderPreserved(t *testing.T) {
	orig := &manifest.Manifest{
		HashAlgorithm: manifest.HashSHA256,
		Compression:   manifest.CompressionLZ4,
		Encryption:    manifest.EncryptionNone,
		IntegrityHash: makeHash(0x10),
		Chunks: []manifest.Chunk{
			{Order: 0, Hash: makeHash(0x01)},
			{Order: 1, Hash: makeHash(0x02)},
			{Order: 2, Hash: makeHash(0x03)},
		},
	}

	data, _ := orig.ToJSON()
	got, err := manifest.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	for i, c := range got.Chunks {
		if c.Order != i {
			t.Errorf("chunk %d: Order = %d, want %d", i, c.Order, i)
		}
	}
}

func TestYAML_MultipleChunks_OrderPreserved(t *testing.T) {
	orig := &manifest.Manifest{
		HashAlgorithm: manifest.HashSHA256,
		Compression:   manifest.CompressionNone,
		Encryption:    manifest.EncryptionNone,
		IntegrityHash: makeHash(0x20),
		Chunks: []manifest.Chunk{
			{Order: 0, Hash: makeHash(0x04)},
			{Order: 1, Hash: makeHash(0x05)},
		},
	}

	data, _ := orig.ToYAML()
	got, err := manifest.FromYAML(data)
	if err != nil {
		t.Fatalf("FromYAML: %v", err)
	}
	for i, c := range got.Chunks {
		if c.Order != i {
			t.Errorf("chunk %d: Order = %d, want %d", i, c.Order, i)
		}
	}
}
