//go:build linux

package typer

/*
#cgo LDFLAGS: -lX11 -lXtst
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <X11/extensions/XTest.h>

typedef struct {
	Display *dpy;
	int scratch_kc;
	int orig_syms_per_kc;
	KeySym *orig_syms;
} kp_x11_ctx;

static kp_x11_ctx* kp_x11_open(void) {
	Display *dpy = XOpenDisplay(NULL);
	if (dpy == NULL) {
		return NULL;
	}

	int ev_base = 0, err_base = 0, major = 0, minor = 0;
	if (!XTestQueryExtension(dpy, &ev_base, &err_base, &major, &minor)) {
		XCloseDisplay(dpy);
		return NULL;
	}

	int min_kc = 8, max_kc = 255;
	XDisplayKeycodes(dpy, &min_kc, &max_kc);

	int count = max_kc - min_kc + 1;
	int syms_per_kc = 0;
	KeySym *map = XGetKeyboardMapping(dpy, min_kc, count, &syms_per_kc);

	int chosen_kc = max_kc;
	if (map != NULL && syms_per_kc > 0) {
		for (int kc = max_kc; kc >= min_kc; kc--) {
			int empty = 1;
			KeySym *row = &map[(kc - min_kc) * syms_per_kc];
			for (int col = 0; col < syms_per_kc; col++) {
				if (row[col] != NoSymbol) {
					empty = 0;
					break;
				}
			}
			if (empty) {
				chosen_kc = kc;
				break;
			}
		}
	}

	kp_x11_ctx *ctx = (kp_x11_ctx *)calloc(1, sizeof(kp_x11_ctx));
	ctx->dpy = dpy;
	ctx->scratch_kc = chosen_kc;

	if (map != NULL && syms_per_kc > 0) {
		ctx->orig_syms_per_kc = syms_per_kc;
		ctx->orig_syms = (KeySym *)calloc(syms_per_kc, sizeof(KeySym));
		if (ctx->orig_syms != NULL) {
			memcpy(ctx->orig_syms, &map[(chosen_kc - min_kc) * syms_per_kc], syms_per_kc * sizeof(KeySym));
		}
		XFree(map);
	}

	return ctx;
}

static void kp_x11_close(kp_x11_ctx *ctx) {
	if (ctx == NULL) {
		return;
	}
	if (ctx->dpy != NULL) {
		if (ctx->orig_syms != NULL && ctx->orig_syms_per_kc > 0) {
			XChangeKeyboardMapping(ctx->dpy, ctx->scratch_kc, ctx->orig_syms_per_kc, ctx->orig_syms, 1);
			XSync(ctx->dpy, False);
		}
		XCloseDisplay(ctx->dpy);
	}
	if (ctx->orig_syms != NULL) {
		free(ctx->orig_syms);
	}
	free(ctx);
}

static int kp_x11_tap_keysym(kp_x11_ctx *ctx, KeySym sym, int with_shift, int prefer_existing) {
	if (ctx == NULL || ctx->dpy == NULL) {
		return -1;
	}
	Display *dpy = ctx->dpy;
	KeyCode shift_kc = XKeysymToKeycode(dpy, XK_Shift_L);

	if (prefer_existing) {
		KeyCode kc = XKeysymToKeycode(dpy, sym);
		if (kc != 0 && (int)kc != ctx->scratch_kc) {
			int syms_per_kc = 0;
			KeySym *syms = XGetKeyboardMapping(dpy, kc, 1, &syms_per_kc);
			int matched_level = -1;
			if (syms != NULL) {
				if (syms_per_kc >= 1 && syms[0] == sym) {
					matched_level = 0;
				} else if (syms_per_kc >= 2 && syms[1] == sym) {
					matched_level = 1;
				}
				XFree(syms);
			}
			if (matched_level == 0 || matched_level == 1) {
				int need_shift = (matched_level == 1) || with_shift;
				if (need_shift && shift_kc != 0) {
					XTestFakeKeyEvent(dpy, shift_kc, True, CurrentTime);
				}
				XTestFakeKeyEvent(dpy, kc, True, CurrentTime);
				XTestFakeKeyEvent(dpy, kc, False, CurrentTime);
				if (need_shift && shift_kc != 0) {
					XTestFakeKeyEvent(dpy, shift_kc, False, CurrentTime);
				}
				XSync(dpy, False);
				return 0;
			}
		}
	}

	// Dynamic mapping on scratch keycode (supports arbitrary Unicode / CJK characters).
	KeySym syms[2] = { sym, sym };
	XChangeKeyboardMapping(dpy, ctx->scratch_kc, 2, syms, 1);
	XSync(dpy, False);
	usleep(2500);

	if (with_shift && shift_kc != 0) {
		XTestFakeKeyEvent(dpy, shift_kc, True, CurrentTime);
	}
	XTestFakeKeyEvent(dpy, (KeyCode)ctx->scratch_kc, True, CurrentTime);
	XTestFakeKeyEvent(dpy, (KeyCode)ctx->scratch_kc, False, CurrentTime);
	if (with_shift && shift_kc != 0) {
		XTestFakeKeyEvent(dpy, shift_kc, False, CurrentTime);
	}
	XSync(dpy, False);
	return 0;
}

static int kp_x11_is_esc_down(kp_x11_ctx *ctx) {
	if (ctx == NULL || ctx->dpy == NULL) {
		return 0;
	}
	KeyCode esc = XKeysymToKeycode(ctx->dpy, XK_Escape);
	if (esc == 0) {
		return 0;
	}
	char keys[32];
	memset(keys, 0, sizeof(keys));
	XQueryKeymap(ctx->dpy, keys);
	return (keys[esc >> 3] & (1 << (esc & 7))) ? 1 : 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

type linuxDriver struct {
	mu       sync.Mutex
	x11      *C.kp_x11_ctx
	wtypeBin string
}

// NewPlatformDriver creates the Linux keyboard simulation driver (X11/XTest + Wayland wtype fallback).
func NewPlatformDriver() Driver {
	return &linuxDriver{}
}

func (d *linuxDriver) CheckPermission(_ bool) error {
	return nil
}

func (d *linuxDriver) Prepare(_ InputMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.x11 != nil {
		C.kp_x11_close(d.x11)
		d.x11 = nil
	}

	// Prefer native X11/XTest when DISPLAY is available (works for X11 and XWayland apps).
	if os.Getenv("DISPLAY") != "" {
		ctx := C.kp_x11_open()
		if ctx != nil {
			d.x11 = ctx
			return nil
		}
	}

	// Fallback to wtype under pure Wayland sessions.
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if path, err := exec.LookPath("wtype"); err == nil {
			d.wtypeBin = path
			return nil
		}
	}

	// Final attempt to open default X11 display even if DISPLAY env was non-standard.
	if ctx := C.kp_x11_open(); ctx != nil {
		d.x11 = ctx
		return nil
	}

	return errors.New("无法连接 Linux 图形输入服务：请确保在 X11/XWayland 环境下运行，或在纯 Wayland 下安装 wtype")
}

func (d *linuxDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.x11 != nil {
		C.kp_x11_close(d.x11)
		d.x11 = nil
	}
	d.wtypeBin = ""
	return nil
}

func (d *linuxDriver) TypeRune(r rune, mode InputMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.x11 != nil {
		sym := runeToX11KeySym(r)
		preferExisting := C.int(1)
		if mode == ModeUnicode && r > 0x7E {
			preferExisting = 0
		}
		if C.kp_x11_tap_keysym(d.x11, sym, 0, preferExisting) != 0 {
			return fmt.Errorf("X11 XTest 发送字符 %q 失败", r)
		}
		return nil
	}

	if d.wtypeBin != "" {
		cmd := exec.Command(d.wtypeBin, "--", string(r))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("wtype 发送字符 %q 失败: %w", r, err)
		}
		return nil
	}

	return errors.New("Linux 键盘驱动尚未初始化")
}

func (d *linuxDriver) PressEnter(shift bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.x11 != nil {
		s := C.int(0)
		if shift {
			s = 1
		}
		if C.kp_x11_tap_keysym(d.x11, C.XK_Return, s, 1) != 0 {
			return errors.New("X11 XTest 发送回车键失败")
		}
		return nil
	}

	if d.wtypeBin != "" {
		args := []string{"-k", "Return"}
		if shift {
			args = []string{"-M", "shift", "-k", "Return", "-m", "shift"}
		}
		if err := exec.Command(d.wtypeBin, args...).Run(); err != nil {
			return fmt.Errorf("wtype 发送回车键失败: %w", err)
		}
		return nil
	}

	return errors.New("Linux 键盘驱动尚未初始化")
}

func (d *linuxDriver) PressTab() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.x11 != nil {
		if C.kp_x11_tap_keysym(d.x11, C.XK_Tab, 0, 1) != 0 {
			return errors.New("X11 XTest 发送 Tab 键失败")
		}
		return nil
	}

	if d.wtypeBin != "" {
		if err := exec.Command(d.wtypeBin, "-k", "Tab").Run(); err != nil {
			return fmt.Errorf("wtype 发送 Tab 键失败: %w", err)
		}
		return nil
	}

	return errors.New("Linux 键盘驱动尚未初始化")
}

func (d *linuxDriver) IsEscapePressed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.x11 != nil {
		return C.kp_x11_is_esc_down(d.x11) != 0
	}
	return false
}

func runeToX11KeySym(r rune) C.KeySym {
	// Latin-1 printable characters map 1:1 in X11 keysymdef.h.
	if (r >= 0x20 && r <= 0x7E) || (r >= 0xA0 && r <= 0xFF) {
		return C.KeySym(r)
	}
	// Xorg standard Unicode KeySym encoding for U+0100 .. U+10FFFF.
	if r >= 0x0100 {
		return C.KeySym(0x01000000 | uint32(r))
	}
	return C.KeySym(r)
}
