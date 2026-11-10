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

func (m *mockDriver) IsEscapePressed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.escAfter > 0 && m.typedCount >= m.escAfter
}

func TestPreprocessText(t *testing.T) {
	raw := "func main() {\r\n\tfmt.Println(\"你好\")\r    return\n}"

	gotDefault := PreprocessText(raw, DefaultOptions())
	wantDefault := "func main() {\n\tfmt.Println(\"你好\")\n    return\n}"
	if gotDefault != wantDefault {
		t.Fatalf("PreprocessText default = %q, want %q", gotDefault, wantDefault)
	}

	optsCode := DefaultOptions()
	optsCode.StripLeadingIndent = true
	optsCode.TabToSpaces = 4
	gotCode := PreprocessText(raw, optsCode)
	wantCode := "func main() {\nfmt.Println(\"你好\")\nreturn\n}"
	if gotCode != wantCode {
		t.Fatalf("PreprocessText strip indent = %q, want %q", gotCode, wantCode)
	}

	optsTab := DefaultOptions()
	optsTab.TabToSpaces = 2
	optsTab.Newline = NewlineSkip
	gotTab := PreprocessText("a\tb\r\nc", optsTab)
	if gotTab != "a  bc" {
		t.Fatalf("PreprocessText tab+skip = %q, want %q", gotTab, "a  bc")
	}
}

func TestEngineRunUnicodeAndSpecialKeys(t *testing.T) {
	drv := &mockDriver{}
	eng := NewEngine(drv)

	opts := DefaultOptions()
	opts.Delay = 60 * time.Millisecond
	opts.CharInterval = 2 * time.Millisecond
	opts.RandomJitter = false
	opts.Newline = NewlineShiftEnter

	var phases []Phase
	err := eng.Run(context.Background(), "A你\r\n\t!", opts, func(p Progress) {
		phases = append(phases, p.Phase)
	})
	if err != nil {
		t.Fatalf("Engine.Run unexpected error: %v", err)
	}

	wantEvents := []string{
		"unicode:A",
		"unicode:你",
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
