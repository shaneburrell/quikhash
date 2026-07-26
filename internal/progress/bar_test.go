package progress

import "testing"

func TestHumanBytes(t *testing.T) {
	if humanBytes(500) != "500 B" {
		t.Fatalf("%q", humanBytes(500))
	}
	if humanBytes(2048) != "2.0 KiB" {
		t.Fatalf("%q", humanBytes(2048))
	}
}

func TestBarNoopWhenDisabled(t *testing.T) {
	b := New("x", 100, false)
	b.Add(50)
	b.Finish()
}
