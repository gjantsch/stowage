# stowage CLI — Usage Examples

`stowage` is a command-line tool for storing, restoring, and managing files
through the DocumentEngine pipeline (compress → encrypt → chunk → BlobStore).

---

## Configuration file

Most default values can be set in `~/.stowage` so you don't have to repeat
them on every invocation. Create the file with:

```sh
./stowage init
```

The generated file looks like:

```yaml
# BlobStore root directory (required for most commands)
root: ~/stowage-data

# Compression algorithm: none | gzip | zstd | lz4
compression: zstd

# Encryption algorithm: none | aes256gcm
# The encryption key is never stored here.
encryption: none

# Hash algorithm: sha256 | blake3
hash: blake3

# Chunk size in bytes (default 4 MiB)
chunk_size: 4194304

# Manifest output format: json | yaml
format: json

# Log verbosity: debug | info | warn | error | silent
log_level: info

# Enable BlobStore deduplication
deduplicate: false
```

**Resolution order** (highest precedence first): CLI flag → config file → built-in default.

> The encryption key is **never** stored in the config file. Provide it via
> the `-key` flag, the `STOWAGE_KEY` environment variable, or an interactive
> terminal prompt.

---

## Quick start

```sh
# Build the binary
go build -o stowage ./cmd/stowage

# Create config (sets root and preferred defaults)
./stowage init
# edit ~/.stowage: set root to your desired blob directory

# Store a file — encryption defaults to none, compression to zstd
./stowage store -input photo.jpg -manifest photo.json

# Restore it
./stowage restore -manifest photo.json
# output path is resolved from the encrypted filename in the manifest

# Verify the round-trip
diff photo.jpg photo_restored.jpg && echo "identical"
```

---

## Key management

The MEK (Master Encryption Key) must be exactly 32 bytes, passed as 64
lowercase hex characters. Generate one with:

```sh
openssl rand -hex 32
```

Store it securely (a secrets manager, password manager, or encrypted file).
**The key is never stored anywhere by stowage.** Without it, encrypted files
cannot be retrieved.

### Key resolution order

1. `-key <hex>` CLI flag
2. `STOWAGE_KEY` environment variable
3. Interactive terminal prompt (when stdin is a TTY)
4. Error if none of the above apply

```sh
# Pass key as flag
./stowage store -input secret.txt -manifest secret.json \
  -encryption aes256gcm -key "$KEY"

# Pass key via environment variable
STOWAGE_KEY="$KEY" ./stowage store \
  -input secret.txt -manifest secret.json -encryption aes256gcm

# Interactive prompt (key is not echoed)
./stowage store -input secret.txt -manifest secret.json -encryption aes256gcm
# Enter encryption key (hex, 32 bytes): ▌
```

---

## Global flags

| Flag | Default | Description |
|---|---|---|
| `-log-level` | `info` | Log verbosity: `debug` \| `info` \| `warn` \| `error` \| `silent` |

```sh
# Suppress all log output
./stowage -log-level silent store -input data.bin -manifest data.json
```

---

## Subcommands

### `init`

```
stowage init
```

Write a default `~/.stowage` config file. Exits with an error if the file
already exists.

```sh
./stowage init
# config written to: /Users/alice/.stowage
```

---

### `store`

```
stowage store -input <file> -manifest <file> [flags]
```

Compress, encrypt, and chunk a file into the BlobStore. Writes a manifest
describing how to reconstruct the file. When encryption is active, the
original filename is AES-256-GCM encrypted inside the manifest so that file
names like `bank-investments.xls` are not exposed to anyone who can read the
manifest.

| Flag | Config key | Default | Description |
|---|---|---|---|
| `-root` | `root` | — | BlobStore root directory **(required)** |
| `-input` | — | — | File to store **(required)** |
| `-manifest` | — | — | Path to write the manifest **(required)** |
| `-compression` | `compression` | `zstd` | `none` \| `gzip` \| `zstd` \| `lz4` |
| `-encryption` | `encryption` | `none` | `none` \| `aes256gcm` |
| `-key` | — | — | Hex-encoded 32-byte MEK (required when encryption ≠ `none`) |
| `-hash` | `hash` | `blake3` | `sha256` \| `blake3` |
| `-chunk-size` | `chunk_size` | `4194304` | Chunk size in bytes |
| `-format` | `format` | `json` | Manifest format: `json` \| `yaml` |

**Examples:**

```sh
# Minimal: root and compression already set in ~/.stowage
./stowage store -input archive.tar -manifest archive.json

# Explicit flags override config
./stowage store \
  -root /data/blobs \
  -input archive.tar \
  -manifest archive.json \
  -encryption none \
  -compression zstd

# Encrypted — original filename is hidden in the manifest
KEY=$(openssl rand -hex 32)
./stowage store \
  -root /data/blobs \
  -input "my personal diary.docx" \
  -manifest diary.json \
  -encryption aes256gcm \
  -key "$KEY"

# YAML manifest, lz4 compression
./stowage store \
  -root /data/blobs \
  -input database.sql \
  -manifest database.yaml \
  -format yaml \
  -compression lz4 \
  -encryption aes256gcm \
  -key "$KEY"
```

---

### `retrieve`

```
stowage retrieve -manifest <file> [flags]
```

Restore a file from the BlobStore. The algorithm choices (compression,
encryption, hash) are read from the manifest — no need to specify them again.
Verifies the integrity hash after full reassembly.

When the manifest contains an encrypted filename (stored by `store` when
encryption is active), the `-output` path defaults to the decrypted original
filename — you only need to supply `-output` to override it.

| Flag | Config key | Default | Description |
|---|---|---|---|
| `-root` | `root` | — | BlobStore root directory **(required)** |
| `-manifest` | — | — | Manifest file **(required)** |
| `-output` | — | decrypted filename | Path to write the restored file |
| `-key` | — | — | Hex-encoded 32-byte MEK (required when manifest uses encryption) |

**Examples:**

```sh
# No -output needed when filename is in the manifest
./stowage retrieve -manifest diary.json -key "$KEY"
# retrieved: 2 chunks, integrity ok (original: my personal diary.docx)

# Explicit output path
./stowage retrieve \
  -root /data/blobs \
  -manifest archive.json \
  -output archive_restored.tar

# Encrypted file, explicit output
./stowage retrieve \
  -root /data/blobs \
  -manifest database.yaml \
  -output database_restored.sql \
  -key "$KEY"
```

On success the command prints `retrieved: N chunks, integrity ok`. If the
stored data has been tampered with, it exits non-zero with an integrity error.

---

### `erase`

```
stowage erase -manifest <file> [flags]
```

Remove all BlobStore chunks referenced by a manifest. The manifest file itself
is not deleted — keep or remove it separately.

| Flag | Config key | Default | Description |
|---|---|---|---|
| `-root` | `root` | — | BlobStore root directory **(required)** |
| `-manifest` | — | — | Manifest file **(required)** |

**Example:**

```sh
./stowage erase -manifest archive.json
# erased: 3 chunks
```

---

### `gc`

```
stowage gc [flags]
```

Scan the BlobStore root and remove orphaned temporary files (`_tmp_*`) and
stale lock files (`_lock_*`) left by interrupted operations. Committed blobs
are never touched.

| Flag | Config key | Default | Description |
|---|---|---|---|
| `-root` | `root` | — | BlobStore root directory **(required)** |

**Example:**

```sh
./stowage gc
# gc: removed 2 orphan files
```

Run this after a crash or interrupted store operation to reclaim disk space.

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
  "encrypted_filename": "base64encryptedName==",
  "integrity_hash": "4af36c...",
  "chunks": [
    { "order": 0, "hash": "9ea6e4..." },
    { "order": 1, "hash": "2f5318..." }
  ]
}
```

- `encrypted_dek` — the per-file Data Encryption Key, wrapped with the MEK.
  Present only when `encryption` ≠ `none`.
- `encrypted_filename` — the original base name of the stored file, encrypted
  with the MEK using AES-256-GCM. Present only when `encryption` ≠ `none`.
  The plaintext name is never written to the manifest.
- `integrity_hash` — BLAKE3 (or SHA-256) hash of the original plaintext,
  computed before any compression or encryption. Verified on retrieve.

Keep the manifest alongside the BlobStore (or in a separate location for
added security). Without the manifest, stored chunks cannot be reassembled.
