//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"easypackage/internal/archive"
)

// Legacy Win32 controls remain available for Windows architectures without an
// amd64 WebView2 loader. Windows amd64 uses the embedded WebView2 front end.
const (
	wmDestroy            = 0x0002
	wmCreate             = 0x0001
	wmSize               = 0x0005
	wmPaint              = 0x000F
	wmClose              = 0x0010
	wmCommand            = 0x0111
	wmVScroll            = 0x0115
	wmTimer              = 0x0113
	wmEraseBackground    = 0x0014
	wmCtlColorStatic     = 0x0138
	wmSetFont            = 0x0030
	wmGetText            = 0x000D
	wmSetText            = 0x000C
	wmGetTextLength      = 0x000E
	wmGetSel             = 0x00B0
	wmSetSel             = 0x00B1
	wmSetPos             = 0x0402
	wmSetRange           = 0x0406
	wmSetMarquee         = 0x040A
	wmUser               = 0x0400
	wmPreviewClose       = wmUser + 17
	wsOverlappedWindow   = 0x00CF0000
	wsChild              = 0x40000000
	wsVisible            = 0x10000000
	wsBorder             = 0x00800000
	wsVScroll            = 0x00200000
	wsTabStop            = 0x00010000
	wsHScroll            = 0x00100000
	wsExClientEdge       = 0x00000200
	wsExDialogModalFrame = 0x00000001
	wsPopup              = 0x80000000
	wsSysMenu            = 0x00080000
	wsThickFrame         = 0x00040000
	wsMaximizeBox        = 0x00010000
	wsMinimizeBox        = 0x00020000
	wsExAppWindow        = 0x00040000

	bsAutoCheckBox  = 0x00000003
	bsPushButton    = 0x00000000
	esAutoHScroll   = 0x0080
	esPassword      = 0x0020
	esMultiLine     = 0x0004
	esReadOnly      = 0x0800
	esWantReturn    = 0x1000
	ssLeft          = 0x00000000
	cbsDropDownList = 0x0003
	wsScroll        = 0x00200000

	bnClicked      = 0
	cbGetCurSel    = 0x0147
	cbGetLBText    = 0x0148
	cbAddString    = 0x0143
	cbSetCurSel    = 0x014E
	cbResetContent = 0x014B
	bmGetCheck     = 0x00F0
	bmSetCheck     = 0x00F1
	bmGetState     = 0x00F2

	mbOK           = 0
	mbYesNo        = 0x00000004
	mbIconError    = 0x00000010
	mbIconWarning  = 0x00000030
	mbIconQuestion = 0x00000020
	idYes          = 6

	ofPathMustExist      = 0x00000800
	ofFileMustExist      = 0x00001000
	ofOverwritePrompt    = 0x00000002
	ofHideReadOnly       = 0x00000004
	ofNoChangeDir        = 0x00000008
	ofNoDereferenceLinks = 0x00100000

	shReturnOnlyFSDirs = 0x00000001
	shNewDialogStyle   = 0x00000040

	colorWindow     = 5
	sbVert          = 1
	sbLineUp        = 0
	sbLineDown      = 1
	sbPageUp        = 2
	sbPageDown      = 3
	sbThumbPosition = 4
	sbThumbTrack    = 5
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	idHeader              = 10
	idQuit                = 11
	idPackNav             = 12
	idRestoreNav          = 13
	idSource              = 100
	idSourceBrowse        = 101
	idSourceRecent        = 102
	idClass               = 103
	idLarge               = 104
	idLimit               = 105
	idUnit                = 106
	idOutput              = 107
	idOutputBrowse        = 108
	idCoverEnabled        = 109
	idCover               = 110
	idCoverBrowse         = 111
	idPasswordEnabled     = 112
	idPassword            = 113
	idPasswordShow        = 114
	idPasswordGenerate    = 115
	idPreview             = 116
	idStart               = 117
	idRestoreFile         = 200
	idRestoreBrowse       = 201
	idRestoreFolder       = 202
	idRestoreFolderBrowse = 203
	idRestoreName         = 204
	idRestorePassword     = 205
	idRestorePasswordShow = 206
	idRestoreStart        = 207
	idCancel              = 208
	idShowOutput          = 209
	idRestoreRecent       = 210
	idStatus              = 300
	idProgress            = 301
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type winPoint struct{ X, Y int32 }
type winMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   winPoint
	Private uint32
}
type winPaint struct {
	DC       uintptr
	Erase    int32
	Paint    winRect
	Restore  int32
	Update   int32
	Reserved [32]byte
}
type winScrollInfo struct {
	Size, Mask uint32
	Min, Max   int32
	Page       uint32
	Pos, Track int32
}
type winWindowClass struct {
	Size                               uint32
	Style                              uint32
	WndProc                            uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, ClassName                    *uint16
	IconSmall                          uintptr
}
type winOpenFile struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File, FileTitle              *uint16
	MaxFile, MaxFileTitle        uint32
	InitialDirectory, Title      *uint16
	Flags                        uint32
	FileOffset, FileExtension    uint16
	DefaultExtension             *uint16
	CustomData                   uintptr
	Hook, TemplateName           uintptr
	Reserved                     uintptr
	Reserved2                    uint32
	FlagsEx                      uint32
}
type winBrowseInfo struct {
	Owner, Root     uintptr
	DisplayName     *uint16
	Title           *uint16
	Flags           uint32
	Callback, Param uintptr
	Image           int32
}
type nativeControl struct {
	id      int
	hwnd    uintptr
	value   string
	label   string
	checked bool
}
type nativePlacement struct {
	id         int
	label      string
	x, y, w, h int32
	kind       string
	style      uintptr
}

var (
	user32         = syscall.NewLazyDLL("user32.dll")
	gdi32          = syscall.NewLazyDLL("gdi32.dll")
	shell32        = syscall.NewLazyDLL("shell32.dll")
	comdlg32       = syscall.NewLazyDLL("comdlg32.dll")
	controlsKernel = syscall.NewLazyDLL("kernel32.dll")
	comctl32       = syscall.NewLazyDLL("comctl32.dll")
	activeNative   *windowsNativeUI
)

type windowsNativeUI struct {
	hwnd, instance, font, smallFont uintptr
	controls                        map[int]nativeControl
	placements                      []nativePlacement
	page                            string
	app                             *app
	children                        []uintptr
	busy                            bool
	closePending                    bool
	scroll                          int32
	contentHeight                   int32
	clientW, clientH                int32
	lastOutput                      string
	lastOutputIsDir                 bool
	mu                              sync.Mutex
	ready                           bool
}

func nativeProc(dll *syscall.LazyDLL, name string) *syscall.LazyProc { return dll.NewProc(name) }
func nativeText(s string) *uint16                                    { p, _ := syscall.UTF16PtrFromString(s); return p }
func nativeAppIcon(instance uintptr) uintptr {
	icon, _, _ := nativeProc(user32, "LoadImageW").Call(instance, 1, 1, 0, 0, 0x40)
	return icon
}
func nativeSend(h uintptr, msg uint32, w, l uintptr) uintptr {
	r, _, _ := nativeProc(user32, "SendMessageW").Call(h, uintptr(msg), w, l)
	return r
}
func nativeCreateWindow(class, title string, style uint32, ex uintptr, x, y, w, h int32, parent, instance uintptr, id int32) uintptr {
	r, _, _ := nativeProc(user32, "CreateWindowExW").Call(ex, uintptr(unsafe.Pointer(nativeText(class))), uintptr(unsafe.Pointer(nativeText(title))), uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, uintptr(id), instance, 0)
	return r
}
func nativeSetText(hwnd uintptr, text string) {
	nativeProc(user32, "SetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(nativeText(text))))
}
func nativeWindowText(hwnd uintptr) string {
	n := nativeSend(hwnd, wmGetTextLength, 0, 0)
	if n == 0 {
		return ""
	}
	buffer := make([]uint16, int(n)+1)
	nativeSend(hwnd, wmGetText, n+1, uintptr(unsafe.Pointer(&buffer[0])))
	return syscall.UTF16ToString(buffer)
}
func nativeMessage(hwnd uintptr, title, message string, flags uint32) int32 {
	r, _, _ := nativeProc(user32, "MessageBoxW").Call(hwnd, uintptr(unsafe.Pointer(nativeText(message))), uintptr(unsafe.Pointer(nativeText(title))), uintptr(flags))
	return int32(r)
}
func nativeShow(hwnd uintptr, show int32) { nativeProc(user32, "ShowWindow").Call(hwnd, uintptr(show)) }
func nativeMove(hwnd uintptr, x, y, w, h int32) {
	nativeProc(user32, "SetWindowPos").Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
}

func runLegacyWindowsUI() {
	_, _, _ = nativeProc(user32, "SetProcessDPIAware").Call()
	icc := struct{ Size, Classes uint32 }{Size: 8, Classes: 0x00000020}
	if r, _, _ := nativeProc(comctl32, "InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&icc))); r == 0 {
		nativeMessage(0, "EasyPackage", "无法初始化 Windows 通用控件。", mbOK|mbIconError)
		return
	}
	instance, _, _ := nativeProc(controlsKernel, "GetModuleHandleW").Call(0)
	className := nativeText("EasyPackageNativeWindowV24")
	proc := syscall.NewCallback(nativeWindowProc)
	cursor, _, _ := nativeProc(user32, "LoadCursorW").Call(0, 32512)
	icon := nativeAppIcon(instance)
	wc := winWindowClass{Size: uint32(unsafe.Sizeof(winWindowClass{})), Style: 0x0002 | 0x0001, WndProc: proc, Instance: instance, Icon: icon, Cursor: cursor, Background: 0, ClassName: className, IconSmall: icon}
	if r, _, e := nativeProc(user32, "RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		nativeMessage(0, "EasyPackage", "无法创建桌面窗口："+e.Error(), mbOK|mbIconError)
		return
	}
	config, err := os.UserConfigDir()
	if err != nil {
		nativeMessage(0, "EasyPackage", err.Error(), mbOK|mbIconError)
		return
	}
	model := &app{state: job{Status: "idle"}, places: newPlacesStore(filepath.Join(config, "EasyPackage", "preferences.json")), stop: make(chan struct{})}
	ui := &windowsNativeUI{instance: instance, controls: map[int]nativeControl{}, page: "pack", app: model}
	activeNative = ui
	ui.font, _, _ = nativeProc(gdi32, "CreateFontW").Call(16, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(nativeText("Segoe UI"))))
	ui.smallFont, _, _ = nativeProc(gdi32, "CreateFontW").Call(12, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(nativeText("Segoe UI"))))
	screenW, _, _ := nativeProc(user32, "GetSystemMetrics").Call(0)
	screenH, _, _ := nativeProc(user32, "GetSystemMetrics").Call(1)
	width, height := int32(1200), int32(930)
	if screenW > 80 && uintptr(width) > screenW-80 {
		width = int32(screenW - 80)
	}
	if screenH > 80 && uintptr(height) > screenH-80 {
		height = int32(screenH - 80)
	}
	x, y := int32(40), int32(35)
	if screenW > uintptr(width) {
		x = int32((screenW - uintptr(width)) / 2)
	}
	if screenH > uintptr(height) {
		y = int32((screenH - uintptr(height)) / 2)
	}
	ui.hwnd = nativeCreateWindow("EasyPackageNativeWindowV24", "EasyPackage · 轻松打包", wsOverlappedWindow|wsVScroll, wsExAppWindow, x, y, width, height, 0, instance, 0)
	if ui.hwnd == 0 {
		nativeMessage(0, "EasyPackage", "无法创建应用程序窗口。", mbOK|mbIconError)
		return
	}
	ui.ready = true
	ui.layout(ui.clientW, ui.clientH)
	nativeShow(ui.hwnd, 1)
	nativeProc(user32, "UpdateWindow").Call(ui.hwnd)
	nativeProc(user32, "SetTimer").Call(ui.hwnd, 1, 180, 0)
	var msg winMessage
	for {
		r, _, _ := nativeProc(user32, "GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == 0 {
			break
		}
		if int32(r) == -1 {
			nativeMessage(ui.hwnd, "EasyPackage", "Windows 消息循环发生错误。", mbOK|mbIconError)
			break
		}
		nativeProc(user32, "TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		nativeProc(user32, "DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	if ui.font != 0 {
		nativeProc(gdi32, "DeleteObject").Call(ui.font)
	}
	if ui.smallFont != 0 {
		nativeProc(gdi32, "DeleteObject").Call(ui.smallFont)
	}
	activeNative = nil
}

// runNativeDesktopUI is used only by Darwin and Linux builds.
func runNativeDesktopUI() {}

func nativeWindowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	u := activeNative
	switch msg {
	case wmCreate:
		if u != nil {
			u.hwnd = hwnd
			u.clientW = 1168
			u.clientH = 876
			return 0
		}
	case wmSize:
		if u != nil {
			u.clientW, u.clientH = int32(uint16(lparam)), int32(uint16(lparam>>16))
			if u.ready {
				u.savePage()
				u.layout(u.clientW, u.clientH)
			}
			return 0
		}
	case wmEraseBackground:
		if u != nil {
			dc := wparam
			r := winRect{0, 0, u.clientW, u.clientH}
			brush, _, _ := nativeProc(gdi32, "CreateSolidBrush").Call(0x00F8F7F6)
			nativeProc(user32, "FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), brush)
			nativeProc(gdi32, "DeleteObject").Call(brush)
			return 1
		}
	case wmPaint:
		if u != nil {
			u.paint()
			return 0
		}
	case wmCtlColorStatic:
		if u != nil {
			nativeProc(gdi32, "SetBkMode").Call(wparam, 1)
			nativeProc(gdi32, "SetTextColor").Call(wparam, 0x00717A91)
			brush, _, _ := nativeProc(gdi32, "GetStockObject").Call(5)
			return brush
		}
	case wmCommand:
		if u != nil && uint16(lparam) != 0 && uint16(wparam>>16) == bnClicked {
			u.command(int(uint16(wparam)))
			return 0
		}
		if u != nil && uint16(lparam) != 0 && uint16(wparam>>16) == 1 { // CBN_SELCHANGE
			switch int(uint16(wparam)) {
			case idSourceRecent:
				u.readRecentChoice(idSourceRecent, idSource)
				return 0
			case idRestoreRecent:
				u.readRecentChoice(idRestoreRecent, idRestoreFolder)
				return 0
			}
		}
	case wmVScroll:
		if u != nil {
			u.scrollBy(uint16(wparam), int16(wparam>>16))
			return 0
		}
	case wmTimer:
		if u != nil {
			u.refreshStatus()
			return 0
		}
	case wmClose:
		if u != nil && u.busy {
			if nativeMessage(hwnd, "正在处理", "当前任务尚未完成。退出会取消任务，是否继续？", mbYesNo|mbIconQuestion) != idYes {
				return 0
			}
			u.closePending = true
			u.app.mu.Lock()
			if u.app.cancel != nil {
				u.app.cancel()
			}
			u.app.mu.Unlock()
			nativeSend(hwnd, wmSetText, 0, uintptr(unsafe.Pointer(nativeText("EasyPackage · 正在安全取消任务…"))))
			return 0
		}
		nativeProc(user32, "DestroyWindow").Call(hwnd)
		return 0
	case wmDestroy:
		nativeProc(user32, "KillTimer").Call(hwnd, 1)
		nativeProc(user32, "PostQuitMessage").Call(0)
		return 0
	}
	ret, _, _ := nativeProc(user32, "DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

func (u *windowsNativeUI) paint() {
	var ps winPaint
	dc, _, _ := nativeProc(user32, "BeginPaint").Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
	if dc == 0 {
		return
	}
	defer nativeProc(user32, "EndPaint").Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
	paintRect := func(rect winRect, color uintptr, round int32) {
		brush, _, _ := nativeProc(gdi32, "CreateSolidBrush").Call(color)
		old, _, _ := nativeProc(gdi32, "SelectObject").Call(dc, brush)
		if round > 0 {
			nativeProc(gdi32, "RoundRect").Call(dc, uintptr(rect.Left), uintptr(rect.Top), uintptr(rect.Right), uintptr(rect.Bottom), uintptr(round), uintptr(round))
		} else {
			nativeProc(user32, "FillRect").Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
		}
		nativeProc(gdi32, "SelectObject").Call(dc, old)
		if brush != 0 {
			nativeProc(gdi32, "DeleteObject").Call(brush)
		}
	}
	text := func(s string, x, y int32, color uintptr, size uintptr) {
		nativeProc(gdi32, "SetBkMode").Call(dc, 1)
		nativeProc(gdi32, "SetTextColor").Call(dc, color)
		font := u.font
		if size == 2 {
			font = u.smallFont
		}
		old, _, _ := nativeProc(gdi32, "SelectObject").Call(dc, font)
		r := winRect{x, y, u.clientW - 18, y + 27}
		nativeProc(user32, "DrawTextW").Call(dc, uintptr(unsafe.Pointer(nativeText(s))), uintptr(^uint32(0)), uintptr(unsafe.Pointer(&r)), 0x0000|0x0800)
		nativeProc(gdi32, "SelectObject").Call(dc, old)
	}
	paintRect(winRect{0, 0, u.clientW, 94}, 0x00FFFFFF, 0)
	paintRect(winRect{0, 0, 218, u.clientH}, 0x002C263D, 0)
	text("E / P", 26, 22, 0x00FFFFFF, 1)
	text("EasyPackage", 89, 23, 0x00FFFFFF, 0)
	text("独立运行 · 纯原生窗口", 89, 49, 0x00D1D2DE, 2)
	text("让项目打包更轻松", 250, 20, 0x00272A3E, 0)
	text("本地处理 · 文件不会上传，也不会自动打开浏览器。", 250, 53, 0x00828A9B, 2)
	for _, rect := range u.cardRects() {
		rect.Top -= u.scroll
		rect.Bottom -= u.scroll
		if rect.Bottom < 108 || rect.Top > u.clientH-95 {
			continue
		}
		paintRect(rect, 0x00FFFFFF, 13)
	}
	text("最近的工作", 24, u.clientH-193, 0x00CDCEDB, 2)
	text("本机保存 · 独立运行", 24, u.clientH-44, 0x00CDCEDB, 2)
}
func (u *windowsNativeUI) cardRects() []winRect {
	left := winRect{238, 122, math.MaxInt32, 276}
	filters := winRect{238, 288, math.MaxInt32, 423}
	output := winRect{238, 435, math.MaxInt32, 990}
	side := winRect{math.MaxInt32, 122, math.MaxInt32, 430}
	restore1 := winRect{238, 122, math.MaxInt32, 278}
	restore2 := winRect{238, 290, math.MaxInt32, 483}
	restore3 := winRect{238, 495, math.MaxInt32, 660}
	formW := u.clientW - 554
	if formW < 510 {
		formW = 510
	}
	for r := range []*winRect{&left, &filters, &output, &restore1, &restore2, &restore3} {
		(*[]*winRect{&left, &filters, &output, &restore1, &restore2, &restore3}[r]).Right = 238 + formW
	}
	side.Left = 238 + formW + 18
	side.Right = u.clientW - 22
	if u.page == "restore" {
		return []winRect{restore1, restore2, restore3, side}
	}
	return []winRect{left, filters, output, side}
}

func (u *windowsNativeUI) buildCommonControls() {
	static := func(label string, x, y, w, h int) {
		u.placements = append(u.placements, nativePlacement{label: label, x: int32(x), y: int32(y), w: int32(w), h: int32(h), kind: "label"})
	}
	button := func(id int, label string, x, y, w, h int) {
		u.placements = append(u.placements, nativePlacement{id: id, label: label, x: int32(x), y: int32(y), w: int32(w), h: int32(h), kind: "button"})
	}
	edit := func(id int, value, placeholder string, x, y, w, h int, password bool) {
		kind := "edit"
		style := uintptr(esAutoHScroll)
		if password {
			style |= esPassword
		}
		u.placements = append(u.placements, nativePlacement{id: id, label: value, x: int32(x), y: int32(y), w: int32(w), h: int32(h), kind: kind, style: style})
		_ = placeholder
	}
	check := func(id int, label string, checked bool, x, y, w int) {
		p := nativePlacement{id: id, label: label, x: int32(x), y: int32(y), w: int32(w), h: 23, kind: "check"}
		u.placements = append(u.placements, p)
		if _, ok := u.controls[id]; !ok {
			u.controls[id] = nativeControl{id: id, checked: checked}
		}
	}
	combo := func(id int, label string, x, y, w, h int) {
		u.placements = append(u.placements, nativePlacement{id: id, label: label, x: int32(x), y: int32(y), w: int32(w), h: int32(h), kind: "combo"})
	}
	cardLeft := 250
	formW := int(u.clientW) - 552
	if formW < 510 {
		formW = 510
	}
	right := int(u.clientW) - 24
	button(idQuit, "退出", 24, int(u.clientH)-94, 170, 38)
	button(idPackNav, "◈  项目打包", 18, 118, 182, 42)
	button(idRestoreNav, "↻  JPG / ZIP 还原", 18, 169, 182, 42)
	if u.page == "pack" {
		textLabel := func(s string, x, y, w int) { static(s, x, y, w, 24) }
		textLabel("01   选择要打包的项目", cardLeft, 136, 410)
		textLabel("项目文件夹", cardLeft, 170, 86)
		edit(idSource, "", "", cardLeft+90, 166, formW-215, 30, false)
		button(idSourceBrowse, "浏览…", cardLeft+formW-111, 166, 85, 31)
		textLabel("常用 / 最近目录", cardLeft, 207, 130)
		combo(idSourceRecent, "", cardLeft+132, 202, formW-150, 220)
		textLabel("02   设置过滤规则", cardLeft, 302, 410)
		check(idClass, "自动过滤 .class 编译文件", true, cardLeft, 334, 240)
		check(idLarge, "过滤超过此大小的文件", false, cardLeft+270, 334, 240)
		edit(idLimit, "100", "", cardLeft+202, 365, 78, 29, false)
		combo(idUnit, "MB", cardLeft+286, 365, 77, 220)
		textLabel("默认排除 .git、.idea、.codegraph、.vscode、.vs、.svn、.hg、.fleet 及其内部文件", cardLeft, 397, formW-18)
		textLabel("03   保存与安全", cardLeft, 450, 410)
		textLabel("输出 ZIP / JPG 文件", cardLeft, 484, 120)
		edit(idOutput, "", "", cardLeft+120, 480, formW-224, 30, false)
		button(idOutputBrowse, "另存为…", cardLeft+formW-98, 480, 94, 31)
		check(idCoverEnabled, "附加 JPG 封面", false, cardLeft, 526, 200)
		edit(idCover, "", "", cardLeft+124, 552, formW-235, 29, false)
		button(idCoverBrowse, "选取 JPG…", cardLeft+formW-104, 552, 100, 30)
		check(idPasswordEnabled, "启用 AES-256 压缩密码", false, cardLeft, 597, 260)
		edit(idPassword, "", "", cardLeft+3, 630, formW-230, 31, true)
		button(idPasswordShow, "显示", cardLeft+formW-220, 630, 64, 31)
		button(idPasswordGenerate, "生成密码", cardLeft+formW-148, 630, 123, 31)
		button(idPreview, "预览文件", cardLeft, 686, 130, 39)
		button(idStart, "开始打包", cardLeft+formW-158, 686, 158, 39)
		u.fillLabel(idUnit, []string{"MB", "GB", "KB"}, 0)
		u.fillLabel(idSourceRecent, u.quickFolderPaths(), 0)
	} else {
		static("01   选择要还原的 JPG 或 ZIP", cardLeft, 136, formW, 24)
		static("支持普通 ZIP、AES ZIP 及 EasyPackage JPG。", cardLeft, 162, formW, 22)
		edit(idRestoreFile, "", "", cardLeft, 193, formW-106, 30, false)
		button(idRestoreBrowse, "选择文件…", cardLeft+formW-100, 193, 100, 31)
		static("02   选择解压位置", cardLeft, 304, formW, 24)
		static("解压到一个新的文件夹，不会覆盖已存在内容。", cardLeft, 330, formW, 22)
		edit(idRestoreFolder, "", "", cardLeft, 361, formW-106, 30, false)
		button(idRestoreFolderBrowse, "浏览…", cardLeft+formW-100, 361, 100, 31)
		combo(idRestoreRecent, "", cardLeft, 396, formW, 220)
		static("新文件夹名称", cardLeft, 437, 115, 24)
		edit(idRestoreName, "restored", "", cardLeft+120, 433, formW-126, 30, false)
		static("03   解压密码（未加密文件留空）", cardLeft, 509, formW, 24)
		edit(idRestorePassword, "", "", cardLeft, 544, formW-88, 31, true)
		button(idRestorePasswordShow, "显示", cardLeft+formW-80, 544, 80, 31)
		button(idRestoreStart, "直接解压到新文件夹", cardLeft+formW-222, 604, 222, 40)
		u.fillLabel(idRestoreRecent, u.quickFolderPaths(), 0)
	}
	static("开始后在此显示预览、进度和任务状态。", right-244, 164, 235, 70)
	u.createControls()
	u.createStatusControls()
}
func (u *windowsNativeUI) layout(w, h int32) {
	if w < 800 || h < 580 {
		return
	}
	u.clientW = w
	u.clientH = h
	u.destroyControls()
	u.buildCommonControls()
	pageHeight := int32(760)
	if u.page == "restore" {
		pageHeight = 650
	}
	visible := h - 116
	maxScroll := pageHeight - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if u.scroll > maxScroll {
		u.scroll = maxScroll
	}
	u.contentHeight = pageHeight
	info := winScrollInfo{Size: uint32(unsafe.Sizeof(winScrollInfo{})), Mask: 0x0001 | 0x0002 | 0x0004, Min: 0, Max: pageHeight - 1, Page: uint32(visible), Pos: u.scroll}
	nativeProc(user32, "SetScrollInfo").Call(u.hwnd, sbVert, uintptr(unsafe.Pointer(&info)), 1)
}
func (u *windowsNativeUI) destroyControls() {
	u.savePage()
	for _, hwnd := range u.children {
		if hwnd != 0 {
			nativeProc(user32, "DestroyWindow").Call(hwnd)
		}
	}
	u.children = nil
	for id, c := range u.controls {
		if c.hwnd != 0 {
			c.hwnd = 0
			u.controls[id] = c
		}
	}
	u.placements = nil
}
func (u *windowsNativeUI) createControls() {
	for _, p := range u.placements {
		y := p.y - u.scroll
		var hwnd uintptr
		style := uintptr(wsChild | wsVisible)
		class := "STATIC"
		switch p.kind {
		case "button":
			class = "BUTTON"
			style |= wsTabStop | bsPushButton
		case "check":
			class = "BUTTON"
			style |= wsTabStop | bsAutoCheckBox
		case "edit":
			class = "EDIT"
			style |= wsTabStop | wsBorder | p.style
		case "combo":
			class = "COMBOBOX"
			style |= wsTabStop | cbsDropDownList | wsVScroll
			p.h = 210
		}
		ex := uintptr(0)
		if p.kind == "edit" && (p.id == idSource || p.id == idOutput || p.id == idRestoreFile || p.id == idRestoreFolder || p.id == idRestoreName || p.id == idPassword || p.id == idRestorePassword) {
			ex = wsExClientEdge
		}
		hwnd = nativeCreateWindow(class, p.label, uint32(style), ex, p.x, y, p.w, p.h, u.hwnd, u.instance, int32(p.id))
		if hwnd == 0 {
			continue
		}
		font := u.font
		if p.kind == "label" {
			font = u.smallFont
		}
		nativeSend(hwnd, wmSetFont, font, 1)
		if p.kind == "check" {
			state := uintptr(0)
			if c, ok := u.controls[p.id]; ok && c.checked {
				state = 1
			}
			nativeSend(hwnd, bmSetCheck, state, 0)
		}
		if p.kind == "edit" {
			c := u.controls[p.id]
			value := p.label
			if c.value != "" {
				value = c.value
			} else {
				c.value = value
			}
			nativeSetText(hwnd, value)
		}
		if p.kind == "combo" {
			if c, ok := u.controls[p.id]; ok {
				for i, opt := range strings.Split(c.value, "\x00") {
					nativeSend(hwnd, cbAddString, 0, uintptr(unsafe.Pointer(nativeText(opt))))
					if opt == c.label {
						nativeSend(hwnd, cbSetCurSel, uintptr(i), 0)
					}
				}
			}
		}
		c := u.controls[p.id]
		c.id = p.id
		c.hwnd = hwnd
		u.controls[p.id] = c
		u.children = append(u.children, hwnd)
	}
	u.updateOptionalControls()
}
func (u *windowsNativeUI) createStatusControls() {
	header := nativeCreateWindow("STATIC", "", wsChild|wsVisible, wsExClientEdge, 220, u.clientH-101, u.clientW-238, 1, u.hwnd, u.instance, idHeader)
	u.children = append(u.children, header)
	status := nativeCreateWindow("STATIC", "准备就绪 · 选择项目文件夹开始", wsChild|wsVisible|ssLeft, 0, 241, u.clientH-87, u.clientW-470, 23, u.hwnd, u.instance, idStatus)
	nativeSend(status, wmSetFont, u.smallFont, 1)
	u.controls[idStatus] = nativeControl{id: idStatus, hwnd: status}
	u.children = append(u.children, status)
	progress := nativeCreateWindow("msctls_progress32", "", wsChild|wsVisible|0x00000008, 0, 241, u.clientH-56, u.clientW-472, 8, u.hwnd, u.instance, idProgress)
	nativeSend(progress, wmSetRange, 0, 100)
	nativeSend(progress, wmSetPos, 0, 0)
	u.controls[idProgress] = nativeControl{id: idProgress, hwnd: progress}
	u.children = append(u.children, progress)
	cancel := nativeCreateWindow("BUTTON", "取消", wsChild|wsVisible|wsTabStop|bsPushButton, 0, u.clientW-218, u.clientH-86, 76, 32, u.hwnd, u.instance, idCancel)
	nativeSend(cancel, wmSetFont, u.font, 1)
	u.controls[idCancel] = nativeControl{id: idCancel, hwnd: cancel}
	u.children = append(u.children, cancel)
	nativeShow(cancel, 0)
	open := nativeCreateWindow("BUTTON", "查看位置", wsChild|wsVisible|wsTabStop|bsPushButton, 0, u.clientW-132, u.clientH-86, 108, 32, u.hwnd, u.instance, idShowOutput)
	nativeSend(open, wmSetFont, u.font, 1)
	u.controls[idShowOutput] = nativeControl{id: idShowOutput, hwnd: open}
	u.children = append(u.children, open)
	nativeShow(open, 0)
}

func (u *windowsNativeUI) cardRectsFixed() {}
func (u *windowsNativeUI) setTextControl(id int, value string) {
	c := u.controls[id]
	c.id = id
	c.value = value
	u.controls[id] = c
}
func (u *windowsNativeUI) fillLabel(id int, items []string, selected int) {
	c := u.controls[id]
	c.id = id
	c.value = strings.Join(items, "\x00")
	c.label = ""
	if selected >= 0 && selected < len(items) {
		c.label = items[selected]
	}
	u.controls[id] = c
}
func (u *windowsNativeUI) readRecentChoice(control, target int) {
	source := u.controls[control]
	idx := nativeSend(source.hwnd, cbGetCurSel, 0, 0)
	if int32(idx) < 0 {
		return
	}
	buffer := make([]uint16, 32768)
	nativeSend(source.hwnd, cbGetLBText, idx, uintptr(unsafe.Pointer(&buffer[0])))
	path := syscall.UTF16ToString(buffer)
	if filepath.IsAbs(path) {
		u.setControlText(target, path)
		u.remember(path)
	}
}
func (u *windowsNativeUI) setControlText(id int, text string) {
	if c, ok := u.controls[id]; ok {
		nativeSetText(c.hwnd, text)
	}
}
func (u *windowsNativeUI) getControlText(id int) string {
	if c, ok := u.controls[id]; ok {
		return nativeWindowText(c.hwnd)
	}
	return ""
}
func (u *windowsNativeUI) isChecked(id int) bool {
	if c, ok := u.controls[id]; ok {
		return nativeSend(c.hwnd, bmGetCheck, 0, 0) == 1
	}
	return false
}
func (u *windowsNativeUI) savePage() {
	for id, c := range u.controls {
		if c.hwnd == 0 {
			continue
		}
		if c.id == idClass || c.id == idLarge || c.id == idCoverEnabled || c.id == idPasswordEnabled {
			c.checked = nativeSend(c.hwnd, bmGetCheck, 0, 0) == 1
		} else if c.id != idCancel && c.id != idShowOutput && c.id != idProgress && c.id != idStatus && c.id != idQuit && c.id != idPackNav && c.id != idRestoreNav && c.id != idPreview && c.id != idStart && c.id != idRestoreStart && c.id != idSourceBrowse && c.id != idSourceRecent && c.id != idRestoreRecent && c.id != idOutputBrowse && c.id != idCoverBrowse && c.id != idRestoreBrowse && c.id != idRestoreFolderBrowse && c.id != idPasswordShow && c.id != idPasswordGenerate && c.id != idRestorePasswordShow {
			if c.id != idUnit {
				c.value = nativeWindowText(c.hwnd)
			} else {
				idx := nativeSend(c.hwnd, cbGetCurSel, 0, 0)
				b := make([]uint16, 128)
				nativeSend(c.hwnd, cbGetLBText, idx, uintptr(unsafe.Pointer(&b[0])))
				c.label = syscall.UTF16ToString(b)
			}
		}
		u.controls[id] = c
	}
}

func (u *windowsNativeUI) command(id int) {
	if u.busy && id != idCancel && id != idQuit {
		return
	}
	switch id {
	case idQuit:
		nativeSend(u.hwnd, wmClose, 0, 0)
	case idPackNav:
		if u.page != "pack" {
			u.savePage()
			u.page = "pack"
			u.layout(u.clientW, u.clientH)
			nativeProc(user32, "InvalidateRect").Call(u.hwnd, 0, 1)
		}
	case idRestoreNav:
		if u.page != "restore" {
			u.savePage()
			u.page = "restore"
			u.layout(u.clientW, u.clientH)
			nativeProc(user32, "InvalidateRect").Call(u.hwnd, 0, 1)
		}
	case idSourceBrowse:
		if path := nativeChooseFolder(u.hwnd, u.getControlText(idSource)); path != "" {
			u.setControlText(idSource, path)
			if strings.TrimSpace(u.getControlText(idOutput)) == "" {
				u.setControlText(idOutput, filepath.Join(filepath.Dir(path), filepath.Base(path)+".zip"))
			}
			u.remember(path)
		}
	case idOutputBrowse:
		ext := ".zip"
		filter := "ZIP 压缩包 (*.zip)\x00*.zip\x00"
		if u.isChecked(idCoverEnabled) {
			ext = ".jpg"
			filter = "JPEG 图片 (*.jpg;*.jpeg)\x00*.jpg;*.jpeg\x00"
		}
		filter += "所有文件 (*.*)\x00*.*\x00\x00"
		if path := nativeSaveFile(u.hwnd, u.getControlText(idOutput), "保存 EasyPackage 压缩包", filter, ext); path != "" {
			u.setControlText(idOutput, path)
			u.remember(filepath.Dir(path))
		}
	case idCoverBrowse:
		if path := nativeOpenFile(u.hwnd, u.getControlText(idCover), "选择 JPG 图片", "JPEG 图片 (*.jpg;*.jpeg)\x00*.jpg;*.jpeg\x00所有文件 (*.*)\x00*.*\x00\x00"); path != "" {
			u.setControlText(idCover, path)
			u.remember(filepath.Dir(path))
		}
	case idCoverEnabled, idPasswordEnabled:
		u.updateOptionalControls()
	case idRestoreBrowse:
		if path := nativeOpenFile(u.hwnd, u.getControlText(idRestoreFile), "选择待解压的 JPG 或 ZIP", "EasyPackage / ZIP 文件 (*.jpg;*.jpeg;*.zip)\x00*.jpg;*.jpeg;*.zip\x00所有文件 (*.*)\x00*.*\x00\x00"); path != "" {
			u.setControlText(idRestoreFile, path)
			base := filepath.Base(path)
			u.setControlText(idRestoreName, strings.TrimSuffix(base, filepath.Ext(base))+"-restored")
			u.remember(filepath.Dir(path))
		}
	case idRestoreFolderBrowse:
		if path := nativeChooseFolder(u.hwnd, u.getControlText(idRestoreFolder)); path != "" {
			u.setControlText(idRestoreFolder, path)
			u.remember(path)
		}
	case idPasswordShow:
		u.togglePassword(idPassword, idPasswordShow)
	case idRestorePasswordShow:
		u.togglePassword(idRestorePassword, idRestorePasswordShow)
	case idPasswordGenerate:
		u.generatePassword()
	case idPreview:
		u.runTask("preview")
	case idStart:
		u.runTask("pack")
	case idRestoreStart:
		u.runTask("restore")
	case idCancel:
		u.app.mu.Lock()
		if u.app.cancel != nil {
			u.app.cancel()
		}
		u.app.mu.Unlock()
		u.setStatus("正在安全取消任务…", 0, true)
		c := u.controls[idCancel]
		nativeShow(c.hwnd, 0)
	case idShowOutput:
		u.openOutput()
	}
}
func (u *windowsNativeUI) togglePassword(editID, buttonID int) {
	input := u.controls[editID]
	button := u.controls[buttonID]
	if input.hwnd == 0 || button.hwnd == 0 {
		return
	}
	style, _, _ := nativeProc(user32, "GetWindowLongPtrW").Call(input.hwnd, uintptr(^uint(15)))
	start, end := uintptr(0), uintptr(0)
	nativeSend(input.hwnd, wmGetSel, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)))
	if style&esPassword != 0 {
		style &^= esPassword
		nativeSetText(button.hwnd, "隐藏")
	} else {
		style |= esPassword
		nativeSetText(button.hwnd, "显示")
	}
	nativeProc(user32, "SetWindowLongPtrW").Call(input.hwnd, uintptr(^uint(15)), style)
	nativeProc(user32, "SetWindowPos").Call(input.hwnd, 0, 0, 0, 0, 0, swpNoZOrder|swpNoActivate|swpFrameChanged)
	nativeSend(input.hwnd, wmSetSel, start, end)
}
func (u *windowsNativeUI) generatePassword() {
	alphabet := "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%*-_+"
	b := make([]byte, 32)
	out := make([]byte, 0, 20)
	ceiling := byte(256 - 256%len(alphabet))
	for len(out) < 20 {
		if _, err := rand.Read(b); err != nil {
			nativeMessage(u.hwnd, "随机生成失败", err.Error(), mbOK|mbIconError)
			return
		}
		for _, v := range b {
			if v < ceiling {
				out = append(out, alphabet[int(v)%len(alphabet)])
				if len(out) == 20 {
					break
				}
			}
		}
	}
	u.setControlText(idPassword, string(out))
	u.setStatus("已生成 20 位密码 · 默认隐藏", 0, false)
}
func (u *windowsNativeUI) remember(paths ...string) {
	_ = u.app.places.Remember(paths...)
	u.refreshRecentCombo()
}
func (u *windowsNativeUI) refreshRecentCombo() {
	for _, id := range []int{idSourceRecent, idRestoreRecent} {
		c := u.controls[id]
		if c.hwnd == 0 {
			continue
		}
		nativeSend(c.hwnd, cbResetContent, 0, 0)
		items := u.quickFolderPaths()
		for _, path := range items {
			nativeSend(c.hwnd, cbAddString, 0, uintptr(unsafe.Pointer(nativeText(path))))
		}
		if len(items) > 0 {
			nativeSend(c.hwnd, cbSetCurSel, 0, 0)
		}
	}
}

func (u *windowsNativeUI) quickFolderPaths() []string {
	result := []string{}
	add := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range result {
			if sameFolderPath(existing, path) {
				return
			}
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			result = append(result, path)
		}
	}
	for _, folder := range commonFolders() {
		add(folder.Path)
	}
	if u.app != nil && u.app.places != nil {
		for _, path := range u.app.places.Recent() {
			add(path)
		}
	}
	return result
}

func (u *windowsNativeUI) updateOptionalControls() {
	output := strings.TrimSpace(u.getControlText(idOutput))
	if output != "" {
		desired := ".zip"
		if u.isChecked(idCoverEnabled) {
			desired = ".jpg"
		}
		ext := strings.ToLower(filepath.Ext(output))
		if ext == ".zip" || ext == ".jpg" || ext == ".jpeg" {
			if ext != desired {
				u.setControlText(idOutput, strings.TrimSuffix(output, filepath.Ext(output))+desired)
			}
		}
	}
	for _, id := range []int{idCover, idCoverBrowse} {
		if c := u.controls[id]; c.hwnd != 0 {
			nativeProc(user32, "EnableWindow").Call(c.hwnd, boolUint(u.isChecked(idCoverEnabled)))
		}
	}
	for _, id := range []int{idPassword, idPasswordShow, idPasswordGenerate} {
		if c := u.controls[id]; c.hwnd != 0 {
			nativeProc(user32, "EnableWindow").Call(c.hwnd, boolUint(u.isChecked(idPasswordEnabled)))
		}
	}
}

func (u *windowsNativeUI) runTask(action string) {
	var opts archive.Options
	if action == "restore" {
		source, folder, name := strings.TrimSpace(u.getControlText(idRestoreFile)), strings.TrimSpace(u.getControlText(idRestoreFolder)), strings.TrimSpace(u.getControlText(idRestoreName))
		if source == "" || folder == "" {
			nativeMessage(u.hwnd, "请选择文件和位置", "请选择待还原的 JPG / ZIP 文件和解压位置。", mbOK|mbIconWarning)
			return
		}
		if !safeNativeFolderName(name) {
			nativeMessage(u.hwnd, "文件夹名称无效", "请输入有效的新文件夹名称。", mbOK|mbIconWarning)
			return
		}
		password := u.getControlText(idRestorePassword)
		opts = archive.Options{Source: source, Output: filepath.Join(folder, name), Password: password}
	} else {
		source := strings.TrimSpace(u.getControlText(idSource))
		output := strings.TrimSpace(u.getControlText(idOutput))
		if source == "" || (output == "" && action != "preview") {
			nativeMessage(u.hwnd, "请选择项目和保存位置", "请选择要打包的项目文件夹和输出压缩包。", mbOK|mbIconWarning)
			return
		}
		if output == "" {
			output = filepath.Join(filepath.Dir(source), "."+filepath.Base(source)+"-preview.zip")
		}
		filterClass := u.isChecked(idClass)
		cover := ""
		if action != "preview" && u.isChecked(idCoverEnabled) {
			cover = strings.TrimSpace(u.getControlText(idCover))
			if cover == "" {
				nativeMessage(u.hwnd, "请选择 JPG 封面", "启用了 JPG 封面，请选择一张有效的 JPG 图片。", mbOK|mbIconWarning)
				return
			}
		}
		maxBytes := int64(0)
		if u.isChecked(idLarge) {
			value, err := strconv.ParseFloat(strings.TrimSpace(u.getControlText(idLimit)), 64)
			if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				nativeMessage(u.hwnd, "大小无效", "请输入有效的大文件大小阈值。", mbOK|mbIconWarning)
				return
			}
			unit := strings.ToUpper(strings.TrimSpace(u.comboText(idUnit)))
			scale := float64(1024 * 1024)
			if unit == "GB" {
				scale *= 1024
			} else if unit == "KB" {
				scale = 1024
			}
			bytes := value * scale
			if bytes < 1 || bytes > float64(math.MaxInt64) {
				nativeMessage(u.hwnd, "大小超出范围", "阈值超出支持范围。", mbOK|mbIconWarning)
				return
			}
			maxBytes = int64(math.Round(bytes))
		}
		password := ""
		if action != "preview" && u.isChecked(idPasswordEnabled) {
			password = u.getControlText(idPassword)
			if password == "" {
				nativeMessage(u.hwnd, "请输入密码", "启用密码保护后，请输入压缩密码。", mbOK|mbIconWarning)
				return
			}
		}
		opts = archive.Options{Source: source, Output: output, FilterClass: filterClass, MaxBytes: maxBytes, Cover: cover, Password: password, Overwrite: false}
	}
	if action != "preview" && action != "restore" {
		if _, err := os.Lstat(opts.Output); err == nil {
			if action == "pack" {
				if nativeMessage(u.hwnd, "确认覆盖", "目标文件已存在：\n"+opts.Output+"\n\n要替换这个文件吗？", mbYesNo|mbIconQuestion) != idYes {
					return
				}
				opts.Overwrite = true
			} else {
				nativeMessage(u.hwnd, "目标文件夹已存在", "为保护已有内容，请更换解压后的新文件夹名称。", mbOK|mbIconWarning)
				return
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			nativeMessage(u.hwnd, "输出位置无效", err.Error(), mbOK|mbIconError)
			return
		}
	}
	if action == "restore" {
		if _, err := os.Lstat(opts.Output); err == nil {
			nativeMessage(u.hwnd, "目标文件夹已存在", "请更换新文件夹名称；不会覆盖或合并已有文件夹。", mbOK|mbIconWarning)
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			nativeMessage(u.hwnd, "还原位置无效", err.Error(), mbOK|mbIconError)
			return
		}
	}
	u.app.mu.Lock()
	if u.app.state.Status == "running" {
		u.app.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.app.cancel = cancel
	u.app.state = job{ID: u.app.state.ID + 1, Status: "running", Phase: "正在扫描项目", Preview: action == "preview"}
	u.app.mu.Unlock()
	u.busy = true
	u.setBusyControls(true)
	nativeShow(u.controls[idShowOutput].hwnd, 0)
	u.setStatus(map[string]string{"restore": "正在验证并直接解压…", "pack": "正在准备打包…", "preview": "正在扫描项目文件…"}[action], 0, true)
	go u.app.run(ctx, task{Options: opts, Action: action})
}
func safeNativeFolderName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\:\x00<>\"|?*") && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}
func (u *windowsNativeUI) comboText(id int) string {
	c := u.controls[id]
	if c.hwnd == 0 {
		return c.label
	}
	idx := nativeSend(c.hwnd, cbGetCurSel, 0, 0)
	if int32(idx) < 0 {
		return c.label
	}
	b := make([]uint16, 32)
	nativeSend(c.hwnd, cbGetLBText, idx, uintptr(unsafe.Pointer(&b[0])))
	return syscall.UTF16ToString(b)
}
func (u *windowsNativeUI) setBusyControls(busy bool) {
	ids := []int{idSource, idSourceBrowse, idSourceRecent, idClass, idLarge, idLimit, idUnit, idOutput, idOutputBrowse, idCoverEnabled, idCover, idCoverBrowse, idPasswordEnabled, idPassword, idPasswordShow, idPasswordGenerate, idPreview, idStart, idRestoreFile, idRestoreBrowse, idRestoreFolder, idRestoreFolderBrowse, idRestoreRecent, idRestoreName, idRestorePassword, idRestorePasswordShow, idRestoreStart}
	for _, id := range ids {
		if c, ok := u.controls[id]; ok && c.hwnd != 0 {
			nativeProc(user32, "EnableWindow").Call(c.hwnd, boolUint(!busy))
		}
	}
	if c := u.controls[idCancel]; c.hwnd != 0 {
		nativeProc(user32, "EnableWindow").Call(c.hwnd, boolUint(busy))
		nativeShow(c.hwnd, boolInt(busy))
	}
	nativeProc(user32, "EnableWindow").Call(u.controls[idPackNav].hwnd, boolUint(!busy))
	nativeProc(user32, "EnableWindow").Call(u.controls[idRestoreNav].hwnd, boolUint(!busy))
	u.updateOptionalControls()
}
func boolUint(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}
func boolInt(b bool) int32 {
	if b {
		return 5
	}
	return 0
}
func (u *windowsNativeUI) setStatus(text string, percent int, marquee bool) {
	if c := u.controls[idStatus]; c.hwnd != 0 {
		nativeSetText(c.hwnd, text)
	}
	if c := u.controls[idProgress]; c.hwnd != 0 {
		if marquee {
			nativeSend(c.hwnd, wmSetMarquee, 1, 35)
		} else {
			nativeSend(c.hwnd, wmSetMarquee, 0, 0)
			nativeSend(c.hwnd, wmSetPos, uintptr(percent), 0)
		}
	}
}
func (u *windowsNativeUI) refreshStatus() {
	u.app.mu.Lock()
	state := u.app.state
	u.app.mu.Unlock()
	if u.busy {
		status := state.Phase
		if state.Status == "running" && state.Current != "" {
			status = state.Current
		}
		if state.Status == "running" && state.Plan != nil && state.Plan.Bytes > 0 {
			percent := int(math.Min(100, float64(state.Done)*100/float64(state.Plan.Bytes)))
			u.setStatus(fmt.Sprintf("%s   ·   %d%%", state.Phase, percent), percent, false)
		} else {
			u.setStatus(status, 0, true)
		}
		if state.Status != "running" {
			u.busy = false
			u.setBusyControls(false)
			u.setStatus(state.Phase, 100, false)
			if state.Status == "error" {
				nativeMessage(u.hwnd, "任务未完成", state.Error, mbOK|mbIconError)
			} else if state.Status == "cancelled" {
				nativeMessage(u.hwnd, "任务已取消", "临时文件已清理；已有输出文件保持不变。", mbOK|mbIconWarning)
			} else if state.Status == "done" {
				u.lastOutput = state.Output
				u.lastOutputIsDir = state.OutputDirectory
				summary := "输出文件：\n" + state.Output
				if state.Preview {
					summary = "文件清单预览"
					if state.Plan != nil {
						summary += fmt.Sprintf("\n\n%d 个文件 · %s\n过滤 %d 个文件 / %d 个目录", state.Plan.Files, nativeHuman(state.Plan.Bytes), state.Plan.Filtered, state.Plan.ExcludedDirs)
						for i, item := range state.Plan.Items {
							if i == 45 {
								summary += "\n…（更多内容请查看完整清单）"
								break
							}
							if item.Directory {
								summary += "\n[目录] " + item.Name
							} else {
								summary += "\n" + item.Name
							}
						}
					}
				} else if state.OutputDirectory {
					summary = "项目已直接解压到：\n" + state.Output
				}
				if state.Plan != nil && !state.Preview {
					summary += fmt.Sprintf("\n\n%d 个文件 · %s · 过滤 %d 个文件 / %d 个目录", state.Plan.Files, nativeHuman(state.Plan.Bytes), state.Plan.Filtered, state.Plan.ExcludedDirs)
				}
				nativeMessage(u.hwnd, "EasyPackage · 完成", summary, mbOK)
				if state.Output != "" {
					nativeShow(u.controls[idShowOutput].hwnd, 5)
				}
				if u.closePending {
					nativeProc(user32, "DestroyWindow").Call(u.hwnd)
					return
				}
			}
		}
	}
	if u.closePending && !u.busy {
		nativeProc(user32, "DestroyWindow").Call(u.hwnd)
	}
}
func nativeHuman(n int64) string {
	if n >= 1073741824 {
		return fmt.Sprintf("%.2f GB", float64(n)/1073741824)
	}
	if n >= 1048576 {
		return fmt.Sprintf("%.2f MB", float64(n)/1048576)
	}
	if n >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}

func nativeChooseFolder(owner uintptr, current string) string {
	name := make([]uint16, 32768)
	title := nativeText("选择文件夹（桌面、文档、下载及最近目录可在界面中快速选择）")
	var startPath []uint16
	var start *uint16
	if current != "" {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			startPath = syscall.StringToUTF16(current)
			start = &startPath[0]
		}
	}
	callback := syscall.NewCallback(func(hwnd uintptr, message uint32, lparam, data uintptr) uintptr {
		if message == 1 && data != 0 { // BFFM_INITIALIZED / BFFM_SETSELECTIONW
			nativeSend(hwnd, 0x467, 1, data)
		}
		return 0
	})
	b := winBrowseInfo{Owner: owner, DisplayName: &name[0], Title: title, Flags: shReturnOnlyFSDirs | shNewDialogStyle, Callback: callback, Param: uintptr(unsafe.Pointer(start))}
	pidl, _, _ := nativeProc(shell32, "SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&b)))
	runtime.KeepAlive(startPath)
	if pidl == 0 {
		return ""
	}
	defer nativeProc(syscall.NewLazyDLL("ole32.dll"), "CoTaskMemFree").Call(pidl)
	out := make([]uint16, 32768)
	r, _, _ := nativeProc(shell32, "SHGetPathFromIDListW").Call(pidl, uintptr(unsafe.Pointer(&out[0])))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(out)
}
func nativeOpenFile(owner uintptr, current, title, filter string) string {
	return nativeFileDialog(owner, current, title, filter, "", false)
}
func nativeSaveFile(owner uintptr, current, title, filter, extension string) string {
	return nativeFileDialog(owner, current, title, filter, extension, true)
}
func nativeFileDialog(owner uintptr, current, title, filter, extension string, save bool) string {
	name := make([]uint16, 32768)
	initialDirectory := ""
	if current != "" {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			initialDirectory = current
		} else {
			copy(name, syscall.StringToUTF16(current))
			initialDirectory = filepath.Dir(current)
		}
	} else if save {
		copy(name, syscall.StringToUTF16("project"+extension))
	}
	filter16 := syscall.StringToUTF16(filter)
	flags := uint32(ofPathMustExist | ofHideReadOnly | ofNoChangeDir | ofNoDereferenceLinks)
	if save {
		flags |= ofOverwritePrompt
	} else {
		flags |= ofFileMustExist
	}
	defaultExt := syscall.StringToUTF16Ptr(strings.TrimPrefix(extension, "."))
	var initial16 []uint16
	var initialPtr *uint16
	if initialDirectory != "" {
		initial16 = syscall.StringToUTF16(initialDirectory)
		initialPtr = &initial16[0]
	}
	dlg := winOpenFile{Size: uint32(unsafe.Sizeof(winOpenFile{})), Owner: owner, Filter: &filter16[0], FilterIndex: 1, File: &name[0], MaxFile: uint32(len(name)), InitialDirectory: initialPtr, Title: nativeText(title), Flags: flags, DefaultExtension: defaultExt}
	proc := "GetOpenFileNameW"
	if save {
		proc = "GetSaveFileNameW"
	}
	r, _, _ := nativeProc(comdlg32, proc).Call(uintptr(unsafe.Pointer(&dlg)))
	runtime.KeepAlive(filter16)
	runtime.KeepAlive(initial16)
	if r == 0 {
		return ""
	}
	path := syscall.UTF16ToString(name)
	if save && filepath.Ext(path) == "" {
		path += extension
	}
	return path
}
func (u *windowsNativeUI) scrollBy(code uint16, track int16) {
	max := u.contentHeight - (u.clientH - 116)
	if max < 0 {
		max = 0
	}
	step := int32(38)
	switch code {
	case sbLineUp:
		u.scroll -= step
	case sbLineDown:
		u.scroll += step
	case sbPageUp:
		u.scroll -= u.clientH / 2
	case sbPageDown:
		u.scroll += u.clientH / 2
	case sbThumbPosition, sbThumbTrack:
		u.scroll = int32(track)
	}
	if u.scroll < 0 {
		u.scroll = 0
	}
	if u.scroll > max {
		u.scroll = max
	}
	info := winScrollInfo{Size: uint32(unsafe.Sizeof(winScrollInfo{})), Mask: 0x4, Pos: u.scroll}
	nativeProc(user32, "SetScrollInfo").Call(u.hwnd, sbVert, uintptr(unsafe.Pointer(&info)), 1)
	u.layout(u.clientW, u.clientH)
}
func (u *windowsNativeUI) openOutput() {
	if u.lastOutput == "" {
		return
	}
	target := filepath.Dir(u.lastOutput)
	if u.lastOutputIsDir {
		target = u.lastOutput
	}
	nativeProc(shell32, "ShellExecuteW").Call(u.hwnd, uintptr(unsafe.Pointer(nativeText("open"))), uintptr(unsafe.Pointer(nativeText("explorer.exe"))), uintptr(unsafe.Pointer(nativeText(target))), 0, 1)
}

var _ = time.Second
