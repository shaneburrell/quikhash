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
	upper := strings.ToUpper(d.String())
	got2, err := ParseDigest(upper)
	if err != nil || got2 != d {
		t.Fatalf("upper: %v %v", got2, err)
	}
	if _, err := ParseDigest("zz"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseDigest(strings.Repeat("g", 64)); err == nil {
		t.Fatal("expected invalid hex")
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

func TestValidateRejectsNonHexDigests(t *testing.T) {
	traversal := "../../../../etc/passwd"
	m := Manifest{
		Version: Version,
		Size:    4,
		Digest:  strings.Repeat("a", 64),
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: traversal},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected non-hex chunk digest error")
	}

	m.Chunks[0].Digest = strings.Repeat("g", 64)
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid hex chunk digest error")
	}

	m.Chunks[0].Digest = strings.Repeat("b", 64)
	m.Digest = traversal
	if err := m.Validate(); err == nil {
		t.Fatal("expected non-hex file digest error")
	}
}

func TestReconstructRejectsPathTraversalDigest(t *testing.T) {
	dir := t.TempDir()
	chunks := filepath.Join(dir, "chunks")
	if err := os.MkdirAll(chunks, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(secret, []byte("leak"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		Version: Version,
		Size:    4,
		Digest:  strings.Repeat("a", 64),
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: "../secret.bin"},
		},
	}
	out := filepath.Join(dir, "out.bin")
	if err := Reconstruct(m, ReconstructOptions{ChunksDir: chunks, OutPath: out}); err == nil {
		t.Fatal("expected validate error for path traversal digest")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("reconstruct must not create output for invalid manifest")
	}
}

func TestDumpRejectsInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(src, []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		Version: Version,
		Size:    4,
		Digest:  strings.Repeat("a", 64),
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: "../secret"},
		},
	}
	if _, err := Dump(src, m, DumpOptions{OutDir: filepath.Join(dir, "chunks")}); err == nil {
		t.Fatal("expected dump validate error")
	}
}

func TestDigestEqualCaseInsensitive(t *testing.T) {
	d := chunk.Sum([]byte("case"))
	lower := d.String()
	upper := strings.ToUpper(lower)
	if !DigestEqual(lower, upper) {
		t.Fatal("expected case-insensitive match")
	}
	if DigestEqual(lower, strings.Repeat("0", 64)) {
		t.Fatal("expected mismatch")
	}
	if !DigestEqual("", "") {
		t.Fatal("empty digests should match")
	}
}

func TestDiffDuplicateChunkCounts(t *testing.T) {
	digA := strings.Repeat("a", 64)
	digB := strings.Repeat("b", 64)
	digC := strings.Repeat("c", 64)
	ma := Manifest{
		Size:   12,
		Digest: digA,
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: digB},
			{Offset: 4, Length: 4, Digest: digB},
			{Offset: 8, Length: 4, Digest: digB},
		},
	}
	mb := Manifest{
		Size:   8,
		Digest: digC,
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: digB},
			{Offset: 4, Length: 4, Digest: digB},
		},
	}
	d := Diff(ma, mb)
	if d.Shared != 2 {
		t.Fatalf("shared=%d want 2", d.Shared)
	}
	if d.ChangedA != 1 {
		t.Fatalf("changed_a=%d want 1", d.ChangedA)
	}
	if d.ChangedB != 0 {
		t.Fatalf("changed_b=%d want 0", d.ChangedB)
	}
	if d.DigestEqual {
		t.Fatal("expected digests to differ")
	}

	// Uppercase digests should still count as shared.
	mbUpper := Manifest{
		Size:   8,
		Digest: strings.ToUpper(digC),
		Chunks: []ChunkEntry{
			{Offset: 0, Length: 4, Digest: strings.ToUpper(digB)},
			{Offset: 4, Length: 4, Digest: strings.ToUpper(digB)},
		},
	}
	d2 := Diff(ma, mbUpper)
	if d2.Shared != 2 || d2.ChangedA != 1 {
		t.Fatalf("case-normalized diff: shared=%d changed_a=%d", d2.Shared, d2.ChangedA)
	}
}

func TestVerifyAcceptsUppercaseDigest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	data := []byte("uppercase-digest-verify")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	opt := chunk.Options{AvgSize: 1 * 1024, MinSize: 256, MaxSize: 4 * 1024}
	m, err := HashFile(src, opt)
	if err != nil {
		t.Fatal(err)
	}
	m.Digest = strings.ToUpper(m.Digest)
	for i := range m.Chunks {
		m.Chunks[i].Digest = strings.ToUpper(m.Chunks[i].Digest)
	}
	vr, err := Verify(src, m)
	if err != nil || !vr.OK {
		t.Fatalf("verify uppercase: %+v %v", vr, err)
	}
}

func TestValidateAndDecodeErrors(t *testing.T) {
	cases := []Manifest{
		{Version: Version, Size: -1, Digest: strings.Repeat("a", 64)},
		{Version: Version, Size: 1, Digest: "", Chunks: []ChunkEntry{{Offset: 0, Length: 1, Digest: strings.Repeat("b", 64)}}},
		{Version: Version, Size: 1, Digest: strings.Repeat("a", 64), Chunks: []ChunkEntry{{Offset: 0, Length: 0, Digest: strings.Repeat("b", 64)}}},
		{Version: Version, Size: 1, Digest: strings.Repeat("a", 64), Chunks: []ChunkEntry{{Offset: 0, Length: 1, Digest: ""}}},
		{Version: Version, Size: 2, Digest: strings.Repeat("a", 64), Chunks: []ChunkEntry{{Offset: 0, Length: 1, Digest: strings.Repeat("b", 64)}}},
	}
	for i, m := range cases {
		if err := m.Validate(); err == nil {
			t.Fatalf("case %d: expected validate error", i)
		}
	}

	if _, err := Decode(strings.NewReader(`{`)); err == nil {
		t.Fatal("expected json error")
	}
	if _, err := Decode(strings.NewReader(`{"version":99,"size":0,"digest":"","chunks":[]}`)); err == nil {
		t.Fatal("expected version error")
	}
	m, err := Decode(strings.NewReader(`{"size":0,"digest":"","chunks":[]}`))
	if err != nil || m.Version != Version {
		t.Fatalf("default version: %+v %v", m, err)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected missing file")
	}
}

func TestVerifyMismatchAndDumpErrors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	data := bytes.Repeat([]byte("abcdefghij"), 2000)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	opt := chunk.Options{AvgSize: 2 * 1024, MinSize: 512, MaxSize: 8 * 1024}
	m, err := HashFile(src, opt)
	if err != nil {
		t.Fatal(err)
	}

	// Truncate file so chunk counts diverge (exercises abs).
	if err := os.WriteFile(src, data[:len(data)/3], 0o644); err != nil {
		t.Fatal(err)
	}
	vr, err := Verify(src, m)
	if err != nil {
		t.Fatal(err)
	}
	if vr.OK || vr.ChunksFail == 0 {
		t.Fatalf("expected mismatch: %+v", vr)
	}

	if _, err := Dump(src, m, DumpOptions{}); err == nil {
		t.Fatal("expected empty out dir error")
	}

	// Reconstruct missing chunk.
	chunkDir := filepath.Join(dir, "chunks")
	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.bin")
	if err := Reconstruct(m, ReconstructOptions{ChunksDir: chunkDir, OutPath: out}); err == nil {
		t.Fatal("expected missing chunk error")
	}
	if err := Reconstruct(m, ReconstructOptions{}); err == nil {
		t.Fatal("expected missing args")
	}
}

func TestFromSignatureDefaults(t *testing.T) {
	sig := chunk.FileSignature{Size: 0, Digest: chunk.Digest{}}
	m := FromSignature("p", sig, chunk.Options{})
	if m.AvgSize != chunk.DefaultAvgSize || m.MinSize != chunk.DefaultMinSize || m.MaxSize != chunk.DefaultMaxSize {
		t.Fatalf("%+v", m)
	}
}

func TestAbs(t *testing.T) {
	if abs(-3) != 3 || abs(2) != 2 {
		t.Fatal("abs")
	}
}
