# Stowage — Implementation Agenda

Three epics, delivered in order. Each epic is a fully working vertical slice
before the next begins. No epic depends on the next one being started.

---

## Epic 1 — BlobStore (Layer 0)

**Goal:** a working, tested physical storage engine that stores and retrieves
opaque blobs on disk with atomicity and path sharding.

### Stories

**1.1 — Project scaffold**
- Initialise `go.mod` as `github.com/gjantsch/stowage`
- Create directory structure: `pkg/blobstore`, `internal/blobstore`, `docs`, `tests/integration`
- Add `Makefile` targets: `build`, `test`, `lint`

**1.2 — Public interface**
- Define `BlobStore` interface in `pkg/blobstore/blobstore.go`
- Define `Hash32` type with `MarshalJSON`/`UnmarshalJSON` (lowercase hex)
- Define sentinel errors: `ErrNotFound`, `ErrHashMismatch`
- Define `Config` struct (`RootDir`, `ChunkSize`)

**1.3 — Path sharding**
- Implement `internal/blobstore/shard` — derive a sharded path from a `Hash32`
- Example: `4af36c...` → `<root>/4a/f3/4af36c....bin`
- Unit tests: known hashes produce expected paths, no collisions

**1.4 — Atomic write (`Store`)**
- Implement `internal/blobstore/staging` — write to `_tmp_<uuid>`, stream
  and hash on the fly
- Acquire `_lock_<hex>` with `O_CREATE|O_EXCL` before rename; release after
- Scan for lowest free slot (`<hex>.<NNN>.bin`, zero-padded 3 digits, max 999)
  and rename staging file to that slot
- If lock already held by another process, skip staging and return hash
- On any failure: delete `_tmp_<uuid>` before returning error
- Integration tests: store succeeds → file at `<hex>.000.bin`; same content
  stored twice → `<hex>.000.bin` and `<hex>.001.bin` both present; store fails
  mid-write → no file left on disk

**1.5 — Retrieve**
- Implement `Retrieve` — always open slot 0 (`<hex>.000.bin`) directly; no
  directory scan needed
- Returns `ErrNotFound` for any file not at slot 0 (including `_tmp_*`,
  `_lock_*`, and higher slots)
- Integration tests: retrieve after store returns identical bytes; retrieve
  unknown hash returns `ErrNotFound`

**1.6 — Erase**
- Implement `Erase` — LIFO hard delete: scan shard dir for highest slot and
  remove it; `Retrieve` continues serving slot 0 until it is the last copy
- Returns `ErrNotFound` if no slot exists for the hash
- Integration tests: erase after single store removes the file; two stores
  then erase removes slot 1, slot 0 still retrievable; second erase removes
  slot 0; double erase on empty returns `ErrNotFound`

**1.7 — Garbage collector**
- Implement `internal/blobstore/gc` — scan storage tree, delete any file
  matching `_tmp_*` (failed staging) or `_lock_*` (stale lock from crashed
  process)
- Expose `GC(ctx context.Context) (deleted int, err error)` on the `BlobStore`
  interface
- Integration tests: leftover `_tmp_*` and `_lock_*` files are removed;
  committed blobs (`<hex>.<NNN>.bin`) are untouched

**1.8 — Observability**
- Add `slog` structured logging to all operations: store, retrieve, erase, gc
- Log hash, size, path, and error fields consistently

---

## Epic 2 — DocumentEngine (Layer 1)

**Goal:** a working, tested transform pipeline that compresses, encrypts,
chunks a stream and stores each chunk via BlobStore, returning a `Manifest`.
Restore path reassembles, decrypts, decompresses back to the original stream
and verifies `IntegrityHash`.

### Stories

**2.1 — Shared types**
- Define `Manifest`, `Chunk`, `CompressionType`, `EncryptionType`,
  `HashAlgorithm` in `internal/manifest`
- Implement `Manifest` JSON and YAML marshal/unmarshal round-trips
- Unit tests: marshal → unmarshal produces identical struct

**2.2 — Hasher interface and implementations**
- Define `Hasher` interface in `internal/documentengine/crypto`
- Implement SHA-256 (`crypto/sha256` stdlib)
- Implement BLAKE3 (`github.com/zeebo/blake3`)
- Unit tests: known inputs produce expected digests for both algorithms

**2.3 — Compressor interface and implementations**
- Define `Compressor` interface in `internal/documentengine/compress`
- Implement `none`, `gzip`, `zstd`, `lz4`
- Unit tests: table-driven round-trip compress → decompress for all algorithms

**2.4 — Encryptor interface and implementation**
- Define `Encryptor` interface in `internal/documentengine/crypto`
- Implement AES-256-GCM envelope encryption: generate DEK, encrypt stream,
  encrypt DEK with caller-provided MEK
- Unit tests: encrypt → decrypt produces original bytes; wrong MEK returns error

**2.5 — Chunker**
- Implement `internal/documentengine/chunk` — split a stream into fixed-size
  chunks, emit each as an `io.Reader`
- Configurable chunk size (default 4MB)
- Unit tests: boundary conditions — single byte, exact chunk size, multiple
  chunks, empty stream

**2.6 — Store pipeline**
- Wire `Compress → Encrypt → Chunk` pipeline in DocumentEngine
- Compute `IntegrityHash` over the original plaintext stream before any transform
- For each chunk: hash the final transformed bytes, call `BlobStore.Store()`
- Hold all chunks in staging until all succeed, then commit; roll back all on
  any failure
- Return populated `Manifest`
- Integration tests: store a file, verify all chunks exist in BlobStore,
  verify `Manifest` fields

**2.7 — Retrieve pipeline**
- Wire `Reassemble → Decrypt → Decompress` pipeline in DocumentEngine
- Retrieve each chunk from BlobStore in `Chunk.Order` sequence
- After full reassembly: compute hash of plaintext output, compare against
  `Manifest.IntegrityHash` — return `ErrIntegrityMismatch` on failure
- Integration tests: store → retrieve produces bit-identical output;
  tampered chunk returns `ErrIntegrityMismatch`

**2.8 — Erase**
- Implement DocumentEngine `Erase(manifest)` — call `BlobStore.Erase()` for
  each chunk in the manifest
- Integration tests: all chunks removed after erase; double erase is safe

**2.9 — Observability**
- Add `slog` structured logging to pipeline stages: compression ratio,
  chunk count, encryption algorithm, integrity check result

---

## Epic 3 — `stowage` CLI

**Goal:** a working command-line tool that wraps DocumentEngine and outputs
the `Manifest` as JSON or YAML, enabling store and restore from the terminal.

### Stories

**3.1 — CLI scaffold**
- Create `cmd/stowage/main.go` entry point
- Add stdlib `flag` for subcommand routing
- Subcommands: `store`, `retrieve`, `erase`, `gc`

**3.2 — `store` command**
- Accept: input file path, output manifest path, flags for compression
  algorithm, encryption key (optional), hash algorithm, chunk size
- Call `DocumentEngine.Store()`, write `Manifest` to output path as JSON
  (default) or YAML (`--format yaml`)
- Print summary to stdout: chunk count, compressed size, hash algorithm used

**3.3 — `retrieve` command**
- Accept: manifest file path, output file path, decryption key (if encrypted)
- Call `DocumentEngine.Retrieve()`, stream output to the given path
- Print integrity check result to stdout

**3.4 — `erase` command**
- Accept: manifest file path
- Call `DocumentEngine.Erase()`, confirm each chunk removed
- Print count of erased chunks

**3.5 — `gc` command**
- Accept: storage root path
- Call `BlobStore.GC()`, print count of orphan files removed

**3.6 — E2E tests**
- Store a file → read manifest → retrieve to temp path → `diff` original and
  restored — must be identical
- Store with each compression algorithm and verify restore
- Store with encryption, retrieve with correct key → success; wrong key →
  error
- Store large file (>3 chunks), erase, verify BlobStore is empty

**3.7 — Documentation**
- `--help` text for all subcommands
- Add usage examples to `docs/`
