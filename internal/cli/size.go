package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/shaneburrell/quikhash/internal/chunk"
)

func parseSize(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	s = strings.ReplaceAll(s, "_", "")
	mul := uint64(1)
	end := len(s)
	for end > 0 && unicode.IsLetter(rune(s[end-1])) {
		end--
	}
	if end == 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	numPart := s[:end]
	suf := strings.ToLower(s[end:])
	switch suf {
	case "", "b":
		mul = 1
	case "k", "kb", "ki", "kib":
		mul = 1024
	case "m", "mb", "mi", "mib":
		mul = 1024 * 1024
	case "g", "gb", "gi", "gib":
		mul = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown size suffix %q", suf)
	}
	n, err := strconv.ParseUint(numPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	v := n * mul
	if v == 0 || v > 1<<32-1 {
		return 0, fmt.Errorf("size out of range: %q", s)
	}
	return uint32(v), nil
}

func chunkOptsFromFlags(f ChunkSizeFlags) (chunk.Options, error) {
	avg, err := parseSize(f.AvgSize)
	if err != nil {
		return chunk.Options{}, fmt.Errorf("avg-size: %w", err)
	}
	min, err := parseSize(f.MinSize)
	if err != nil {
		return chunk.Options{}, fmt.Errorf("min-size: %w", err)
	}
	max, err := parseSize(f.MaxSize)
	if err != nil {
		return chunk.Options{}, fmt.Errorf("max-size: %w", err)
	}
	return chunk.Options{AvgSize: avg, MinSize: min, MaxSize: max}, nil
}
