package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaneburrell/quikhash/internal/manifest"
)

func TestE2ECommands(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "file.dat")
	data := []byte(strings.Repeat("quikhash-e2e-payload-", 4000))
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	man := filepath.Join(dir, "m.json")
	chunks := filepath.Join(dir, "chunks")
	out := filepath.Join(dir, "out.dat")

	if err := ExecuteArgs([]string{"hash", src, "--out", man, "--avg-size", "4K", "--min-size", "1K", "--max-size", "16K", "-q"}); err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := ExecuteArgs([]string{"verify", src, "--manifest", man, "-q"}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := ExecuteArgs([]string{"dump", src, "--manifest", man, "--out", chunks, "-q"}); err != nil {
		t.Fatalf("dump: %v", err)
	}
	if err := ExecuteArgs([]string{"reconstruct", "--chunks", chunks, "--manifest", man, "--out", out, "-q"}); err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("round-trip mismatch")
	}

	// Mutate and hash a second file for diff.
	src2 := filepath.Join(dir, "file2.dat")
	data2 := append([]byte("X"), data...)
	if err := os.WriteFile(src2, data2, 0o644); err != nil {
		t.Fatal(err)
	}
	man2 := filepath.Join(dir, "m2.json")
	if err := ExecuteArgs([]string{"hash", src2, "--out", man2, "--avg-size", "4K", "--min-size", "1K", "--max-size", "16K", "-q"}); err != nil {
		t.Fatalf("hash2: %v", err)
	}
	if err := ExecuteArgs([]string{"diff", man, man2, "--json", "-q"}); err != nil {
		t.Fatalf("diff: %v", err)
	}
	if err := ExecuteArgs([]string{"diff", man, man2, "-q"}); err != nil {
		t.Fatalf("diff text: %v", err)
	}
}

func TestHashDirectoryAndResume(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "sub/c.txt"} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte("content-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Pre-existing manifest sidecar should be skipped as an input file.
	if err := os.WriteFile(filepath.Join(root, "skip.quikhash.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	hashArgs := []string{
		"hash", root, "--out", out,
		"--avg-size", "1K", "--min-size", "256", "--max-size", "4K",
		"--workers", "2", "-q",
	}
	if err := ExecuteArgs(append(hashArgs, "--json")); err != nil {
		t.Fatalf("hash dir: %v", err)
	}
	for _, name := range []string{"a.txt", "b.txt", "sub/c.txt"} {
		man := filepath.Join(out, name+".quikhash.json")
		if _, err := os.Stat(man); err != nil {
			t.Fatalf("missing manifest %s: %v", man, err)
		}
	}

	manA := filepath.Join(out, "a.txt.quikhash.json")
	st1, err := os.Stat(manA)
	if err != nil {
		t.Fatal(err)
	}

	// Resume path: second run should skip when content+params match.
	if err := ExecuteArgs(hashArgs); err != nil {
		t.Fatalf("hash dir resume: %v", err)
	}
	st2, err := os.Stat(manA)
	if err != nil {
		t.Fatal(err)
	}
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("expected resume to leave sidecar mtime unchanged")
	}

	// Same-size content change with equalized mtimes must re-hash.
	aPath := filepath.Join(root, "a.txt")
	oldContent, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(aPath)
	if err != nil {
		t.Fatal(err)
	}
	newContent := []byte(strings.Repeat("X", len(oldContent)))
	if err := os.WriteFile(aPath, newContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(aPath, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Touch sidecar so sameMTime still passes (old bug path).
	if err := os.Chtimes(manA, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	oldDigest := readManifestDigest(t, manA)
	if err := ExecuteArgs(hashArgs); err != nil {
		t.Fatalf("hash after content change: %v", err)
	}
	newDigest := readManifestDigest(t, manA)
	if newDigest == oldDigest {
		t.Fatal("expected re-hash after same-size content change")
	}

	// Chunk-param change must re-hash even when content is unchanged.
	if err := ExecuteArgs([]string{
		"hash", root, "--out", out,
		"--avg-size", "2K", "--min-size", "512", "--max-size", "8K",
		"--workers", "2", "-q",
	}); err != nil {
		t.Fatalf("hash with new params: %v", err)
	}
	m, err := manifest.ReadFile(manA)
	if err != nil {
		t.Fatal(err)
	}
	if m.AvgSize != 2*1024 {
		t.Fatalf("avg_size=%d want %d after param change", m.AvgSize, 2*1024)
	}
}

func readManifestDigest(t *testing.T, path string) string {
	t.Helper()
	m, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return m.Digest
}

func TestHashStdoutAndVerifyFail(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.bin")
	if err := os.WriteFile(src, []byte("abc123"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteArgs([]string{"hash", src, "--avg-size", "1K", "--min-size", "256", "--max-size", "4K", "-q"}); err != nil {
		t.Fatalf("hash stdout: %v", err)
	}
	man := filepath.Join(dir, "m.json")
	if err := ExecuteArgs([]string{"hash", src, "-o", man, "--avg-size", "1K", "--min-size", "256", "--max-size", "4K", "--json", "-q"}); err != nil {
		t.Fatalf("hash json summary: %v", err)
	}
	if err := os.WriteFile(src, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteArgs([]string{"verify", src, "--manifest", man, "--json", "-q"}); err == nil {
		t.Fatal("expected verify failure")
	}
}

func TestDumpWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.bin")
	if err := os.WriteFile(src, bytes.Repeat([]byte("z"), 8000), 0o644); err != nil {
		t.Fatal(err)
	}
	chunks := filepath.Join(dir, "chunks")
	if err := ExecuteArgs([]string{
		"dump", src, "--out", chunks,
		"--avg-size", "1K", "--min-size", "256", "--max-size", "4K",
		"--json", "-q",
	}); err != nil {
		t.Fatalf("dump: %v", err)
	}
}

func TestSameMTime(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(b, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !sameMTime(a, b) {
		t.Fatal("expected b newer/equal")
	}
	if sameMTime(b, a) {
		t.Fatal("expected a older than b")
	}
	if sameMTime(a, filepath.Join(dir, "missing")) {
		t.Fatal("missing should be false")
	}
}

func TestParseSizeErrors(t *testing.T) {
	for _, s := range []string{"", "nope", "K", "1T", "0"} {
		if _, err := parseSize(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
	if _, err := chunkOptsFromFlags(ChunkSizeFlags{AvgSize: "bad", MinSize: "1K", MaxSize: "4K"}); err == nil {
		t.Fatal("expected avg-size error")
	}
}

func TestIsTTY(t *testing.T) {
	_ = isTTY()
}
