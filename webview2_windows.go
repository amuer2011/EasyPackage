//go:build windows && amd64

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// WebView2Loader.dll is Microsoft's signed loader redistributable. The WebView2
// browser runtime itself is supplied by Microsoft Edge on the target machine.
//
//go:embed assets/windows/amd64/WebView2Loader.dll
var embeddedWebView2Loader []byte

const (
	wvWindowClass     = "EasyPackageWebView2WindowV24"
	wvTimerID         = 742
	wvLoadMemFixed    = 0
	wvLoadMemZero     = 0x40
	wvCoInitApartment = 0x2
)

var (
	webView2LoaderDLL = syscall.NewLazyDLL("kernel32.dll")
	webView2User32    = syscall.NewLazyDLL("user32.dll")
	webView2GDI       = syscall.NewLazyDLL("gdi32.dll")
	webView2Ole32     = syscall.NewLazyDLL("ole32.dll")
	webView2Callbacks sync.Map // native callback object address -> Go callback
	activeWebView2    *windowsWebView
)

type webviewCallback struct {
	iid    webviewGUID
	invoke func(result, object uintptr) uintptr
}

type webviewGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	webviewIUnknownIID             = webviewGUID{Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	webviewEnvironmentCompletedIID = webviewGUID{0x4E8A3389, 0xC9D8, 0x4BD2, [8]byte{0xB6, 0xB5, 0x12, 0x4F, 0xEE, 0x6C, 0xC1, 0x4D}}
	webviewControllerCompletedIID  = webviewGUID{0x6C4819F3, 0xC9B7, 0x4260, [8]byte{0x81, 0x27, 0xC9, 0xF5, 0xBD, 0xE7, 0xF6, 0x8C}}
)

type windowsWebView struct {
	hwnd, environment, controller, view uintptr
	url                                 string
	loaderPath                          string
	stop                                <-chan struct{}
	closed                              bool
}

func runWindowsWebViewUI() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	loaderPath, err := installWebView2Loader()
	if err != nil {
		nativeMessage(0, "EasyPackage 启动失败", "无法准备 Microsoft WebView2 组件：\n"+err.Error(), mbOK|mbIconError)
		return
	}
	loader, err := syscall.LoadDLL(loaderPath)
	if err != nil {
		nativeMessage(0, "EasyPackage 启动失败", "无法加载 Microsoft WebView2 组件：\n"+err.Error(), mbOK|mbIconError)
		return
	}
	defer loader.Release()
	versionProc, err := loader.FindProc("GetAvailableCoreWebView2BrowserVersionString")
	if err != nil {
		nativeMessage(0, "EasyPackage 启动失败", "WebView2 运行环境检测失败。", mbOK|mbIconError)
		return
	}
	var version *uint16
	versionResult, _, _ := versionProc.Call(0, uintptr(unsafe.Pointer(&version)))
	if version != nil {
		webView2Ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(version)))
	}
	if int32(versionResult) < 0 {
		nativeMessage(0, "需要安装 WebView2", "本机尚未检测到 Microsoft Edge WebView2 Runtime。\n\n请安装 Microsoft Edge WebView2 Runtime 后重新打开 EasyPackage。安装完成后无需联网，文件仍只在本机处理。\n\n官方下载：developer.microsoft.com/microsoft-edge/webview2/", mbOK|mbIconWarning)
		return
	}

	model, server, _, address, err := startEmbeddedService()
	if err != nil {
		nativeMessage(0, "EasyPackage 启动失败", "无法启动本机界面：\n"+err.Error(), mbOK|mbIconError)
		return
	}
	ui := &windowsWebView{url: address, stop: model.stop, loaderPath: loaderPath}
	activeWebView2 = ui
	if err = ui.run(); err != nil {
		nativeMessage(ui.hwnd, "EasyPackage 启动失败", err.Error(), mbOK|mbIconError)
	}
	activeWebView2 = nil
	model.once.Do(func() { close(model.stop) })
	stopEmbeddedService(model, server)
}

func runNativeWindowsUI() { runWindowsWebViewUI() }

func installWebView2Loader() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(cache, "EasyPackage", "webview2")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "WebView2Loader.dll")
	if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, embeddedWebView2Loader) {
		return path, nil
	}
	if err = os.WriteFile(path, embeddedWebView2Loader, 0600); err != nil {
		return "", err
	}
	return path, nil
}

func (ui *windowsWebView) run() error {
	hr, _, _ := webView2Ole32.NewProc("CoInitializeEx").Call(0, wvCoInitApartment)
	if int32(hr) < 0 {
		return fmt.Errorf("无法初始化 Windows 界面线程（HRESULT 0x%08X）", uint32(hr))
	}
	defer webView2Ole32.NewProc("CoUninitialize").Call()
	defer ui.close()

	_, _, _ = webView2User32.NewProc("SetProcessDPIAware").Call()
	instance, _, _ := controlsKernel.NewProc("GetModuleHandleW").Call(0)
	className := nativeText(wvWindowClass)
	windowProc := syscall.NewCallback(webView2WindowProc)
	cursor, _, _ := webView2User32.NewProc("LoadCursorW").Call(0, 32512)
	class := winWindowClass{
		Size: uint32(unsafe.Sizeof(winWindowClass{})), Style: 0x0002 | 0x0001,
		WndProc: windowProc, Instance: instance, Icon: nativeAppIcon(instance), Cursor: cursor,
		Background: 0, ClassName: className,
	}
	class.IconSmall = class.Icon
	if result, _, callErr := webView2User32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("无法注册 EasyPackage 窗口（%v）", callErr)
	}
	width, height := int32(1260), int32(900)
	screenWidth, _, _ := webView2User32.NewProc("GetSystemMetrics").Call(0)
	screenHeight, _, _ := webView2User32.NewProc("GetSystemMetrics").Call(1)
	if screenWidth > 100 && uintptr(width) > screenWidth-80 {
		width = int32(screenWidth - 80)
	}
	if screenHeight > 100 && uintptr(height) > screenHeight-80 {
		height = int32(screenHeight - 80)
	}
	x, y := int32(40), int32(35)
	if screenWidth > uintptr(width) {
		x = int32((screenWidth - uintptr(width)) / 2)
	}
	if screenHeight > uintptr(height) {
		y = int32((screenHeight - uintptr(height)) / 2)
	}
	const wsClipChildren = uint32(0x02000000)
	ui.hwnd = nativeCreateWindow(wvWindowClass, "EasyPackage · 轻松打包", wsOverlappedWindow|wsClipChildren, wsExAppWindow, x, y, width, height, 0, instance, 0)
	if ui.hwnd == 0 {
		return errors.New("无法创建 EasyPackage 软件窗口")
	}
	webView2User32.NewProc("ShowWindow").Call(ui.hwnd, 5)
	webView2User32.NewProc("UpdateWindow").Call(ui.hwnd)
	webView2User32.NewProc("SetTimer").Call(ui.hwnd, wvTimerID, 180, 0)
	if err := ui.createEnvironment(); err != nil {
		webView2User32.NewProc("DestroyWindow").Call(ui.hwnd)
		return err
	}

	var message winMessage
	for {
		result, _, _ := webView2User32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == 0 {
			break
		}
		if int32(result) == -1 {
			return errors.New("Windows 界面消息循环发生错误")
		}
		webView2User32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
		webView2User32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
	}
	return nil
}

func (ui *windowsWebView) createEnvironment() error {
	loader, err := syscall.LoadDLL(ui.loaderPath)
	if err != nil {
		return err
	}
	defer loader.Release()
	create, err := loader.FindProc("CreateCoreWebView2EnvironmentWithOptions")
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	profile := filepath.Join(cache, "EasyPackage", "WebView2Profile")
	if err = os.MkdirAll(profile, 0700); err != nil {
		return err
	}
	profilePath, err := syscall.UTF16PtrFromString(profile)
	if err != nil {
		return err
	}
	callback := newWebViewCallback(webviewEnvironmentCompletedIID, func(code, environment uintptr) uintptr {
		if int32(code) < 0 || environment == 0 {
			ui.fail(fmt.Sprintf("WebView2 初始化失败（HRESULT 0x%08X）。请确认 Microsoft Edge WebView2 Runtime 已安装。", uint32(code)))
			return 0
		}
		if ui.closed {
			return 0
		}
		webViewRetain(environment)
		ui.environment = environment
		return ui.createController()
	})
	if callback == 0 {
		return errors.New("无法创建 WebView2 环境回调")
	}
	result, _, _ := create.Call(0, uintptr(unsafe.Pointer(profilePath)), 0, callback)
	runtime.KeepAlive(profilePath)
	if int32(result) < 0 {
		releaseWebViewCallback(callback)
		return fmt.Errorf("WebView2 环境创建失败（HRESULT 0x%08X）", uint32(result))
	}
	releaseWebViewCallback(callback)
	return nil
}

func (ui *windowsWebView) createController() uintptr {
	callback := newWebViewCallback(webviewControllerCompletedIID, func(code, controller uintptr) uintptr {
		if int32(code) < 0 || controller == 0 {
			ui.fail(fmt.Sprintf("WebView2 控件创建失败（HRESULT 0x%08X）", uint32(code)))
			return 0
		}
		if ui.closed {
			return 0
		}
		webViewRetain(controller)
		ui.controller = controller
		ui.resize()
		webViewCall(controller, 4, 1) // ICoreWebView2Controller::put_IsVisible
		var view uintptr
		result, _, _ := webViewCall(controller, 25, uintptr(unsafe.Pointer(&view))) // ICoreWebView2Controller::get_CoreWebView2
		if int32(result) < 0 || view == 0 {
			ui.fail(fmt.Sprintf("无法打开 EasyPackage 页面（HRESULT 0x%08X）", uint32(result)))
			return 0
		}
		ui.view = view
		ui.disableBrowserChrome()
		address, _ := syscall.UTF16PtrFromString(ui.url)
		result, _, _ = webViewCall(view, 5, uintptr(unsafe.Pointer(address))) // Navigate
		if int32(result) < 0 {
			ui.fail(fmt.Sprintf("无法显示 EasyPackage 页面（HRESULT 0x%08X）", uint32(result)))
			return 0
		}
		webView2User32.NewProc("InvalidateRect").Call(ui.hwnd, 0, 1)
		return 0
	})
	if callback == 0 {
		ui.fail("无法创建 WebView2 窗口回调")
		return uintptr(uint32(0x8007000E)) // E_OUTOFMEMORY
	}
	result, _, _ := webViewCall(ui.environment, 3, ui.hwnd, callback) // CreateCoreWebView2Controller
	if int32(result) < 0 {
		releaseWebViewCallback(callback)
		ui.fail(fmt.Sprintf("WebView2 窗口创建失败（HRESULT 0x%08X）", uint32(result)))
		return result
	}
	releaseWebViewCallback(callback)
	return result
}

func (ui *windowsWebView) disableBrowserChrome() {
	var settings uintptr
	result, _, _ := webViewCall(ui.view, 3, uintptr(unsafe.Pointer(&settings))) // get_Settings
	if int32(result) < 0 || settings == 0 {
		return
	}
	webViewCall(settings, 10, 0) // put_IsStatusBarEnabled(FALSE)
	webViewCall(settings, 12, 0) // put_AreDevToolsEnabled(FALSE)
	webViewCall(settings, 14, 0) // put_AreDefaultContextMenusEnabled(FALSE)
	webViewCall(settings, 18, 0) // put_IsZoomControlEnabled(FALSE)
	webViewRelease(settings)
}

func (ui *windowsWebView) resize() {
	if ui.controller == 0 || ui.hwnd == 0 {
		return
	}
	var bounds winRect
	if result, _, _ := webView2User32.NewProc("GetClientRect").Call(ui.hwnd, uintptr(unsafe.Pointer(&bounds))); result != 0 {
		webViewCall(ui.controller, 6, uintptr(unsafe.Pointer(&bounds))) // put_Bounds
	}
}

func (ui *windowsWebView) fail(message string) {
	if ui.closed {
		return
	}
	nativeMessage(ui.hwnd, "EasyPackage 启动失败", message, mbOK|mbIconError)
	webView2User32.NewProc("DestroyWindow").Call(ui.hwnd)
}

func (ui *windowsWebView) close() {
	if ui.closed {
		return
	}
	ui.closed = true
	if ui.controller != 0 {
		webViewCall(ui.controller, 24) // ICoreWebView2Controller::Close
		webViewRelease(ui.controller)
		ui.controller = 0
	}
	if ui.view != 0 {
		webViewRelease(ui.view)
		ui.view = 0
	}
	if ui.environment != 0 {
		webViewRelease(ui.environment)
		ui.environment = 0
	}
}

func webviewCallbackInvoke(this, result, object uintptr) uintptr {
	if callback, ok := webView2Callbacks.Load(this); ok {
		return callback.(*webviewCallback).invoke(result, object)
	}
	return 0
}

var webviewCallbackThunks = [...]uintptr{
	syscall.NewCallback(webviewQueryInterface),
	syscall.NewCallback(webviewAddRef),
	syscall.NewCallback(webviewRelease),
	syscall.NewCallback(webviewCallbackInvoke),
}

func newWebViewCallback(iid webviewGUID, invoke func(result, object uintptr) uintptr) uintptr {
	object, _, _ := webView2LoaderDLL.NewProc("LocalAlloc").Call(wvLoadMemFixed|wvLoadMemZero, 2*unsafe.Sizeof(uintptr(0)))
	if object == 0 {
		return 0
	}
	table, _, _ := webView2LoaderDLL.NewProc("LocalAlloc").Call(wvLoadMemFixed|wvLoadMemZero, uintptr(len(webviewCallbackThunks))*unsafe.Sizeof(uintptr(0)))
	if table == 0 {
		webView2LoaderDLL.NewProc("LocalFree").Call(object)
		return 0
	}
	copy(unsafe.Slice((*uintptr)(unsafe.Pointer(table)), len(webviewCallbackThunks)), webviewCallbackThunks[:])
	*(*uintptr)(unsafe.Pointer(object)) = table
	*(*int32)(unsafe.Pointer(object + unsafe.Sizeof(uintptr(0)))) = 1
	webView2Callbacks.Store(object, &webviewCallback{iid: iid, invoke: invoke})
	return object
}

func releaseWebViewCallback(object uintptr) { webviewRelease(object) }

func webviewQueryInterface(this, iid, output uintptr) uintptr {
	if this == 0 || iid == 0 || output == 0 {
		return uintptr(uint32(0x80004003)) // E_POINTER
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	callbackValue, ok := webView2Callbacks.Load(this)
	if !ok {
		return uintptr(uint32(0x80004002)) // E_NOINTERFACE
	}
	requested := *(*webviewGUID)(unsafe.Pointer(iid))
	callback := callbackValue.(*webviewCallback)
	if requested != webviewIUnknownIID && requested != callback.iid {
		return uintptr(uint32(0x80004002)) // E_NOINTERFACE
	}
	*(*uintptr)(unsafe.Pointer(output)) = this
	webviewAddRef(this)
	return 0
}

func webviewAddRef(this uintptr) uintptr {
	if this == 0 {
		return 0
	}
	return uintptr(atomic.AddInt32((*int32)(unsafe.Pointer(this+unsafe.Sizeof(uintptr(0)))), 1))
}

func webviewRelease(this uintptr) uintptr {
	if this == 0 {
		return 0
	}
	value := atomic.AddInt32((*int32)(unsafe.Pointer(this+unsafe.Sizeof(uintptr(0)))), -1)
	if value == 0 {
		webView2Callbacks.Delete(this)
		vtable := *(*uintptr)(unsafe.Pointer(this))
		webView2LoaderDLL.NewProc("LocalFree").Call(vtable)
		webView2LoaderDLL.NewProc("LocalFree").Call(this)
	}
	return uintptr(value)
}

func webViewCall(object uintptr, index uintptr, args ...uintptr) (uintptr, uintptr, error) {
	if object == 0 {
		return uintptr(uint32(0x80004003)), 0, syscall.EINVAL
	}
	vtable := *(*uintptr)(unsafe.Pointer(object))
	method := *(*uintptr)(unsafe.Pointer(vtable + index*unsafe.Sizeof(uintptr(0))))
	parameters := make([]uintptr, 1, len(args)+1)
	parameters[0] = object
	parameters = append(parameters, args...)
	return syscall.SyscallN(method, parameters...)
}

func webViewRelease(object uintptr) {
	if object == 0 {
		return
	}
	vtable := *(*uintptr)(unsafe.Pointer(object))
	method := *(*uintptr)(unsafe.Pointer(vtable + 2*unsafe.Sizeof(uintptr(0))))
	syscall.SyscallN(method, object)
}

func webViewRetain(object uintptr) {
	if object == 0 {
		return
	}
	vtable := *(*uintptr)(unsafe.Pointer(object))
	method := *(*uintptr)(unsafe.Pointer(vtable + unsafe.Sizeof(uintptr(0))))
	syscall.SyscallN(method, object)
}

func webView2WindowProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	ui := activeWebView2
	switch message {
	case wmSize:
		if ui != nil {
			ui.resize()
		}
		return 0
	case wmTimer:
		if ui != nil {
			select {
			case <-ui.stop:
				webView2User32.NewProc("DestroyWindow").Call(hwnd)
			default:
			}
		}
		return 0
	case wmEraseBackground:
		rect := winRect{}
		webView2User32.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		brush, _, _ := webView2GDI.NewProc("CreateSolidBrush").Call(0x00FFFFFF)
		webView2User32.NewProc("FillRect").Call(wparam, uintptr(unsafe.Pointer(&rect)), brush)
		webView2GDI.NewProc("DeleteObject").Call(brush)
		return 1
	case wmPaint:
		if ui == nil || ui.controller == 0 {
			var paint winPaint
			dc, _, _ := webView2User32.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			if dc != 0 {
				var rect winRect
				webView2User32.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
				webView2GDI.NewProc("SetBkMode").Call(dc, 1)
				webView2GDI.NewProc("SetTextColor").Call(dc, 0x00717A91)
				font, _, _ := webView2GDI.NewProc("GetStockObject").Call(17)
				old, _, _ := webView2GDI.NewProc("SelectObject").Call(dc, font)
				webView2User32.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(nativeText("EasyPackage 正在启动…\n首次启动浏览器组件可能需要几秒钟，请稍候"))), uintptr(^uint32(0)), uintptr(unsafe.Pointer(&rect)), 0x0001|0x0010|0x0020)
				webView2GDI.NewProc("SelectObject").Call(dc, old)
				webView2User32.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			}
			return 0
		}
	case wmClose:
		webView2User32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case wmDestroy:
		webView2User32.NewProc("KillTimer").Call(hwnd, wvTimerID)
		if ui != nil {
			ui.close()
		}
		webView2User32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	result, _, _ := webView2User32.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wparam, lparam)
	return result
}
