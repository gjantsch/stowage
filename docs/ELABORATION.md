# Stowage — Document Storage Engine

## Architecture Overview

The project is organized in three independent layers. Each layer depends only
on the layer below it and exposes a clean interface upward.

### Layer 0 — BlobStore (this repository)

Responsible for storing and retrieving opaque blobs on disk. It has no
knowledge of encryption, compression, or file semantics.

**Interface:**
- `Store(reader io.Reader) (hash32, error)` — writes the stream to disk,
  returns the content hash as its identity
- `Retrieve(hash32) (io.Reader, error)` — opens a read stream for the given hash
- `Erase(hash32) error` — removes the blob from disk

**Internals:** path sharding, atomic staging writes, garbage collection.

### Layer 1 — DocumentEngine (this repository)

Sits on top of Layer 0. Responsible for applying transformations to a file
before storage: encryption, compression, and chunking. The caller provides
a reader and a processing config; the library splits, transforms, and hands
each chunk to Layer 0.

Transformations are applied in a fixed order to maximise compression ratio
and security:

```
[Input Stream] → Compress → Encrypt → Chunk → [BlobStore per chunk]
[BlobStore per chunk] → Reassemble → Decrypt → Decompress → [Output Stream]
```

**Hashing:** Layer 1 computes the hash of each chunk and passes it to Layer 0
as the blob identity. The hash algorithm is a per-file choice made by the
caller and recorded in the `Manifest`. Both SHA-256 and BLAKE3 are supported;
both produce a 32-byte digest so Layer 0 treats them identically.

The hash implementation is isolated behind an internal `Hasher` interface so
the underlying package can be swapped without touching Layer 1 logic. The
current implementation uses `github.com/zeebo/blake3` for BLAKE3.

**Compression:** All compression operates on streams (`io.Reader`/`io.Writer`)
— no temporary files are written. The algorithm is a per-file choice recorded
in the `Manifest`. Three algorithms are supported:

| Algorithm | Package | Use case |
|---|---|---|
| `gzip` | `compress/gzip` (stdlib) | Default interoperable option; widely supported on Unix systems |
| `zstd` | `github.com/klauspost/compress/zstd` | Best ratio/speed tradeoff; recommended for large files |
| `lz4` | `github.com/pierrec/lz4/v4` | Maximum speed; suited for already-compressed content |
| `none` | — | Skip compression; use when content is already compressed or encrypted |

All algorithms are isolated behind a `Compressor` interface so adding or
swapping an implementation never touches DocumentEngine logic:

```go
type CompressionType string

const (
    CompressionNone CompressionType = "none"
    CompressionGZIP CompressionType = "gzip"
    CompressionZSTD CompressionType = "zstd"
    CompressionLZ4  CompressionType = "lz4"
)
```

```go
type HashAlgorithm string

const (
    HashSHA256 HashAlgorithm = "sha256"
    HashBLAKE3 HashAlgorithm = "blake3"
)

// Hash32 is a 32-byte digest. It implements MarshalJSON/UnmarshalJSON
// using lowercase hex encoding for human-readable manifests.
type Hash32 [32]byte

// Chunk represents a single stored piece of the original file.
// Order is explicit so the manifest remains correct across serialization
// boundaries and future partial-manifest operations.
// Hash is computed from the final transformed bytes (compressed + encrypted
// where applicable) — these are the exact bytes stored by BlobStore and
// used as the blob's filename on disk.
type Chunk struct {
    Order int
    Hash  Hash32
}

type Manifest struct {
    HashAlgorithm HashAlgorithm
    Compression   CompressionType
    Encryption    EncryptionType
    EncryptedDEK  []byte
    // IntegrityHash is the hash of the original plaintext stream, computed
    // before compression and encryption using HashAlgorithm. Verified after
    // full retrieval to detect accidental corruption end-to-end.
    IntegrityHash Hash32
    // Chunks are ordered by Chunk.Order, all hashed with HashAlgorithm
    Chunks        []Chunk
}
```

**Returns:** a `Manifest` struct describing how to reconstruct the original
file — encryption used, compression used, ordered list of chunk hashes.

**Input to restore:** the same `Manifest` is passed back to retrieve and
reassemble the original file from BlobStore.

### Layer 2 — User Interfaces (separate projects)

Built on top of DocumentEngine. Two separate interfaces:

- **CLI tool (`stowage`)** — for local use and testing. Outputs the `Manifest` as JSON
  or YAML so it can be stored and used later to restore the file.
- **REST API server** — a separate repository. Adds authentication, a
  metadata database, and remote access over HTTP/gRPC. Stores `Manifest`
  structs in the database on behalf of authenticated users.

## Core concept

### Engine Core & Physical Layout

The physical engine manages how files sit on the drive to prevent operating system slowdowns.

- **Path Sharding:** Files are split across subdirectories based on their cryptographic hashes (e.g., `/data/4a/f3/4af36c.dat`). This keeps folders small and avoids OS folder file-limit crashes.
- **Atomic Writing:** Files are written to a temporary `staging/` folder first. Once the upload finishes completely, the engine performs an atomic filesystem swap to move it to its permanent location. This prevents file corruption from partial uploads.
- ** File Chunking:** Files may be stored in chunks scattered on different folders. This will be an optional feature and is defined on the storage process.

### Core Engine API (The Internal Contract)

The storage engine runs as an isolated subsystem using a strict programming interface. It exposes clean, programmatic functions like `Store(stream)`, `Retrieve(hash32)`, and `Erase(hash32)` that return data streams rather than raw disk paths.

### Operation Semantics

**`Store()` — synchronous, atomic, leave-no-trash guarantee**

`Store()` generates a UUID at the start of the operation and writes to a
temporary file named `_tmp_<uuid>` in the staging area. The content is
streamed and hashed on the fly — the hash is not known until the write
completes. Only when the stream is fully written and the hash computed is
the file renamed to its final sharded path. On any failure, the
`_tmp_<uuid>` file is deleted before the error is returned. The caller is
guaranteed that either the file is fully committed or nothing is written —
there is no partial state left on disk.

For chunked files, each chunk follows the same `_tmp_<uuid>` pattern
individually, but none of the chunks are moved to their final sharded
location until all chunks of the file have been successfully written and
hashed. If any single chunk fails, all staging files for that operation are
cleaned up and the error is returned. The file is either fully committed as
a complete set of chunks or not committed at all.

The `_tmp_` prefix makes leftovers trivially identifiable by the GC: any file
matching `_tmp_*` in the storage tree is a failed or interrupted write and
safe to delete.

**`Retrieve()` — returns not-found for any non-committed file**

`Retrieve()` only resolves files at their final sharded path. A file still
in staging (i.e. named `_tmp_<hash>`) is treated as non-existent and returns
a not-found error. This is safe by design: `Store()` is synchronous, so a
caller that received a successful return from `Store()` is guaranteed the file
is committed and retrievable. A staging file visible to `Retrieve()` would
only occur due to a crash, and in that case the correct response is not-found.

**`Erase()` — synchronous hard delete**

`Erase()` immediately removes the file from disk and returns only after the
deletion is confirmed. There is no soft-delete or deferred removal at this
layer. Higher-level systems (such as the REST API server) that need soft-delete
semantics must implement them in their own metadata layer and call `Erase()`
only when the hard delete is appropriate.

---

> **The sections below describe a future, separate project.**
> The REST API server is a standalone repository that consumes this library
> as a dependency. Nothing in this repository implements or depends on it.

---

## REST API Server _(future — separate repository)_

The REST API server is a storage service built on top of this library. It adds
user-facing concerns that are out of scope for the storage engine itself:
authentication, a metadata database, and remote network access.

### Transactional Metadata Layer

The metadata layer tracks where files live and what they are, stored in an
embedded transactional database like **SQLite** or **RocksDB**.

- **State Separation:** The database tracks if a file is `STAGING`, `ACTIVE`, or `DELETED`.
- **Data Deduplication:** By indexing files by their hash, the server can detect
  if the exact same file is uploaded twice and point both metadata records to a
  single physical file on disk to save space.
- **Manifest Storage:** The `Manifest` produced by Layer 1 is persisted in the
  database on behalf of the authenticated user, removing the need for the caller
  to manage the manifest file manually.

### Network & Security Isolation

The REST API or gRPC server acts strictly as an outer routing envelope.

- **Streaming Pipelines:** The network layer streams bytes directly from incoming
  HTTP/gRPC requests into the engine's staging buffers, keeping RAM usage near
  zero even for multi-gigabyte files.
- **Decoupled Auth:** The storage engine has no knowledge of users or passwords.
  The gRPC/REST gateway handles authentication tokens, checks permissions, and
  invokes the storage engine only if the request is valid.

## Security and Privacy

### Encryption with Two Key Levels

To support both a **generic key** (system-wide) and a **user-provided key**, we should use an industry standard called **Envelope Encryption**.

- **The Problem:** Re-encrypting a 10GB file because a user changed their password is too slow.
- **The Solution:**
    1. The storage engine generates a random, unique **Data Encryption Key (DEK)** for every single file.
    2. The file chunks are encrypted using this DEK.
    3. The DEK itself is encrypted using a **Master Encryption Key (MEK)**. The MEK can be the system's generic key _or_ derived from the user's password.
    4. The encrypted DEK is stored safely in the metadata database.

### Security Chunking & Sharding

Breaking the file into chunks prevents an intruder from reading anything, even if they bypass the operating system's permissions.

- **Fixed-Size Chunks:** We split files into chunks of a fixed size (for example, 4MB).
- **No File Extensions:** Chunks are saved on disk named only by their hash or a random UUID (e.g., `/data/chunks/ab3f-91e2-44cc.bin`).
- **Obfuscation:** An intruder looking at the folder will only see millions of identical 4MB files with random names. Without the metadata database, it is mathematically impossible to know which chunks belong together or what order they go in.

### Solving the Orphan Chunk Problem

When chunking files, an upload transaction requires writing multiple chunk files to disk before updating the database. If the server crashes on chunk #3 of 5, chunks #1 and #2 become "orphaned."

We can prevent and clean this up using two core strategies:

Strategy A: Prevention (The Staging Manifest)

Never write chunks directly into the main production folder.

1. **The Staging Area:** Create a temporary folder for the current upload transaction (e.g., `/staging/upload_tx_99/`).
2. **Write Chunks:** Write all encrypted chunks into this isolated folder.
3. **The Database Commit:** Only when _all_ chunks are successfully written, we write the metadata to the database and move the chunks to the main storage folder.
4. **Crash Recovery:** If the system crashes mid-upload, the database will have no record of `upload_tx_99`. On reboot, the engine simply deletes everything inside the `/staging/` folder. It is safe because no active files live there.

Strategy B: Cleanup (The Mark-and-Sweep Garbage Collector)

For chunks that somehow bypass prevention, we run a background process called a **Garbage Collector (GC)**. Because the chunks are encrypted and have random names, the GC cannot look inside the chunk to see who owns it. Instead, it must talk to the database.

- **Step 1 (Mark):** The engine scans the physical `/data/chunks/` directory and creates a list of all chunk IDs found on the hard drive.
- **Step 2 (Compare):** The engine queries the metadata database: _"Give me a list of all chunk IDs that belong to active files."_
- **Step 3 (Sweep):** The engine subtracts the database list from the physical list. Any chunk ID found on the hard drive but **not** found in the database is an orphan. The engine safely deletes it.

---

Visualizing the Secure Storage Pipeline

```
[User File]
     │
     ▼
[Compressor] ──► Reduces size before encryption
     │
     ▼
[Encryptor] ───► Encrypts compressed stream with a unique DEK
     │
     ▼
[Chunker] ─────► Splits into fixed-size pieces, hashes each chunk
     │
     ▼
[Staging Folder] ─► Holds chunks until all are written successfully
     │
     ▼
[Atomic Commit] ──► Chunks move to /data/ sharded by hash
```

---

## System Design Guidelines

### Project Structure

Follows the Standard Go Project Layout combined with Clean Architecture's
dependency rule: dependencies point inward, never outward.

```
go-document-storage-engine/
├── pkg/
│   ├── blobstore/          # BlobStore public interface
│   └── documentengine/     # DocumentEngine public interface
├── internal/
│   ├── blobstore/          # BlobStore implementation
│   │   ├── shard/          # path sharding logic
│   │   ├── staging/        # atomic write / _tmp_ file logic
│   │   └── gc/             # garbage collector
│   ├── documentengine/     # DocumentEngine implementation
│   │   ├── compress/       # Compressor interface + implementations
│   │   ├── crypto/         # Hasher + Encryptor interfaces + implementations
│   │   └── chunk/          # chunker logic
│   └── manifest/           # Manifest, Chunk, Hash32 shared types
├── cmd/
│   └── stowage/             # CLI tool entry point
├── docs/
└── tests/
    └── integration/        # integration tests against real filesystem
```

Rules:
- **`internal/`** — nothing outside this module can import it. Enforces the
  boundary between BlobStore and DocumentEngine implementation details.
- **`pkg/`** — the public contract. Only interfaces and types, no concrete
  implementations.
- **`cmd/stowage/`** — entry point only, minimal logic. The only place where a
  YAML/JSON library is a direct dependency.

### Interface-First Design

Define behaviour as interfaces before writing implementations. Concrete types
live in `internal/` and are returned via constructor functions in `pkg/`.
This is what makes `Compressor`, `Hasher`, and `Encryptor` swappable without
touching DocumentEngine logic.

```go
// pkg/blobstore/blobstore.go
type BlobStore interface {
    Store(ctx context.Context, r io.Reader) (Hash32, error)
    Retrieve(ctx context.Context, hash Hash32) (io.ReadCloser, error)
    Erase(ctx context.Context, hash Hash32) error
}
```

```go
// internal/blobstore/fs/store.go — returned as the BlobStore interface
func New(cfg Config) blobstore.BlobStore { ... }
```

### Context Everywhere

Every public method that touches I/O takes a `context.Context` as its first
argument. This enables cancellation, deadlines, and tracing at zero cost when
not needed by the caller.

### Error Design

Use sentinel errors and `errors.Is` / `errors.As` — never string matching.
Callers can distinguish error types without parsing messages.

```go
var ErrNotFound    = errors.New("blob not found")
var ErrHashMismatch = errors.New("stored hash does not match content")
```

### Configuration

Each package exposes a `Config` struct. No global state, no `init()` functions.
Constructors take a config, making every component independently testable with
different settings.

```go
type Config struct {
    RootDir   string
    ChunkSize int64  // bytes, default 4MB
}
```

### Testing Strategy

| Layer | Test type | Approach |
|---|---|---|
| `internal/compress` | Unit | Table-driven round-trip compress/decompress |
| `internal/chunk` | Unit | Boundary conditions: single-byte, exact chunk size |
| `internal/blobstore/fs` | Integration | Real `os.TempDir()`, no mocks |
| `pkg/documentengine` | Integration | Real BlobStore + real transforms |
| `cmd/stowage` | E2E | Store a file, read back manifest, restore, diff |

The filesystem is never mocked — BlobStore's entire value is its interaction
with disk. Mocking it would test nothing real.

### Observability

Structured logging via `log/slog` (stdlib since Go 1.21) from day one — zero
external dependency, consistent field names across both layers:

```go
slog.Info("blob stored", "hash", hash, "size_bytes", n)
slog.Error("staging cleanup failed", "path", tmpPath, "err", err)
```

Tracing hooks are wired as no-ops via `context` propagation so the REST API
server can plug in OpenTelemetry later without modifying the engine.

### Dependency Management

- One `go.mod` at the repository root.
- `go.sum` is committed and kept up to date.
- Third-party dependencies are pinned to explicit versions — no `latest`.
- Current third-party dependencies: `github.com/zeebo/blake3`,
  `github.com/klauspost/compress`, `github.com/pierrec/lz4/v4`.
