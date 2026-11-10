//go:build windows

package typer

import (
	"testing"
	"unsafe"
)

func TestWinInputStructSize(t *testing.T) {
	got := unsafe.Sizeof(winInput{})
	var want uintptr = 40
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 28
	}
	if got != want {
		t.Fatalf("sizeof(winInput) = %d, want %d (must match Win32 INPUT union layout)", got, want)
	}
}
