# CLI Improvements — Elaboration

**Status:** Draft for review  
**Scope:** `cmd/stowage` and `internal/manifest` — minimal, contained changes

---

## Motivation

The current CLI requires every flag to be spelled out on every invocation. For a
single-user install this creates unnecessary friction: the storage root, compression
algorithm, and hash choice rarely change between runs, yet the user must repeat them
every time. The improvements below introduce a config file that holds user-level
defaults, an environment variable for the encryption key, interactive key prompting,
log-level control, and encrypted filename storage in the manifest.

---

## 1 — Config file (`~/.stowage`)

### Location and format

The config file lives at `$HOME/.stowage`. YAML is used because it is human-readable,
supports inline comments, and is already a project dependency (`gopkg.in/yaml.v3`).

```yaml
# ~/.stowage

root: ~/stowage-data        # BlobStore root directory
compression: zstd           # none | gzip | zstd | lz4
encryption: none            # none | aes256gcm
hash: blake3                # sha256 | blake3
chunk_size: 4194304         # bytes (default 4 MiB)
format: json                # manifest output format: json | yaml
log_level: info             # debug | info | warn | error | silent
deduplicate: false          # enable BlobStore deduplication
```

All fields are optional. A missing field falls back to the same hardcoded default
the CLI uses today. A missing file is silently ignored — the tool works without one.

### Resolution order (lowest → highest priority)

```
config file < environment variable < CLI flag
```

A flag explicitly passed on the command line always wins. This makes one-off
overrides trivial without touching the config.

### Path expansion

`~` in the `root` value is expanded to `$HOME` at load time so the file is
portable across user accounts.

---

## 2 — Encryption key handling

The encryption key must never be stored in the config file. It is a 32-byte
secret; placing it alongside non-sensitive defaults (compression algorithm,
log level) would make the file unsafe to share, back up to cloud storage, or
commit to a dotfiles repository. Instead, three escalating mechanisms are supported:

### 2.1 — `STOWAGE_KEY` environment variable (recommended for scripting)

```sh
export STOWAGE_KEY=<64-hex-chars>
./stowage store -root /data -input photo.jpg -manifest photo.json -encryption aes256gcm
```

The key is read from the environment, never written to disk, and not visible in
shell history. Suitable for cron jobs, CI pipelines, and scripted workflows.

### 2.2 — Interactive prompt (recommended for interactive use)

When `encryption` is `aes256gcm` (either from config or a `-encryption` flag),
`STOWAGE_KEY` is not set, and no `-key` flag was passed, the CLI prompts:

```
Enter encryption key (hex, 32 bytes): 
```

Input is read with terminal echo disabled (via `golang.org/x/term`) so the key
does not appear on screen. This keeps keys out of shell history while remaining
ergonomic for interactive sessions.

### 2.3 — `-key` flag (power-user / CI override)

The existing `-key` flag remains available. It overrides both the env var and
the prompt. Use it when the key must come from a secret injected directly into
a command (e.g., a secrets-manager CLI pipe).

### Key resolution order

```
-key flag > STOWAGE_KEY env var > interactive prompt
```

If none of the three yields a valid 32-byte key and encryption is required,
the command exits with a clear error.

---

## 3 — Log level

The `log_level` field (config) and a `-log-level` flag (all subcommands) control
the verbosity of the `log/slog` output. The flag overrides the config value.

| Level | What is shown |
|---|---|
| `debug` | All slog messages including internal engine details |
| `info` | Normal operation messages (store ok, retrieve ok, erase ok, gc ok) |
| `warn` | Warnings only (e.g. rollback erase failures) |
| `error` | Errors only |
| `silent` | No log output at all |

Default is `info`, matching the current behaviour. Users who find the slog lines
noisy in daily use can set `log_level: silent` in `~/.stowage`.

Implementation note: a `slog.NewTextHandler` is constructed at startup with the
resolved level, then set as the default logger via `slog.SetDefault`. All existing
engine log calls remain unchanged.

---

## 4 — Encrypted filename in the manifest

### Motivation

The filename is sensitive metadata. A manifest directory whose files are named
`bank-investments.xls`, `my-secret-passwords`, or `personal-diary.docx` is a map
of everything worth stealing — even if the file bytes are completely opaque. An
attacker who obtains the manifests should learn nothing: not the content, not the
file type, not whether the data is worth pursuing.

The solution is to encrypt the original filename using the same MEK that protects
the file content, and store the result in the manifest.

### Design

A new optional field `EncryptedFilename` of type `Base64Bytes` is added to
`internal/manifest.Manifest`:

```go
// EncryptedFilename is the original file name encrypted with the MEK using
// AES-256-GCM. Nil when encryption is EncryptionNone or when the filename
// was not provided at store time.
EncryptedFilename Base64Bytes `json:"encrypted_filename,omitempty" yaml:"encrypted_filename,omitempty"`
```

The field is `omitempty` so unencrypted manifests remain clean and backward-compatible.

### Encryption primitive

AES-256-GCM is used directly with the MEK (no separate DEK for the filename — the
filename is a small, fixed-size value and does not benefit from per-operation key
rotation). The wire format matches the existing `sealKey`/`openKey` helpers already
implemented in `internal/documentengine/crypto/aes256gcm.go`:

```
12-byte random nonce || GCM ciphertext+tag
```

To avoid introducing a dependency on the internal crypto package into the CLI, two
small unexported helpers are added to `cmd/stowage/key.go`:

```go
func encryptFilename(name string, mek []byte) ([]byte, error)
func decryptFilename(ciphertext []byte, mek []byte) (string, error)
```

These replicate the same AES-256-GCM nonce-prefix pattern using only `crypto/aes`
and `crypto/cipher` from the stdlib — no new dependencies.

### Behaviour by command

**`store`:**
- The original filename (base name only, no directory path) is encrypted with the
  MEK and stored in `Manifest.EncryptedFilename`.
- If `encryption` is `none`, `EncryptedFilename` is left nil. There is no point
  encrypting the filename when the file content is also in plaintext.

**`retrieve`:**
- If `EncryptedFilename` is present and a MEK is available, the filename is
  decrypted and printed to stdout alongside the integrity result:
  ```
  retrieved: 3 chunks, integrity ok (original: photo.jpg)
  ```
- If `-output` is omitted and `EncryptedFilename` is present, the decrypted
  filename is used as the default output path in the current directory. This makes
  `retrieve` self-contained: given only the manifest and the MEK, the file is
  restored under its original name with no extra flags.

**`erase`:** no change — `EncryptedFilename` is ignored.

### Security properties

| Scenario | What an attacker learns |
|---|---|
| Manifest obtained, no MEK | Nothing — chunk hashes, algorithm names only |
| Manifest obtained, MEK known | Filename + full file content via retrieve |
| BlobStore obtained, no manifest | Nothing — chunks are opaque, unordered blobs |

This matches the security model of the file content: the MEK is the single secret
that protects everything. Losing it means losing access; keeping it means full
recovery.

---

## 5 — `stowage init` command

`stowage init` writes a `~/.stowage` file scaffolded with all supported fields
and their defaults, each annotated with an inline comment. If the file already
exists the command exits with an error rather than overwriting it silently.

```sh
$ stowage init
config written to /Users/alice/.stowage
```

The generated file is immediately usable and human-readable. Users edit it once
and never touch CLI flags for routine operations again.

This command requires no flags and no BlobStore access. Its only side effect is
writing the config file.

---

## 6 — New dependency

Interactive key prompting requires reading from the terminal without echo.
The stdlib `os.Stdin` does not expose this; the standard Go solution is:

```
golang.org/x/term
```

This is a well-maintained sub-repository package with no transitive dependencies.
It handles cross-platform terminal raw-mode correctly on Linux, macOS, and Windows.

---

## 7 — Files affected

| File | Change |
|---|---|
| `internal/manifest/manifest.go` | Add `EncryptedFilename Base64Bytes` field |
| `cmd/stowage/config.go` | New — `Config` struct, `loadConfig()`, YAML parsing, path expansion |
| `cmd/stowage/key.go` | New — `resolveKey()`; `encryptFilename()` / `decryptFilename()` helpers |
| `cmd/stowage/main.go` | Load config at startup, set slog level, add `init` case |
| `cmd/stowage/init.go` | New — `runInit()`: scaffold `~/.stowage` |
| `cmd/stowage/store.go` | Apply config defaults; delegate key to `resolveKey()`; encrypt filename |
| `cmd/stowage/retrieve.go` | Apply config defaults; delegate key to `resolveKey()`; decrypt and print filename; use as default output path |
| `cmd/stowage/erase.go` | Apply config defaults |
| `cmd/stowage/gc.go` | Apply config defaults |
| `go.mod` / `go.sum` | Add `golang.org/x/term` |
| `docs/CLI.md` | Document config file, env var, prompt, log level, `init`, encrypted filename |

---

## 8 — What is explicitly out of scope

- **Key storage in config** — the config file must remain safe to share and back up.
- **Encrypting the filename when encryption is `none`** — no MEK is available in
  that mode; mixing plaintext content with an encrypted name would be inconsistent
  and offer false comfort.
- **Storing the full file path** — only the base name is stored. Directory structure
  is the caller's concern and may itself be sensitive.
- **Multiple named profiles** — the single-user use case does not justify the complexity.
- **Remote or database-backed config** — `~/.stowage` is a local file, nothing more.

