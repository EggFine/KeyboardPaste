package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/EggFine/KeyboardPaste/internal/typer"
)

const (
	modeLabelUnicode  = "通用 Unicode 模式（支持中英文 / 防输入法干扰）"
	modeLabelHardware = "硬件按键模式（物理键码 / 适合虚拟机与严格密码框）"

	newlineLabelEnter      = "回车发送 Enter（默认）"
	newlineLabelShiftEnter = "回车发送 Shift+Enter（聊天换行不发送）"
	newlineLabelSkip       = "忽略所有换行符"

	tabLabel4Spaces = "Tab 转 4 个空格（默认推荐）"
	tabLabel2Spaces = "Tab 转 2 个空格"
	tabLabel8Spaces = "Tab 转 8 个空格"
	tabLabelKeepTab = "保留原 Tab 按键"

	indentLabelKeep      = "原样输入空格缩进（普通输入框 / 记事本）"
	indentLabelResetAuto = "清除自动缩进后重打（推荐：在线OJ / IDE）"
	indentLabelStrip     = "去除每行前导缩进（仅靠编辑器缩进）"
)

// Run initializes and runs the KeyboardPaste Fyne desktop application in portable stateless mode.
func Run(version string) {
	fyneApp := app.New()
	fyneApp.SetIcon(AppIcon())

	title := "KeyboardPaste - 模拟键盘输入工具"
	if version != "" && version != "dev" {
		title = fmt.Sprintf("KeyboardPaste %s - 模拟键盘输入工具", version)
	}
	win := fyneApp.NewWindow(title)
	win.Resize(fyne.NewSize(720, 660))

	engine := typer.NewEngine(nil)
	ui := buildMainUI(fyneApp, win, engine)
	win.SetContent(ui.root)

	win.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape && engine.IsRunning() {
			engine.Stop()
		}
	})
	win.SetOnClosed(func() {
		engine.Stop()
	})

	win.ShowAndRun()
}

type appUI struct {
	root *fyne.Container
}

func buildMainUI(fyneApp fyne.App, win fyne.Window, engine *typer.Engine) *appUI {
	defaults := typer.DefaultOptions()

	// --- Monospace Code/Text Editor & Top Bar ---
	editor := widget.NewMultiLineEntry()
	editor.Wrapping = fyne.TextWrapBreak
	editor.TextStyle = fyne.TextStyle{Monospace: true}
	editor.SetPlaceHolder("在此粘贴需要模拟键盘输入的代码或文本...\n\n代码粘贴小贴士：\n• 默认自动将 Tab 缩进对齐转换为 4 个空格，并净化网页复制的特殊空白符 (&nbsp;)\n• 若目标是在线 OJ / VS Code 等会自动缩进的代码编辑器，建议将下方「行首缩进」切换为「清除自动缩进后重打」，完美还原多级缩进与 Python 退格缩进")

	statsLabel := widget.NewLabel("0 字符 · 0 行 · 预计 ~0.0s")

	delayEntry := widget.NewEntry()
	delayEntry.SetText(formatDelay(defaults.Delay.Seconds()))

	intervalEntry := widget.NewEntry()
	intervalEntry.SetText(strconv.Itoa(int(defaults.CharInterval.Milliseconds())))

	updateStats := func() {
		text := editor.Text
		normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
		runeCount := utf8.RuneCountInString(normalized)
		lineCount := 0
		if runeCount > 0 {
			lineCount = strings.Count(normalized, "\n") + 1
		}
		intervalMs := parseIntervalMs(intervalEntry.Text, 30)
		estSec := float64(runeCount*intervalMs) / 1000.0
		statsLabel.SetText(fmt.Sprintf("%d 字符 · %d 行 · 预计 ~%.1fs", runeCount, lineCount, estSec))
	}

	editor.OnChanged = func(_ string) {
		updateStats()
	}

	pasteBtn := widget.NewButtonWithIcon("粘贴剪贴板", theme.ContentPasteIcon(), func() {
		content := fyneApp.Clipboard().Content()
		if content == "" {
			return
		}
		editor.SetText(content)
	})
	appendBtn := widget.NewButtonWithIcon("追加粘贴", theme.ContentAddIcon(), func() {
		content := fyneApp.Clipboard().Content()
		if content == "" {
			return
		}
		if editor.Text == "" {
			editor.SetText(content)
		} else {
			editor.SetText(editor.Text + "\n" + content)
		}
	})
	clearBtn := widget.NewButtonWithIcon("清空", theme.DeleteIcon(), func() {
		editor.SetText("")
	})

	topBar := container.NewHBox(
		pasteBtn,
		appendBtn,
		clearBtn,
		layout.NewSpacer(),
		statsLabel,
	)

	// --- Timing Controls ---
	delayMinusBtn := widget.NewButton(" - ", func() {
		v := math.Max(0, parseDelaySeconds(delayEntry.Text, 3.0)-1.0)
		delayEntry.SetText(formatDelay(v))
	})
	delayPlusBtn := widget.NewButton(" + ", func() {
		v := math.Min(60, parseDelaySeconds(delayEntry.Text, 3.0)+1.0)
		delayEntry.SetText(formatDelay(v))
	})
	delayRow := container.NewBorder(
		nil, nil,
		widget.NewLabel("启动延迟:"),
		container.NewHBox(delayMinusBtn, delayPlusBtn, widget.NewLabel("秒")),
		delayEntry,
	)

	intervalMinusBtn := widget.NewButton(" - ", func() {
		cur := parseIntervalMs(intervalEntry.Text, 30)
		step := 10
		if cur <= 10 {
			step = 2
		}
		next := cur - step
		if next < 1 {
			next = 1
		}
		intervalEntry.SetText(strconv.Itoa(next))
		updateStats()
	})
	intervalPlusBtn := widget.NewButton(" + ", func() {
		cur := parseIntervalMs(intervalEntry.Text, 30)
		step := 10
		if cur < 10 {
			step = 5
		}
		next := cur + step
		if next > 2000 {
			next = 2000
		}
		intervalEntry.SetText(strconv.Itoa(next))
		updateStats()
	})
	intervalEntry.OnChanged = func(_ string) {
		updateStats()
	}
	intervalRow := container.NewBorder(
		nil, nil,
		widget.NewLabel("单字间隔:"),
		container.NewHBox(intervalMinusBtn, intervalPlusBtn, widget.NewLabel("ms")),
		intervalEntry,
	)

	timingGrid := container.NewGridWithColumns(2, delayRow, intervalRow)

	// --- Mode, Newline, Tab & Indentation Selectors ---
	modeSelect := widget.NewSelect([]string{modeLabelUnicode, modeLabelHardware}, nil)
	modeSelect.SetSelected(modeLabelUnicode)

	newlineSelect := widget.NewSelect([]string{newlineLabelEnter, newlineLabelShiftEnter, newlineLabelSkip}, nil)
	newlineSelect.SetSelected(newlineLabelEnter)

	tabSelect := widget.NewSelect([]string{tabLabel4Spaces, tabLabel2Spaces, tabLabel8Spaces, tabLabelKeepTab}, nil)
	tabSelect.SetSelected(tabLabel4Spaces)

	indentSelect := widget.NewSelect([]string{indentLabelKeep, indentLabelResetAuto, indentLabelStrip}, nil)
	indentSelect.SetSelected(indentLabelKeep)

	selectGrid1 := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, widget.NewLabel("输入模式:"), nil, modeSelect),
		container.NewBorder(nil, nil, widget.NewLabel("换行策略:"), nil, newlineSelect),
	)
	selectGrid2 := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, widget.NewLabel("Tab 转换:"), nil, tabSelect),
		container.NewBorder(nil, nil, widget.NewLabel("行首缩进:"), nil, indentSelect),
	)

	// --- Checkboxes ---
	jitterCheck := widget.NewCheck("拟人随机打字速率抖动", nil)
	jitterCheck.SetChecked(defaults.RandomJitter)

	escStopCheck := widget.NewCheck("按 ESC 键随时紧急停止", nil)
	escStopCheck.SetChecked(defaults.EnableEscStop)

	cleanSpaceCheck := widget.NewCheck("净化网页特殊空白（&nbsp; 转标准空格）", nil)
	cleanSpaceCheck.SetChecked(defaults.NormalizeCodeSpaces)

	autoBracketCheck := widget.NewCheck("消除编辑器自动补全右括号/引号", nil)
	autoBracketCheck.SetChecked(defaults.ClearAutoCloseBracket)

	checksGrid := container.NewGridWithColumns(2,
		jitterCheck,
		escStopCheck,
		cleanSpaceCheck,
		autoBracketCheck,
	)

	// --- Progress & Status ---
	progressBar := widget.NewProgressBar()
	progressBar.SetValue(0)

	defaultTip := buildPlatformHint()
	statusLabel := widget.NewLabel(defaultTip)
	statusLabel.Wrapping = fyne.TextWrapWord

	// --- Action Buttons ---
	var startBtn, pauseBtn, stopBtn *widget.Button

	setControlsRunning := func(running bool) {
		if running {
			startBtn.Disable()
			pauseBtn.Enable()
			stopBtn.Enable()
			editor.Disable()
			pasteBtn.Disable()
			appendBtn.Disable()
			clearBtn.Disable()
			delayEntry.Disable()
			delayMinusBtn.Disable()
			delayPlusBtn.Disable()
			intervalEntry.Disable()
			intervalMinusBtn.Disable()
			intervalPlusBtn.Disable()
			modeSelect.Disable()
			newlineSelect.Disable()
			tabSelect.Disable()
			indentSelect.Disable()
			jitterCheck.Disable()
			escStopCheck.Disable()
			cleanSpaceCheck.Disable()
			autoBracketCheck.Disable()
		} else {
			startBtn.Enable()
			pauseBtn.SetText("暂停")
			pauseBtn.SetIcon(theme.MediaPauseIcon())
			pauseBtn.Disable()
			stopBtn.Disable()
			editor.Enable()
			pasteBtn.Enable()
			appendBtn.Enable()
			clearBtn.Enable()
			delayEntry.Enable()
			delayMinusBtn.Enable()
			delayPlusBtn.Enable()
			intervalEntry.Enable()
			intervalMinusBtn.Enable()
			intervalPlusBtn.Enable()
			modeSelect.Enable()
			newlineSelect.Enable()
			tabSelect.Enable()
			indentSelect.Enable()
			jitterCheck.Enable()
			escStopCheck.Enable()
			cleanSpaceCheck.Enable()
			autoBracketCheck.Enable()
		}
	}

	startBtn = widget.NewButtonWithIcon("开始模拟输入", theme.MediaPlayIcon(), func() {
		rawText := editor.Text
		if rawText == "" {
			dialog.ShowInformation("提示", "请先粘贴或输入需要模拟打字的代码或文本内容。", win)
			return
		}

		delaySec := parseDelaySeconds(delayEntry.Text, 3.0)
		intervalMs := parseIntervalMs(intervalEntry.Text, 30)

		opts := typer.Options{
			Delay:                 time.Duration(delaySec * float64(time.Second)),
			CharInterval:          time.Duration(intervalMs) * time.Millisecond,
			RandomJitter:          jitterCheck.Checked,
			Mode:                  selectedInputMode(modeSelect.Selected),
			Newline:               selectedNewlineMode(newlineSelect.Selected),
			Indent:                selectedIndentMode(indentSelect.Selected),
			TabToSpaces:           selectedTabSpaces(tabSelect.Selected),
			NormalizeCodeSpaces:   cleanSpaceCheck.Checked,
			ClearAutoCloseBracket: autoBracketCheck.Checked,
			EnableEscStop:         escStopCheck.Checked,
		}

		if utf8.RuneCountInString(typer.PreprocessText(rawText, opts)) == 0 {
			dialog.ShowInformation("提示", "预处理后的文本为空，请检查内容或换行设置。", win)
			return
		}

		progressBar.SetValue(0)
		setControlsRunning(true)

		go func() {
			err := engine.Run(context.Background(), rawText, opts, func(p typer.Progress) {
				fyne.Do(func() {
					switch p.Phase {
					case typer.PhaseCountdown:
						statusLabel.SetText(fmt.Sprintf(
							"⏳ 倒计时 %.1f 秒 — 请立即将鼠标点击到目标输入框！（按 ESC 可取消）",
							math.Max(0, p.RemainingDelay.Seconds()),
						))
						progressBar.SetValue(0)
					case typer.PhaseTyping:
						ratio := 0.0
						if p.TotalRunes > 0 {
							ratio = float64(p.TypedRunes) / float64(p.TotalRunes)
						}
						progressBar.SetValue(ratio)
						escHint := ""
						if opts.EnableEscStop {
							escHint = "（按 ESC 键可紧急中止）"
						}
						statusLabel.SetText(fmt.Sprintf(
							"⌨️ 正在模拟输入：%d / %d 字符%s",
							p.TypedRunes, p.TotalRunes, escHint,
						))
					case typer.PhasePaused:
						if p.TypedRunes == 0 && p.RemainingDelay > 0 {
							statusLabel.SetText(fmt.Sprintf("⏸️ 倒计时已暂停（剩余 %.1f 秒）— 点击「继续」恢复", p.RemainingDelay.Seconds()))
						} else {
							statusLabel.SetText(fmt.Sprintf("⏸️ 已暂停：%d / %d 字符 — 点击「继续」恢复输入", p.TypedRunes, p.TotalRunes))
						}
					case typer.PhaseDone:
						progressBar.SetValue(1)
						statusLabel.SetText(fmt.Sprintf("✅ 输入完成！共成功模拟输入 %d 个字符。", p.TotalRunes))
					case typer.PhaseStopped:
						if p.StoppedByEsc {
							statusLabel.SetText(fmt.Sprintf("🛑 已通过 ESC 键紧急停止（已输入 %d / %d 字符）。", p.TypedRunes, p.TotalRunes))
						} else {
							statusLabel.SetText(fmt.Sprintf("⏹️ 已手动停止（已输入 %d / %d 字符）。", p.TypedRunes, p.TotalRunes))
						}
					case typer.PhaseError:
						statusLabel.SetText(fmt.Sprintf("❌ 输入出错：%v", p.Err))
					}
				})
			})

			fyne.Do(func() {
				setControlsRunning(false)
				if err != nil && !errors.Is(err, typer.ErrStopped) && !errors.Is(err, typer.ErrStoppedByEsc) {
					dialog.ShowError(err, win)
				}
			})
		}()
	})
	startBtn.Importance = widget.HighImportance

	pauseBtn = widget.NewButtonWithIcon("暂停", theme.MediaPauseIcon(), func() {
		paused := engine.TogglePause()
		if paused {
			pauseBtn.SetText("继续")
			pauseBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			pauseBtn.SetText("暂停")
			pauseBtn.SetIcon(theme.MediaPauseIcon())
		}
	})
	pauseBtn.Disable()

	stopBtn = widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), func() {
		engine.Stop()
	})
	stopBtn.Importance = widget.DangerImportance
	stopBtn.Disable()

	actionButtons := container.NewGridWithColumns(3, startBtn, pauseBtn, stopBtn)

	bottomPanel := container.NewVBox(
		widget.NewSeparator(),
		timingGrid,
		selectGrid1,
		selectGrid2,
		checksGrid,
		progressBar,
		statusLabel,
		actionButtons,
	)

	root := container.NewBorder(
		container.NewVBox(topBar, widget.NewSeparator()),
		bottomPanel,
		nil,
		nil,
		container.NewPadded(editor),
	)

	updateStats()
	return &appUI{root: container.NewPadded(root)}
}

func buildPlatformHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "💡 就绪：首次使用需在「系统设置 -> 隐私与安全性 -> 辅助功能」中授权允许控制电脑。"
	case "windows":
		return "💡 就绪：粘贴代码后点击开始；若目标程序以管理员权限运行，请右键「以管理员身份运行」本工具。"
	default:
		return "💡 就绪：粘贴代码后点击「开始模拟输入」，在倒计时结束前点击目标输入框即可。"
	}
}

func parseDelaySeconds(raw string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	if v < 0 {
		return 0
	}
	if v > 120 {
		return 120
	}
	return v
}

func formatDelay(v float64) string {
	if math.Abs(v-math.Round(v)) < 0.01 {
		return strconv.Itoa(int(math.Round(v)))
	}
	return fmt.Sprintf("%.1f", v)
}

func parseIntervalMs(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	if v < 1 {
		return 1
	}
	if v > 5000 {
		return 5000
	}
	return v
}

func selectedInputMode(label string) typer.InputMode {
	if label == modeLabelHardware {
		return typer.ModeHardware
	}
	return typer.ModeUnicode
}

func selectedNewlineMode(label string) typer.NewlineMode {
	switch label {
	case newlineLabelShiftEnter:
		return typer.NewlineShiftEnter
	case newlineLabelSkip:
		return typer.NewlineSkip
	default:
		return typer.NewlineEnter
	}
}

func selectedTabSpaces(label string) int {
	switch label {
	case tabLabel2Spaces:
		return 2
	case tabLabel8Spaces:
		return 8
	case tabLabelKeepTab:
		return 0
	default:
		return 4
	}
}

func selectedIndentMode(label string) typer.IndentMode {
	switch label {
	case indentLabelResetAuto:
		return typer.IndentResetAuto
	case indentLabelStrip:
		return typer.IndentStrip
	default:
		return typer.IndentKeep
	}
}
