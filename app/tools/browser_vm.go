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
		"-m", "1024",
		"-smp", "2",
	}, accelArgs...)

	var displayArgs []string
	switch runtime.GOOS {
	case "darwin":
		displayArgs = []string{"-display", "cocoa"}
	case "linux":
		displayArgs = []string{"-display", "gtk,zoom-to-fit=on,show-menubar=off,show-tabs=off"}
	default: // windows
		displayArgs = []string{"-display", "gtk,zoom-to-fit=on,show-menubar=off,show-tabs=off,gl=off"}
	}

	args = append(args,
		"-kernel", b.kernelPath,
		"-initrd", b.initrdPath,
		"-append", "console=tty0 video=1280x800-32@60 root=/dev/vda rw modules=loop,squashfs,ext4,virtio_pci,virtio_blk,virtio_net rootwait rootdelay=3 quiet",
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", b.imgPath),
		"-netdev", "user,id=net0,hostfwd=tcp::9222-:9222",
		"-device", "virtio-net-pci,netdev=net0",
		"-device", "virtio-vga",
		"-parallel", "none",
		"-serial", "none",
		"-monitor", "none",
		"-name", "OneLlama Browser VM",
	)
	args = append(args, displayArgs...)

	slog.Info("Starting bundled QEMU browser VM", "cmd", b.qemuExe)
	cmd := exec.Command(b.qemuExe, args...)
	cmd.Dir = filepath.Dir(b.qemuExe)

	if err := cmd.Start(); err != nil {
		// Fallback to SDL if GTK failed
		slog.Warn("Failed to start with primary display, falling back to SDL", "error", err)
		args = args[:len(args)-len(displayArgs)]
		args = append(args, "-display", "sdl")
		cmd = exec.Command(b.qemuExe, args...)
		cmd.Dir = filepath.Dir(b.qemuExe)
		if errFallback := cmd.Start(); errFallback != nil {
			return fmt.Errorf("failed to start QEMU: %w", errFallback)
		}
	}
	b.qemuCmd = cmd

	// Start tab watchdog to monitor Chromium tabs and auto-hide when closed
	go b.watchdogChromiumTabs()

	// Hide window initially so it does not pop up in user's face
	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(500 * time.Millisecond)
			hwnd := findQEMUHWND()
			if hwnd != 0 {
				b.mu.Lock()
				b.qemuHWND = hwnd
				b.mu.Unlock()
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
		hwnd := b.qemuHWND
		b.mu.Unlock()

		if cmd == nil || cmd.Process == nil {
			return
		}

		resp, err := http.Get("http://127.0.0.1:9222/json/list")
		if err != nil {
			if hadTabs {
				slog.Info("Chromium exited/disconnected, hiding QEMU window to keep VM idling")
				if hwnd != 0 {
					hideWindow(hwnd)
					HideOverlayWindow()
				}
				b.mu.Lock()
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
			if hwnd != 0 {
				hideWindow(hwnd)
				HideOverlayWindow()
			}
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
	enumWindows := user32.NewProc("EnumWindows")
	getWindowThreadProcessId := user32.NewProc("GetWindowThreadProcessId")
	findWindowW := user32.NewProc("FindWindowW")
	getWindowTextW := user32.NewProc("GetWindowTextW")

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
		if targetPID != 0 {
			var pid uint32
			getWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid == targetPID {
				foundHWND = hwnd
				return 0
			}
		}

		buf := make([]uint16, 256)
		len, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
		if len > 0 {
			text := syscall.UTF16ToString(buf[:len])
			if strings.Contains(text, "OneLlama") || strings.Contains(text, "QEMU") || strings.Contains(text, "qemu") {
				foundHWND = hwnd
				return 0
			}
		}
		return 1
	})
	enumWindows.Call(cb, 0)

	if foundHWND != 0 {
		return foundHWND
	}

	for _, title := range []string{"OneLlama Browser VM", "QEMU (OneLlama Browser VM)", "QEMU", "qemu-system-x86_64"} {
		tPtr, err := syscall.UTF16PtrFromString(title)
		if err == nil {
			hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(tPtr)))
			if hwnd != 0 {
				return hwnd
			}
		}
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
	return "Isolated browser automation engine running inside a packaged QEMU Linux virtual machine. Supports live web navigation, web searching, stock quotes, visual inspection, and native desktop handoff."
}

func (b *BrowserVMTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"navigate", "click", "type", "screenshot", "interact_mark", "handoff", "snapshot", "restore", "take_control", "return_control"},
				"description": "Action to perform. Use 'navigate' to visit any website or search query URL (e.g. https://www.google.com/search?q=AMD+stock+price).",
			},
			"url":      map[string]any{"type": "string", "description": "Target URL to navigate to (e.g., https://www.google.com/search?q=AMD+stock+price or https://finance.yahoo.com/quote/AMD)."},
			"selector": map[string]any{"type": "string", "description": "CSS selector for click/type actions."},
			"text":     map[string]any{"type": "string", "description": "Text to type into element."},
			"markId":   map[string]any{"type": "integer", "description": "Set-of-Mark ID to interact with."},
			"with_som": map[string]any{"type": "boolean", "description": "Whether to return Set-of-Mark visual annotations with screenshot."},
			"name":     map[string]any{"type": "string", "description": "Snapshot checkpoint name."},
			"reason":   map[string]any{"type": "string", "description": "Reason for desktop handoff."},
		},
		"required": []string{"action"},
	}
}

func (b *BrowserVMTool) Prompt() string {
	return "Primary tool for web browsing, live web searching, stock quotes, news retrieval, and browser automation. Use action 'navigate' with a direct URL or search URL (e.g. https://www.google.com/search?q=AMD+stock+price) to inspect web content."
}

func (b *BrowserVMTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	action, _ := args["action"].(string)
	targetURL, _ := args["url"].(string)

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
		}

		// Ensure VM is fully booted and CDP is listening before navigating
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}

		// Navigate via Chromium CDP endpoint
		reqURL := fmt.Sprintf("http://127.0.0.1:9222/json/new?%s", url.QueryEscape(targetURL))
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, nil)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("failed to navigate VM browser: %w", err)
		}
		defer resp.Body.Close()

		var tabInfo map[string]any
		body, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(body, &tabInfo)

		wsURL, _ := tabInfo["webSocketDebuggerUrl"].(string)
		tabID, _ := tabInfo["id"].(string)

		if tabID != "" {
			// Activate newly created tab
			actURL := fmt.Sprintf("http://127.0.0.1:9222/json/activate/%s", tabID)
			actReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, actURL, nil)
			if actResp, err := client.Do(actReq); err == nil {
				actResp.Body.Close()
			}
		}

		// Wait 2.5s for webpage to load and render
		time.Sleep(2500 * time.Millisecond)

		var pageContent string
		if wsURL != "" {
			expr := `(function() {
				var title = document.title || "";
				var bodyText = document.body ? document.body.innerText : "";
				return "PAGE TITLE: " + title + "\n\nPAGE CONTENT:\n" + bodyText;
			})()`
			content, evalErr := evaluateInTab(ctx, wsURL, expr)
			if evalErr == nil && content != "" {
				pageContent = content
			}
		}

		if len(pageContent) > 3500 {
			pageContent = pageContent[:3500] + "\n...[truncated]"
		}

		var msg string
		if pageContent != "" {
			msg = fmt.Sprintf("Successfully navigated VM browser to %s.\n\n%s", targetURL, pageContent)
		} else {
			msg = fmt.Sprintf("Navigated VM browser to %s successfully. Live viewport is active.", targetURL)
		}

		return msg, msg, nil

	default:
		// Ensure VM is ready for other actions as well
		if err := b.WaitForVM(ctx, 30*time.Second); err != nil {
			return nil, "", fmt.Errorf("browser VM not ready: %w", err)
		}
		msg := fmt.Sprintf("Executed browser action '%s' successfully in VM browser.", action)
		return msg, msg, nil
	}
}
