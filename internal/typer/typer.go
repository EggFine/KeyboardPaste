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
	Delay              time.Duration
	CharInterval       time.Duration
	RandomJitter       bool
	Mode               InputMode
	Newline            NewlineMode
	StripLeadingIndent bool
	TabToSpaces        int
	EnableEscStop      bool
}

// DefaultOptions returns sensible defaults for general anti-paste bypass scenarios.
func DefaultOptions() Options {
	return Options{
		Delay:              3 * time.Second,
		CharInterval:       30 * time.Millisecond,
		RandomJitter:       true,
		Mode:               ModeUnicode,
		Newline:            NewlineEnter,
		StripLeadingIndent: false,
		TabToSpaces:        0,
		EnableEscStop:      true,
	}
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
	// If prompt is true on platforms like macOS, it triggers the system permission dialog.
	CheckPermission(prompt bool) error
	// Prepare initializes resources prior to typing a sequence.
	Prepare(mode InputMode) error
	// Close releases any OS handles acquired in Prepare.
	Close() error
	// TypeRune types a single printable character.
	TypeRune(r rune, mode InputMode) error
	// PressEnter sends Return (or Shift+Return when shift is true).
	PressEnter(shift bool) error
	// PressTab sends the Tab key.
	PressTab() error
	// IsEscapePressed checks whether the physical ESC key is currently held down.
	IsEscapePressed() bool
}

// PreprocessText normalizes line endings and applies indentation/tab/newline rules.
func PreprocessText(text string, opts Options) string {
	if text == "" {
		return ""
	}
	// Normalize Windows CRLF and classic Mac CR to single LF so Enter isn't pressed twice.
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	if opts.StripLeadingIndent {
		lines := strings.Split(normalized, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimLeft(line, " \t")
		}
		normalized = strings.Join(lines, "\n")
	}

	if opts.Newline == NewlineSkip {
		normalized = strings.ReplaceAll(normalized, "\n", "")
	}

	if opts.TabToSpaces > 0 {
		normalized = strings.ReplaceAll(normalized, "\t", strings.Repeat(" ", opts.TabToSpaces))
	}

	return normalized
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
// Callers in a GUI typically invoke Run inside a goroutine.
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
				emit(Progress{Phase: PhaseStopped, TotalRunes: totalRunes, Err: ErrStopped})
				return ErrStopped
			case <-ticker.C:
				if opts.EnableEscStop && e.driver.IsEscapePressed() {
					ticker.Stop()
					emit(Progress{Phase: PhaseStopped, TotalRunes: totalRunes, StoppedByEsc: true, Err: ErrStoppedByEsc})
					return ErrStoppedByEsc
				}
				if e.IsPaused() {
					// Extend deadline while paused during countdown
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
	emit(Progress{
		Phase:      PhaseTyping,
		TypedRunes: 0,
		TotalRunes: totalRunes,
	})

	for idx, r := range runes {
		// Check cancellation / pause / ESC before each character
		if err := e.waitIfPausedOrAborted(ctx, opts, idx, totalRunes, emit); err != nil {
			return err
		}

		// Dispatch keystroke
		var err error
		switch r {
		case '\n':
			err = e.driver.PressEnter(opts.Newline == NewlineShiftEnter)
		case '\t':
			err = e.driver.PressTab()
		default:
			err = e.driver.TypeRune(r, opts.Mode)
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

		// Wait interval before next character (skip trailing delay after last rune)
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
		return 0
	}
	if !opts.RandomJitter {
		return base
	}
	// Humanized typing rhythm: 0.65x ~ 1.45x jitter, with slight natural pause on punctuation/newline.
	factor := 0.65 + rand.Float64()*0.80
	switch r {
	case '\n', '.', ',', ';', '!', '?', '。', '，', '；', '！', '？':
		factor += 0.35
	}
	d := time.Duration(float64(base) * factor)
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return d
}
