package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shaneburrell/quikhash/internal/manifest"
)

func newVerifyCmd() *cobra.Command {
	var manifestPath string
	cmd := &cobra.Command{
		Use:   "verify <path>",
		Short: "Verify a file against a manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if manifestPath == "" {
				return fmt.Errorf("--manifest is required")
			}
			m, err := manifest.ReadFile(manifestPath)
			if err != nil {
				return err
			}
			res, err := manifest.Verify(args[0], m)
			if err != nil && res.Error == "" {
				return err
			}
			if jsonOut {
				if err := printJSON(res); err != nil {
					return err
				}
				if !res.OK {
					return fmt.Errorf("verification failed")
				}
				return nil
			}
			if res.OK {
				fmt.Printf("OK  %s  digest %s  %d chunks\n", args[0], m.Digest, len(m.Chunks))
				return nil
			}
			fmt.Fprintf(os.Stderr, "FAIL  size_match=%v digest_match=%v chunks_ok=%d chunks_fail=%d\n",
				res.SizeMatch, res.DigestMatch, res.ChunksOK, res.ChunksFail)
			for _, d := range res.FailedChunks {
				fmt.Fprintf(os.Stderr, "  mismatched chunk %s\n", d)
			}
			return fmt.Errorf("verification failed")
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "manifest file to verify against")
	_ = cmd.MarkFlagRequired("manifest")
	return cmd
}
