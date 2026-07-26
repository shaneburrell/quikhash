package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	jsonOut bool
	quiet   bool
)

// ChunkSizeFlags holds FastCDC parameters shared by hash/dump.
type ChunkSizeFlags struct {
	AvgSize string
	MinSize string
	MaxSize string
}

func Execute() error {
	return ExecuteArgs(os.Args[1:])
}

// ExecuteArgs runs the CLI with the given arguments (excluding program name).
func ExecuteArgs(args []string) error {
	root := newRoot()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return err
	}
	return nil
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "quikhash",
		Short: "FastCDC + BLAKE3 content-addressed hasher with reconstruction",
		Long: `QuikHash chunks files with FastCDC, fingerprints each chunk with BLAKE3,
and treats reconstruction from chunk sets as a first-class operation.

Produce manifests, verify, dump content-addressed chunks, reconstruct files,
and diff manifests by changed chunks only.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress progress bar")
	root.Version = version

	root.AddCommand(newHashCmd())
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newDumpCmd())
	root.AddCommand(newReconstructCmd())
	root.AddCommand(newDiffCmd())
	return root
}

func addChunkSizeFlags(cmd *cobra.Command, f *ChunkSizeFlags) {
	cmd.Flags().StringVar(&f.AvgSize, "avg-size", "64K", "FastCDC average chunk size")
	cmd.Flags().StringVar(&f.MinSize, "min-size", "16K", "FastCDC minimum chunk size")
	cmd.Flags().StringVar(&f.MaxSize, "max-size", "256K", "FastCDC maximum chunk size")
}
