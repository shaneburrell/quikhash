package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/shaneburrell/quikhash/internal/chunk"
	"github.com/shaneburrell/quikhash/internal/manifest"
	"github.com/shaneburrell/quikhash/internal/progress"
)

func newHashCmd() *cobra.Command {
	var (
		outPath string
		sizes   ChunkSizeFlags
		workers int
	)
	cmd := &cobra.Command{
		Use:   "hash <path>",
		Short: "Produce a FastCDC + BLAKE3 manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opt, err := chunkOptsFromFlags(sizes)
			if err != nil {
				return err
			}
			path := args[0]
			st, err := os.Stat(path)
			if err != nil {
				return err
			}
			if st.IsDir() {
				return hashDir(path, outPath, opt, workers)
			}
			return hashOne(path, outPath, opt)
		},
	}
	addChunkSizeFlags(cmd, &sizes)
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "write manifest to file (default: stdout; for dirs, write beside each file)")
	cmd.Flags().IntVar(&workers, "workers", runtime.NumCPU(), "parallel workers when hashing a directory")
	return cmd
}

func hashOne(path, outPath string, opt chunk.Options) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	bar := progress.New("hash", st.Size(), !quiet && !jsonOut && isTTY())
	m, err := manifest.HashFile(path, opt)
	if err != nil {
		return err
	}
	bar.Add(st.Size())
	bar.Finish()

	if outPath != "" {
		if err := manifest.WriteFile(outPath, m); err != nil {
			return err
		}
		if jsonOut {
			return printJSON(map[string]any{
				"path":   path,
				"out":    outPath,
				"size":   m.Size,
				"digest": m.Digest,
				"chunks": len(m.Chunks),
			})
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d chunks, digest %s)\n", outPath, len(m.Chunks), m.Digest)
		return nil
	}
	return manifest.WriteJSON(os.Stdout, m)
}

func hashDir(root, outDir string, opt chunk.Options, workers int) error {
	if outDir == "" {
		outDir = root
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".quikhash.json") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}

	bar := progress.New("hash", int64(len(files)), !quiet && !jsonOut && isTTY())
	type result struct {
		Path   string `json:"path"`
		Out    string `json:"out"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
		Chunks int    `json:"chunks"`
		Error  string `json:"error,omitempty"`
	}
	results := make([]result, len(files))
	jobs := make(chan int, workers)
	var wg sync.WaitGroup
	var failCount atomic.Int64

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				path := files[idx]
				rel, err := filepath.Rel(root, path)
				if err != nil {
					rel = filepath.Base(path)
				}
				out := filepath.Join(outDir, rel+".quikhash.json")
				if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
					results[idx] = result{Path: path, Error: err.Error()}
					failCount.Add(1)
					bar.Add(1)
					continue
				}
				// Resume-friendly: skip when existing manifest matches size and is not older than the file.
				if st, err := os.Stat(path); err == nil {
					if prev, err := manifest.ReadFile(out); err == nil && prev.Size == st.Size() && sameMTime(path, out) {
						results[idx] = result{
							Path: path, Out: out, Size: prev.Size,
							Digest: prev.Digest, Chunks: len(prev.Chunks),
						}
						bar.Add(1)
						continue
					}
				}
				m, err := manifest.HashFile(path, opt)
				if err != nil {
					results[idx] = result{Path: path, Error: err.Error()}
					failCount.Add(1)
					bar.Add(1)
					continue
				}
				if err := manifest.WriteFile(out, m); err != nil {
					results[idx] = result{Path: path, Error: err.Error()}
					failCount.Add(1)
					bar.Add(1)
					continue
				}
				results[idx] = result{
					Path: path, Out: out, Size: m.Size,
					Digest: m.Digest, Chunks: len(m.Chunks),
				}
				bar.Add(1)
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	bar.Finish()

	if jsonOut {
		return printJSON(map[string]any{
			"files":  results,
			"ok":     failCount.Load() == 0,
			"failed": failCount.Load(),
			"total":  len(files),
		})
	}
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(os.Stderr, "FAIL %s: %s\n", r.Path, r.Error)
			continue
		}
		short := r.Digest
		if len(short) > 16 {
			short = short[:16] + "…"
		}
		fmt.Printf("%s  %s  %d chunks\n", short, r.Out, r.Chunks)
	}
	if failCount.Load() > 0 {
		return fmt.Errorf("%d file(s) failed", failCount.Load())
	}
	return nil
}

func sameMTime(file, manifestPath string) bool {
	a, err1 := os.Stat(file)
	b, err2 := os.Stat(manifestPath)
	if err1 != nil || err2 != nil {
		return false
	}
	return !b.ModTime().Before(a.ModTime())
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func isTTY() bool {
	st, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
