//go:build !windows

package tools

func ShowPiPOverlayWindow(x, y, w, h int) {
	// No-op on non-windows platforms
}

func ShowExpandedOverlayWindow(x, y, w, h int) {
	// No-op on non-windows platforms
}

func HideOverlayWindow() {
	// No-op on non-windows platforms
}
