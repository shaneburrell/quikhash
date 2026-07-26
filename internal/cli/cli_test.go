package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
}
