package typer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockDriver struct {
	mu          sync.Mutex
	events      []string
	escAfter    int
	typedCount  int
	permErr     error
	prepareErr  error
	closedCount int
}

func (m *mockDriver) CheckPermission(_ bool) error {
	return m.permErr
}

func (m *mockDriver) Prepare(_ InputMode) error {
	return m.prepareErr
}

func (m *mockDriver) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closedCount++
	return nil
}

func (m *mockDriver) TypeRune(r rune, mode InputMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typedCount++
	m.events = append(m.events, string(mode)+":"+string(r))
	return nil
}

func (m *mockDriver) PressEnter(shift bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typedCount++
	if shift {
		m.events = append(m.events, "key:Shift+Enter")
	} else {
		m.events = append(m.events, "key:Enter")
	}
	return nil
}

func (m *mockDriver) PressTab() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typedCount++
	m.events = append(m.events, "key:Tab")
	return nil
}

func (m *mockDriver) ClearLineIndent(_ InputMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "key:ClearLineIndent")
	return nil
}

func (m *mockDriver) PressDelete() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "key:Delete")
	return nil
}

func (m *mockDriver) IsEscapePressed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.escAfter > 0 && m.typedCount >= m.escAfter
}

// smartEditorSim simulates an online IDE / Monaco / CodeMirror editor with auto-indent and auto-closing brackets!
type smartEditorSim struct {
	lines     []string
	row       int
	col       int
	autoClose bool
}

func newSmartEditorSim(autoClose bool) *smartEditorSim {
	return &smartEditorSim{lines: []string{""}, autoClose: autoClose}
}

func (s *smartEditorSim) CheckPermission(_ bool) error { return nil }
func (s *smartEditorSim) Prepare(_ InputMode) error    { return nil }
func (s *smartEditorSim) Close() error                 { return nil }
func (s *smartEditorSim) IsEscapePressed() bool        { return false }

func (s *smartEditorSim) TypeRune(r rune, _ InputMode) error {
	line := []rune(s.lines[s.row])
	before := string(line[:s.col])
	after := string(line[s.col:])

	inserted := string(r)
	if s.autoClose {
		switch r {
		case '{':
			inserted = "{}"
		case '(':
			inserted = "()"
		case '[':
			inserted = "[]"
		case '"':
			inserted = `""`
		case '\'':
			inserted = `''`
		}
	}
	s.lines[s.row] = before + inserted + after
	s.col++
	return nil
}

func (s *smartEditorSim) PressEnter(_ bool) error {
	line := []rune(s.lines[s.row])
	before := string(line[:s.col])
	after := string(line[s.col:])

	// Compute auto-indent like VS Code / Monaco: copy current leading spaces + 4 extra spaces if line ends with ':' or '{'.
	indent := 0
	for _, ch := range before {
		if ch == ' ' {
			indent++
		} else {
			break
		}
	}
	trimmed := strings.TrimRight(before, " ")
	if strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "{") {
		indent += 4
	}

	s.lines[s.row] = before
	newLine := strings.Repeat(" ", indent) + after
	s.lines = append(s.lines[:s.row+1], append([]string{newLine}, s.lines[s.row+1:]...)...)
	s.row++
	s.col = indent
	return nil
}

func (s *smartEditorSim) PressTab() error {
	return s.TypeRune('\t', ModeUnicode)
}

func (s *smartEditorSim) ClearLineIndent(_ InputMode) error {
	// Simulates sentinel space + Shift+Home + Backspace clearing everything before cursor on current line.
	line := []rune(s.lines[s.row])
	s.lines[s.row] = string(line[s.col:])
	s.col = 0
	return nil
}

func (s *smartEditorSim) PressDelete() error {
	line := []rune(s.lines[s.row])
	if s.col < len(line) {
		s.lines[s.row] = string(line[:s.col]) + string(line[s.col+1:])
	}
	return nil
}

func (s *smartEditorSim) Text() string {
	return strings.Join(s.lines, "\n")
}

func TestPreprocessCodeSpacesTabsAndNewlines(t *testing.T) {
	// Contains UTF-8 BOM, HTML &nbsp; (\u00A0), zero-width space (\u200B), CRLF, CR, LF, Tabs, mixed spaces+Tab, blank lines.
	raw := "\uFEFFpackage main\r\n\r\nfunc add(a, b int) int {\r\t\u00A0\u200Breturn a + b\n  \t// aligned comment\n}"

	got := PreprocessText(raw, DefaultOptions())
	want := "package main\n\nfunc add(a, b int) int {\n     return a + b\n    // aligned comment\n}"
	if got != want {
		t.Fatalf("PreprocessText default =\n%q\nwant:\n%q", got, want)
	}

	// Keep raw Tab when TabToSpaces == 0
	optsKeepTab := DefaultOptions()
	optsKeepTab.TabToSpaces = 0
	gotKeepTab := PreprocessText("if true {\r\n\treturn\r\n}", optsKeepTab)
	wantKeepTab := "if true {\n\treturn\n}"
	if gotKeepTab != wantKeepTab {
		t.Fatalf("PreprocessText keep tab = %q, want %q", gotKeepTab, wantKeepTab)
	}

	// Strip leading indent
	optsStrip := DefaultOptions()
	optsStrip.Indent = IndentStrip
	gotStrip := PreprocessText("def f():\n    \treturn 1", optsStrip)
	wantStrip := "def f():\nreturn 1"
	if gotStrip != wantStrip {
		t.Fatalf("PreprocessText strip = %q, want %q", gotStrip, wantStrip)
	}
}

func TestSmartEditorPythonAndCIndentationExactMatch(t *testing.T) {
	pythonCode := strings.Join([]string{
		"def solve(nums):",
		"\tif not nums:",
		"\t\treturn 0",
		"\ttotal = 0",
		"\tfor x in nums:",
		"\t\tif x > 0:",
		"\t\t\ttotal += x",
		"\treturn total",
		"",
		"print(solve([1, -2, 3]))",
	}, "\r\n")

	expectedPython := strings.ReplaceAll(strings.ReplaceAll(pythonCode, "\r\n", "\n"), "\t", "    ")

	sim := newSmartEditorSim(true)
	eng := NewEngine(sim)

	opts := DefaultOptions()
	opts.Delay = 0
	opts.CharInterval = 0
	opts.RandomJitter = false
	opts.TabToSpaces = 4
	opts.Indent = IndentResetAuto
	opts.ClearAutoCloseBracket = true

	if err := eng.Run(context.Background(), pythonCode, opts, nil); err != nil {
		t.Fatalf("Engine.Run python failed: %v", err)
	}
	if sim.Text() != expectedPython {
		t.Fatalf("Python code mismatch in smart editor!\ngot:\n%s\nwant:\n%s", sim.Text(), expectedPython)
	}

	cCode := "int main() {\n    if (a == 1) {\n        printf(\"hello world\\n\");\n    }\n    return 0;\n}"
	simC := newSmartEditorSim(true)
	engC := NewEngine(simC)
	if err := engC.Run(context.Background(), cCode, opts, nil); err != nil {
		t.Fatalf("Engine.Run C failed: %v", err)
	}
	if simC.Text() != cCode {
		t.Fatalf("C code mismatch in smart editor!\ngot:\n%s\nwant:\n%s", simC.Text(), cCode)
	}
}

func TestEngineRunUnicodeAndSpecialKeys(t *testing.T) {
	drv := &mockDriver{}
	eng := NewEngine(drv)

	opts := DefaultOptions()
	opts.Delay = 40 * time.Millisecond
	opts.CharInterval = time.Millisecond
	opts.RandomJitter = false
	opts.TabToSpaces = 0
	opts.Newline = NewlineShiftEnter

	var phases []Phase
	err := eng.Run(context.Background(), "A \r\n\t!", opts, func(p Progress) {
		phases = append(phases, p.Phase)
	})
	if err != nil {
		t.Fatalf("Engine.Run unexpected error: %v", err)
	}

	wantEvents := []string{
		"unicode:A",
		"unicode: ",
		"key:Shift+Enter",
		"key:Tab",
		"unicode:!",
	}
	if strings.Join(drv.events, "|") != strings.Join(wantEvents, "|") {
		t.Fatalf("events = %v, want %v", drv.events, wantEvents)
	}
	if drv.closedCount != 1 {
		t.Fatalf("driver Close count = %d, want 1", drv.closedCount)
	}
	if len(phases) == 0 || phases[len(phases)-1] != PhaseDone {
		t.Fatalf("final phase = %v, want PhaseDone", phases)
	}
}

func TestEngineEscEmergencyStop(t *testing.T) {
	drv := &mockDriver{escAfter: 3}
	eng := NewEngine(drv)

	opts := DefaultOptions()
	opts.Delay = 0
	opts.CharInterval = time.Millisecond
	opts.RandomJitter = false
	opts.EnableEscStop = true

	var lastProgress Progress
	err := eng.Run(context.Background(), "abcdefghij", opts, func(p Progress) {
		lastProgress = p
	})
	if !errors.Is(err, ErrStoppedByEsc) {
		t.Fatalf("err = %v, want ErrStoppedByEsc", err)
	}
	if !lastProgress.StoppedByEsc || lastProgress.Phase != PhaseStopped {
		t.Fatalf("lastProgress = %+v, want StoppedByEsc=true Phase=stopped", lastProgress)
	}
	if len(drv.events) != 3 {
		t.Fatalf("typed %d chars before ESC abort, want 3", len(drv.events))
	}
}

func TestEngineManualStopDuringCountdown(t *testing.T) {
	drv := &mockDriver{}
	eng := NewEngine(drv)

	opts := DefaultOptions()
	opts.Delay = 5 * time.Second

	done := make(chan error, 1)
	go func() {
		done <- eng.Run(context.Background(), "hello", opts, nil)
	}()

	time.Sleep(80 * time.Millisecond)
	eng.Stop()

	select {
	case err := <-done:
		if !errors.Is(err, ErrStopped) {
			t.Fatalf("err = %v, want ErrStopped", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Engine.Stop did not cancel countdown promptly")
	}
}

func TestMacANSIKeyCoverage(t *testing.T) {
	const printable = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}\\|;:'\",.<>/?`~ "
	for _, r := range printable {
		if _, _, ok := MacANSIKey(r); !ok {
			t.Errorf("MacANSIKey(%q) returned ok=false", r)
		}
	}
	if _, _, ok := MacANSIKey('中'); ok {
		t.Errorf("MacANSIKey('中') expected ok=false")
	}
}
