#!/bin/sh
set -e

echo "[1/5] Installing build tools..."
apk update
apk add e2fsprogs qemu-img curl tar

ROOTFS="/tmp/rootfs"
mkdir -p "$ROOTFS"

echo "[2/5] Setting up Alpine Linux rootfs..."
curl -sL https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.0-x86_64.tar.gz | tar -xz -C "$ROOTFS"

# Copy resolv.conf for chroot network access
cp /etc/resolv.conf "$ROOTFS/etc/resolv.conf"

echo "[3/5] Installing Chromium, Xvfb, Linux kernel, and dependencies into rootfs..."
chroot "$ROOTFS" /bin/sh -c "
  apk update &&
  apk add --no-cache openrc linux-virt chromium xorg-server xf86-video-fbdev xf86-video-vesa xf86-input-libinput xf86-input-evdev font-noto mesa-gl dbus eudev eudev-openrc socat
"

# Configure Xorg for fbdev
mkdir -p "$ROOTFS/etc/X11"
cat << 'XCONF' > "$ROOTFS/etc/X11/xorg.conf"
Section "Device"
    Identifier "Card0"
    Driver "fbdev"
EndSection
Section "Screen"
    Identifier "Screen0"
    Device "Card0"
    DefaultDepth 24
    SubSection "Display"
        Depth 24
        Modes "1280x800"
    EndSubSection
EndSection
XCONF

# Configure root autologin and networking
echo 'root::19000:0:99999:7:::' > "$ROOTFS/etc/shadow"
echo 'ttyS0::respawn:/bin/sh' >> "$ROOTFS/etc/inittab"
cat << 'NET' > "$ROOTFS/etc/network/interfaces"
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
NET

# Create startup script for Chromium remote debugging on 9222
mkdir -p "$ROOTFS/etc/local.d"
cat << 'START' > "$ROOTFS/etc/local.d/chromium.start"
#!/bin/sh
exec > /dev/console 2>&1
echo "=== CHROMIUM START SCRIPT INITIALIZING ==="
Xorg :0 -ac -nolisten tcp vt1 &
export DISPLAY=:0
sleep 2

# Forward incoming port 9222 to local Chromium 9223
socat TCP-LISTEN:9222,bind=0.0.0.0,fork,reuseaddr TCP:127.0.0.1:9223 &

echo "=== STARTING CHROMIUM ==="
chromium-browser \
  --remote-debugging-port=9223 \
  --remote-allow-origins=* \
  --no-sandbox \
  --disable-dev-shm-usage \
  --disable-gpu \
  --window-size=1280,800 \
  --window-position=0,0 \
  --start-maximized \
  --no-first-run \
  --no-default-browser-check \
  --user-data-dir=/tmp/chromium-data \
  https://www.google.com &
START
chmod +x "$ROOTFS/etc/local.d/chromium.start"

# Enable OpenRC services
chroot "$ROOTFS" /bin/sh -c "
  rc-update add udev sysinit
  rc-update add udev-trigger sysinit
  rc-update add networking boot
  rc-update add local default
"

echo "[4/5] Copying kernel and initramfs to output..."
cp "$ROOTFS/boot/vmlinuz-virt" /output/vmlinuz-virt
cp "$ROOTFS/boot/initramfs-virt" /output/initramfs-virt

echo "[5/5] Packaging rootfs into virium-base.qcow2..."
truncate -s 1500M /tmp/rootfs.raw
mke2fs -F -t ext4 -d "$ROOTFS" /tmp/rootfs.raw
qemu-img convert -f raw -O qcow2 -c /tmp/rootfs.raw /output/virium-base.qcow2
rm -f /tmp/rootfs.raw

echo "=== VM Image Build Complete! ==="
ls -lh /output/
