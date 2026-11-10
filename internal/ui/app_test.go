package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/EggFine/KeyboardPaste/internal/typer"
)

type stubDriver struct {
	mu    sync.Mutex
	typed []rune
}

func (s *stubDriver) CheckPermission(_ bool) error    { return nil }
func (s *stubDriver) Prepare(_ typer.InputMode) error { return nil }
func (s *stubDriver) Close() error                    { return nil }
func (s *stubDriver) TypeRune(r rune, _ typer.InputMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.typed = append(s.typed, r)
	return nil
}
func (s *stubDriver) PressEnter(_ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.typed = append(s.typed, '\n')
	return nil
}
func (s *stubDriver) PressTab() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.typed = append(s.typed, '\t')
	return nil
}
func (s *stubDriver) ClearLineIndent(_ typer.InputMode) error { return nil }
func (s *stubDriver) PressDelete() error                      { return nil }
func (s *stubDriver) IsEscapePressed() bool                   { return false }

func TestHelpersParsing(t *testing.T) {
	if got := parseDelaySeconds("2.5", 3.0); got != 2.5 {
		t.Fatalf("parseDelaySeconds(2.5) = %v, want 2.5", got)
	}
	if got := parseDelaySeconds("invalid", 3.0); got != 3.0 {
		t.Fatalf("parseDelaySeconds(invalid) = %v, want 3.0", got)
	}
	if got := parseIntervalMs("15", 30); got != 15 {
		t.Fatalf("parseIntervalMs(15) = %d, want 15", got)
	}
	if got := parseIntervalMs("-5", 30); got != 1 {
		t.Fatalf("parseIntervalMs(-5) = %d, want 1", got)
	}
	if got := selectedTabSpaces(tabLabel4Spaces); got != 4 {
		t.Fatalf("selectedTabSpaces(4) = %d, want 4", got)
	}
	if got := selectedTabSpaces(tabLabel2Spaces); got != 2 {
		t.Fatalf("selectedTabSpaces(2) = %d, want 2", got)
	}
	if got := selectedTabSpaces(tabLabelKeepTab); got != 0 {
		t.Fatalf("selectedTabSpaces(keep) = %d, want 0", got)
	}
	if got := selectedIndentMode(indentLabelResetAuto); got != typer.IndentResetAuto {
		t.Fatalf("selectedIndentMode(reset) = %v, want IndentResetAuto", got)
	}
}

func TestBuildMainUIHeadlessCodeTyping(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("test")
	defer w.Close()

	drv := &stubDriver{}
	eng := typer.NewEngine(drv)
	built := buildMainUI(a, w, eng)
	if built == nil || built.root == nil {
		t.Fatal("buildMainUI returned nil root container")
	}
	w.SetContent(built.root)

	opts := typer.DefaultOptions()
	opts.Delay = 10 * time.Millisecond
	opts.CharInterval = time.Millisecond
	opts.RandomJitter = false

	code := "func main() {\r\n\tfmt.Println(\"hi world\")\r\n}"
	want := "func main() {\n    fmt.Println(\"hi world\")\n}"
	if err := eng.Run(context.Background(), code, opts, nil); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if string(drv.typed) != want {
		t.Fatalf("typed =\n%q\nwant:\n%q", string(drv.typed), want)
	}
}
