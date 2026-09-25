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

type POINT struct {
	X, Y int32
}

var (
	isDragging      bool
	dragStartCursor struct{ x, y int32 }
	dragStartWindow struct{ x, y int }
)

const (
	WM_TIMER = 0x0113
)

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
			setTimer := user32.NewProc("SetTimer")

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
				case WM_TIMER:
					handleTimer(hwnd)
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
				// ColorKey 0x00000000 (black) is transparent
				setLayeredWindowAttributes.Call(hwnd, 0, 0, LWA_COLORKEY)
				overlayMu.Lock()
				overlayHWND = hwnd
				overlayMu.Unlock()

				// Start 30ms timer for cursor tracking, hover detection, and window dragging
				setTimer.Call(hwnd, 1, 30, 0)
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

func handleTimer(hwnd uintptr) {
	overlayMu.Lock()
	mode := overlayMode
	bounds := overlayBounds
	wasHovered := overlayHovered
	dragging := isDragging
	overlayMu.Unlock()

	user32 := syscall.NewLazyDLL("user32.dll")
	getCursorPos := user32.NewProc("GetCursorPos")
	getAsyncKeyState := user32.NewProc("GetAsyncKeyState")
	invalidateRect := user32.NewProc("InvalidateRect")
	setWindowPos := user32.NewProc("SetWindowPos")

	var pt POINT
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	keyState, _, _ := getAsyncKeyState.Call(1) // VK_LBUTTON
	isLButtonDown := (keyState & 0x8000) != 0

	if mode == "pip" {
		inBounds := int(pt.X) >= bounds.x && int(pt.X) <= bounds.x+bounds.w &&
			int(pt.Y) >= bounds.y && int(pt.Y) <= bounds.y+bounds.h

		btnW := int32(140)
		btnH := int32(34)
		btnX := int32(bounds.x) + (int32(bounds.w)-btnW)/2
		btnY := int32(bounds.y) + int32(bounds.h) - btnH - 18
		inButton := pt.X >= btnX && pt.X <= btnX+btnW && pt.Y >= btnY && pt.Y <= btnY+btnH

		if isLButtonDown {
			if !dragging {
				if wasHovered && inButton {
					// User clicked "Take Control"
					ResizeBrowserWindow("full")
					return
				} else if inBounds {
					// User started dragging PiP
					overlayMu.Lock()
					isDragging = true
					dragStartCursor = struct{ x, y int32 }{pt.X, pt.Y}
					dragStartWindow = struct{ x, y int }{bounds.x, bounds.y}
					overlayMu.Unlock()
				}
			} else {
				// Dragging in progress: reposition both QEMU and Overlay
				overlayMu.Lock()
				dx := int(pt.X - dragStartCursor.x)
				dy := int(pt.Y - dragStartCursor.y)
				newX := dragStartWindow.x + dx
				newY := dragStartWindow.y + dy
				overlayBounds.x = newX
				overlayBounds.y = newY
				overlayMu.Unlock()

				qHwnd := findQEMUHWND()
				if qHwnd != 0 {
					setWindowPos.Call(qHwnd, ^uintptr(0), uintptr(newX), uintptr(newY), uintptr(bounds.w), uintptr(bounds.h), SWP_SHOWWINDOW)
				}
				setWindowPos.Call(hwnd, ^uintptr(0), uintptr(newX), uintptr(newY), uintptr(bounds.w), uintptr(bounds.h), SWP_SHOWWINDOW)
				invalidateRect.Call(hwnd, 0, 0)
			}
		} else {
			if dragging {
				overlayMu.Lock()
				isDragging = false
				overlayMu.Unlock()
			}

			// Update hover state
			if inBounds != wasHovered {
				overlayMu.Lock()
				overlayHovered = inBounds
				overlayMu.Unlock()
				invalidateRect.Call(hwnd, 0, 0)
			}
		}
	} else if mode == "expanded" {
		btnW := int32(bounds.w)
		btnH := int32(bounds.h)
		inButton := int(pt.X) >= bounds.x && int(pt.X) <= bounds.x+int(btnW) &&
			int(pt.Y) >= bounds.y && int(pt.Y) <= bounds.y+int(btnH)

		if isLButtonDown && inButton {
			ResizeBrowserWindow("pip")
		}
	}
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
			// Subtle top drag handle
			dragW := int32(60)
			dragH := int32(4)
			dragX := (rect.Right - dragW) / 2
			dragY := int32(8)
			handleBrush, _, _ := createSolidBrush.Call(0x00808080)
			oldHBrush, _, _ := selectObject.Call(hdc, handleBrush)
			roundRect.Call(hdc, uintptr(dragX), uintptr(dragY), uintptr(dragX+dragW), uintptr(dragY+dragH), 4, 4)
			selectObject.Call(hdc, oldHBrush)
			deleteObject.Call(handleBrush)

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
