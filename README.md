# Stowage

Stowage is a Go document storage engine built around content-addressable blob
storage. It is designed to store large files safely on a local filesystem and
to provide the foundation for encrypted, compressed, chunked document storage.

The project separates physical storage from document processing:

```text
DocumentEngine
		compresses, encrypts, chunks, and reassembles documents
				|
BlobStore
		stores and retrieves opaque content-addressed blobs
				|
Filesystem
		sharded paths, staging files, and garbage collection
```

Stowage is a library rather than a network service. A future CLI can use it
for local workflows, and a separate REST or gRPC service can add
authentication, metadata, and remote access without coupling those concerns
to the storage engine.

## Why Stowage

The storage layer is designed for files that should be streamed instead of
loaded into memory or exposed as raw filesystem paths. Each stored blob is
identified by a 32-byte digest of its content. This gives callers stable
identities, naturally supports deduplication, and allows the physical layout
to remain independent of filenames and document semantics.

The design also treats failed and interrupted writes as normal operating
conditions. Data is written to a temporary staging file and moved into its
final hash-derived location only after the input has been completely read and
hashed. A failed write is cleaned up before the error is returned.

## Current Status

The repository is under active development. The current implementation
contains:

- The public `BlobStore` interface with context-aware store, retrieve, erase,
	and garbage-collection operations.
- `Hash32`, a 32-byte digest type with human-readable lowercase hexadecimal
	JSON serialization.
- Hash-based two-level filesystem sharding.
- UUID-backed staging writes and atomic promotion to a permanent path.
- Unit and integration tests for sharding and staging behavior.

The DocumentEngine public API and its compression, encryption, and chunking
implementations are represented in the repository structure and design
documentation, but are not yet complete. The README describes the target
architecture as well as the functionality currently available.

## Storage Layers

### Layer 0: BlobStore

BlobStore knows nothing about files, encryption, compression, or users. It
accepts and returns streams of opaque bytes:

```go
type BlobStore interface {
		Store(ctx context.Context, r io.Reader) (Hash32, error)
		Retrieve(ctx context.Context, hash Hash32) (io.ReadCloser, error)
		Erase(ctx context.Context, hash Hash32) error
		GC(ctx context.Context) (int, error)
}
```

The hash returned by `Store` is the blob's identity. Callers use that hash to
retrieve or erase the blob; they never need to know its path on disk.

### Layer 1: DocumentEngine

DocumentEngine will build file semantics on top of BlobStore. Its processing
pipeline is intentionally ordered so compression happens before encryption:

```text
Store:    input -> compress -> encrypt -> chunk -> BlobStore
Retrieve: BlobStore -> reassemble -> decrypt -> decompress -> output
```

The engine records the choices needed to reverse the pipeline in a manifest.
Each chunk is identified by the hash of its final transformed bytes, while an
integrity hash of the original plaintext stream provides end-to-end
verification after restoration.

The planned manifest contains:

- The hash algorithm used for chunks and integrity verification.
- The compression and encryption algorithms.
- The encrypted data-encryption key, when encryption is enabled.
- An ordered list of chunk hashes.
- The integrity hash of the original plaintext stream.

Both SHA-256 and BLAKE3 are intended to produce the same `Hash32` shape.
Compression is planned to support gzip, zstd, lz4, and no compression. These
algorithms are isolated behind interfaces so implementations can be replaced
without changing the engine's orchestration logic.

## Filesystem Behavior

For a blob whose hexadecimal hash begins with `4af3`, the default layout is:

```text
<root>/4a/f3/4af3...<64 hex characters>....bin
```

The first two pairs of hexadecimal characters form directory levels. Sharding
keeps individual directories bounded as the repository grows and avoids
operating-system performance problems caused by very large flat directories.

Writes follow this sequence:

1. Create a uniquely named `_tmp_<uuid>` file in the staging area.
2. Stream the input into it while computing the content hash.
3. Create the hash-derived shard directory.
4. Rename the completed temporary file into its final location.
5. Remove the temporary file if any earlier step fails.

`Retrieve` resolves only committed, final paths. A staging file is never a
valid blob, including after a process crash. `Erase` is a synchronous hard
delete; soft-delete behavior belongs in a higher-level metadata service.

The `GC` operation is intended to remove abandoned staging files and other
unreferenced physical data according to the owning storage implementation.

## Repository Layout

```text
.
├── cmd/stowage/                 # CLI entry point (planned local interface)
├── docs/
│   ├── AGENDA.md                # Project topics and decisions to address
│   └── ELABORATION.md           # Detailed architecture and design direction
├── internal/
│   ├── blobstore/
│   │   ├── gc/                  # Garbage-collection implementation
│   │   ├── shard/               # Hash-derived filesystem paths
│   │   └── staging/             # Temporary writes and atomic promotion
│   ├── documentengine/
│   │   ├── chunk/               # Chunking implementation
│   │   ├── compress/            # Compression implementations
│   │   └── crypto/              # Hashing and encryption implementations
│   └── manifest/                # Shared manifest model (planned)
├── pkg/
│   ├── blobstore/               # Public BlobStore contract and Hash32
│   └── documentengine/          # Public DocumentEngine contract (planned)
├── tests/integration/           # Tests using a real filesystem
├── go.mod                       # Module and dependency declarations
└── Makefile                     # Development commands
```

The `internal/` directory protects implementation details from external
imports. The `pkg/` directory defines public contracts. Constructors should
return interfaces from `pkg/`, allowing concrete implementations to evolve
without exposing filesystem details to callers.

## Design Principles

- **Streaming first:** I/O uses `io.Reader` and `io.ReadCloser` so large files
	do not need to fit in memory.
- **Atomic operations:** A successful store represents a complete committed
	blob; failed stores leave no staging file behind.
- **Content addressing:** Hashes provide stable identities and support
	deduplication at higher layers.
- **Interface-first boundaries:** Blob storage, compression, hashing, and
	encryption are replaceable behind small contracts.
- **Context-aware I/O:** Public operations accept `context.Context` for
	cancellation, deadlines, and tracing.
- **Explicit errors:** Callers should use `errors.Is` and `errors.As`, rather
	than matching error strings. `ErrNotFound` and `ErrHashMismatch` are part of
	the BlobStore contract.
- **No user concerns in the engine:** Authentication, authorization, metadata,
	and soft deletion belong to an outer service.

## Development

The project requires Go 1.26. Clone the repository and run the complete test
suite with:

```sh
git clone https://github.com/gjantsch/stowage.git
cd stowage
go test ./...
```

The tests intentionally use real temporary directories for filesystem
behavior. Mocking the filesystem would miss the atomic rename, sharding, and
cleanup guarantees that are central to this project.

## Future Interfaces

The planned `stowage` CLI will store and restore local files and serialize
manifests as JSON or YAML. A separate REST or gRPC server may later add:

- Authentication and authorization.
- Transactional metadata for file ownership and lifecycle state.
- Manifest persistence.
- Remote streaming uploads and downloads.
- User-facing soft deletion and retention policies.

Those features are deliberately outside this repository's storage-engine
boundary. The engine should remain usable as a small, embeddable library.

## Documentation

See [docs/ELABORATION.md](docs/ELABORATION.md) for the detailed architecture,
manifest model, security direction, and future service design. [docs/AGENDA.md](docs/AGENDA.md)
tracks topics for ongoing development.
