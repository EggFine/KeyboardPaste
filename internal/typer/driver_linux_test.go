//go:build linux

package typer

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveX11CodeTypingIntoWindow(t *testing.T) {
	if os.Getenv("KP_LIVE_X11_TEST") != "1" {
		t.Skip("set KP_LIVE_X11_TEST=1 under Xvfb with a focused target window to run live X11 typing test")
	}

	eng := NewEngine(nil)
	opts := DefaultOptions()
	opts.Delay = 0
	opts.CharInterval = 8 * time.Millisecond
	opts.RandomJitter = false
	opts.TabToSpaces = 4
	if os.Getenv("KP_TEST_RESET_AUTO") == "1" {
		opts.Indent = IndentResetAuto
	}

	sampleCode := "package main\r\n\r\nfunc add(a, b int) int {\r\n\tif a > 0 {\r\n\t\treturn a + b // 中文OK\r\n\t}\r\n\treturn 0\r\n}\r\n/*END*/"
	if err := eng.Run(context.Background(), sampleCode, opts, nil); err != nil {
		t.Fatalf("live X11 Engine.Run failed: %v", err)
	}
}
