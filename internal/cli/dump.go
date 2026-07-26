package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/shaneburrell/quikhash/internal/manifest"
	"github.com/shaneburrell/quikhash/internal/progress"
)

func newDumpCmd() *cobra.Command {
	var (
		outDir       string
		manifestPath string
		sizes        ChunkSizeFlags
		workers      int
		resume       bool
	)
	cmd := &cobra.Command{
		Use:   "dump <path>",
		Short: "Extract individual content-addressed chunks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				return fmt.Errorf("--out is required")
			}
			path := args[0]
			var m manifest.Manifest
			var err error
			if manifestPath != "" {
				m, err = manifest.ReadFile(manifestPath)
				if err != nil {
					return err
				}
			} else {
				opt, err := chunkOptsFromFlags(sizes)
				if err != nil {
					return err
				}
				m, err = manifest.HashFile(path, opt)
				if err != nil {
					return err
				}
			}
			bar := progress.New("dump", int64(len(m.Chunks)), !quiet && !jsonOut && isTTY())
			res, err := manifest.Dump(path, m, manifest.DumpOptions{
				OutDir:  outDir,
				Workers: workers,
				Resume:  resume,
			})
			bar.Add(int64(res.Written + res.Skipped))
			bar.Finish()
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(res)
			}
			fmt.Printf("wrote %d chunks (%d skipped) → %s\n", res.Written, res.Skipped, outDir)
			return nil
		},
	}
	addChunkSizeFlags(cmd, &sizes)
	cmd.Flags().StringVarP(&outDir, "out", "o", "", "directory to write chunks into")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "use existing manifest (otherwise hash first)")
	cmd.Flags().IntVar(&workers, "workers", runtime.NumCPU(), "parallel writers")
	cmd.Flags().BoolVar(&resume, "resume", true, "skip chunks that already exist with matching content")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
