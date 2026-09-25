import { spawn, ChildProcess } from 'child_process';
import * as net from 'net';

export interface VMConfig {
  imagePath: string;
  ramMB?: number;
  cpuCores?: number;
  cdpPort?: number;
  qmpPort?: number;
}

export class QemuController {
  private process: ChildProcess | null = null;
  private qmpSocket: net.Socket | null = null;
  private qmpCallbacks: Map<number, (res: any) => void> = new Map();
  private qmpSeq = 0;
  private qmpBuffer = '';
  private config: Required<VMConfig>;
  public isUnlocked = false;

  constructor(config: VMConfig) {
    this.config = {
      imagePath: config.imagePath,
      ramMB: config.ramMB || 2048,
      cpuCores: config.cpuCores || 2,
      cdpPort: config.cdpPort || 9222,
      qmpPort: config.qmpPort || 4444,
    };
  }

  public async startHiddenBackground(): Promise<void> {
    this.isUnlocked = false;
    await this.launchQemu(['-display', 'sdl,show-cursor=on']);
    await this.setWindowState('hide');
  }

  public async setWindowState(state: 'hide' | 'show' | 'topmost'): Promise<void> {
    if (process.platform === 'win32') {
      const psScript = `
        $code = @"
        [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
        [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr hWnd, IntPtr hWndInsertAfter, int X, int Y, int cx, int cy, uint uFlags);
        "@
        $type = Add-Type -MemberDefinition $code -Name "Win32Utils" -Namespace "VM" -PassThru
        $proc = Get-Process qemu-system-x86_64 -ErrorAction SilentlyContinue
        if ($proc) {
          if ('${state}' -eq 'hide') { $type::ShowWindow($proc.MainWindowHandle, 0) }
          if ('${state}' -eq 'show') { $type::ShowWindow($proc.MainWindowHandle, 5) }
          if ('${state}' -eq 'topmost') { $type::SetWindowPos($proc.MainWindowHandle, [IntPtr](-1), 0, 0, 0, 0, 0x0003) }
        }
      `;
      const child = spawn('powershell', ['-Command', psScript.replace(/\n/g, ' ')]);
      await new Promise<void>((resolve) => child.on('exit', () => resolve()));
    }
  }

  public async openBrowserPreview(): Promise<void> {
    await this.sendQmpCommand('human-monitor-command', { 'command-line': 'xres 480 300' }).catch(() => {});
    await this.setWindowState('show');
    await this.setWindowState('topmost');
  }

  public async unlockAndResizeFull(): Promise<void> {
    this.isUnlocked = true;
    await this.sendQmpCommand('human-monitor-command', { 'command-line': 'xres 1280 800' }).catch(() => {});
  }

  public async lockAndResizeSmall(): Promise<void> {
    this.isUnlocked = false;
    await this.sendQmpCommand('human-monitor-command', { 'command-line': 'xres 480 300' }).catch(() => {});
  }

  public async closeBrowserPreview(): Promise<void> {
    this.isUnlocked = false;
    await this.setWindowState('hide');
  }

  private async launchQemu(displayArgs: string[]): Promise<void> {
    const isWindows = process.platform === 'win32';
    const accel = isWindows ? 'whpx' : 'kvm';
    const qemuBin = isWindows ? 'qemu-system-x86_64.exe' : 'qemu-system-x86_64';

    const args = [
      '-m', `${this.config.ramMB}M`,
      '-smp', `${this.config.cpuCores}`,
      '-accel', accel,
      '-drive', `file=${this.config.imagePath},if=virtio`,
      '-netdev', `user,id=net0,hostfwd=tcp::${this.config.cdpPort}-:9222`,
      '-device', 'virtio-net-pci,netdev=net0',
      '-qmp', `tcp:127.0.0.1:${this.config.qmpPort},server,nowait`,
      '-vga', 'std',
      ...displayArgs
    ];

    this.process = spawn(qemuBin, args, { stdio: 'ignore' });
    await this.connectQmp();
    await this.waitForCDP();
  }

  private async connectQmp(): Promise<void> {
    return new Promise((resolve, reject) => {
      let attempts = 0;
      const tryConnect = () => {
        attempts++;
        if (attempts > 50) return reject(new Error('QMP connection timed out'));
        const sock = net.createConnection({ port: this.config.qmpPort, host: '127.0.0.1' }, () => {
          this.qmpSocket = sock;
          this.setupQmpListener();
          this.sendQmpCommand('qmp_capabilities').then(() => resolve());
        });
        sock.on('error', () => setTimeout(tryConnect, 100));
      };
      tryConnect();
    });
  }

  private setupQmpListener(): void {
    if (!this.qmpSocket) return;
    this.qmpSocket.on('data', (chunk) => {
      this.qmpBuffer += chunk.toString();
      const lines = this.qmpBuffer.split('\r\n');
      this.qmpBuffer = lines.pop() || '';
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const parsed = JSON.parse(line);
          if (parsed.id && this.qmpCallbacks.has(parsed.id)) {
            const cb = this.qmpCallbacks.get(parsed.id)!;
            this.qmpCallbacks.delete(parsed.id);
            cb(parsed);
          }
        } catch (_) {}
      }
    });
  }

  public async sendQmpCommand(execute: string, args: Record<string, any> = {}): Promise<any> {
    const id = ++this.qmpSeq;
    const msg = JSON.stringify({ execute, arguments: args, id }) + '\r\n';
    return new Promise((resolve, reject) => {
      this.qmpCallbacks.set(id, (res) => {
        if (res.error) reject(new Error(res.error.desc || JSON.stringify(res.error)));
        else resolve(res.return);
      });
      this.qmpSocket?.write(msg);
    });
  }

  public async saveSnapshot(name: string): Promise<void> {
    await this.sendQmpCommand('human-monitor-command', { 'command-line': `savevm ${name}` });
  }

  public async restoreSnapshot(name: string): Promise<void> {
    await this.sendQmpCommand('human-monitor-command', { 'command-line': `loadvm ${name}` });
    await this.sendQmpCommand('rtc-reset-reinjection').catch(() => {});
  }

  private async waitForCDP(): Promise<void> {
    for (let i = 0; i < 60; i++) {
      try {
        const res = await fetch(`http://127.0.0.1:${this.config.cdpPort}/json/version`);
        if (res.ok) return;
      } catch (_) {}
      await new Promise((r) => setTimeout(r, 100));
    }
    throw new Error('Chromium CDP failed to initialize');
  }

  public async stop(): Promise<void> {
    if (this.qmpSocket) {
      try { await this.sendQmpCommand('quit'); } catch (_) {}
      this.qmpSocket.destroy();
      this.qmpSocket = null;
    }
    if (this.process) {
      this.process.kill();
      this.process = null;
    }
  }
}
