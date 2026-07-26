package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Bar is a simple stderr progress indicator for long-running ops.
type Bar struct {
	mu       sync.Mutex
	w        io.Writer
	total    int64
	current  int64
	label    string
	start    time.Time
	lastDraw time.Time
	enabled  bool
	done     bool
}

// New creates a progress bar writing to stderr when enabled.
func New(label string, total int64, enabled bool) *Bar {
	w := io.Writer(os.Stderr)
	if !enabled {
		w = io.Discard
	}
	return &Bar{
		w:       w,
		total:   total,
		label:   label,
		start:   time.Now(),
		enabled: enabled,
	}
}

// Add increments progress by n bytes/items.
func (b *Bar) Add(n int64) {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current += n
	now := time.Now()
	if now.Sub(b.lastDraw) < 100*time.Millisecond && b.current < b.total {
		return
	}
	b.lastDraw = now
	b.drawLocked()
}

// SetTotal updates the total (useful when discovered lazily).
func (b *Bar) SetTotal(total int64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total = total
}

// Finish completes the bar and prints a newline.
func (b *Bar) Finish() {
	if b == nil || !b.enabled || b.done {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.total > 0 {
		b.current = b.total
	}
	b.drawLocked()
	_, _ = fmt.Fprintln(b.w)
	b.done = true
}

func (b *Bar) drawLocked() {
	elapsed := time.Since(b.start).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}
	rate := float64(b.current) / elapsed
	pct := 0.0
	if b.total > 0 {
		pct = 100 * float64(b.current) / float64(b.total)
		if pct > 100 {
			pct = 100
		}
	}
	_, _ = fmt.Fprintf(b.w, "\r%s  %6.1f%%  %s / %s  %s/s",
		b.label,
		pct,
		humanBytes(b.current),
		humanBytes(b.total),
		humanBytes(int64(rate)),
	)
}

func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
