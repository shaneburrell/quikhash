package manifest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shaneburrell/quikhash/internal/chunk"
)

const Version = 1

// Manifest describes a content-addressed file as ordered FastCDC chunks.
type Manifest struct {
	Version int          `json:"version"`
	Path    string       `json:"path,omitempty"`
	Size    int64        `json:"size"`
	Digest  string       `json:"digest"`
	AvgSize uint32       `json:"avg_size"`
	MinSize uint32       `json:"min_size"`
	MaxSize uint32       `json:"max_size"`
	Chunks  []ChunkEntry `json:"chunks"`
}

// ChunkEntry is one content-defined chunk in a manifest.
type ChunkEntry struct {
	Offset uint64 `json:"offset"`
	Length uint32 `json:"length"`
	Digest string `json:"digest"`
}

// FromSignature builds a Manifest from a chunk.FileSignature.
func FromSignature(path string, sig chunk.FileSignature, opt chunk.Options) Manifest {
	opt = normalizeOpt(opt)
	m := Manifest{
		Version: Version,
		Path:    path,
		Size:    sig.Size,
		Digest:  sig.Digest.String(),
		AvgSize: opt.AvgSize,
		MinSize: opt.MinSize,
		MaxSize: opt.MaxSize,
		Chunks:  make([]ChunkEntry, len(sig.Chunks)),
	}
	for i, c := range sig.Chunks {
		m.Chunks[i] = ChunkEntry{
			Offset: c.Offset,
			Length: c.Length,
			Digest: c.Digest.String(),
		}
	}
	return m
}

func normalizeOpt(o chunk.Options) chunk.Options {
	if o.AvgSize == 0 {
		o.AvgSize = chunk.DefaultAvgSize
	}
	if o.MinSize == 0 {
		o.MinSize = chunk.DefaultMinSize
	}
	if o.MaxSize == 0 {
		o.MaxSize = chunk.DefaultMaxSize
	}
	return o
}

// WriteJSON encodes m as indented JSON to w.
func WriteJSON(w io.Writer, m Manifest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// WriteFile writes m as JSON to path (atomic via temp + rename).
func WriteFile(path string, m Manifest) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := WriteJSON(f, m); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// ReadFile loads a Manifest from path.
func ReadFile(path string) (Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	return Decode(f)
}

// Decode reads a Manifest from r.
func Decode(r io.Reader) (Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(r)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if m.Version == 0 {
		m.Version = Version
	}
	if m.Version != Version {
		return Manifest{}, fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Validate checks structural consistency of the manifest.
func (m Manifest) Validate() error {
	if m.Size < 0 {
		return fmt.Errorf("negative size")
	}
	if m.Digest != "" {
		if _, err := ParseDigest(m.Digest); err != nil {
			return fmt.Errorf("digest: %w", err)
		}
	} else if m.Size > 0 {
		return fmt.Errorf("missing digest")
	}
	var offset uint64
	var total uint64
	for i, c := range m.Chunks {
		if c.Length == 0 {
			return fmt.Errorf("chunk %d: zero length", i)
		}
		if _, err := ParseDigest(c.Digest); err != nil {
			return fmt.Errorf("chunk %d: digest: %w", i, err)
		}
		if c.Offset != offset {
			return fmt.Errorf("chunk %d: offset %d want %d", i, c.Offset, offset)
		}
		offset += uint64(c.Length)
		total += uint64(c.Length)
	}
	if int64(total) != m.Size {
		return fmt.Errorf("chunk sizes sum to %d, manifest size %d", total, m.Size)
	}
	return nil
}

// ParseDigest converts a hex digest string to chunk.Digest.
func ParseDigest(s string) (chunk.Digest, error) {
	var d chunk.Digest
	if len(s) != 64 {
		return d, fmt.Errorf("digest length %d, want 64", len(s))
	}
	for i := 0; i < 32; i++ {
		hi, ok1 := fromHex(s[i*2])
		lo, ok2 := fromHex(s[i*2+1])
		if !ok1 || !ok2 {
			return d, fmt.Errorf("invalid hex digest")
		}
		d[i] = hi<<4 | lo
	}
	return d, nil
}

func fromHex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

// DiffResult summarizes chunk-level differences between two manifests.
type DiffResult struct {
	OnlyA       []ChunkEntry `json:"only_a"`
	OnlyB       []ChunkEntry `json:"only_b"`
	Shared      int          `json:"shared"`
	ChangedA    int          `json:"changed_a"`
	ChangedB    int          `json:"changed_b"`
	SizeA       int64        `json:"size_a"`
	SizeB       int64        `json:"size_b"`
	DigestEqual bool         `json:"digest_equal"`
}

// DigestEqual reports whether two hex digests refer to the same BLAKE3 value.
// Comparison is case-insensitive for valid hex; empty strings match only each other.
func DigestEqual(a, b string) bool {
	if a == b {
		return true
	}
	da, err1 := ParseDigest(a)
	db, err2 := ParseDigest(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return da == db
}

func normalizeDigestKey(s string) string {
	d, err := ParseDigest(s)
	if err != nil {
		return strings.ToLower(s)
	}
	return d.String()
}

// Diff compares two manifests by chunk digest (content-addressed multiset).
func Diff(a, b Manifest) DiffResult {
	countA := make(map[string]int, len(a.Chunks))
	countB := make(map[string]int, len(b.Chunks))
	sampleA := make(map[string]ChunkEntry, len(a.Chunks))
	sampleB := make(map[string]ChunkEntry, len(b.Chunks))
	for _, c := range a.Chunks {
		key := normalizeDigestKey(c.Digest)
		countA[key]++
		sampleA[key] = c
	}
	for _, c := range b.Chunks {
		key := normalizeDigestKey(c.Digest)
		countB[key]++
		sampleB[key] = c
	}
	res := DiffResult{
		SizeA:       a.Size,
		SizeB:       b.Size,
		DigestEqual: DigestEqual(a.Digest, b.Digest),
	}
	for dig, ca := range countA {
		cb := countB[dig]
		shared := ca
		if cb < shared {
			shared = cb
		}
		res.Shared += shared
		if ca > cb {
			res.ChangedA += ca - cb
			res.OnlyA = append(res.OnlyA, sampleA[dig])
		}
	}
	for dig, cb := range countB {
		ca := countA[dig]
		if cb > ca {
			res.ChangedB += cb - ca
			res.OnlyB = append(res.OnlyB, sampleB[dig])
		}
	}
	return res
}
