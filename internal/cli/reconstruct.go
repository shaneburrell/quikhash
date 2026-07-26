package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/shaneburrell/quikhash/internal/manifest"
	"github.com/shaneburrell/quikhash/internal/progress"
)

func newReconstructCmd() *cobra.Command {
	var (
		chunksDir    string
		manifestPath string
		outPath      string
		workers      int
	)
	cmd := &cobra.Command{
		Use:   "reconstruct",
		Short: "Reconstruct a file from chunks + manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if chunksDir == "" || manifestPath == "" || outPath == "" {
				return fmt.Errorf("--chunks, --manifest, and --out are required")
			}
			m, err := manifest.ReadFile(manifestPath)
			if err != nil {
				return err
			}
			bar := progress.New("reconstruct", m.Size, !quiet && !jsonOut && isTTY())
			err = manifest.Reconstruct(m, manifest.ReconstructOptions{
				ChunksDir: chunksDir,
				OutPath:   outPath,
				Workers:   workers,
			})
			if err != nil {
				return err
			}
			bar.Add(m.Size)
			bar.Finish()
			if jsonOut {
				return printJSON(map[string]any{
					"out":    outPath,
					"size":   m.Size,
					"digest": m.Digest,
					"chunks": len(m.Chunks),
				})
			}
			fmt.Printf("wrote %s  %d bytes  digest %s\n", outPath, m.Size, m.Digest)
			return nil
		},
	}
	cmd.Flags().StringVar(&chunksDir, "chunks", "", "directory containing chunk files")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "manifest describing the file")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "output file path")
	cmd.Flags().IntVar(&workers, "workers", runtime.NumCPU(), "parallel chunk readers")
	_ = cmd.MarkFlagRequired("chunks")
	_ = cmd.MarkFlagRequired("manifest")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
