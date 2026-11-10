//go:build windows

package typer

import (
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	inputKeyboard = 1

	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	keyeventfUnicode     = 0x0004

	vkBack    = 0x08
	vkTab     = 0x09
	vkReturn  = 0x0D
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12
	vkEscape  = 0x1B
	vkEnd     = 0x23
	vkHome    = 0x24
	vkLeft    = 0x25
	vkDelete  = 0x2E

	mapvkVkToVsc = 0
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procVkKeyScanW       = user32.NewProc("VkKeyScanW")
	procMapVirtualKeyW   = user32.NewProc("MapVirtualKeyW")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

// keybdInput matches Win32 KEYBDINPUT padded to union size (sizeof(MOUSEINPUT)):
// 32 bytes on 64-bit Windows, 24 bytes on 32-bit Windows.
type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
	_pad        [8]byte
}

// winInput matches Win32 INPUT:
// 40 bytes on 64-bit Windows, 28 bytes on 32-bit Windows.
type winInput struct {
	inputType uint32
	ki        keybdInput
}

type windowsDriver struct{}

// NewPlatformDriver creates the Windows native SendInput keyboard driver.
func NewPlatformDriver() Driver {
	return &windowsDriver{}
}

func (d *windowsDriver) CheckPermission(_ bool) error {
	return nil
}

func (d *windowsDriver) Prepare(_ InputMode) error {
	return nil
}

func (d *windowsDriver) Close() error {
	return nil
}

func (d *windowsDriver) TypeRune(r rune, mode InputMode) error {
	if mode == ModeHardware && r <= 0xFFFF {
		if ok, err := d.typeRuneHardware(r); ok {
			return err
		}
	}
	return d.typeRuneUnicode(r)
}

func (d *windowsDriver) typeRuneUnicode(r rune) error {
	units := utf16.Encode([]rune{r})
	inputs := make([]winInput, 0, len(units)*2)
	for _, u := range units {
		inputs = append(inputs,
			winInput{
				inputType: inputKeyboard,
				ki: keybdInput{
					wVk:     0,
					wScan:   u,
					dwFlags: keyeventfUnicode,
				},
			},
			winInput{
				inputType: inputKeyboard,
				ki: keybdInput{
					wVk:     0,
					wScan:   u,
					dwFlags: keyeventfUnicode | keyeventfKeyUp,
				},
			},
		)
	}
	return sendWinInputs(inputs)
}

func (d *windowsDriver) typeRuneHardware(r rune) (bool, error) {
	ret, _, _ := procVkKeyScanW.Call(uintptr(uint16(r)))
	packed := int16(ret)
	if packed == -1 {
		return false, nil
	}

	vk := uint16(packed & 0xFF)
	shiftState := uint16((packed >> 8) & 0xFF)

	var mods []uint16
	if shiftState&1 != 0 {
		mods = append(mods, vkShift)
	}
	if shiftState&2 != 0 {
		mods = append(mods, vkControl)
	}
	if shiftState&4 != 0 {
		mods = append(mods, vkMenu)
	}

	inputs := make([]winInput, 0, len(mods)*2+2)
	for _, m := range mods {
		inputs = append(inputs, makeVKInput(m, false, false))
	}
	inputs = append(inputs, makeVKInput(vk, false, false), makeVKInput(vk, true, false))
	for i := len(mods) - 1; i >= 0; i-- {
		inputs = append(inputs, makeVKInput(mods[i], true, false))
	}
	return true, sendWinInputs(inputs)
}

func (d *windowsDriver) PressEnter(shift bool) error {
	if shift {
		return sendWinInputs([]winInput{
			makeVKInput(vkShift, false, false),
			makeVKInput(vkReturn, false, false),
			makeVKInput(vkReturn, true, false),
			makeVKInput(vkShift, true, false),
		})
	}
	return sendWinInputs([]winInput{
		makeVKInput(vkReturn, false, false),
		makeVKInput(vkReturn, true, false),
	})
}

func (d *windowsDriver) PressTab() error {
	return sendWinInputs([]winInput{
		makeVKInput(vkTab, false, false),
		makeVKInput(vkTab, true, false),
	})
}

func (d *windowsDriver) ClearLineIndent(mode InputMode) error {
	// 1. Insert one sentinel space so the current line always has >= 1 whitespace char before cursor.
	if err := d.TypeRune(' ', mode); err != nil {
		return err
	}
	time.Sleep(8 * time.Millisecond)

	// 2. Shift + Home (with EXTENDEDKEY so NumLock ON never turns Home into Numpad 7) selects all leading spaces on this line.
	if err := sendWinInputs([]winInput{
		makeVKInput(vkShift, false, false),
		makeVKInput(vkHome, false, true),
		makeVKInput(vkHome, true, true),
		makeVKInput(vkShift, true, false),
	}); err != nil {
		return err
	}
	time.Sleep(8 * time.Millisecond)

	// 3. Backspace deletes only the non-empty whitespace selection on this line, leaving cursor at column 0.
	return sendWinInputs([]winInput{
		makeVKInput(vkBack, false, false),
		makeVKInput(vkBack, true, false),
	})
}

func (d *windowsDriver) ClearLineRight(mode InputMode) error {
	// 1. Insert one sentinel space, then move Left 1 char so >= 1 char (' ' + any auto-closed symbols) is strictly on this line before '\n'.
	if err := d.TypeRune(' ', mode); err != nil {
		return err
	}
	time.Sleep(6 * time.Millisecond)

	if err := sendWinInputs([]winInput{
		makeVKInput(vkLeft, false, true),
		makeVKInput(vkLeft, true, true),
	}); err != nil {
		return err
	}
	time.Sleep(6 * time.Millisecond)

	// 2. Shift + End (with EXTENDEDKEY) selects strictly to the end of the current line (never crossing '\n').
	if err := sendWinInputs([]winInput{
		makeVKInput(vkShift, false, false),
		makeVKInput(vkEnd, false, true),
		makeVKInput(vkEnd, true, true),
		makeVKInput(vkShift, true, false),
	}); err != nil {
		return err
	}
	time.Sleep(6 * time.Millisecond)

	// 3. Backspace removes the non-empty selection on this line, leaving '\n' and lines below 100% untouched.
	return sendWinInputs([]winInput{
		makeVKInput(vkBack, false, false),
		makeVKInput(vkBack, true, false),
	})
}

func (d *windowsDriver) IsEscapePressed() bool {
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(vkEscape))
	return (uint16(ret) & 0x8000) != 0
}

func makeVKInput(vk uint16, keyUp bool, extended bool) winInput {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), uintptr(mapvkVkToVsc))
	var flags uint32
	if extended {
		flags |= keyeventfExtendedKey
	}
	if keyUp {
		flags |= keyeventfKeyUp
	}
	return winInput{
		inputType: inputKeyboard,
		ki: keybdInput{
			wVk:     vk,
			wScan:   uint16(scan),
			dwFlags: flags,
		},
	}
}

func sendWinInputs(inputs []winInput) error {
	if len(inputs) == 0 {
		return nil
	}
	sent, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if sent != uintptr(len(inputs)) {
		if err != nil && err != syscall.Errno(0) {
			return fmt.Errorf("Windows SendInput 调用失败 (%w)；若目标窗口以管理员运行，请以管理员身份运行本程序", err)
		}
		return fmt.Errorf("Windows SendInput 未能完整注入按键 (%d/%d)；若目标窗口以管理员运行，请以管理员身份运行本程序", sent, len(inputs))
	}
	return nil
}
