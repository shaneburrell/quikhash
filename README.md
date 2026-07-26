# QuikHash

**FastCDC + BLAKE3 content-addressed hasher** that can also reconstruct files from chunk sets.

[![CI](https://github.com/shaneburrell/quikhash/actions/workflows/ci.yml/badge.svg)](https://github.com/shaneburrell/quikhash/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/shaneburrell/quikhash.svg)](https://pkg.go.dev/github.com/shaneburrell/quikhash)

Plenty of FastCDC libraries exist. Almost none ship a polished single-binary CLI where **chunking, strong hashing, and reconstruction** are first-class operations. QuikHash does.

```bash
quikhash hash ./video.mp4 -o video.quikhash.json
quikhash verify ./video.mp4 --manifest video.quikhash.json
quikhash dump ./video.mp4 --manifest video.quikhash.json --out chunks/
quikhash reconstruct --chunks chunks/ --manifest video.quikhash.json --out video.restored.mp4
quikhash diff old.json new.json
```

## Why QuikHash?

| Capability | What you get |
|------------|--------------|
| Streaming FastCDC | Content-defined chunks that survive inserts/shifts |
| BLAKE3 | Fast cryptographic digests for each chunk + whole file |
| Manifests | JSON with offsets, lengths, digests — easy to inspect and diff |
| Dump | Extract content-addressed chunk files by digest |
| Reconstruct | Rebuild a file from any complete chunk set + manifest |
| Diff | Show only changed chunks between two manifests |
| Parallel + resume | Directory hashing and dump/reconstruct use workers; dump skips existing chunks |

## Install

### Homebrew

```bash
brew install shaneburrell/tap/quikhash
```

The formula is maintained in [`shaneburrell/homebrew-tap`](https://github.com/shaneburrell/homebrew-tap) and tracks GitHub Releases.

### Prebuilt binaries

Download from [Releases](https://github.com/shaneburrell/quikhash/releases) for macOS, Linux, and Windows (amd64 & arm64).

### From source

Requires [Go 1.22+](https://go.dev/dl/).

```bash
go install github.com/shaneburrell/quikhash/cmd/quikhash@latest
```

Or clone and build:

```bash
git clone https://github.com/shaneburrell/quikhash.git
cd quikhash
make build
./bin/quikhash --version
```

Cross-compile everything:

```bash
make build-all   # → dist/ for darwin/linux/windows × amd64/arm64
```

## Commands

### `hash` — produce a manifest

```bash
quikhash hash <path>                         # JSON manifest → stdout
quikhash hash <path> -o file.quikhash.json   # write to file
quikhash hash ./dir -o ./manifests           # parallel, one manifest per file
```

### `verify` — check a file against a manifest

```bash
quikhash verify <path> --manifest m.json
```

### `dump` — extract individual chunks

```bash
quikhash dump <path> --out chunks/
quikhash dump <path> --manifest m.json --out chunks/   # skip re-hash
```

Chunk files are named by their BLAKE3 hex digest. `--resume` (default) skips chunks that already exist with matching content.

### `reconstruct` — rebuild from chunks + manifest

```bash
quikhash reconstruct --chunks chunks/ --manifest m.json --out file
```

Writes to a temp file, verifies the whole-file BLAKE3 digest, then atomically renames.

### `diff` — changed chunks only

```bash
quikhash diff manifest1.json manifest2.json
quikhash diff a.json b.json --json
```

## Flags

| Flag | Description |
|------|-------------|
| `--avg-size` | FastCDC average chunk size (default `64K`) |
| `--min-size` | FastCDC minimum chunk size (default `16K`) |
| `--max-size` | FastCDC maximum chunk size (default `256K`) |
| `--json` | Machine-readable JSON output |
| `-q, --quiet` | Suppress progress bar |
| `--workers` | Parallelism for directory hash / dump / reconstruct |

Sizes accept `K`/`KiB`, `M`/`MiB`, `G`/`GiB` suffixes.

## Manifest format

```json
{
  "version": 1,
  "path": "/data/video.mp4",
  "size": 10485760,
  "digest": "<blake3-256 hex of whole file>",
  "avg_size": 65536,
  "min_size": 16384,
  "max_size": 262144,
  "chunks": [
    { "offset": 0, "length": 42188, "digest": "<blake3-256 hex>" }
  ]
}
```

## Differentiation

Most CDC tools stop at “print the chunks.” QuikHash treats **reconstruction + strong crypto hash in one tiny static binary** as the product — the same FastCDC + BLAKE3 core used by [QuikSync](https://github.com/shaneburrell/quiksync).

## Development

```bash
make build       # → bin/quikhash
make test        # unit + e2e
make test-race
make cover       # coverage gate
make build-all   # cross-compile to dist/
```

Tagged releases (`v*`) are published via GoReleaser.

## License

[MIT](LICENSE) © 2026 Shane Burrell
