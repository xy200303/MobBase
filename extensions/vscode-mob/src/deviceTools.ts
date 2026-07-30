import * as vscode from "vscode";
import { MobClient } from "./mobClient";
import { DeviceTreeItem, MobDevice } from "./views";
import { showMobError } from "./workflow";

interface DeviceListData {
  devices: MobDevice[];
}

interface ScreenshotData {
  path?: string;
}

interface UIHierarchyNode {
  class?: string;
  text?: string;
  resourceId?: string;
  contentDesc?: string;
  bounds?: string;
  enabled?: boolean;
  clickable?: boolean;
  children?: UIHierarchyNode[];
}

interface UIHierarchyData {
  device?: MobDevice;
  nodes?: UIHierarchyNode[];
}

export async function captureDeviceScreenshot(client: MobClient, item?: DeviceTreeItem): Promise<void> {
  const device = await selectReadyAndroidDevice(client, item?.device);
  if (!device) {
    return;
  }
  try {
    const result = await client.query<ScreenshotData>(["device", "screenshot", device.id]);
    const screenshot = result.data?.path;
    if (!screenshot) {
      throw new Error("Mob did not return a screenshot path.");
    }
    await vscode.commands.executeCommand("vscode.open", vscode.Uri.file(screenshot));
  } catch (error) {
    showMobError(error);
  }
}

export async function inspectDeviceUI(client: MobClient, item?: DeviceTreeItem): Promise<void> {
  const device = await selectReadyAndroidDevice(client, item?.device);
  if (!device) {
    return;
  }
  try {
    const result = await client.query<UIHierarchyData>(["device", "ui-tree", "--device", device.id]);
    showUIHierarchy(result.data?.device ?? device, result.data?.nodes ?? []);
  } catch (error) {
    showMobError(error);
  }
}

export async function selectReadyAndroidDevice(client: MobClient, requested?: MobDevice): Promise<MobDevice | undefined> {
  if (requested && requested.id !== "none") {
    if (requested.platform !== "android" || requested.state !== "ready") {
      vscode.window.showWarningMessage(`Mob device ${requested.name || requested.id} is not ready.`);
      return undefined;
    }
    return requested;
  }
  try {
    const result = await client.query<DeviceListData>(["device", "list", "--platform", "android"]);
    const devices = (result.data?.devices ?? []).filter((device) => device.platform === "android" && device.state === "ready");
    if (devices.length === 0) {
      vscode.window.showWarningMessage("No ready Android device is available.");
      return undefined;
    }
    if (devices.length === 1) {
      return devices[0];
    }
    const selected = await vscode.window.showQuickPick(
      devices.map((device) => ({ label: device.name || device.id, description: `${device.kind} · ${device.id}`, device })),
      { title: "Android device", placeHolder: "Choose a ready Android device" },
    );
    return selected?.device;
  } catch (error) {
    showMobError(error);
    return undefined;
  }
}

function showUIHierarchy(device: MobDevice, nodes: UIHierarchyNode[]): void {
  const panel = vscode.window.createWebviewPanel(
    "mob.uiInspector",
    `Mob UI: ${device.name || device.id}`,
    vscode.ViewColumn.Beside,
    { enableScripts: false, retainContextWhenHidden: false },
  );
  panel.webview.html = uiHierarchyHtml(device, nodes);
}

function uiHierarchyHtml(device: MobDevice, nodes: UIHierarchyNode[]): string {
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body { color: var(--vscode-foreground); background: var(--vscode-editor-background); font-family: var(--vscode-font-family); font-size: var(--vscode-font-size); margin: 16px; }
h1 { font-size: 16px; font-weight: 600; margin: 0 0 12px; }
ul { list-style: none; margin: 0; padding-left: 16px; border-left: 1px solid var(--vscode-widget-border); }
li { margin: 6px 0; }
.node { font-family: var(--vscode-editor-font-family); line-height: 1.4; }
.class { color: var(--vscode-symbolIcon-classForeground); }
.meta { color: var(--vscode-descriptionForeground); margin-left: 8px; }
</style></head><body><h1>${escapeHtml(device.name || device.id)}</h1>${renderNodes(nodes)}</body></html>`;
}

function renderNodes(nodes: UIHierarchyNode[]): string {
  if (nodes.length === 0) {
    return "<p>No UI nodes were returned.</p>";
  }
  return `<ul>${nodes.map(renderNode).join("")}</ul>`;
}

function renderNode(node: UIHierarchyNode): string {
  const title = node.class || "node";
  const details = [
    node.resourceId ? `id=${node.resourceId}` : "",
    node.text ? `text=${node.text}` : "",
    node.contentDesc ? `desc=${node.contentDesc}` : "",
    node.bounds ? node.bounds : "",
    node.clickable ? "clickable" : "",
    node.enabled === false ? "disabled" : "",
  ].filter(Boolean).join(" · ");
  return `<li><span class="node"><span class="class">${escapeHtml(title)}</span>${details ? `<span class="meta">${escapeHtml(details)}</span>` : ""}</span>${node.children?.length ? renderNodes(node.children) : ""}</li>`;
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" })[character] ?? character);
}
