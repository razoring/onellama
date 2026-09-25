package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
)

var (
	globalBrowserVM *BrowserVMTool
	globalVMOnce    sync.Once
)

type BrowserVMTool struct {
	mu           sync.Mutex
	qemuCmd      *exec.Cmd
	qemuHWND     uintptr
	baseDir      string
	qemuExe      string
	kernelPath   string
	initrdPath   string
	imgPath      string
	lastURL      string
	isControlled bool
}

func NewBrowserVMTool() *BrowserVMTool {
	globalVMOnce.Do(func() {
		exe, _ := os.Executable()
		baseDir := filepath.Dir(exe)

		qemuBin := "qemu-system-x86_64"
		if runtime.GOOS == "windows" {
			qemuBin = "qemu-system-x86_64.exe"
		} else if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			qemuBin = "qemu-system-aarch64"
		}

		// Search for assets in executable dir or current working dir
		qemuExe := filepath.Join(baseDir, "assets", "vm", "qemu", qemuBin)
		kernelPath := filepath.Join(baseDir, "assets", "vm", "vmlinuz-virt")
		initrdPath := filepath.Join(baseDir, "assets", "vm", "initramfs-virt")
		imgPath := filepath.Join(baseDir, "assets", "vm", "virium-base.qcow2")

		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			// Fallback to relative path from project root
			baseDir, _ = os.Getwd()
			qemuExe = filepath.Join(baseDir, "app", "assets", "vm", "qemu", qemuBin)
			kernelPath = filepath.Join(baseDir, "app", "assets", "vm", "vmlinuz-virt")
			initrdPath = filepath.Join(baseDir, "app", "assets", "vm", "initramfs-virt")
			imgPath = filepath.Join(baseDir, "app", "assets", "vm", "virium-base.qcow2")
		}

		// Fallback to system PATH if bundled QEMU is not present
		if _, err := os.Stat(qemuExe); os.IsNotExist(err) {
			if pathExe, err := exec.LookPath(qemuBin); err == nil {
				qemuExe = pathExe
			}
		}

		globalBrowserVM = &BrowserVMTool{
			baseDir:    baseDir,
			qemuExe:    qemuExe,
			kernelPath: kernelPath,
			initrdPath: initrdPath,
			imgPath:    imgPath,
			lastURL:    "https://www.google.com",
		}

		// Ensure VM starts in background
		go globalBrowserVM.EnsureVMRunning()
	})
	return globalBrowserVM
}

func (b *BrowserVMTool) EnsureVMRunning() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if already responding on port 9222
	resp, err := http.Get("http://127.0.0.1:9222/json/version")
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return nil
	}

	if b.qemuCmd != nil && b.qemuCmd.Process != nil {
		// Process is already running, wait a bit for CDP
		return nil
	}

	if _, err := os.Stat(b.qemuExe); os.IsNotExist(err) {
		return fmt.Errorf("qemu binary not found at %s", b.qemuExe)
	}

	var accelArgs []string
	switch runtime.GOOS {
	case "darwin":
		accelArgs = []string{"-accel", "hvf", "-accel", "tcg"}
	case "linux":
		accelArgs = []string{"-accel", "kvm", "-accel", "tcg"}
	default: // windows
		accelArgs = []string{"-accel", "whpx", "-accel", "tcg"}
	}

	args := append([]string{
		"-m", "2048",
		"-smp", "4",
	}, accelArgs...)

	var displayArgs []string
	switch runtime.GOOS {
	case "darwin":
		displayArgs = []string{"-display", "cocoa"}
	case "linux":
		displayArgs = []string{"-display", "gtk,zoom-to-fit=on,show-menubar=off,show-tabs=off,window-close=off"}
	default: // windows
		displayArgs = []string{"-display", "gtk,zoom-to-fit=on,show-menubar=off,show-tabs=off,window-close=off,gl=off"}
	}

	args = append(args,
		"-kernel", b.kernelPath,
		"-initrd", b.initrdPath,
		"-append", "console=tty0 video=1280x800-32@60 root=/dev/vda rw modules=loop,squashfs,ext4,virtio_pci,virtio_blk,virtio_net rootwait rootdelay=3 quiet",
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", b.imgPath),
		"-netdev", "user,id=net0,hostfwd=tcp::9222-:9222",
		"-device", "virtio-net-pci,netdev=net0",
		"-device", "virtio-vga",
		"-usb",
		"-device", "usb-tablet",
		"-parallel", "none",
		"-serial", "none",
		"-monitor", "none",
		"-name", "OneLlama Browser VM",
	)
	args = append(args, displayArgs...)

	slog.Info("Starting bundled QEMU browser VM in background", "cmd", b.qemuExe)
	cmd := exec.Command(b.qemuExe, args...)
	cmd.Dir = filepath.Dir(b.qemuExe)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	if err := cmd.Start(); err != nil {
		// Fallback to SDL if GTK failed
		slog.Warn("Failed to start with primary display, falling back to SDL", "error", err)
		args = args[:len(args)-len(displayArgs)]
		args = append(args, "-display", "sdl,window-close=off")
		cmd = exec.Command(b.qemuExe, args...)
		cmd.Dir = filepath.Dir(b.qemuExe)
		if runtime.GOOS == "windows" {
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		}
		if errFallback := cmd.Start(); errFallback != nil {
			return fmt.Errorf("failed to start QEMU: %w", errFallback)
		}
	}
	b.qemuCmd = cmd

	// Start tab watchdog to monitor Chromium tabs and auto-hide when closed
	go b.watchdogChromiumTabs()

	// Rapidly detect and hide window at boot so it never appears on taskbar or desktop
	go func() {
		for i := 0; i < 100; i++ {
			time.Sleep(30 * time.Millisecond)
			hwnd := findQEMUHWND()
			if hwnd != 0 {
				b.mu.Lock()
				b.qemuHWND = hwnd
				b.mu.Unlock()
				makeFrameless(hwnd)
				hideWindow(hwnd)
				break
			}
		}
	}()

	return nil
}

func (b *BrowserVMTool) watchdogChromiumTabs() {
	var hadTabs bool
	for {
		time.Sleep(500 * time.Millisecond)

		b.mu.Lock()
		cmd := b.qemuCmd
		b.mu.Unlock()

		if cmd == nil || cmd.Process == nil {
			return
		}

		resp, err := http.Get("http://127.0.0.1:9222/json/list")
		if err != nil {
			if hadTabs {
				slog.Info("Chromium exited/disconnected, killing QEMU to respawn later")
				realHWND := findQEMUHWND()
				if realHWND != 0 {
					hideWindow(realHWND)
				}
				HideOverlayWindow()
				b.mu.Lock()
				if b.qemuCmd != nil && b.qemuCmd.Process != nil {
					b.qemuCmd.Process.Kill()
				}
				b.qemuCmd = nil
				b.isControlled = false
				b.mu.Unlock()
				hadTabs = false
			}
			continue
		}

		var tabs []map[string]any
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		_ = json.Unmarshal(body, &tabs)

		var pageTabs int
		for _, t := range tabs {
			if tType, _ := t["type"].(string); tType == "page" || tType == "" {
				pageTabs++
			}
		}

		if pageTabs > 0 {
			hadTabs = true
		} else if hadTabs && pageTabs == 0 {
			slog.Info("Last tab closed, hiding QEMU window and keeping VM idling in background")
			realHWND := findQEMUHWND()
			if realHWND != 0 {
				hideWindow(realHWND)
			}
			HideOverlayWindow()
			b.mu.Lock()
			b.isControlled = false
			b.mu.Unlock()
			hadTabs = false
		}
	}
}

func (b *BrowserVMTool) WaitForVM(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = b.EnsureVMRunning()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := http.Get("http://127.0.0.1:9222/json/version")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for browser VM to boot and listen on port 9222")
}

func evaluateInTab(ctx context.Context, wsURL string, expr string) (string, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	req := map[string]any{
		"id":     1,
		"method": "Runtime.evaluate",
		"params": map[string]any{
			"expression":    expr,
			"returnByValue": true,
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		return "", err
	}

	conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	for {
		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Result struct {
					Type  string `json:"type"`
					Value any    `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		if err := conn.ReadJSON(&resp); err != nil {
			return "", err
		}
		if resp.ID == 1 {
			if s, ok := resp.Result.Result.Value.(string); ok {
				return s, nil
			}
			return fmt.Sprintf("%v", resp.Result.Result.Value), nil
		}
	}
}

func findQEMUHWND() uintptr {
	if runtime.GOOS != "windows" {
		return 0
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	isWindow := user32.NewProc("IsWindow")
	enumWindows := user32.NewProc("EnumWindows")
	getWindowThreadProcessId := user32.NewProc("GetWindowThreadProcessId")
	getWindowTextW := user32.NewProc("GetWindowTextW")
	getClassNameW := user32.NewProc("GetClassNameW")
	getWindowRect := user32.NewProc("GetWindowRect")

	overlayMu.Lock()
	ovHWND := overlayHWND
	overlayMu.Unlock()

	// 1. Check if cached HWND is still valid and not the overlay
	if globalBrowserVM != nil {
		globalBrowserVM.mu.Lock()
		saved := globalBrowserVM.qemuHWND
		globalBrowserVM.mu.Unlock()
		if saved != 0 && saved != ovHWND {
			res, _, _ := isWindow.Call(saved)
			if res != 0 {
				var rect RECT
				getWindowRect.Call(saved, uintptr(unsafe.Pointer(&rect)))
				if (rect.Right-rect.Left) > 50 && (rect.Bottom-rect.Top) > 50 {
					return saved
				}
			}
		}
	}

	var targetPID uint32
	if globalBrowserVM != nil {
		globalBrowserVM.mu.Lock()
		if globalBrowserVM.qemuCmd != nil && globalBrowserVM.qemuCmd.Process != nil {
			targetPID = uint32(globalBrowserVM.qemuCmd.Process.Pid)
		}
		globalBrowserVM.mu.Unlock()
	}

	var foundHWND uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, lParam uintptr) uintptr {
		if hwnd == ovHWND {
			return 1 // Never match overlay window
		}

		classBuf := make([]uint16, 256)
		cLen, _, _ := getClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), 256)
		className := ""
		if cLen > 0 {
			className = syscall.UTF16ToString(classBuf[:cLen])
		}
		if className == "OneLlamaPiPOverlay" || strings.Contains(className, "IME") {
			return 1
		}

		titleBuf := make([]uint16, 256)
		tLen, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), 256)
		title := ""
		if tLen > 0 {
			title = syscall.UTF16ToString(titleBuf[:tLen])
		}
		if title == "OneLlama Overlay" {
			return 1
		}

		var rect RECT
		getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		w := rect.Right - rect.Left
		h := rect.Bottom - rect.Top

		if targetPID != 0 {
			var pid uint32
			getWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid == targetPID {
				// Must have non-trivial client dimensions to be the real display window
				if w > 100 && h > 100 {
					foundHWND = hwnd
					return 0
				}
				if strings.Contains(title, "OneLlama") || strings.Contains(title, "QEMU") || strings.Contains(title, "qemu") {
					if w > 20 && h > 20 {
						foundHWND = hwnd
						return 0
					}
				}
				return 1
			}
		}

		// Fallback by window title if targetPID is 0 or unmatched
		if strings.Contains(title, "OneLlama Browser VM") || strings.Contains(title, "QEMU (OneLlama") {
			if w > 50 && h > 50 {
				foundHWND = hwnd
				return 0
			}
		}
		return 1
	})
	enumWindows.Call(cb, 0)

	if foundHWND != 0 {
		if globalBrowserVM != nil {
			globalBrowserVM.mu.Lock()
			globalBrowserVM.qemuHWND = foundHWND
			globalBrowserVM.mu.Unlock()
		}
		return foundHWND
	}

	return 0
}

func hideWindow(hwnd uintptr) {
	if runtime.GOOS != "windows" || hwnd == 0 {
		return
	}
	user32 := syscall.NewLazyDLL("user32.dll")
	showWindow := user32.NewProc("ShowWindow")
	showWindow.Call(hwnd, 0) // SW_HIDE
}

func makeFrameless(hwnd uintptr) {
	if runtime.GOOS != "windows" || hwnd == 0 {
		return
	}
	user32 := syscall.NewLazyDLL("user32.dll")
	getWindowLong := user32.NewProc("GetWindowLongW")
	setWindowLong := user32.NewProc("SetWindowLongW")
	setWindowPos := user32.NewProc("SetWindowPos")

	style, _, _ := getWindowLong.Call(hwnd, uintptr(0xFFFFFFF0)) // GWL_STYLE = -16
	if style != 0 {
		// Strip WS_CAPTION, WS_THICKFRAME, WS_MINIMIZEBOX, WS_MAXIMIZEBOX, WS_SYSMENU, WS_BORDER
		style &^= uintptr(0x00C00000 | 0x00040000 | 0x00020000 | 0x00010000 | 0x00080000 | 0x00800000)
		style |= uintptr(0x80000000) // WS_POPUP
		setWindowLong.Call(hwnd, uintptr(0xFFFFFFF0), style)

		// Extended style: remove taskbar icon (WS_EX_APPWINDOW) and make tool window (WS_EX_TOOLWINDOW)
		exStyle, _, _ := getWindowLong.Call(hwnd, uintptr(0xFFFFFFEC)) // GWL_EXSTYLE = -20
		if exStyle != 0 {
			exStyle &^= uintptr(0x00040000) // Strip WS_EX_APPWINDOW
			exStyle |= uintptr(0x00000080)  // Add WS_EX_TOOLWINDOW
			setWindowLong.Call(hwnd, uintptr(0xFFFFFFEC), exStyle)
		}

		setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x0001|0x0002|0x0004|0x0020) // SWP_NOSIZE | SWP_NOMOVE | SWP_NOZORDER | SWP_FRAMECHANGED
	}
}

func OpenBrowserWindow() error {
	return OpenBrowserWindowWithHost("")
}

func OpenBrowserWindowWithHost(host string) error {
	vm := NewBrowserVMTool()
	_ = vm.EnsureVMRunning()

	if runtime.GOOS == "windows" {
		user32 := syscall.NewLazyDLL("user32.dll")
		getSystemMetrics := user32.NewProc("GetSystemMetrics")
		setWindowPos := user32.NewProc("SetWindowPos")
		showWindow := user32.NewProc("ShowWindow")
		enableWindow := user32.NewProc("EnableWindow")
		setForegroundWindow := user32.NewProc("SetForegroundWindow")
		bringWindowToTop := user32.NewProc("BringWindowToTop")

		var hwnd uintptr
		for i := 0; i < 15; i++ {
			hwnd = findQEMUHWND()
			if hwnd != 0 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		if hwnd != 0 {
			makeFrameless(hwnd)

			vm.mu.Lock()
			vm.qemuHWND = hwnd
			vm.isControlled = false
			vm.mu.Unlock()

			sw, _, _ := getSystemMetrics.Call(0) // SM_CXSCREEN
			sh, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN
			if sw == 0 {
				sw = 1920
			}
			if sh == 0 {
				sh = 1080
			}

			// Position at bottom-right corner (480x300 PiP preview mode)
			w := uintptr(480)
			h := uintptr(300)
			x := sw - w - 24
			y := sh - h - 64

			// Disable input in preview mode (read-only monitoring)
			enableWindow.Call(hwnd, 0)

			// Show & restore window
			showWindow.Call(hwnd, 9) // SW_RESTORE
			showWindow.Call(hwnd, 5) // SW_SHOW
			setWindowPos.Call(hwnd, ^uintptr(0), x, y, w, h, 0x0040) // HWND_TOPMOST | SWP_SHOWWINDOW
			bringWindowToTop.Call(hwnd)
			setForegroundWindow.Call(hwnd)

			// Attach hover-sensitive Take Control overlay
			ShowPiPOverlayWindow(int(x), int(y), int(w), int(h))
			return nil
		}
	}

	return nil
}

func ResizeBrowserWindow(size string) {
	if runtime.GOOS != "windows" {
		return
	}

	vm := NewBrowserVMTool()
	hwnd := findQEMUHWND()
	if hwnd == 0 {
		return
	}

	makeFrameless(hwnd)

	user32 := syscall.NewLazyDLL("user32.dll")
	getSystemMetrics := user32.NewProc("GetSystemMetrics")
	setWindowPos := user32.NewProc("SetWindowPos")
	showWindow := user32.NewProc("ShowWindow")
	enableWindow := user32.NewProc("EnableWindow")
	setForegroundWindow := user32.NewProc("SetForegroundWindow")

	sw, _, _ := getSystemMetrics.Call(0)
	sh, _, _ := getSystemMetrics.Call(1)
	if sw == 0 {
		sw = 1920
	}
	if sh == 0 {
		sh = 1080
	}

	if size == "full" {
		// Take Control: unlock interactions, resize to 1280x800 (1:1 native guest res), center on screen
		vm.mu.Lock()
		vm.isControlled = true
		vm.mu.Unlock()

		w := uintptr(1280)
		h := uintptr(800)
		if w > sw {
			w = sw
		}
		if h > sh {
			h = sh
		}
		x := (sw - w) / 2
		y := (sh - h) / 2

		// Unlock user interaction
		enableWindow.Call(hwnd, 1)

		// Resize and center window
		showWindow.Call(hwnd, 5) // SW_SHOW
		setWindowPos.Call(hwnd, ^uintptr(0), x, y, w, h, 0x0040)
		setForegroundWindow.Call(hwnd)

		// Show bottom-center pill button overlay to Return to Agent
		ShowExpandedOverlayWindow(int(x), int(y), int(w), int(h))

		// Clear any leftover Set-of-Marks badges so the user has a clean browser view
		go func() {
			_, wsURL, err := getActiveTab(context.Background())
			if err == nil && wsURL != "" {
				_, _ = evaluateInTab(context.Background(), wsURL, `document.querySelectorAll('.onellama-som-badge').forEach(function(b) { b.remove(); });`)
			}
		}()
	} else if size == "close" || size == "hide" {
		hideWindow(hwnd)
		HideOverlayWindow()
		vm.mu.Lock()
		vm.isControlled = false
		vm.mu.Unlock()
	} else {
		// Return Control to agent: lock interactions, shrink to 480x300, pin to bottom-right
		vm.mu.Lock()
		vm.isControlled = false
		vm.mu.Unlock()

		w := uintptr(480)
		h := uintptr(300)
		x := sw - w - 24
		y := sh - h - 64

		// Disable interaction in preview mode
		enableWindow.Call(hwnd, 0)

		// Shrink and pin topmost
		showWindow.Call(hwnd, 5)
		setWindowPos.Call(hwnd, ^uintptr(0), x, y, w, h, 0x0040) // HWND_TOPMOST

		// Restore hover PiP overlay
		ShowPiPOverlayWindow(int(x), int(y), int(w), int(h))
	}
}

func (b *BrowserVMTool) Name() string {
	return "browser_vm"
}

func (b *BrowserVMTool) Description() string {
	return "Isolated browser automation engine running inside a packaged QEMU Linux virtual machine. Supports live web navigation, Set-of-Marks (SoM) interactive browsing, clicking elements by markId, typing into inputs/search bars, scrolling, and desktop handoff."
}

func (b *BrowserVMTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"navigate", "click", "type", "scroll", "interact_mark", "screenshot", "handoff", "take_control", "return_control"},
				"description": "Action to perform. Use 'navigate' to visit a URL or search. Use 'click' with markId to click a marked element. Use 'type' with markId and text to input text. Use 'scroll' to scroll down.",
			},
			"url":      map[string]any{"type": "string", "description": "Target URL to navigate to (e.g., https://www.canadacomputers.com or search URL)."},
			"markId":   map[string]any{"type": "integer", "description": "Set-of-Mark numeric ID ([#1], [#2], etc.) of the element to click or type into."},
			"text":     map[string]any{"type": "string", "description": "Text to type into an input field or search bar."},
			"selector": map[string]any{"type": "string", "description": "Optional CSS selector fallback for click/type actions."},
			"reason":   map[string]any{"type": "string", "description": "Reason for desktop handoff."},
		},
		"required": []string{"action"},
	}
}

func (b *BrowserVMTool) Prompt() string {
	return `Primary tool for web browsing, live searching, and site navigation.
Use action 'navigate' with a target URL to open any website or search page.
Each action returns marked interactive elements with IDs [#1], [#2], etc.
- To click an element, use action 'click' with 'markId' (e.g. markId: 3) or 'text'.
- To type into a search bar or text field, use action 'type' with 'markId' (e.g. markId: 1) and 'text' (e.g. 'RTX 3060 12GB').
- To scroll down, use action 'scroll'.`
}

func getActiveTab(ctx context.Context) (string, string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	respList, err := client.Get("http://127.0.0.1:9222/json/list")
	if err != nil {
		return "", "", err
	}
	defer respList.Body.Close()
	bodyList, _ := io.ReadAll(respList.Body)
	var tabs []map[string]any
	_ = json.Unmarshal(bodyList, &tabs)
	for _, t := range tabs {
		if tType, _ := t["type"].(string); tType == "page" || tType == "" {
			tabID, _ := t["id"].(string)
			wsURL, _ := t["webSocketDebuggerUrl"].(string)
			return tabID, wsURL, nil
		}
	}

	// If no tab exists, create one
	reqURL := "http://127.0.0.1:9222/json/new?https://www.google.com"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var tabInfo map[string]any
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &tabInfo)
	wsURL, _ := tabInfo["webSocketDebuggerUrl"].(string)
	tabID, _ := tabInfo["id"].(string)
	return tabID, wsURL, nil
}

func extractPageContentWithSoM(ctx context.Context, wsURL string) (string, error) {
	js := `(function() {
		try {
			var marks = [];
			var idCounter = 1;

			document.querySelectorAll('.onellama-som-badge').forEach(function(b) { b.remove(); });
			if (window.__onellama_som_timer) clearTimeout(window.__onellama_som_timer);

			var selectors = 'a[href], button, input, textarea, select, [role="button"], [role="link"], [role="searchbox"]';
			var elements = document.querySelectorAll(selectors);

			var colors = [
				'#e11d48', '#2563eb', '#16a34a', '#d97706', '#9333ea', 
				'#0891b2', '#ea580c', '#4f46e5', '#059669', '#c026d3', 
				'#db2777', '#0284c7', '#7c3aed', '#b45309', '#0d9488', '#dc2626'
			];

			for (var i = 0; i < elements.length; i++) {
				var el = elements[i];
				var rect = el.getBoundingClientRect();
				var style = window.getComputedStyle(el);
				if (rect.width > 4 && rect.height > 4 && style.visibility !== 'hidden' && style.display !== 'none' && rect.bottom > 0 && rect.top < window.innerHeight * 3) {
					var id = idCounter++;
					el.setAttribute('data-som-id', id);

					var badge = document.createElement('div');
					badge.className = 'onellama-som-badge';
					badge.textContent = '#' + id;
					badge.style.position = 'absolute';
					badge.style.left = (window.scrollX + rect.left) + 'px';
					badge.style.top = (window.scrollY + rect.top) + 'px';
					var color = colors[(id - 1) % colors.length];
					badge.style.background = color;
					badge.style.color = '#ffffff';
					badge.style.fontSize = '12px';
					badge.style.fontWeight = '800';
					badge.style.padding = '2px 5px';
					badge.style.borderRadius = '4px';
					badge.style.border = '1px solid rgba(255,255,255,0.7)';
					badge.style.boxShadow = '0 2px 4px rgba(0,0,0,0.35)';
					badge.style.zIndex = '999999';
					badge.style.pointerEvents = 'none';
					document.body.appendChild(badge);

					var tag = el.tagName.toLowerCase();
					var text = (el.innerText || el.value || el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.getAttribute('title') || '').trim().replace(/\s+/g, ' ');
					if (text.length > 50) text = text.substring(0, 47) + '...';

					var href = el.getAttribute('href') || '';
					var typeAttr = el.getAttribute('type') || '';
					var placeholder = el.getAttribute('placeholder') || '';

					var desc = '[#' + id + '] <' + tag;
					if (typeAttr) desc += ' type="' + typeAttr + '"';
					if (placeholder) desc += ' placeholder="' + placeholder + '"';
					if (href && href !== '#' && !href.startsWith('javascript:')) desc += ' href="' + href + '"';
					desc += '>';
					if (text) desc += ' "' + text + '"';

					marks.push(desc);
					if (marks.length >= 60) break;
				}
			}

			// Automatically remove badges after 7 seconds if agent is idle
			window.__onellama_som_timer = setTimeout(function() {
				document.querySelectorAll('.onellama-som-badge').forEach(function(b) { b.remove(); });
			}, 7000);

			var title = document.title || "";
			var currentUrl = window.location.href;
			var rawBody = document.body ? document.body.innerText : "";
			var lines = rawBody.split('\n').map(function(l){ return l.trim(); }).filter(function(l){ return l.length > 0; }).slice(0, 40);

			return JSON.stringify({
				title: title,
				url: currentUrl,
				marks: marks,
				summary: lines.join('\n')
			});
		} catch(err) {
			return JSON.stringify({error: err.toString()});
		}
	})()`

	var res string
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		res, err = evaluateInTab(ctx, wsURL, js)
		if err == nil {
			break
		}
		time.Sleep(700 * time.Millisecond)
		_, newWsURL, tabErr := getActiveTab(ctx)
		if tabErr == nil && newWsURL != "" {
			wsURL = newWsURL
		}
	}
	if err != nil {
		return fmt.Sprintf("Navigation in progress or page loaded: %v", err), nil
	}

	var data struct {
		Title   string   `json:"title"`
		URL     string   `json:"url"`
		Marks   []string `json:"marks"`
		Summary string   `json:"summary"`
		Error   string   `json:"error"`
	}
	_ = json.Unmarshal([]byte(res), &data)

	if data.Error != "" {
		return fmt.Sprintf("Error extracting page content: %s", data.Error), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CURRENT URL: %s\nPAGE TITLE: %s\n\n", data.URL, data.Title))

	if len(data.Marks) > 0 {
		sb.WriteString("INTERACTIVE ELEMENTS (Set-of-Marks):\n")
		for _, m := range data.Marks {
			sb.WriteString(m + "\n")
		}
		sb.WriteString("\n")
	}

	if data.Summary != "" {
		sb.WriteString("PAGE CONTENT SUMMARY:\n")
		sb.WriteString(data.Summary + "\n\n")
	}

	sb.WriteString("AVAILABLE NEXT ACTIONS:\n")
	sb.WriteString("- Click element: action 'click' with 'markId' (e.g. markId: 3) or 'text'\n")
	sb.WriteString("- Type in search/field: action 'type' with 'markId' (e.g. markId: 1) and 'text'\n")
	sb.WriteString("- Scroll down: action 'scroll'\n")
	sb.WriteString("- Direct navigation: action 'navigate' with 'url'\n")

	return sb.String(), nil
}

func (b *BrowserVMTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	action, _ := args["action"].(string)
	targetURL, _ := args["url"].(string)
	markIdRaw, _ := args["markId"].(float64)
	markId := int(markIdRaw)
	if markId == 0 {
		if idInt, ok := args["markId"].(int); ok {
			markId = idInt
		}
	}
	text, _ := args["text"].(string)
	selector, _ := args["selector"].(string)
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	durationRaw, _ := args["duration"].(float64)

	_ = b.EnsureVMRunning()

	b.mu.Lock()
	if targetURL != "" {
		b.lastURL = targetURL
	}
	b.mu.Unlock()

	switch action {
	case "handoff", "take_control":
		ResizeBrowserWindow("full")
		msg := "Desktop handoff triggered: User has taken control of the browser virtual machine."
		return msg, msg, nil

	case "return_control":
		ResizeBrowserWindow("pip")
		msg := "Control returned to agent: Browser VM window locked into background preview."
		return msg, msg, nil

	case "navigate":
		if targetURL == "" {
			targetURL = "https://www.google.com"
		} else if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
			if strings.Contains(targetURL, ".") && !strings.Contains(targetURL, " ") {
				targetURL = "https://" + targetURL
			} else {
				targetURL = "https://www.google.com/search?q=" + url.QueryEscape(targetURL)
			}
		}

		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}

		tabID, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}

		expr := fmt.Sprintf(`window.location.href = %q;`, targetURL)
		_, _ = evaluateInTab(ctx, wsURL, expr)

		// Activate tab
		client := &http.Client{Timeout: 5 * time.Second}
		actURL := fmt.Sprintf("http://127.0.0.1:9222/json/activate/%s", tabID)
		if actReq, err := http.NewRequestWithContext(ctx, http.MethodPost, actURL, nil); err == nil {
			if actResp, err := client.Do(actReq); err == nil {
				actResp.Body.Close()
			}
		}

		time.Sleep(2500 * time.Millisecond)

		pageContent, evalErr := extractPageContentWithSoM(ctx, wsURL)
		if evalErr != nil {
			return nil, "", fmt.Errorf("failed to inspect page: %w", evalErr)
		}

		return pageContent, pageContent, nil

	case "click":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}

		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}

		clickJS := fmt.Sprintf(`(function() {
			var markId = %d;
			var selector = %q;
			var text = %q;
			var el = null;
			if (markId > 0) {
				el = document.querySelector('[data-som-id="' + markId + '"]');
			}
			if (!el && selector) {
				try { el = document.querySelector(selector); } catch(e) {}
			}
			if (!el && text) {
				var all = document.querySelectorAll('a, button, input, span, div, h1, h2, h3, h4, p');
				for (var i = 0; i < all.length; i++) {
					if (all[i].innerText && all[i].innerText.trim().toLowerCase() === text.toLowerCase()) {
						el = all[i];
						break;
					}
				}
			}
			if (el) {
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.focus();
				el.click();
				return "OK";
			}
			return "NOT_FOUND";
		})()`, markId, selector, text)

		res, _ := evaluateInTab(ctx, wsURL, clickJS)
		time.Sleep(2500 * time.Millisecond)

		_, wsURL, _ = getActiveTab(ctx)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Click result (%s):\n\n%s", res, pageContent)
		return msg, msg, nil

	case "type":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}

		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}

		typeJS := fmt.Sprintf(`(function() {
			var markId = %d;
			var selector = %q;
			var text = %q;
			var el = null;
			if (markId > 0) {
				el = document.querySelector('[data-som-id="' + markId + '"]');
			}
			if (!el && selector) {
				try { el = document.querySelector(selector); } catch(e) {}
			}
			if (!el) {
				el = document.querySelector('input[type="text"], input[type="search"], input:not([type="hidden"]), textarea');
			}
			if (el) {
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.focus();
				var proto = el instanceof HTMLTextAreaElement ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
				var descriptor = Object.getOwnPropertyDescriptor(proto, 'value');
				if (descriptor && descriptor.set) {
					descriptor.set.call(el, text);
				} else {
					el.value = text;
				}
				el.dispatchEvent(new Event('input', {bubbles: true}));
				el.dispatchEvent(new Event('change', {bubbles: true}));
				if (el.form) {
					if (typeof el.form.requestSubmit === 'function') {
						el.form.requestSubmit();
					} else {
						el.form.submit();
					}
				}
				el.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true}));
				el.dispatchEvent(new KeyboardEvent('keypress', {key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true}));
				el.dispatchEvent(new KeyboardEvent('keyup', {key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true}));
				return "OK";
			}
			return "NOT_FOUND";
		})()`, markId, selector, text)

		res, _ := evaluateInTab(ctx, wsURL, typeJS)
		time.Sleep(2500 * time.Millisecond)

		_, wsURL, _ = getActiveTab(ctx)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Type result (%s):\n\n%s", res, pageContent)
		return msg, msg, nil

	case "scroll":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}

		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}

		_, _ = evaluateInTab(ctx, wsURL, `window.scrollBy(0, 600);`)
		time.Sleep(1000 * time.Millisecond)

		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		return pageContent, pageContent, nil

	case "evaluate":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		script, _ := args["script"].(string)
		if script == "" {
			script, _ = args["expression"].(string)
		}
		res, err := evaluateInTab(ctx, wsURL, script)
		if err != nil {
			return nil, "", fmt.Errorf("evaluate failed: %w", err)
		}
		return res, fmt.Sprintf("Evaluation result:\n%v", res), nil

	case "hover":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}

		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}

		hoverJS := fmt.Sprintf(`(function() {
			var markId = %d;
			var selector = %q;
			var el = null;
			if (markId > 0) {
				el = document.querySelector('[data-som-id="' + markId + '"]');
			}
			if (!el && selector) {
				try { el = document.querySelector(selector); } catch(e) {}
			}
			if (el) {
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.focus();
				el.dispatchEvent(new MouseEvent('mouseover', {bubbles: true}));
				el.dispatchEvent(new MouseEvent('mouseenter', {bubbles: true}));
				return "OK";
			}
			return "NOT_FOUND";
		})()`, markId, selector)

		res, _ := evaluateInTab(ctx, wsURL, hoverJS)
		time.Sleep(1500 * time.Millisecond)

		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Hover result (%s):\n\n%s", res, pageContent)
		return msg, msg, nil

	case "screenshot":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Screenshot logic overriden, returning SoM DOM representation:\n\n%s", pageContent)
		return msg, msg, nil

	case "back":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		_, _ = evaluateInTab(ctx, wsURL, `window.history.back();`)
		time.Sleep(2000 * time.Millisecond)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		return pageContent, pageContent, nil

	case "select":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		selectJS := fmt.Sprintf(`(function() {
			var markId = %d;
			var selector = %q;
			var val = %q;
			var el = null;
			if (markId > 0) { el = document.querySelector('[data-som-id="' + markId + '"]'); }
			if (!el && selector) { try { el = document.querySelector(selector); } catch(e) {} }
			if (!el) { el = document.querySelector('select'); }
			if (el) {
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.focus();
				el.value = val;
				for (var i = 0; i < el.options.length; i++) {
					if (el.options[i].value === val || el.options[i].text.trim().toLowerCase() === val.toLowerCase()) {
						el.selectedIndex = i;
						break;
					}
				}
				el.dispatchEvent(new Event('input', {bubbles: true}));
				el.dispatchEvent(new Event('change', {bubbles: true}));
				return "OK";
			}
			return "NOT_FOUND";
		})()`, markId, selector, value)
		res, _ := evaluateInTab(ctx, wsURL, selectJS)
		time.Sleep(1500 * time.Millisecond)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Select result (%s):\n\n%s", res, pageContent)
		return msg, msg, nil

	case "press":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		if key == "" {
			key = "Enter"
		}
		pressJS := fmt.Sprintf(`(function() {
			var markId = %d;
			var selector = %q;
			var k = %q;
			var el = document.activeElement || document.body;
			if (markId > 0) {
				var target = document.querySelector('[data-som-id="' + markId + '"]');
				if (target) el = target;
			} else if (selector) {
				try { var target = document.querySelector(selector); if (target) el = target; } catch(e) {}
			}
			el.focus();
			el.dispatchEvent(new KeyboardEvent('keydown', {key: k, bubbles: true}));
			el.dispatchEvent(new KeyboardEvent('keyup', {key: k, bubbles: true}));
			return "OK";
		})()`, markId, selector, key)
		res, _ := evaluateInTab(ctx, wsURL, pressJS)
		time.Sleep(1500 * time.Millisecond)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Press key result (%s):\n\n%s", res, pageContent)
		return msg, msg, nil

	case "drag":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		dragJS := fmt.Sprintf(`(function() {
			var markId = %d;
			var selector = %q;
			var el = null;
			if (markId > 0) { el = document.querySelector('[data-som-id="' + markId + '"]'); }
			if (!el && selector) { try { el = document.querySelector(selector); } catch(e) {} }
			if (el) {
				el.dispatchEvent(new MouseEvent('mousedown', {bubbles: true}));
				el.dispatchEvent(new MouseEvent('mousemove', {bubbles: true, clientX: 100, clientY: 100}));
				el.dispatchEvent(new MouseEvent('mouseup', {bubbles: true}));
				return "OK";
			}
			return "NOT_FOUND";
		})()`, markId, selector)
		res, _ := evaluateInTab(ctx, wsURL, dragJS)
		time.Sleep(1500 * time.Millisecond)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		msg := fmt.Sprintf("Drag result (%s):\n\n%s", res, pageContent)
		return msg, msg, nil

	case "snapshot":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		pageContent, evalErr := extractPageContentWithSoM(ctx, wsURL)
		if evalErr != nil {
			return nil, "", fmt.Errorf("failed to get snapshot: %w", evalErr)
		}
		return pageContent, pageContent, nil

	case "wait":
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, err := getActiveTab(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get browser tab: %w", err)
		}
		durationMs := 2000
		if durationRaw > 0 {
			durationMs = int(durationRaw)
		}
		time.Sleep(time.Duration(durationMs) * time.Millisecond)
		pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
		return pageContent, pageContent, nil

	case "interact_mark":
		if text != "" {
			return b.Execute(ctx, map[string]any{
				"action": "type",
				"markId": markId,
				"text":   text,
			})
		}
		return b.Execute(ctx, map[string]any{
			"action": "click",
			"markId": markId,
		})

	default:
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		_, wsURL, _ := getActiveTab(ctx)
		if wsURL != "" {
			pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
			return pageContent, pageContent, nil
		}
		msg := fmt.Sprintf("Executed browser action '%s' successfully in VM browser.", action)
		return msg, msg, nil
	}
}
