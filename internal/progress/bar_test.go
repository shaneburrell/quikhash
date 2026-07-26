package progress

import (
	"bytes"
	"testing"
	"time"
)

func TestHumanBytes(t *testing.T) {
	if humanBytes(500) != "500 B" {
		t.Fatalf("%q", humanBytes(500))
	}
	if humanBytes(2048) != "2.0 KiB" {
		t.Fatalf("%q", humanBytes(2048))
	}
	if humanBytes(-1) != "0 B" {
		t.Fatalf("%q", humanBytes(-1))
	}
	if humanBytes(1024*1024) != "1.0 MiB" {
		t.Fatalf("%q", humanBytes(1024*1024))
	}
}

func TestBarNoopWhenDisabled(t *testing.T) {
	b := New("x", 100, false)
	b.Add(50)
	b.SetTotal(200)
	b.Finish()
	var nilBar *Bar
	nilBar.Add(1)
	nilBar.SetTotal(1)
	nilBar.Finish()
}

func TestBarEnabledDraws(t *testing.T) {
	var buf bytes.Buffer
	b := New("hash", 100, true)
	b.w = &buf
	b.Add(10)
	time.Sleep(110 * time.Millisecond)
	b.Add(40)
	b.SetTotal(80)
	b.Add(100) // over 100%
	b.Finish()
	b.Finish() // idempotent
	if buf.Len() == 0 {
		t.Fatal("expected progress output")
	}
}
