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

func (m *mockDriver) DismissAutocomplete(_ InputMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "key:DismissAutocomplete")
	return nil
}

func (m *mockDriver) ClearLineIndent(_ InputMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "key:ClearLineIndent")
	return nil
}

func (m *mockDriver) ClearLineRight(_ InputMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "key:ClearLineRight")
	return nil
}

func (m *mockDriver) IsEscapePressed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.escAfter > 0 && m.typedCount >= m.escAfter
}

// smartEditorSim realistically models an online Web code editor (Monaco / CodeMirror 5 & 6 / Ace):
// - Auto-indents on Enter (carries current line indent + adds 4 spaces after ':', '{', '(', '[', and splits '{|}' if unclosed)
// - Implements real Smart Home (Shift+Home) selection toggling between firstNonWhitespace and column 0
// - Implements active Autocomplete Popup on identifiers/properties that hijacks Enter if not dismissed first!
// - Context-aware auto-closing brackets & quotes ({}, (), [], "", ”, “, /**/)
// - Supports pre-existing OJ template footer lines below the cursor!
type smartEditorSim struct {
	lines     []string
	row       int
	col       int
	autoClose bool
	popupOpen bool
}

func newSmartEditorSim(autoClose bool, initialLines ...string) *smartEditorSim {
	if len(initialLines) == 0 {
		initialLines = []string{""}
	}
	return &smartEditorSim{lines: initialLines, autoClose: autoClose}
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
		escaped := strings.HasSuffix(before, `\`)
		inDoubleQuote := (strings.Count(before, `"`)-strings.Count(before, `\"`))%2 == 1
		if !escaped {
			switch r {
			case '{':
				if !inDoubleQuote {
					inserted = "{}"
				}
			case '(':
				if !inDoubleQuote {
					inserted = "()"
				}
			case '[':
				if !inDoubleQuote {
					inserted = "[]"
				}
			case '"':
				// Real Monaco/CodeMirror only auto-closes when NOT already inside an unclosed string!
				if !inDoubleQuote {
					inserted = `""`
				}
			case '\'':
				if !inDoubleQuote && (len(before) == 0 || !isAlphaNum(rune(before[len(before)-1]))) {
					inserted = `''`
				}
			case '`':
				inserted = "``"
			case '*':
				if strings.HasSuffix(before, "/") {
					inserted = "**/"
				}
			}
		}
	}
	s.lines[s.row] = before + inserted + after
	s.col++

	// Simulate Web code editor autocomplete popup opening on identifier/member typing and closing on space/punctuation.
	if shouldDismissAutocomplete(r) {
		s.popupOpen = true
	} else {
		s.popupOpen = false
	}
	return nil
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func (s *smartEditorSim) backspaceOne() {
	if s.col > 0 {
		line := []rune(s.lines[s.row])
		s.lines[s.row] = string(line[:s.col-1]) + string(line[s.col:])
		s.col--
	} else if s.row > 0 {
		prev := []rune(s.lines[s.row-1])
		cur := s.lines[s.row]
		s.lines[s.row-1] = string(prev) + cur
		s.lines = append(s.lines[:s.row], s.lines[s.row+1:]...)
		s.row--
		s.col = len(prev)
	}
	// Backspace when popup is already closed does NOT reopen popup in Monaco/CodeMirror/Ace.
}

func (s *smartEditorSim) DismissAutocomplete(mode InputMode) error {
	if err := s.TypeRune(' ', mode); err != nil {
		return err
	}
	s.backspaceOne()
	return nil
}

func (s *smartEditorSim) PressEnter(_ bool) error {
	// If autocomplete popup is still open when Enter is pressed, real Monaco/CodeMirror/Ace accepts the highlighted completion instead of inserting a newline!
	if s.popupOpen {
		line := []rune(s.lines[s.row])
		hijack := "_AUTOCOMPLETE_HIJACKED"
		s.lines[s.row] = string(line[:s.col]) + hijack + string(line[s.col:])
		s.col += len([]rune(hijack))
		s.popupOpen = false
		return nil
	}

	line := []rune(s.lines[s.row])
	before := string(line[:s.col])
	after := string(line[s.col:])

	indent := 0
	for _, ch := range before {
		if ch == ' ' {
			indent++
		} else {
			break
		}
	}
	trimmed := strings.TrimRight(before, " ")
	extraIndent := strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "(") || strings.HasSuffix(trimmed, "[")

	// Simulate auto-brace-splitting when pressing Enter between '{|}'
	if strings.HasSuffix(trimmed, "{") && strings.HasPrefix(strings.TrimLeft(after, " "), "}") {
		s.lines[s.row] = before
		midLine := strings.Repeat(" ", indent+4)
		closeLine := strings.Repeat(" ", indent) + strings.TrimLeft(after, " ")
		tail := append([]string{midLine, closeLine}, s.lines[s.row+1:]...)
		s.lines = append(s.lines[:s.row+1], tail...)
		s.row++
		s.col = indent + 4
		return nil
	}

	if extraIndent {
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

func (s *smartEditorSim) ClearLineIndent(mode InputMode) error {
	// Faithfully execute the exact driver keystrokes against Web IDE Smart Home:
	// 1. Type sentinel ' '
	if err := s.TypeRune(' ', mode); err != nil {
		return err
	}
	// 2. Single Shift+Home using CodeMirror 6 / Monaco / Ace Smart Home target calculation:
	line := []rune(s.lines[s.row])
	firstNonWS := len(line)
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			firstNonWS = i
			break
		}
	}
	targetCol := 0
	if s.col != firstNonWS {
		targetCol = firstNonWS
	}
	// 3. Backspace deletes selection [min(targetCol, s.col) : max(targetCol, s.col)] (or 1 char if empty selection!)
	selStart, selEnd := targetCol, s.col
	if selStart > selEnd {
		selStart, selEnd = selEnd, selStart
	}
	if selStart == selEnd {
		s.backspaceOne()
	} else {
		s.lines[s.row] = string(line[:selStart]) + string(line[selEnd:])
		s.col = selStart
	}
	return nil
}

func (s *smartEditorSim) ClearLineRight(mode InputMode) error {
	// Faithfully execute sentinel ' ' + Left + Shift+End + Backspace on s.lines[s.row] ONLY:
	if err := s.TypeRune(' ', mode); err != nil {
		return err
	}
	if s.col > 0 {
		s.col-- // Left arrow
	}
	line := []rune(s.lines[s.row])
	// Shift+End selects [s.col : len(line)]; since sentinel ' ' is to the right, len(line) > s.col is ALWAYS non-empty!
	s.lines[s.row] = string(line[:s.col])
	return nil
}

func (s *smartEditorSim) Text() string {
	return strings.Join(s.lines, "\n")
}

func TestPreprocessCodeSpacesTabsAndNewlines(t *testing.T) {
	raw := "\uFEFFpackage main\r\n\r\nfunc add(a, b int) int {\r\t\u00A0\u200Breturn a + b\n  \t// aligned comment\n}"

	opts := DefaultOptions()
	opts.Indent = IndentKeep
	got := PreprocessText(raw, opts)
	want := "package main\n\nfunc add(a, b int) int {\n     return a + b\n    // aligned comment\n}"
	if got != want {
		t.Fatalf("PreprocessText default =\n%q\nwant:\n%q", got, want)
	}

	optsKeepTab := DefaultOptions()
	optsKeepTab.TabToSpaces = 0
	optsKeepTab.Indent = IndentKeep
	gotKeepTab := PreprocessText("if true {\r\n\treturn\r\n}", optsKeepTab)
	wantKeepTab := "if true {\n\treturn\n}"
	if gotKeepTab != wantKeepTab {
		t.Fatalf("PreprocessText keep tab = %q, want %q", gotKeepTab, wantKeepTab)
	}

	optsStrip := DefaultOptions()
	optsStrip.Indent = IndentStrip
	gotStrip := PreprocessText("def f():\n    \treturn 1", optsStrip)
	wantStrip := "def f():\nreturn 1"
	if gotStrip != wantStrip {
		t.Fatalf("PreprocessText strip = %q, want %q", gotStrip, wantStrip)
	}
}

func TestSmartEditorAutoCloseAndOJTemplateFooter(t *testing.T) {
	// Test 1: Python with nested indents, multi-level dedents, blank lines, identifiers before newline (autocomplete), strings, apostrophes ("don't"), and lists
	pythonCode := strings.Join([]string{
		"def solve(nums, target):",
		"\t# don't modify input",
		"\tseen = {}",
		"\tfor i, num in enumerate(nums):",
		"\t\tdiff = target - num",
		"\t\tif diff in seen:",
		"\t\t\tres = [seen[diff], i]",
		"\t\t\treturn res",
		"\t\tseen[num] = i",
		"",
		"\treturn []",
	}, "\r\n")

	expectedPython := strings.ReplaceAll(strings.ReplaceAll(pythonCode, "\r\n", "\n"), "\t", "    ")

	sim := newSmartEditorSim(true)
	eng := NewEngine(sim)

	opts := DefaultOptions() // Out-of-the-box defaults!
	opts.Delay = 0
	opts.CharInterval = 0
	opts.RandomJitter = false

	if err := eng.Run(context.Background(), pythonCode, opts, nil); err != nil {
		t.Fatalf("Engine.Run python failed: %v", err)
	}
	if sim.Text() != expectedPython {
		t.Fatalf("Python code mismatch in smart Web editor!\ngot:\n%s\nwant:\n%s", sim.Text(), expectedPython)
	}

	// Test 2: C++ pasted inside an OJ template that ALREADY has closing footer lines below cursor!
	// Includes /* block comment */, multi-line nested braces, escaped quotes \"hi\", char literal 'x', arrays [], identifiers at EOL.
	simOJ := newSmartEditorSim(true, "", "    }", "};")
	engOJ := NewEngine(simOJ)

	cppSnippet := strings.Join([]string{
		"/* calc */",
		"        int arr[2] = {1, 2};",
		"        if (arr[0] > 0) {",
		"            arr[1] = arr[0] + 1;",
		"        }",
		"        const char* s = \"say \\\"hi\\\" don't\";",
		"        char c = 'x';",
		"        return arr[1];",
	}, "\n")
	if err := engOJ.Run(context.Background(), cppSnippet, opts, nil); err != nil {
		t.Fatalf("Engine.Run C++ OJ template failed: %v", err)
	}
	wantOJ := cppSnippet + "\n    }\n};"
	if simOJ.Text() != wantOJ {
		t.Fatalf("C++ OJ template mismatch!\ngot:\n%s\nwant:\n%s", simOJ.Text(), wantOJ)
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
	opts.Indent = IndentKeep
	opts.ClearAutoCloseBracket = false
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
