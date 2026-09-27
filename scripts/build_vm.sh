#!/bin/sh
set -e

echo "[1/6] Installing build dependencies..."
apk update
apk add e2fsprogs qemu-img curl tar

ROOTFS="/tmp/rootfs"
rm -rf "$ROOTFS"
mkdir -p "$ROOTFS"

echo "[2/6] Extracting Alpine Linux minirootfs..."
curl -sL https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.0-x86_64.tar.gz | tar -xz -C "$ROOTFS"

cp /etc/resolv.conf "$ROOTFS/etc/resolv.conf"

echo "[3/6] Installing Cage, Wayland, Mesa DRM, Chromium, and kernel..."
chroot "$ROOTFS" /bin/sh -c "
  apk update &&
  apk add --no-cache \
    openrc \
    linux-virt \
    chromium \
    cage \
    seatd \
    mesa-dri-gallium \
    mesa-gbm \
    libinput \
    font-noto \
    socat
"

# Configure root autologin and networking
echo 'root::19000:0:99999:7:::' > "$ROOTFS/etc/shadow"
echo 'ttyS0::respawn:/bin/sh' >> "$ROOTFS/etc/inittab"
cat << 'NET' > "$ROOTFS/etc/network/interfaces"
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
NET

# Create persistent Chromium profile dir
mkdir -p "$ROOTFS/root/.config/chromium"

# Create startup script for Wayland Cage Kiosk
mkdir -p "$ROOTFS/etc/local.d"
cat << 'START' > "$ROOTFS/etc/local.d/kiosk.start"
#!/bin/sh
exec > /dev/console 2>&1
echo "=== ONELLAMA WAYLAND KIOSK STARTING ==="

# Mount filesystems
mkdir -p /dev/pts /dev/shm /tmp/run
mount -t devpts devpts /dev/pts 2>/dev/null || true
mount -t tmpfs tmpfs /dev/shm 2>/dev/null || true
chmod 1777 /dev/shm

# Start seat daemon for DRM/KMS card access
seatd -g video &
sleep 0.5

# Forward external 9222 to internal Chromium CDP 9223
socat TCP-LISTEN:9222,bind=0.0.0.0,fork,reuseaddr TCP:127.0.0.1:9223 &

# Export Wayland and seatd environment
export XDG_RUNTIME_DIR=/tmp/run
chmod 0700 "$XDG_RUNTIME_DIR"
export WLR_BACKENDS=drm,libinput
export LIBSEAT_BACKEND=seatd
export XDG_CURRENT_DESKTOP=Wayland

echo "=== LAUNCHING CHROMIUM VIA CAGE ==="
exec cage -s -- chromium-browser \
  --ozone-platform=wayland \
  --enable-features=UseOzonePlatform,WaylandWindowDecorations \
  --remote-debugging-port=9223 \
  --remote-allow-origins=* \
  --no-sandbox \
  --disable-dev-shm-usage \
  --disable-infobars \
  --disable-session-crashed-bubble \
  --window-size=1280,800 \
  --window-position=0,0 \
  --start-maximized \
  --no-first-run \
  --no-default-browser-check \
  --user-data-dir=/root/.config/chromium \
  https://www.google.com &
START
chmod +x "$ROOTFS/etc/local.d/kiosk.start"

# Enable essential services
chroot "$ROOTFS" /bin/sh -c "
  rc-update add networking boot
  rc-update add local default
"

echo "[4/6] Cleaning up unneeded files to shrink disk image..."
rm -rf "$ROOTFS/usr/share/man" "$ROOTFS/usr/share/doc" "$ROOTFS/var/cache/apk"/*

echo "[5/6] Exporting kernel and initramfs..."
mkdir -p /output
cp "$ROOTFS/boot/vmlinuz-virt" /output/vmlinuz-virt
cp "$ROOTFS/boot/initramfs-virt" /output/initramfs-virt

echo "[6/6] Packaging persistent rootfs into virium-base.qcow2..."
truncate -s 800M /tmp/rootfs.raw
mke2fs -F -t ext4 -d "$ROOTFS" /tmp/rootfs.raw
qemu-img convert -f raw -O qcow2 -c /tmp/rootfs.raw /output/virium-base.qcow2
rm -f /tmp/rootfs.raw

echo "=== VM Image Build Complete ==="
ls -lh /output/
