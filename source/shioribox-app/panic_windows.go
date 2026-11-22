//go:build windows

package main

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	u32                                                           = windows.NewLazySystemDLL("user32.dll")
	s32                                                           = windows.NewLazySystemDLL("shell32.dll")
	k32                                                           = windows.NewLazySystemDLL("kernel32.dll")
	pRegClass, pCreateWin, pDefProc, pGetMsg, pTransMsg, pDispMsg = u32.NewProc("RegisterClassExW"), u32.NewProc("CreateWindowExW"), u32.NewProc("DefWindowProcW"), u32.NewProc("GetMessageW"), u32.NewProc("TranslateMessage"), u32.NewProc("DispatchMessageW")
	pPostMsg, pRegKey, pUnregKey, pShowWin, pSetFg                = u32.NewProc("PostMessageW"), u32.NewProc("RegisterHotKey"), u32.NewProc("UnregisterHotKey"), u32.NewProc("ShowWindow"), u32.NewProc("SetForegroundWindow")
	pLoadIcon, pMenu, pAppend, pTrack, pDestroyMenu, pCursor      = u32.NewProc("LoadIconW"), u32.NewProc("CreatePopupMenu"), u32.NewProc("AppendMenuW"), u32.NewProc("TrackPopupMenu"), u32.NewProc("DestroyMenu"), u32.NewProc("GetCursorPos")
	pSetHook, pUnhook, pCallNext                                  = u32.NewProc("SetWindowsHookExW"), u32.NewProc("UnhookWindowsHookEx"), u32.NewProc("CallNextHookEx")
	pNotify                                                       = s32.NewProc("Shell_NotifyIconW")
	pModule                                                       = k32.NewProc("GetModuleHandleW")
)

const (
	wmHotkey  = 0x0312
	wmApp     = 0x8000
	wmTray    = wmApp + 1
	wmSetKey  = wmApp + 2
	wmSetTaps = wmApp + 3
	wmLUp     = 0x0202
	wmRUp     = 0x0205
	swHide    = 0
	swRestore = 9
	nimAdd    = 0
	nimDelete = 2
	nifMsg    = 1
	nifIcon   = 2
	nifTip    = 4
	idHotkey  = 1
	modNoRep  = 0x4000
)

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uTimeout         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guid             [16]byte
	hBalloonIcon     uintptr
}

type winPanic struct {
	w      webview2.WebView
	main   uintptr // the app window
	msgWin uintptr // hidden window that receives the hotkey and tray messages
	icon   uintptr

	mu       sync.Mutex
	hidden   bool
	mods, vk uint32
	res      chan error

	// quick taps (all but the request fields are used on the hotkey thread only)
	tapVK    uint32
	tapCount int
	tapRes   chan error
	hook     uintptr
	hookCB   uintptr
	det      tapDetector
	inst     uintptr
}

// startPanic creates the hotkey/tray thread for the app window and registers the saved hotkey.
func startPanic(app *App, w webview2.WebView) {
	p := &winPanic{w: w, main: uintptr(w.Window()), res: make(chan error, 1), tapRes: make(chan error, 1)}
	ready := make(chan bool, 1)
	go p.loop(ready)
	if !<-ready {
		logf("privacy hotkey: could not create the message window")
		return
	}
	panicMu.Lock()
	panicUI = p
	panicMu.Unlock()
	app.applyPanic()
}

func (p *winPanic) loop(ready chan bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	inst, _, _ := pModule.Call(0)
	p.inst = inst
	p.hookCB = syscall.NewCallback(p.hookProc)
	cls, _ := syscall.UTF16PtrFromString("ShioriBoxPrivacy")
	type wndClassEx struct {
		size                         uint32
		style                        uint32
		proc                         uintptr
		clsExtra, wndExtra           int32
		inst, icon, cursor, bg, menu uintptr
		class                        *uint16
		small                        uintptr
	}
	wc := wndClassEx{proc: syscall.NewCallback(p.proc), inst: inst, class: cls}
	wc.size = uint32(unsafe.Sizeof(wc))
	pRegClass.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := pCreateWin.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, 0, 0, inst, 0)
	if hwnd == 0 {
		ready <- false
		return
	}
	p.msgWin = hwnd
	p.icon, _, _ = pLoadIcon.Call(inst, 1) // the exe's icon resource
	ready <- true
	var msg struct {
		hwnd    uintptr
		message uint32
		wParam  uintptr
		lParam  uintptr
		time    uint32
		pt      struct{ x, y int32 }
	}
	for {
		r, _, _ := pGetMsg.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTransMsg.Call(uintptr(unsafe.Pointer(&msg)))
		pDispMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (p *winPanic) proc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmHotkey:
		go p.Trigger()
		return 0
	case wmSetKey:
		pUnregKey.Call(hwnd, idHotkey)
		p.mu.Lock()
		mods, vk := p.mods, p.vk
		p.mu.Unlock()
		var err error
		if vk != 0 {
			if r, _, _ := pRegKey.Call(hwnd, idHotkey, uintptr(mods|modNoRep), uintptr(vk)); r == 0 {
				err = errors.New("that key combination is already used by Windows or another program; pick another")
			}
		}
		select {
		case <-p.res:
		default:
		}
		p.res <- err
		return 0
	case wmSetTaps:
		if p.hook != 0 {
			pUnhook.Call(p.hook)
			p.hook = 0
		}
		p.mu.Lock()
		vk, n := p.tapVK, p.tapCount
		p.mu.Unlock()
		p.det = tapDetector{vk: vk, need: n, gap: tapGapMs}
		var err error
		if vk != 0 {
			h, _, _ := pSetHook.Call(13, p.hookCB, p.inst, 0) // WH_KEYBOARD_LL
			if h == 0 {
				err = errors.New("could not watch the keyboard for quick taps")
			}
			p.hook = h
		}
		select {
		case <-p.tapRes:
		default:
		}
		p.tapRes <- err
		return 0
	case wmTray:
		switch lParam & 0xFFFF {
		case wmLUp:
			go p.show()
		case wmRUp:
			p.menu(hwnd)
		}
		return 0
	}
	r, _, _ := pDefProc.Call(hwnd, msg, wParam, lParam)
	return r
}

// hookProc sees every key press system-wide (it never swallows one) and feeds the tap detector.
func (p *winPanic) hookProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) >= 0 {
		kb := (*struct {
			vk, scan, flags, time uint32
			extra                 uintptr
		})(*(*unsafe.Pointer)(unsafe.Pointer(&lParam))) // the hook hands over a pointer as a number
		if kb.flags&0x10 == 0 { // ignore keys sent by other programs
			down := wParam == 0x0100 || wParam == 0x0104
			if (down || wParam == 0x0101 || wParam == 0x0105) && p.det.Event(kb.vk, down, kb.time) {
				go p.Trigger()
			}
		}
	}
	r, _, _ := pCallNext.Call(0, nCode, wParam, lParam)
	return r
}

func (p *winPanic) RegisterTaps(vk uint32, count int) error {
	p.mu.Lock()
	p.tapVK, p.tapCount = vk, count
	p.mu.Unlock()
	pPostMsg.Call(p.msgWin, wmSetTaps, 0, 0)
	select {
	case err := <-p.tapRes:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("the hotkey service is not responding")
	}
}

func (p *winPanic) menu(hwnd uintptr) {
	m, _, _ := pMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	show, _ := syscall.UTF16PtrFromString("Show ShioriBox")
	quit, _ := syscall.UTF16PtrFromString("Exit")
	pAppend.Call(m, 0, 1, uintptr(unsafe.Pointer(show)))
	pAppend.Call(m, 0, 2, uintptr(unsafe.Pointer(quit)))
	var pt struct{ x, y int32 }
	pCursor.Call(uintptr(unsafe.Pointer(&pt)))
	pSetFg.Call(hwnd)                                                                    // lets the menu close when you click elsewhere
	cmd, _, _ := pTrack.Call(m, 0x0002|0x0100, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0) // TPM_RIGHTBUTTON | TPM_RETURNCMD
	switch cmd {
	case 1:
		go p.show()
	case 2:
		p.removeTray()
		p.w.Terminate()
	}
}

func (p *winPanic) tray(op uintptr) {
	nd := notifyIconData{hWnd: p.msgWin, uID: 1, uFlags: nifMsg | nifIcon | nifTip, uCallbackMessage: wmTray, hIcon: p.icon}
	nd.cbSize = uint32(unsafe.Sizeof(nd))
	tip, _ := syscall.UTF16FromString("ShioriBox (press your hotkey or click to show)")
	copy(nd.szTip[:], tip)
	pNotify.Call(op, uintptr(unsafe.Pointer(&nd)))
}

func (p *winPanic) removeTray() { p.tray(nimDelete) }

func (p *winPanic) hide() {
	p.mu.Lock()
	if p.hidden {
		p.mu.Unlock()
		return
	}
	p.hidden = true
	p.mu.Unlock()
	// blur first (also pauses any video and its sound), then take the window off screen and the taskbar
	p.w.Dispatch(func() {
		p.w.Eval("window.__panic && window.__panic()")
		pShowWin.Call(p.main, swHide)
	})
	p.tray(nimAdd)
}

func (p *winPanic) show() {
	p.mu.Lock()
	if !p.hidden {
		p.mu.Unlock()
		return
	}
	p.hidden = false
	p.mu.Unlock()
	p.removeTray()
	p.w.Dispatch(func() {
		pShowWin.Call(p.main, swRestore)
		pSetFg.Call(p.main)
	})
}

func (p *winPanic) Trigger() {
	p.mu.Lock()
	h := p.hidden
	p.mu.Unlock()
	if h {
		p.show()
	} else {
		p.hide()
	}
}

func (p *winPanic) Register(mods, vk uint32) error {
	p.mu.Lock()
	p.mods, p.vk = mods, vk
	p.mu.Unlock()
	pPostMsg.Call(p.msgWin, wmSetKey, 0, 0)
	select {
	case err := <-p.res:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("the hotkey service is not responding")
	}
}
