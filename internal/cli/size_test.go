package cli

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want uint32
	}{
		{"64K", 64 * 1024},
		{"16KiB", 16 * 1024},
		{"1M", 1024 * 1024},
		{"1024", 1024},
		{"256KB", 256 * 1024},
	}
	for _, tc := range cases {
		got, err := parseSize(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %d %v want %d", tc.in, got, err, tc.want)
		}
	}
	if _, err := parseSize("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestChunkOptsFromFlags(t *testing.T) {
	opt, err := chunkOptsFromFlags(ChunkSizeFlags{
		AvgSize: "64K", MinSize: "16K", MaxSize: "256K",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opt.AvgSize != 64*1024 || opt.MinSize != 16*1024 || opt.MaxSize != 256*1024 {
		t.Fatalf("%+v", opt)
	}
}

func TestExecuteHashHelp(t *testing.T) {
	if err := ExecuteArgs([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
}
