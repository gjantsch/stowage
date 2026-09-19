# stowage CLI — Usage Examples

`stowage` is a command-line tool for storing, restoring, and managing files
through the DocumentEngine pipeline (compress → encrypt → chunk → BlobStore).

## Quick start

```sh
# Build the binary
go build -o stowage ./cmd/stowage

# Store a file with default settings (zstd compression, AES-256-GCM encryption)
KEY=$(openssl rand -hex 32)
./stowage store \
  -root /data/blobs \
  -input photo.jpg \
  -manifest photo.json \
  -key "$KEY"

# Restore it
./stowage retrieve \
  -root /data/blobs \
  -manifest photo.json \
  -output photo_restored.jpg \
  -key "$KEY"

# Verify the round-trip
diff photo.jpg photo_restored.jpg && echo "identical"
```

---

## Subcommands

### `store`

```
stowage store -root <dir> -input <file> -manifest <file> [flags]
```

Compress, encrypt, and chunk a file into the BlobStore. Writes a manifest
describing how to reconstruct the file.

| Flag | Default | Description |
|---|---|---|
| `-root` | — | BlobStore root directory **(required)** |
| `-input` | — | File to store **(required)** |
| `-manifest` | — | Path to write the manifest **(required)** |
| `-compression` | `zstd` | `none` \| `gzip` \| `zstd` \| `lz4` |
| `-encryption` | `aes256gcm` | `none` \| `aes256gcm` |
| `-key` | — | Hex-encoded 32-byte MEK (required when encryption ≠ `none`) |
| `-hash` | `blake3` | `sha256` \| `blake3` |
| `-chunk-size` | `4194304` | Chunk size in bytes (default 4 MiB) |
| `-format` | `json` | Manifest format: `json` \| `yaml` |

**Examples:**

```sh
# Store without encryption
./stowage store \
  -root /data/blobs \
  -input archive.tar \
  -manifest archive.json \
  -encryption none \
  -compression zstd

# Store with encryption, YAML manifest
KEY=$(openssl rand -hex 32)
./stowage store \
  -root /data/blobs \
  -input database.sql \
  -manifest database.yaml \
  -format yaml \
  -compression lz4 \
  -encryption aes256gcm \
  -key "$KEY"

# Store with smaller chunks (1 MiB) for large files
./stowage store \
  -root /data/blobs \
  -input video.mp4 \
  -manifest video.json \
  -chunk-size 1048576 \
  -encryption none
```

---

### `retrieve`

```
stowage retrieve -root <dir> -manifest <file> -output <file> [flags]
```

Restore a file from the BlobStore. The algorithm choices (compression,
encryption, hash) are read from the manifest — no need to specify them again.
Verifies the integrity hash after full reassembly.

| Flag | Default | Description |
|---|---|---|
| `-root` | — | BlobStore root directory **(required)** |
| `-manifest` | — | Manifest file **(required)** |
| `-output` | — | Path to write the restored file **(required)** |
| `-key` | — | Hex-encoded 32-byte MEK (required when manifest uses encryption) |

**Examples:**

```sh
# Restore an unencrypted file
./stowage retrieve \
  -root /data/blobs \
  -manifest archive.json \
  -output archive_restored.tar

# Restore an encrypted file
./stowage retrieve \
  -root /data/blobs \
  -manifest database.yaml \
  -output database_restored.sql \
  -key "$KEY"
```

The command prints `retrieved: N chunks, integrity ok` on success. If the
stored data has been tampered with, it exits non-zero with an integrity error.

---

### `erase`

```
stowage erase -root <dir> -manifest <file>
```

Remove all BlobStore chunks referenced by a manifest. The manifest file itself
is not deleted — keep or remove it separately.

| Flag | Default | Description |
|---|---|---|
| `-root` | — | BlobStore root directory **(required)** |
| `-manifest` | — | Manifest file **(required)** |

**Example:**

```sh
./stowage erase \
  -root /data/blobs \
  -manifest archive.json

# erased: 3 chunks
```

---

### `gc`

```
stowage gc -root <dir>
```

Scan the BlobStore root and remove orphaned temporary files (`_tmp_*`) and
stale lock files (`_lock_*`) left by interrupted operations. Committed blobs
are never touched.

| Flag | Default | Description |
|---|---|---|
| `-root` | — | BlobStore root directory **(required)** |

**Example:**

```sh
./stowage gc -root /data/blobs

# gc: removed 2 orphan files
```

Run this after a crash or interrupted store operation to reclaim disk space.

---

## Key management

The MEK (Master Encryption Key) must be exactly 32 bytes, passed as 64
lowercase hex characters. Generate one with:

```sh
openssl rand -hex 32
# → e3b0c44298fc1c149afbf4c8996fb924...
```

Store it securely (a secrets manager, password manager, or encrypted file).
**The key is not stored anywhere by stowage.** Without it, encrypted files
cannot be retrieved.

---

## Manifest files

A manifest is a JSON (default) or YAML document that describes how a stored
file is split and how to reassemble it. Example:

```json
{
  "hash_algorithm": "blake3",
  "compression": "zstd",
  "encryption": "aes256gcm",
  "encrypted_dek": "base64encodedDEK==",
  "integrity_hash": "4af36c...",
  "chunks": [
    { "order": 0, "hash": "9ea6e4..." },
    { "order": 1, "hash": "2f5318..." }
  ]
}
```

Keep the manifest alongside the BlobStore (or in a separate location for
added security). Without the manifest, stored chunks cannot be reassembled.
