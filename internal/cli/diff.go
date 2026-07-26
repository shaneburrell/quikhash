package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shaneburrell/quikhash/internal/manifest"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <manifest1> <manifest2>",
		Short: "Show only changed chunks between two manifests",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := manifest.ReadFile(args[0])
			if err != nil {
				return err
			}
			b, err := manifest.ReadFile(args[1])
			if err != nil {
				return err
			}
			d := manifest.Diff(a, b)
			if jsonOut {
				return printJSON(d)
			}
			fmt.Printf("size: %d → %d\n", d.SizeA, d.SizeB)
			fmt.Printf("digest_equal: %v\n", d.DigestEqual)
			fmt.Printf("shared_chunks: %d\n", d.Shared)
			fmt.Printf("only_in_%s: %d\n", args[0], d.ChangedA)
			fmt.Printf("only_in_%s: %d\n", args[1], d.ChangedB)
			if !quiet {
				for _, c := range d.OnlyA {
					fmt.Printf("- %s  offset=%d length=%d\n", c.Digest, c.Offset, c.Length)
				}
				for _, c := range d.OnlyB {
					fmt.Printf("+ %s  offset=%d length=%d\n", c.Digest, c.Offset, c.Length)
				}
			}
			return nil
		},
	}
	return cmd
}
