//go:build windows

package tools

import (
	"log/slog"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	overlayHWND    uintptr
	overlayMu      sync.Mutex
	overlayMode    string // "pip" or "expanded"
	overlayHovered bool
	overlayBounds  struct {
		x, y, w, h int
	}
	overlayInitOnce sync.Once
)

const (
	WS_POPUP          = 0x80000000
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	LWA_COLORKEY      = 0x00000001
	SWP_NOSIZE        = 0x0001
	SWP_NOMOVE        = 0x0002
	SWP_NOZORDER      = 0x0004
	SWP_SHOWWINDOW    = 0x0040
	SW_HIDE           = 0
	SW_SHOW           = 5
	WM_ERASEBKGND     = 0x0014
	WM_PAINT          = 0x000F
	WM_MOUSEMOVE      = 0x0200
	WM_MOUSELEAVE     = 0x02A3
	WM_LBUTTONUP      = 0x0202
	TME_LEAVE         = 0x00000002
	DT_CENTER         = 0x00000001
	DT_VCENTER        = 0x00000004
	DT_SINGLELINE     = 0x00000020
)

type TRACKMOUSEEVENT struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   uintptr
	dwHoverTime uint32
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

func initOverlayWindow() {
	overlayInitOnce.Do(func() {
		go func() {
			runtime.LockOSThread()
			user32 := syscall.NewLazyDLL("user32.dll")
			registerClassEx := user32.NewProc("RegisterClassExW")
			createWindowEx := user32.NewProc("CreateWindowExW")
			defWindowProc := user32.NewProc("DefWindowProcW")
			setLayeredWindowAttributes := user32.NewProc("SetLayeredWindowAttributes")
			getMessage := user32.NewProc("GetMessageW")
			translateMessage := user32.NewProc("TranslateMessage")
			dispatchMessage := user32.NewProc("DispatchMessageW")
			loadCursor := user32.NewProc("LoadCursorW")

			cursor, _, _ := loadCursor.Call(0, uintptr(32512)) // IDC_ARROW

			className, _ := syscall.UTF16PtrFromString("OneLlamaPiPOverlay")
			windowName, _ := syscall.UTF16PtrFromString("OneLlama Overlay")

			wndProc := syscall.NewCallback(func(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
				switch msg {
				case WM_ERASEBKGND:
					return 1
				case WM_PAINT:
					paintOverlay(hwnd)
					return 0
				case WM_MOUSEMOVE:
					handleMouseMove(hwnd)
					return 0
				case WM_MOUSELEAVE:
					handleMouseLeave(hwnd)
					return 0
				case WM_LBUTTONUP:
					handleMouseClick()
					return 0
				}
				ret, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
				return ret
			})

			wc := WNDCLASSEXW{
				CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
				Style:         0x0003, // CS_HREDRAW | CS_VREDRAW
				LpfnWndProc:   wndProc,
				HCursor:       cursor,
				LpszClassName: className,
			}

			registerClassEx.Call(uintptr(unsafe.Pointer(&wc)))

			hwnd, _, _ := createWindowEx.Call(
				uintptr(WS_EX_TOPMOST|WS_EX_LAYERED|WS_EX_TOOLWINDOW),
				uintptr(unsafe.Pointer(className)),
				uintptr(unsafe.Pointer(windowName)),
				uintptr(WS_POPUP),
				0, 0, 480, 300,
				0, 0, 0, 0,
			)

			if hwnd != 0 {
				// ColorKey 0x00000000 (black) is 100% transparent and click-through
				setLayeredWindowAttributes.Call(hwnd, 0, 0, LWA_COLORKEY)
				overlayMu.Lock()
				overlayHWND = hwnd
				overlayMu.Unlock()
			}

			var msg [48]byte
			for {
				ret, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
				if ret == 0 || int32(ret) == -1 {
					break
				}
				translateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
				dispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
			}
		}()
	})
}

func paintOverlay(hwnd uintptr) {
	user32 := syscall.NewLazyDLL("user32.dll")
	gdi32 := syscall.NewLazyDLL("gdi32.dll")
	beginPaint := user32.NewProc("BeginPaint")
	endPaint := user32.NewProc("EndPaint")
	getClientRect := user32.NewProc("GetClientRect")
	fillRect := user32.NewProc("FillRect")
	drawText := user32.NewProc("DrawTextW")
	setBkMode := gdi32.NewProc("SetBkMode")
	setTextColor := gdi32.NewProc("SetTextColor")
	createSolidBrush := gdi32.NewProc("CreateSolidBrush")
	deleteObject := gdi32.NewProc("DeleteObject")
	createFont := gdi32.NewProc("CreateFontW")
	selectObject := gdi32.NewProc("SelectObject")
	roundRect := gdi32.NewProc("RoundRect")

	var ps PAINTSTRUCT
	hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rect RECT
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))

	overlayMu.Lock()
	mode := overlayMode
	hovered := overlayHovered
	overlayMu.Unlock()

	// Clear full client rect with black (colorkey = transparent)
	blackBrush, _, _ := createSolidBrush.Call(0)
	fillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), blackBrush)
	deleteObject.Call(blackBrush)

	fontName, _ := syscall.UTF16PtrFromString("Segoe UI")
	font, _, _ := createFont.Call(
		15, 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(fontName)),
	)
	oldFont, _, _ := selectObject.Call(hdc, font)
	defer func() {
		selectObject.Call(hdc, oldFont)
		deleteObject.Call(font)
	}()

	setBkMode.Call(hdc, 1) // TRANSPARENT

	if mode == "pip" {
		if hovered {
			// Draw "Take Control" pill button inside PiP
			btnW := int32(140)
			btnH := int32(34)
			btnX := (rect.Right - btnW) / 2
			btnY := rect.Bottom - btnH - 18

			btnRect := RECT{Left: btnX, Top: btnY, Right: btnX + btnW, Bottom: btnY + btnH}
			blueBrush, _, _ := createSolidBrush.Call(0x00D97706) // Modern Blue RGB(6, 119, 217)
			oldBrush, _, _ := selectObject.Call(hdc, blueBrush)
			roundRect.Call(hdc, uintptr(btnX), uintptr(btnY), uintptr(btnX+btnW), uintptr(btnY+btnH), 18, 18)
			selectObject.Call(hdc, oldBrush)
			deleteObject.Call(blueBrush)

			setTextColor.Call(hdc, 0x00FFFFFF)
			txt, _ := syscall.UTF16PtrFromString("Take Control")
			drawText.Call(hdc, uintptr(unsafe.Pointer(txt)), uintptr(len("Take Control")), uintptr(unsafe.Pointer(&btnRect)), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		}
	} else if mode == "expanded" {
		// Draw "Return to Agent" pill button pinned at bottom center
		btnRect := RECT{Left: 0, Top: 0, Right: rect.Right, Bottom: rect.Bottom}
		darkBrush, _, _ := createSolidBrush.Call(0x001B1818) // Dark charcoal
		oldBrush, _, _ := selectObject.Call(hdc, darkBrush)
		roundRect.Call(hdc, 0, 0, uintptr(rect.Right), uintptr(rect.Bottom), 20, 20)
		selectObject.Call(hdc, oldBrush)
		deleteObject.Call(darkBrush)

		setTextColor.Call(hdc, 0x00E4E4E7)
		txt, _ := syscall.UTF16PtrFromString("✕  Return to Agent")
		drawText.Call(hdc, uintptr(unsafe.Pointer(txt)), uintptr(len("✕  Return to Agent")), uintptr(unsafe.Pointer(&btnRect)), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}
}

func handleMouseMove(hwnd uintptr) {
	overlayMu.Lock()
	mode := overlayMode
	isHovered := overlayHovered
	overlayMu.Unlock()

	if mode == "pip" && !isHovered {
		user32 := syscall.NewLazyDLL("user32.dll")
		trackMouseEvent := user32.NewProc("TrackMouseEvent")
		invalidateRect := user32.NewProc("InvalidateRect")

		tme := TRACKMOUSEEVENT{
			cbSize:    uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})),
			dwFlags:   TME_LEAVE,
			hwndTrack: hwnd,
		}
		trackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))

		overlayMu.Lock()
		overlayHovered = true
		overlayMu.Unlock()

		invalidateRect.Call(hwnd, 0, 0)
	}
}

func handleMouseLeave(hwnd uintptr) {
	overlayMu.Lock()
	mode := overlayMode
	isHovered := overlayHovered
	overlayMu.Unlock()

	if mode == "pip" && isHovered {
		user32 := syscall.NewLazyDLL("user32.dll")
		invalidateRect := user32.NewProc("InvalidateRect")

		overlayMu.Lock()
		overlayHovered = false
		overlayMu.Unlock()

		invalidateRect.Call(hwnd, 0, 0)
	}
}

func handleMouseClick() {
	overlayMu.Lock()
	mode := overlayMode
	overlayMu.Unlock()

	if mode == "pip" {
		ResizeBrowserWindow("full")
	} else if mode == "expanded" {
		ResizeBrowserWindow("pip")
	}
}

func ShowPiPOverlayWindow(x, y, w, h int) {
	initOverlayWindow()

	overlayMu.Lock()
	hwnd := overlayHWND
	overlayMode = "pip"
	overlayHovered = false
	overlayBounds.x = x
	overlayBounds.y = y
	overlayBounds.w = w
	overlayBounds.h = h
	overlayMu.Unlock()

	if hwnd == 0 {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	setWindowPos := user32.NewProc("SetWindowPos")
	showWindow := user32.NewProc("ShowWindow")
	invalidateRect := user32.NewProc("InvalidateRect")

	setWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_SHOWWINDOW)
	showWindow.Call(hwnd, SW_SHOW)
	invalidateRect.Call(hwnd, 0, 0)
	slog.Info("PiP Overlay active on QEMU window", "x", x, "y", y, "w", w, "h", h)
}

func ShowExpandedOverlayWindow(qemuX, qemuY, qemuW, qemuH int) {
	initOverlayWindow()

	// Pill button floats at bottom center of expanded QEMU window
	btnW := 180
	btnH := 38
	btnX := qemuX + (qemuW-btnW)/2
	btnY := qemuY + qemuH - btnH - 16

	overlayMu.Lock()
	hwnd := overlayHWND
	overlayMode = "expanded"
	overlayHovered = false
	overlayBounds.x = btnX
	overlayBounds.y = btnY
	overlayBounds.w = btnW
	overlayBounds.h = btnH
	overlayMu.Unlock()

	if hwnd == 0 {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	setWindowPos := user32.NewProc("SetWindowPos")
	showWindow := user32.NewProc("ShowWindow")
	invalidateRect := user32.NewProc("InvalidateRect")

	setWindowPos.Call(hwnd, ^uintptr(0), uintptr(btnX), uintptr(btnY), uintptr(btnW), uintptr(btnH), SWP_SHOWWINDOW)
	showWindow.Call(hwnd, SW_SHOW)
	invalidateRect.Call(hwnd, 0, 0)
	slog.Info("Expanded Pill Overlay active on QEMU window", "x", btnX, "y", btnY)
}

func HideOverlayWindow() {
	overlayMu.Lock()
	hwnd := overlayHWND
	overlayMu.Unlock()

	if hwnd != 0 {
		user32 := syscall.NewLazyDLL("user32.dll")
		showWindow := user32.NewProc("ShowWindow")
		showWindow.Call(hwnd, SW_HIDE)
	}
}
