//go:build darwin

package typer

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>

static int kp_check_accessibility(int prompt) {
	if (!prompt) {
		return AXIsProcessTrusted() ? 1 : 0;
	}
	const void *keys[] = { kAXTrustedCheckOptionPrompt };
	const void *values[] = { kCFBooleanTrue };
	CFDictionaryRef options = CFDictionaryCreate(
		kCFAllocatorDefault,
		keys,
		values,
		1,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	Boolean trusted = AXIsProcessTrustedWithOptions(options);
	if (options != NULL) {
		CFRelease(options);
	}
	return trusted ? 1 : 0;
}

static int kp_post_unicode(const UniChar *chars, int length) {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	CGEventRef down = CGEventCreateKeyboardEvent(source, (CGKeyCode)0, true);
	CGEventRef up = CGEventCreateKeyboardEvent(source, (CGKeyCode)0, false);
	if (down == NULL || up == NULL) {
		if (down != NULL) CFRelease(down);
		if (up != NULL) CFRelease(up);
		if (source != NULL) CFRelease(source);
		return -1;
	}
	CGEventSetFlags(down, 0);
	CGEventSetFlags(up, 0);
	CGEventKeyboardSetUnicodeString(down, (UniCharCount)length, chars);
	CGEventKeyboardSetUnicodeString(up, (UniCharCount)length, chars);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
	if (source != NULL) CFRelease(source);
	return 0;
}

static int kp_post_virtual_key(CGKeyCode keyCode, int withShift) {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	CGEventRef down = CGEventCreateKeyboardEvent(source, keyCode, true);
	CGEventRef up = CGEventCreateKeyboardEvent(source, keyCode, false);
	if (down == NULL || up == NULL) {
		if (down != NULL) CFRelease(down);
		if (up != NULL) CFRelease(up);
		if (source != NULL) CFRelease(source);
		return -1;
	}

	CGEventFlags flags = withShift ? kCGEventFlagMaskShift : 0;
	if (withShift) {
		CGEventRef shiftDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)0x38, true);
		if (shiftDown != NULL) {
			CGEventSetFlags(shiftDown, kCGEventFlagMaskShift);
			CGEventPost(kCGHIDEventTap, shiftDown);
			CFRelease(shiftDown);
		}
	}

	CGEventSetFlags(down, flags);
	CGEventSetFlags(up, flags);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);

	if (withShift) {
		CGEventRef shiftUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)0x38, false);
		if (shiftUp != NULL) {
			CGEventSetFlags(shiftUp, 0);
			CGEventPost(kCGHIDEventTap, shiftUp);
			CFRelease(shiftUp);
		}
	}

	CFRelease(down);
	CFRelease(up);
	if (source != NULL) CFRelease(source);
	return 0;
}

static int kp_is_esc_down(void) {
	return CGEventSourceKeyState(kCGEventSourceStateHIDSystemState, (CGKeyCode)0x35) ? 1 : 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unicode/utf16"
	"unsafe"
)

const (
	macVKReturn = 0x24
	macVKTab    = 0x30
)

type darwinDriver struct{}

// NewPlatformDriver creates the macOS CoreGraphics CGEvent keyboard driver.
func NewPlatformDriver() Driver {
	return &darwinDriver{}
}

func (d *darwinDriver) CheckPermission(prompt bool) error {
	p := C.int(0)
	if prompt {
		p = 1
	}
	if C.kp_check_accessibility(p) == 0 {
		return errors.New("未授予 macOS 辅助功能权限：请在「系统设置 -> 隐私与安全性 -> 辅助功能」中允许 KeyboardPaste 控制电脑")
	}
	return nil
}

func (d *darwinDriver) Prepare(_ InputMode) error {
	return nil
}

func (d *darwinDriver) Close() error {
	return nil
}

func (d *darwinDriver) TypeRune(r rune, mode InputMode) error {
	if mode == ModeHardware {
		if code, shift, ok := MacANSIKey(r); ok {
			s := C.int(0)
			if shift {
				s = 1
			}
			if C.kp_post_virtual_key(C.CGKeyCode(code), s) != 0 {
				return fmt.Errorf("macOS CGEvent 发送按键 %q 失败", r)
			}
			return nil
		}
	}

	units := utf16.Encode([]rune{r})
	if len(units) == 0 {
		return nil
	}
	res := C.kp_post_unicode((*C.UniChar)(unsafe.Pointer(&units[0])), C.int(len(units)))
	if res != 0 {
		return fmt.Errorf("macOS CGEvent 发送字符 %q 失败", r)
	}
	return nil
}

func (d *darwinDriver) PressEnter(shift bool) error {
	s := C.int(0)
	if shift {
		s = 1
	}
	if C.kp_post_virtual_key(C.CGKeyCode(macVKReturn), s) != 0 {
		return errors.New("macOS CGEvent 发送回车键失败")
	}
	return nil
}

func (d *darwinDriver) PressTab() error {
	if C.kp_post_virtual_key(C.CGKeyCode(macVKTab), 0) != 0 {
		return errors.New("macOS CGEvent 发送 Tab 键失败")
	}
	return nil
}

func (d *darwinDriver) IsEscapePressed() bool {
	return C.kp_is_esc_down() != 0
}
