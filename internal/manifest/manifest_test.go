package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shaneburrell/quikhash/internal/chunk"
)

func TestHashVerifyDumpReconstructRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	data := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog.\n", 5000))
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	opt := chunk.Options{AvgSize: 4 * 1024, MinSize: 1024, MaxSize: 16 * 1024}
	m, err := HashFile(src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if m.Size != int64(len(data)) || len(m.Chunks) == 0 {
		t.Fatalf("bad manifest: size=%d chunks=%d", m.Size, len(m.Chunks))
	}

	manPath := filepath.Join(dir, "m.json")
	if err := WriteFile(manPath, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadFile(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != m.Digest {
		t.Fatal("reload digest mismatch")
	}

	vr, err := Verify(src, loaded)
	if err != nil || !vr.OK {
		t.Fatalf("verify: %+v %v", vr, err)
	}

	chunkDir := filepath.Join(dir, "chunks")
	dr, err := Dump(src, loaded, DumpOptions{OutDir: chunkDir, Workers: 2, Resume: true})
	if err != nil {
		t.Fatal(err)
	}
	if dr.Written != len(loaded.Chunks) {
		t.Fatalf("wrote %d want %d", dr.Written, len(loaded.Chunks))
	}
	// Resume should skip.
	dr2, err := Dump(src, loaded, DumpOptions{OutDir: chunkDir, Workers: 2, Resume: true})
	if err != nil {
		t.Fatal(err)
	}
	if dr2.Skipped != len(loaded.Chunks) {
		t.Fatalf("skipped %d want %d", dr2.Skipped, len(loaded.Chunks))
	}

	out := filepath.Join(dir, "out.bin")
	if err := Reconstruct(loaded, ReconstructOptions{ChunksDir: chunkDir, OutPath: out, Workers: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("reconstruct mismatch: %d vs %d", len(got), len(data))
	}
}

func TestDiffChangedChunks(t *testing.T) {
	base := []byte(strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789\n", 3000))
	shifted := append([]byte("PREFIX-"), base...)

	opt := chunk.Options{AvgSize: 4 * 1024, MinSize: 1024, MaxSize: 16 * 1024}
	a, err := chunk.ChunkReader(bytes.NewReader(base), int64(len(base)), opt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := chunk.ChunkReader(bytes.NewReader(shifted), int64(len(shifted)), opt)
	if err != nil {
		t.Fatal(err)
	}
	ma := FromSignature("a", a, opt)
	mb := FromSignature("b", b, opt)
	d := Diff(ma, mb)
	if d.DigestEqual {
		t.Fatal("expected digests to differ")
	}
	if d.Shared == 0 {
		t.Fatal("expected some shared chunks after prefix insert")
	}
	if d.ChangedA == 0 && d.ChangedB == 0 {
		t.Fatal("expected some changed chunks")
	}
}

func TestParseDigest(t *testing.T) {
	d := chunk.Sum([]byte("hi"))
	got, err := ParseDigest(d.String())
	if err != nil || got != d {
		t.Fatalf("parse: %v %v", got, err)
	}
	if _, err := ParseDigest("zz"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRejectsGaps(t *testing.T) {
	m := Manifest{
		Version: Version,
		Size:    10,
		Digest:  strings.Repeat("a", 64),
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: strings.Repeat("b", 64)},
			{Offset: 5, Length: 6, Digest: strings.Repeat("c", 64)},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected gap error")
	}
}
