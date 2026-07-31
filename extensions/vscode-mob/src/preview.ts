import * as vscode from "vscode";
import { selectReadyAndroidDevice } from "./deviceTools";
import { ActiveMobProcess, MobClient, MobCommandError, MobEvent } from "./mobClient";
import { MobDevice } from "./views";
import { showMobError } from "./workflow";

interface PreviewSessionData {
  device?: MobDevice;
  protocol?: string;
  platform?: string;
  deviceId?: string;
  endpoint?: string;
  token?: string;
  video?: { codec?: string; format?: string };
  controls?: string[];
}

export async function openLivePreview(context: vscode.ExtensionContext, client: MobClient, requested?: MobDevice): Promise<void> {
  const device = await selectReadyAndroidDevice(client, requested);
  if (!device) {
    return;
  }
  const preview = new LivePreview(context, client, device);
  preview.open();
}

class LivePreview implements vscode.Disposable {
  private readonly panel: vscode.WebviewPanel;
  private process: ActiveMobProcess | undefined;
  private session: PreviewSessionData | undefined;
  private webviewReady = false;
  private disposed = false;

  public constructor(
    private readonly context: vscode.ExtensionContext,
    private readonly client: MobClient,
    private readonly device: MobDevice,
  ) {
    this.panel = vscode.window.createWebviewPanel(
      "mob.livePreview",
      `Mob Preview: ${device.name || device.id}`,
      vscode.ViewColumn.Beside,
      { enableScripts: true, retainContextWhenHidden: true },
    );
  }

  public open(): void {
    this.panel.webview.html = previewHtml(this.panel.webview, this.device);
    this.context.subscriptions.push(
      this.panel,
      this.panel.onDidDispose(() => this.dispose()),
      this.panel.webview.onDidReceiveMessage((message: unknown) => this.receiveMessage(message)),
    );
    this.process = this.client.stream(["device", "preview", "serve", this.device.id], (event) => this.handleEvent(event));
    void this.process.completed.then((code) => {
      if (!this.disposed && code !== 0) {
        this.post({ type: "error", message: this.process?.protocolError || `Preview service stopped with exit code ${code ?? "unknown"}.` });
      }
    });
  }

  public dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    this.process?.dispose();
    this.process = undefined;
  }

  private handleEvent(event: MobEvent): void {
    if (event.event === "preview") {
      const data = event.data as PreviewSessionData | undefined;
      if (!data?.endpoint || !data.token || data.protocol !== "mob.device.session.v1" || data.platform !== "android" || data.deviceId !== this.device.id) {
        this.post({ type: "error", message: "Mob returned an invalid preview session." });
        this.process?.dispose();
        return;
      }
      this.session = data;
      this.connectWhenReady();
      return;
    }
    if (event.event === "error") {
      const error = new MobCommandError(event.error ?? { code: "MOB_COMMAND_FAILED", message: "Mob preview failed." });
      this.post({ type: "error", message: error.message });
      showMobError(error);
    }
  }

  private receiveMessage(message: unknown): void {
    if (!message || typeof message !== "object") {
      return;
    }
    const type = (message as { type?: unknown }).type;
    if (type === "ready") {
      this.webviewReady = true;
      this.connectWhenReady();
      return;
    }
    if (type === "openOutput") {
      void vscode.commands.executeCommand("mob.openOutput");
    }
  }

  private connectWhenReady(): void {
    if (!this.webviewReady || !this.session || this.disposed) {
      return;
    }
    const session = this.session;
    if (!session.endpoint || !session.token) {
      return;
    }
    const endpoint = session.endpoint.replace(/^http:/, "ws:");
    this.post({ type: "connect", endpoint, token: session.token, device: this.device.name || this.device.id, controls: session.controls ?? [] });
  }

  private post(message: Record<string, unknown>): void {
    void this.panel.webview.postMessage(message);
  }
}

function previewHtml(webview: vscode.Webview, device: MobDevice): string {
  const nonce = createNonce();
  const label = escapeHtml(device.name || device.id);
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}'; connect-src ws://127.0.0.1:*;">
<style>
:root { color: var(--vscode-foreground); background: var(--vscode-editor-background); font-family: var(--vscode-font-family); }
body { margin: 0; min-height: 100vh; display: grid; grid-template-rows: auto minmax(0, 1fr) auto; overflow: hidden; }
.bar { min-height: 40px; display: flex; align-items: center; gap: 8px; padding: 0 12px; border-bottom: 1px solid var(--vscode-panel-border); }
.title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; font-weight: 600; flex: 1; }
.state { color: var(--vscode-descriptionForeground); font-size: 12px; }
.screen-wrap { min-height: 0; display: grid; place-items: center; overflow: auto; padding: 16px; background: var(--vscode-sideBar-background); }
canvas { display: block; max-width: 100%; max-height: calc(100vh - 132px); aspect-ratio: 9 / 16; background: #111; cursor: crosshair; touch-action: none; box-shadow: 0 1px 8px rgba(0, 0, 0, .35); }
.controls { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 6px; padding: 8px 12px 12px; border-top: 1px solid var(--vscode-panel-border); }
button, input { box-sizing: border-box; min-height: 32px; font: inherit; font-size: 12px; }
button { color: var(--vscode-button-foreground); background: var(--vscode-button-background); border: 1px solid transparent; cursor: pointer; }
button:hover { background: var(--vscode-button-hoverBackground); }
button:focus-visible, input:focus-visible { outline: 2px solid var(--vscode-focusBorder); outline-offset: 1px; }
input { grid-column: span 2; padding: 0 8px; color: var(--vscode-input-foreground); background: var(--vscode-input-background); border: 1px solid var(--vscode-input-border); }
.error { color: var(--vscode-errorForeground); }
</style></head><body>
<header class="bar"><span class="title">${label}</span><span id="state" class="state" aria-live="polite">Starting</span></header>
<main class="screen-wrap"><canvas id="screen" width="360" height="640" aria-label="Android device preview" tabindex="0"></canvas></main>
<footer class="controls"><button id="back" data-control="key" aria-label="Back">Back</button><button id="home" data-control="key" aria-label="Home">Home</button><button id="recent" data-control="key" aria-label="Recent apps">Recent</button><input id="text" data-control="text" type="text" aria-label="Send text to device" placeholder="Send text"><button id="send" data-control="text" aria-label="Send text">Send</button></footer>
<script nonce="${nonce}">
const vscode = acquireVsCodeApi();
const canvas = document.getElementById('screen');
const context = canvas.getContext('2d', { alpha: false });
const state = document.getElementById('state');
const text = document.getElementById('text');
let videoSocket; let controlSocket; let decoder; let pointer; let controls = new Set();
function setState(value, error) { state.textContent = value; state.className = error ? 'state error' : 'state'; }
function closeSockets() { if (controlSocket && controlSocket.readyState === WebSocket.OPEN) controlSocket.send(JSON.stringify({ type: 'close' })); if (videoSocket) videoSocket.close(); if (controlSocket) controlSocket.close(); videoSocket = undefined; controlSocket = undefined; if (decoder) decoder.close(); decoder = undefined; }
function connect(endpoint, token, declaredControls) {
  closeSockets(); setState('Connecting');
  controls = new Set(Array.isArray(declaredControls) ? declaredControls : []);
  document.querySelectorAll('[data-control]').forEach((element) => { element.disabled = !controls.has(element.dataset.control); });
  const query = '?token=' + encodeURIComponent(token);
  videoSocket = new WebSocket(endpoint + '/video' + query); videoSocket.binaryType = 'arraybuffer';
  controlSocket = new WebSocket(endpoint + '/control' + query);
  videoSocket.onopen = () => setState('Streaming');
  videoSocket.onclose = () => setState('Disconnected', true);
  controlSocket.onclose = () => setState('Control disconnected', true);
  controlSocket.onmessage = (event) => { try { const data = JSON.parse(event.data); if (data.type === 'error') setState(data.message, true); } catch {} };
  videoSocket.onmessage = async (event) => {
    if (typeof event.data === 'string') { configure(JSON.parse(event.data)); return; }
    if (!decoder || !(event.data instanceof ArrayBuffer)) return;
    const packet = new Uint8Array(event.data); if (packet.byteLength < 10) return;
    const timestamp = Number(new DataView(packet.buffer, packet.byteOffset + 1, 8).getBigUint64(0));
    try { decoder.decode(new EncodedVideoChunk({ type: packet[0] === 1 ? 'key' : 'delta', timestamp, data: packet.slice(9) })); } catch (error) { setState('Video decode failed', true); }
  };
}
async function configure(config) {
  if (config.type !== 'video-config' || !config.codec || !('VideoDecoder' in window)) { setState('H.264 preview is unavailable in this VS Code runtime', true); return; }
  if (decoder && decoder.state !== 'closed') decoder.close();
  decoder = new VideoDecoder({ output: (frame) => { if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) { canvas.width = frame.displayWidth; canvas.height = frame.displayHeight; } context.drawImage(frame, 0, 0, canvas.width, canvas.height); frame.close(); }, error: () => setState('Video decoder error', true) });
  try { decoder.configure({ codec: config.codec, optimizeForLatency: true }); } catch (error) { setState('This VS Code runtime cannot decode the device H.264 stream', true); decoder.close(); decoder = undefined; }
}
function send(input) { if (controls.has(input.type) && controlSocket && controlSocket.readyState === WebSocket.OPEN) controlSocket.send(JSON.stringify(input)); }
function point(event) { const rect = canvas.getBoundingClientRect(); return { x: Math.max(0, Math.round((event.clientX - rect.left) * canvas.width / rect.width)), y: Math.max(0, Math.round((event.clientY - rect.top) * canvas.height / rect.height)) }; }
canvas.addEventListener('pointerdown', (event) => { canvas.setPointerCapture(event.pointerId); pointer = { id: event.pointerId, start: point(event), at: performance.now() }; });
canvas.addEventListener('pointerup', (event) => { if (!pointer || pointer.id !== event.pointerId) return; const end = point(event); const distance = Math.hypot(end.x - pointer.start.x, end.y - pointer.start.y); if (distance < 12) send({ type: 'tap', x: end.x, y: end.y }); else send({ type: 'swipe', x: pointer.start.x, y: pointer.start.y, x2: end.x, y2: end.y, duration: Math.max(80, Math.min(1000, Math.round(performance.now() - pointer.at))) }); pointer = undefined; });
canvas.addEventListener('pointercancel', () => { pointer = undefined; });
document.getElementById('back').addEventListener('click', () => send({ type: 'key', value: 'KEYCODE_BACK' }));
document.getElementById('home').addEventListener('click', () => send({ type: 'key', value: 'KEYCODE_HOME' }));
document.getElementById('recent').addEventListener('click', () => send({ type: 'key', value: 'KEYCODE_APP_SWITCH' }));
function sendText() { const value = text.value.trim(); if (!value) return; send({ type: 'text', value }); text.value = ''; }
document.getElementById('send').addEventListener('click', sendText); text.addEventListener('keydown', (event) => { if (event.key === 'Enter') { event.preventDefault(); sendText(); } });
window.addEventListener('message', (event) => { const message = event.data; if (message.type === 'connect') connect(message.endpoint, message.token, message.controls); if (message.type === 'error') setState(message.message, true); });
window.addEventListener('unload', closeSockets); vscode.postMessage({ type: 'ready' });
</script></body></html>`;
}

function createNonce(): string {
  const values = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  let nonce = "";
  for (let index = 0; index < 32; index += 1) {
    nonce += values.charAt(Math.floor(Math.random() * values.length));
  }
  return nonce;
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>'"]/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[character] ?? character);
}
