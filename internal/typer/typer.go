package typer

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// InputMode controls how characters are synthesized at the OS level.
type InputMode string

const (
	// ModeUnicode injects Unicode character events directly.
	// Supports CJK (Chinese/Japanese/Korean), ASCII, symbols, and emojis,
	// and avoids interference from active local IMEs.
	ModeUnicode InputMode = "unicode"

	// ModeHardware simulates physical virtual-key / scancode keystrokes (with Shift).
	// Best for VM consoles (VMware/noVNC/KVM), games, or strict keycode listeners.
	// Non-ASCII characters gracefully fall back to ModeUnicode.
	ModeHardware InputMode = "hardware"
)

// NewlineMode controls how line breaks (\n) are typed.
type NewlineMode string

const (
	NewlineEnter      NewlineMode = "enter"
	NewlineShiftEnter NewlineMode = "shift_enter"
	NewlineSkip       NewlineMode = "skip"
)

// IndentMode controls how leading indentation on each line is handled when typing code.
type IndentMode string

const (
	// IndentKeep types every leading space/tab verbatim (ideal for plain textareas, Notepad, paste-mode terminals).
	IndentKeep IndentMode = "keep"
	// IndentResetAuto clears any editor auto-inserted spaces after Enter before typing exact line indentation
	// (ideal for VS Code, Monaco, CodeMirror, LeetCode, NowCoder, PTA, and Python dedents).
	IndentResetAuto IndentMode = "reset_auto"
	// IndentStrip removes leading spaces/tabs from each line and relies solely on the editor's auto-indent.
	IndentStrip IndentMode = "strip"
)

// Phase describes the current lifecycle stage of a typing session.
type Phase string

const (
	PhaseIdle      Phase = "idle"
	PhaseCountdown Phase = "countdown"
	PhaseTyping    Phase = "typing"
	PhasePaused    Phase = "paused"
	PhaseDone      Phase = "done"
	PhaseStopped   Phase = "stopped"
	PhaseError     Phase = "error"
)

var (
	// ErrStopped indicates typing was canceled by user action.
	ErrStopped = errors.New("typing stopped")
	// ErrStoppedByEsc indicates typing was aborted via the emergency ESC key.
	ErrStoppedByEsc = errors.New("typing aborted by ESC key")
	// ErrAlreadyRunning indicates a typing task is already active.
	ErrAlreadyRunning = errors.New("typer is already running")
	// ErrEmptyText indicates there is no content to type.
	ErrEmptyText = errors.New("text is empty")
)

// Options configures a keyboard simulation run.
type Options struct {
	Delay                 time.Duration
	CharInterval          time.Duration
	RandomJitter          bool
	Mode                  InputMode
	Newline               NewlineMode
	Indent                IndentMode
	StripLeadingIndent    bool // Kept as shorthand alias if Indent == ""
	TabToSpaces           int
	NormalizeCodeSpaces   bool
	ClearAutoCloseBracket bool
	EnableEscStop         bool
}

// DefaultOptions returns defaults tuned for pasting code into online editors & text boxes accurately.
func DefaultOptions() Options {
	return Options{
		Delay:                 3 * time.Second,
		CharInterval:          30 * time.Millisecond,
		RandomJitter:          true,
		Mode:                  ModeUnicode,
		Newline:               NewlineEnter,
		Indent:                IndentResetAuto,
		TabToSpaces:           4,
		NormalizeCodeSpaces:   true,
		ClearAutoCloseBracket: true,
		EnableEscStop:         true,
	}
}

func (o Options) effectiveIndentMode() IndentMode {
	if o.Indent != "" {
		return o.Indent
	}
	if o.StripLeadingIndent {
		return IndentStrip
	}
	return IndentKeep
}

// Progress reports live state back to the UI.
type Progress struct {
	Phase          Phase
	RemainingDelay time.Duration
	TypedRunes     int
	TotalRunes     int
	CurrentRune    rune
	StoppedByEsc   bool
	Err            error
}

// Driver abstracts platform-native keyboard injection.
type Driver interface {
	// CheckPermission verifies OS-level accessibility/input permissions.
	CheckPermission(prompt bool) error
	// Prepare initializes resources prior to typing a sequence.
	Prepare(mode InputMode) error
	// Close releases any OS handles acquired in Prepare.
	Close() error
	// TypeRune types a single printable character (including ASCII space ' ').
	TypeRune(r rune, mode InputMode) error
	// PressEnter sends Return (or Shift+Return when shift is true).
	PressEnter(shift bool) error
	// PressTab sends the Tab key.
	PressTab() error
	// ClearLineIndent clears any auto-indented whitespace before the cursor on the current line
	// without deleting the preceding newline or text after the cursor.
	ClearLineIndent(mode InputMode) error
	// ClearLineRight safely clears any auto-completed closing symbols (e.g. }, ), ], ", ', */, `)
	// to the right of the cursor on the current line ONLY, without ever deleting '\n' or lines below.
	ClearLineRight(mode InputMode) error
	// IsEscapePressed checks whether the physical ESC key is currently held down.
	IsEscapePressed() bool
}

// PreprocessText normalizes line endings, cleans web-copied invisible spaces, and applies tab/indentation rules.
func PreprocessText(text string, opts Options) string {
	if text == "" {
		return ""
	}

	normalized := text
	if opts.NormalizeCodeSpaces {
		// Remove UTF-8 BOM and zero-width characters often injected by web pages.
		normalized = strings.Map(func(r rune) rune {
			switch r {
			case '\uFEFF', '\u200B', '\u200C', '\u200D':
				return -1
			case '\u00A0': // HTML &nbsp; -> standard ASCII space
				return ' '
			default:
				return r
			}
		}, normalized)
	}

	// Normalize Windows CRLF and classic Mac CR to single LF so Enter isn't pressed twice.
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	lines := strings.Split(normalized, "\n")

	if opts.effectiveIndentMode() == IndentStrip {
		for i, line := range lines {
			lines[i] = strings.TrimLeft(line, " \t")
		}
	}

	if opts.TabToSpaces > 0 {
		for i, line := range lines {
			lines[i] = expandTabs(line, opts.TabToSpaces)
		}
	}

	normalized = strings.Join(lines, "\n")

	if opts.Newline == NewlineSkip {
		normalized = strings.ReplaceAll(normalized, "\n", "")
	}

	return normalized
}

// expandTabs expands '\t' characters to spaces aligned to multiples of tabWidth columns.
func expandTabs(line string, tabWidth int) string {
	if tabWidth <= 0 || !strings.ContainsRune(line, '\t') {
		return line
	}
	var b strings.Builder
	b.Grow(len(line) + 8)
	col := 0
	for _, r := range line {
		if r == '\t' {
			spaces := tabWidth - (col % tabWidth)
			for range spaces {
				b.WriteByte(' ')
			}
			col += spaces
		} else {
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}

// Engine coordinates countdown, pause/resume, ESC emergency stop, and character dispatch.
type Engine struct {
	mu      sync.Mutex
	driver  Driver
	running bool
	paused  bool
	cancel  context.CancelFunc
}

// NewEngine creates a typing Engine backed by the given platform Driver.
// If driver is nil, NewPlatformDriver() is used.
func NewEngine(driver Driver) *Engine {
	if driver == nil {
		driver = NewPlatformDriver()
	}
	return &Engine{driver: driver}
}

// CheckPermission checks (and optionally requests) OS input simulation permissions.
func (e *Engine) CheckPermission(prompt bool) error {
	return e.driver.CheckPermission(prompt)
}

// IsRunning reports whether a countdown or typing session is currently active.
func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// IsPaused reports whether the current session is paused.
func (e *Engine) IsPaused() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused
}

// SetPaused pauses or resumes an active typing session.
func (e *Engine) SetPaused(paused bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		e.paused = paused
	}
}

// TogglePause flips the pause state and returns the new paused value.
func (e *Engine) TogglePause() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return false
	}
	e.paused = !e.paused
	return e.paused
}

// Stop cancels any active countdown or typing session immediately.
func (e *Engine) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Run executes a full countdown + keyboard simulation session synchronously.
func (e *Engine) Run(parentCtx context.Context, rawText string, opts Options, onProgress func(Progress)) error {
	processed := PreprocessText(rawText, opts)
	totalRunes := utf8.RuneCountInString(processed)
	if totalRunes == 0 {
		return ErrEmptyText
	}

	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return ErrAlreadyRunning
	}
	ctx, cancel := context.WithCancel(parentCtx)
	e.running = true
	e.paused = false
	e.cancel = cancel
	e.mu.Unlock()

	defer func() {
		cancel()
		e.mu.Lock()
		e.running = false
		e.paused = false
		e.cancel = nil
		e.mu.Unlock()
	}()

	emit := func(p Progress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	if err := e.driver.CheckPermission(true); err != nil {
		emit(Progress{Phase: PhaseError, TotalRunes: totalRunes, Err: err})
		return err
	}

	if err := e.driver.Prepare(opts.Mode); err != nil {
		emit(Progress{Phase: PhaseError, TotalRunes: totalRunes, Err: err})
		return err
	}
	defer func() {
		_ = e.driver.Close()
	}()

	// 1. Countdown phase
	if opts.Delay > 0 {
		deadline := time.Now().Add(opts.Delay)
		emit(Progress{
			Phase:          PhaseCountdown,
			RemainingDelay: opts.Delay,
			TotalRunes:     totalRunes,
		})

		ticker := time.NewTicker(50 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				emit(Progress{Phase: PhaseStopped, TotalRunes: totalRunes, Err: ErrStopped})
				return ErrStopped
			case <-ticker.C:
				if opts.EnableEscStop && e.driver.IsEscapePressed() {
					ticker.Stop()
					emit(Progress{Phase: PhaseStopped, TotalRunes: totalRunes, StoppedByEsc: true, Err: ErrStoppedByEsc})
					return ErrStoppedByEsc
				}
				if e.IsPaused() {
					deadline = deadline.Add(50 * time.Millisecond)
					emit(Progress{
						Phase:          PhasePaused,
						RemainingDelay: time.Until(deadline),
						TotalRunes:     totalRunes,
					})
					continue
				}
				rem := time.Until(deadline)
				if rem <= 0 {
					ticker.Stop()
					goto startTyping
				}
				emit(Progress{
					Phase:          PhaseCountdown,
					RemainingDelay: rem,
					TotalRunes:     totalRunes,
				})
			}
		}
	}

startTyping:
	runes := []rune(processed)
	indentMode := opts.effectiveIndentMode()

	emit(Progress{
		Phase:      PhaseTyping,
		TypedRunes: 0,
		TotalRunes: totalRunes,
	})

	for idx, r := range runes {
		if err := e.waitIfPausedOrAborted(ctx, opts, idx, totalRunes, emit); err != nil {
			return err
		}

		var err error
		switch r {
		case '\n':
			err = e.driver.PressEnter(opts.Newline == NewlineShiftEnter)
			if err == nil && indentMode == IndentResetAuto {
				// Let the target code editor complete its Enter auto-indent before clearing line indent.
				if sleepErr := e.sleepInterruptible(ctx, 15*time.Millisecond, opts.EnableEscStop, idx+1, totalRunes, emit); sleepErr != nil {
					return sleepErr
				}
				err = e.driver.ClearLineIndent(opts.Mode)
			}
		case '\t':
			err = e.driver.PressTab()
		default:
			err = e.driver.TypeRune(r, opts.Mode)
			if err == nil && opts.ClearAutoCloseBracket && isAutoClosableOpener(r) {
				if sleepErr := e.sleepInterruptible(ctx, 10*time.Millisecond, opts.EnableEscStop, idx+1, totalRunes, emit); sleepErr != nil {
					return sleepErr
				}
				err = e.driver.ClearLineRight(opts.Mode)
			}
		}

		if err != nil {
			emit(Progress{
				Phase:       PhaseError,
				TypedRunes:  idx,
				TotalRunes:  totalRunes,
				CurrentRune: r,
				Err:         err,
			})
			return err
		}

		typed := idx + 1
		emit(Progress{
			Phase:       PhaseTyping,
			TypedRunes:  typed,
			TotalRunes:  totalRunes,
			CurrentRune: r,
		})

		if typed < totalRunes {
			interval := computeCharInterval(r, opts)
			if err := e.sleepInterruptible(ctx, interval, opts.EnableEscStop, typed, totalRunes, emit); err != nil {
				return err
			}
		}
	}

	emit(Progress{
		Phase:      PhaseDone,
		TypedRunes: totalRunes,
		TotalRunes: totalRunes,
	})
	return nil
}

func isAutoClosableOpener(r rune) bool {
	switch r {
	case '{', '[', '(', '"', '\'', '`', '*':
		return true
	default:
		return false
	}
}

func (e *Engine) waitIfPausedOrAborted(
	ctx context.Context,
	opts Options,
	typed int,
	total int,
	emit func(Progress),
) error {
	notifiedPause := false
	for {
		select {
		case <-ctx.Done():
			emit(Progress{Phase: PhaseStopped, TypedRunes: typed, TotalRunes: total, Err: ErrStopped})
			return ErrStopped
		default:
		}

		if opts.EnableEscStop && e.driver.IsEscapePressed() {
			emit(Progress{Phase: PhaseStopped, TypedRunes: typed, TotalRunes: total, StoppedByEsc: true, Err: ErrStoppedByEsc})
			return ErrStoppedByEsc
		}

		if !e.IsPaused() {
			if notifiedPause {
				emit(Progress{Phase: PhaseTyping, TypedRunes: typed, TotalRunes: total})
			}
			return nil
		}

		if !notifiedPause {
			notifiedPause = true
			emit(Progress{Phase: PhasePaused, TypedRunes: typed, TotalRunes: total})
		}

		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			emit(Progress{Phase: PhaseStopped, TypedRunes: typed, TotalRunes: total, Err: ErrStopped})
			return ErrStopped
		case <-timer.C:
		}
	}
}

func (e *Engine) sleepInterruptible(
	ctx context.Context,
	d time.Duration,
	enableEsc bool,
	typed int,
	total int,
	emit func(Progress),
) error {
	if d <= 0 {
		return nil
	}
	const step = 40 * time.Millisecond
	remaining := d
	for remaining > 0 {
		chunk := remaining
		if chunk > step {
			chunk = step
		}
		timer := time.NewTimer(chunk)
		select {
		case <-ctx.Done():
			timer.Stop()
			emit(Progress{Phase: PhaseStopped, TypedRunes: typed, TotalRunes: total, Err: ErrStopped})
			return ErrStopped
		case <-timer.C:
		}
		if enableEsc && e.driver.IsEscapePressed() {
			emit(Progress{Phase: PhaseStopped, TypedRunes: typed, TotalRunes: total, StoppedByEsc: true, Err: ErrStoppedByEsc})
			return ErrStoppedByEsc
		}
		remaining -= chunk
	}
	return nil
}

func computeCharInterval(r rune, opts Options) time.Duration {
	base := opts.CharInterval
	if base <= 0 {
		if r == '\n' {
			return 15 * time.Millisecond
		}
		return 0
	}
	d := base
	if opts.RandomJitter {
		factor := 0.65 + rand.Float64()*0.80
		switch r {
		case '\n', '.', ',', ';', '!', '?', '。', '，', '；', '！', '？':
			factor += 0.35
		}
		d = time.Duration(float64(base) * factor)
		if d < time.Millisecond {
			d = time.Millisecond
		}
	}
	// Ensure target editor finishes processing Enter before typing next line's leading spaces.
	if r == '\n' && d < 15*time.Millisecond {
		d = 15 * time.Millisecond
	}
	return d
}
