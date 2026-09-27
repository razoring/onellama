package tools

import (
	"runtime"
	"strings"
	"testing"
)

func TestBrowserVMArgsTier1(t *testing.T) {
	vm := &BrowserVMTool{
		qemuExe:    "qemu-system-x86_64",
		kernelPath: "/path/to/vmlinuz",
		initrdPath: "/path/to/initramfs",
		imgPath:    "/path/to/virium.qcow2",
	}

	args := vm.buildQEMUArgs(1, 4444)
	argsStr := strings.Join(args, " ")

	// Check core hardware args
	if !strings.Contains(argsStr, "-m 1024M,maxmem=2048M") {
		t.Errorf("Expected 1024M base RAM, got args: %s", argsStr)
	}

	if !strings.Contains(argsStr, "-smp 2,cores=2,threads=1,sockets=1") {
		t.Errorf("Expected 2 vCPUs, got args: %s", argsStr)
	}

	// Check VirtIO drivers
	requiredDevices := []string{
		"virtio-gpu-pci,xres=1280,yres=800",
		"virtio-tablet-pci",
		"virtio-keyboard-pci",
		"virtio-balloon-pci",
	}
	for _, dev := range requiredDevices {
		if !strings.Contains(argsStr, dev) {
			t.Errorf("Expected device %s in Tier 1 args, got: %s", dev, argsStr)
		}
	}

	// Check OS native hypervisor tier 1 acceleration
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(argsStr, "whpx") {
			t.Errorf("Expected WHPX accelerator on Windows Tier 1, got: %s", argsStr)
		}
	case "linux":
		if !strings.Contains(argsStr, "kvm") {
			t.Errorf("Expected KVM accelerator on Linux Tier 1, got: %s", argsStr)
		}
	case "darwin":
		if !strings.Contains(argsStr, "hvf") {
			t.Errorf("Expected HVF accelerator on macOS Tier 1, got: %s", argsStr)
		}
	}
}

func TestBrowserVMArgsNoDelay(t *testing.T) {
	vm := &BrowserVMTool{
		qemuExe:    "qemu-system-x86_64",
		kernelPath: "/path/to/vmlinuz",
		initrdPath: "/path/to/initramfs",
		imgPath:    "/path/to/virium.qcow2",
	}

	argsStr := strings.Join(vm.buildQEMUArgs(1, 4444), " ")
	if strings.Contains(argsStr, "rootdelay=") {
		t.Errorf("Expected no rootdelay in kernel cmdline for sub-second boot, got: %s", argsStr)
	}
}

func TestBrowserVMArgsTier2And3(t *testing.T) {
	vm := &BrowserVMTool{
		qemuExe:    "qemu-system-x86_64",
		kernelPath: "/path/to/vmlinuz",
		initrdPath: "/path/to/initramfs",
		imgPath:    "/path/to/virium.qcow2",
	}

	tier2Args := strings.Join(vm.buildQEMUArgs(2, 4444), " ")
	if !strings.Contains(tier2Args, "virtio-vga") {
		t.Errorf("Expected virtio-vga in Tier 2 fallback, got: %s", tier2Args)
	}

	tier3Args := strings.Join(vm.buildQEMUArgs(3, 4444), " ")
	if !strings.Contains(tier3Args, "usb-tablet") {
		t.Errorf("Expected usb-tablet in Tier 3 fallback, got: %s", tier3Args)
	}
}

func TestQMPClientState(t *testing.T) {
	qmp := &qmpClient{addr: "127.0.0.1:4444"}

	if qmp.IsPaused() {
		t.Errorf("Expected initial state unpaused, got paused")
	}

	// Simulate state tracking
	qmp.paused = true
	if !qmp.IsPaused() {
		t.Errorf("Expected state paused")
	}
}

func TestDemoteProcessPriorityNil(t *testing.T) {
	// Must not panic on nil command
	demoteProcessPriority(nil)
}
