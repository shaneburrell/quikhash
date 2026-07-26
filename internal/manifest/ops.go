package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/shaneburrell/quikhash/internal/chunk"
)

// HashFile streams path with FastCDC + BLAKE3 and returns a Manifest.
func HashFile(path string, opt chunk.Options) (Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Manifest{}, err
	}
	sig, err := chunk.ChunkReader(f, st.Size(), opt)
	if err != nil {
		return Manifest{}, err
	}
	return FromSignature(path, sig, opt), nil
}

// VerifyResult is the outcome of verifying a file against a manifest.
type VerifyResult struct {
	OK           bool     `json:"ok"`
	SizeMatch    bool     `json:"size_match"`
	DigestMatch  bool     `json:"digest_match"`
	ChunksOK     int      `json:"chunks_ok"`
	ChunksFail   int      `json:"chunks_fail"`
	FailedChunks []string `json:"failed_chunks,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// Verify checks path against m by re-chunking and comparing digests.
func Verify(path string, m Manifest) (VerifyResult, error) {
	opt := chunk.Options{
		AvgSize: m.AvgSize,
		MinSize: m.MinSize,
		MaxSize: m.MaxSize,
	}
	got, err := HashFile(path, opt)
	if err != nil {
		return VerifyResult{Error: err.Error()}, err
	}
	res := VerifyResult{
		SizeMatch:   got.Size == m.Size,
		DigestMatch: got.Digest == m.Digest,
	}
	n := min(len(got.Chunks), len(m.Chunks))
	for i := 0; i < n; i++ {
		if got.Chunks[i].Digest == m.Chunks[i].Digest &&
			got.Chunks[i].Offset == m.Chunks[i].Offset &&
			got.Chunks[i].Length == m.Chunks[i].Length {
			res.ChunksOK++
		} else {
			res.ChunksFail++
			if len(res.FailedChunks) < 32 {
				res.FailedChunks = append(res.FailedChunks, m.Chunks[i].Digest)
			}
		}
	}
	if len(got.Chunks) != len(m.Chunks) {
		res.ChunksFail += abs(len(got.Chunks) - len(m.Chunks))
	}
	res.OK = res.SizeMatch && res.DigestMatch && res.ChunksFail == 0
	return res, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// DumpOptions controls chunk extraction.
type DumpOptions struct {
	OutDir  string
	Workers int
	Resume  bool // skip chunks that already exist with matching size
}

// DumpResult summarizes a dump operation.
type DumpResult struct {
	Written int `json:"written"`
	Skipped int `json:"skipped"`
	Total   int `json:"total"`
}

// Dump extracts content-addressed chunks from path into OutDir.
// Chunk files are named <digest> (64 hex chars, no extension).
func Dump(path string, m Manifest, opt DumpOptions) (DumpResult, error) {
	if opt.OutDir == "" {
		return DumpResult{}, fmt.Errorf("out dir required")
	}
	if err := os.MkdirAll(opt.OutDir, 0o755); err != nil {
		return DumpResult{}, err
	}
	workers := opt.Workers
	if workers <= 0 {
		workers = 4
	}

	f, err := os.Open(path)
	if err != nil {
		return DumpResult{}, err
	}
	defer f.Close()

	type job struct {
		entry ChunkEntry
		data  []byte
	}
	jobs := make(chan job, workers*2)
	var written, skipped atomic.Int64
	var wg sync.WaitGroup
	var firstErr sync.Once
	var dumpErr error
	setErr := func(e error) {
		firstErr.Do(func() { dumpErr = e })
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				out := filepath.Join(opt.OutDir, j.entry.Digest)
				if opt.Resume {
					if st, err := os.Stat(out); err == nil && st.Size() == int64(j.entry.Length) {
						// Quick size check; optionally verify digest for resume-friendliness.
						got := chunk.Sum(j.data)
						want, err := ParseDigest(j.entry.Digest)
						if err == nil && got == want {
							skipped.Add(1)
							continue
						}
					}
				}
				tmp := out + ".tmp"
				if err := os.WriteFile(tmp, j.data, 0o644); err != nil {
					setErr(err)
					continue
				}
				if err := os.Rename(tmp, out); err != nil {
					_ = os.Remove(tmp)
					setErr(err)
					continue
				}
				written.Add(1)
			}
		}()
	}

	for _, c := range m.Chunks {
		buf := make([]byte, c.Length)
		if _, err := f.ReadAt(buf, int64(c.Offset)); err != nil {
			close(jobs)
			wg.Wait()
			return DumpResult{}, fmt.Errorf("read chunk at %d: %w", c.Offset, err)
		}
		got := chunk.Sum(buf)
		want, err := ParseDigest(c.Digest)
		if err != nil {
			close(jobs)
			wg.Wait()
			return DumpResult{}, err
		}
		if got != want {
			close(jobs)
			wg.Wait()
			return DumpResult{}, fmt.Errorf("chunk digest mismatch at offset %d", c.Offset)
		}
		jobs <- job{entry: c, data: buf}
	}
	close(jobs)
	wg.Wait()
	if dumpErr != nil {
		return DumpResult{}, dumpErr
	}
	return DumpResult{
		Written: int(written.Load()),
		Skipped: int(skipped.Load()),
		Total:   len(m.Chunks),
	}, nil
}

// ReconstructOptions controls file reconstruction from chunks.
type ReconstructOptions struct {
	ChunksDir string
	OutPath   string
	Workers   int
}

// Reconstruct builds a file from content-addressed chunks covering the manifest.
// Any subset that covers every chunk in the manifest is sufficient (full set required).
func Reconstruct(m Manifest, opt ReconstructOptions) error {
	if opt.ChunksDir == "" || opt.OutPath == "" {
		return fmt.Errorf("chunks dir and out path required")
	}
	if err := m.Validate(); err != nil {
		return err
	}

	workers := opt.Workers
	if workers <= 0 {
		workers = 4
	}

	tmp := opt.OutPath + ".quikhash.tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if err := f.Truncate(m.Size); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}

	type job struct {
		entry ChunkEntry
	}
	jobs := make(chan job, workers*2)
	var wg sync.WaitGroup
	var firstErr sync.Once
	var reconErr error
	setErr := func(e error) {
		firstErr.Do(func() { reconErr = e })
	}
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				p := filepath.Join(opt.ChunksDir, j.entry.Digest)
				data, err := os.ReadFile(p)
				if err != nil {
					setErr(fmt.Errorf("missing chunk %s: %w", j.entry.Digest, err))
					continue
				}
				if uint32(len(data)) != j.entry.Length {
					setErr(fmt.Errorf("chunk %s: length %d want %d", j.entry.Digest, len(data), j.entry.Length))
					continue
				}
				got := chunk.Sum(data)
				want, err := ParseDigest(j.entry.Digest)
				if err != nil {
					setErr(err)
					continue
				}
				if got != want {
					setErr(fmt.Errorf("chunk %s: digest mismatch", j.entry.Digest))
					continue
				}
				mu.Lock()
				_, err = f.WriteAt(data, int64(j.entry.Offset))
				mu.Unlock()
				if err != nil {
					setErr(err)
				}
			}
		}()
	}

	for _, c := range m.Chunks {
		jobs <- job{entry: c}
	}
	close(jobs)
	wg.Wait()
	if reconErr != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return reconErr
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	rf, err := os.Open(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	digest, n, err := chunk.HashFile(rf)
	_ = rf.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if n != m.Size {
		_ = os.Remove(tmp)
		return fmt.Errorf("reconstructed size %d want %d", n, m.Size)
	}
	want, err := ParseDigest(m.Digest)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if digest != want {
		_ = os.Remove(tmp)
		return fmt.Errorf("reconstructed digest mismatch")
	}
	return os.Rename(tmp, opt.OutPath)
}
