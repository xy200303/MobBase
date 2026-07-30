import * as vscode from "vscode";
import { MobClient } from "./mobClient";

interface DoctorCheck {
  id: string;
  label: string;
  status: "ready" | "missing";
  required: boolean;
  detail?: string;
  fix?: string;
}

interface DoctorData {
  ready: boolean;
  checks: DoctorCheck[];
}

interface AndroidSDK {
  name: string;
  path: string;
  ownership: string;
  current: boolean;
  components: {
    platforms: string[];
    buildTools: string[];
    ndk: string[];
    systemImages: string[];
    platformTools: boolean;
    commandLineTools: boolean;
    emulator: boolean;
  };
}

interface JavaSDK {
  name: string;
  version: number;
  path: string;
  ownership: string;
}

interface FlutterSDK {
  version: string;
  path: string;
}

interface StatusData {
  androidSdks: AndroidSDK[];
  javaSdks: JavaSDK[];
  flutter: { sdks: FlutterSDK[]; currentSdk?: string };
}

export interface MobHealth {
  ready: boolean;
  available: boolean;
}

export interface MobDevice {
  id: string;
  platform: string;
  nativeId: string;
  kind: string;
  name: string;
  state: string;
}

interface DevicesData {
  devices: MobDevice[];
  defaultDevice: string;
}

export class ToolchainsTreeDataProvider implements vscode.TreeDataProvider<ToolchainTreeItem> {
  private readonly changed = new vscode.EventEmitter<void>();
  public readonly onDidChangeTreeData = this.changed.event;
  private checks: DoctorCheck[] = [];
  private status: StatusData = { androidSdks: [], javaSdks: [], flutter: { sdks: [] } };

  public constructor(private readonly client: MobClient) {}

  public async refresh(): Promise<MobHealth> {
    try {
      const [doctor, status] = await Promise.all([
        this.client.query<DoctorData>(["doctor"]),
        this.client.query<StatusData>(["status"]),
      ]);
      this.checks = doctor.data?.checks ?? [];
      this.status = status.data ?? { androidSdks: [], javaSdks: [], flutter: { sdks: [] } };
      this.changed.fire();
      return { ready: doctor.data?.ready ?? false, available: true };
    } catch (error) {
      this.checks = [{ id: "mob", label: "Mob CLI", status: "missing", required: true, detail: errorMessage(error) }];
      this.status = { androidSdks: [], javaSdks: [], flutter: { sdks: [] } };
      this.changed.fire();
      return { ready: false, available: false };
    }
  }

  public getTreeItem(item: ToolchainTreeItem): vscode.TreeItem {
    return item;
  }

  public getChildren(item?: ToolchainTreeItem): ToolchainTreeItem[] {
    if (!item) {
      return [
        new ToolchainGroup("Diagnostics", this.checks.map(checkTreeItem)),
        new ToolchainGroup("Android SDK", this.status.androidSdks.map(androidSDKTreeItem)),
        new ToolchainGroup("JDK", this.status.javaSdks.map(javaTreeItem)),
        new ToolchainGroup("Flutter", this.status.flutter.sdks.map((sdk) => flutterTreeItem(sdk, sdk.version === this.status.flutter.currentSdk))),
      ];
    }
    return item instanceof ToolchainGroup ? item.children : [];
  }
}

export class ToolchainGroup extends vscode.TreeItem {
  public constructor(label: string, public readonly children: ToolchainTreeItem[]) {
    super(label, children.length > 0 ? vscode.TreeItemCollapsibleState.Expanded : vscode.TreeItemCollapsibleState.None);
    this.description = String(children.length);
    this.iconPath = new vscode.ThemeIcon("tools");
  }
}

export class ToolchainLeaf extends vscode.TreeItem {
  public constructor(label: string, description: string, tooltip: string, icon: vscode.ThemeIcon) {
    super(label, vscode.TreeItemCollapsibleState.None);
    this.description = description;
    this.tooltip = tooltip;
    this.iconPath = icon;
  }
}

type ToolchainTreeItem = ToolchainGroup | ToolchainLeaf;

function checkTreeItem(check: DoctorCheck): ToolchainLeaf {
  const item = new ToolchainLeaf(
    check.label,
    check.status === "ready" ? "ready" : check.required ? "required" : "optional",
    [check.detail, check.fix].filter(Boolean).join("\n"),
    new vscode.ThemeIcon(check.status === "ready" ? "pass-filled" : "warning", check.status === "ready" ? new vscode.ThemeColor("testing.iconPassed") : new vscode.ThemeColor("testing.iconFailed")),
  );
  return item;
}

function androidSDKTreeItem(sdk: AndroidSDK): ToolchainLeaf {
  const components = [
    `${sdk.components.platforms.length} platform${sdk.components.platforms.length === 1 ? "" : "s"}`,
    sdk.components.platformTools ? "ADB" : "no ADB",
    sdk.components.emulator ? "Emulator" : "no Emulator",
  ];
  const description = [sdk.ownership, sdk.current ? "current" : "", components.join(", ")].filter(Boolean).join(" · ");
  const tooltip = [sdk.path, `Build Tools: ${sdk.components.buildTools.join(", ") || "none"}`, `NDK: ${sdk.components.ndk.join(", ") || "none"}`, `System images: ${sdk.components.systemImages.length}`].join("\n");
  return new ToolchainLeaf(sdk.name, description, tooltip, new vscode.ThemeIcon("android"));
}

function javaTreeItem(sdk: JavaSDK): ToolchainLeaf {
  return new ToolchainLeaf(sdk.name, `Java ${sdk.version} · ${sdk.ownership}`, sdk.path, new vscode.ThemeIcon("coffee"));
}

function flutterTreeItem(sdk: FlutterSDK, current: boolean): ToolchainLeaf {
  return new ToolchainLeaf(sdk.version, current ? "current" : "managed", sdk.path, new vscode.ThemeIcon("symbol-color"));
}

export class DevicesTreeDataProvider implements vscode.TreeDataProvider<DeviceTreeItem> {
  private readonly changed = new vscode.EventEmitter<void>();
  public readonly onDidChangeTreeData = this.changed.event;
  private devices: MobDevice[] = [];
  private defaultDevice = "";
  private unavailableReason = "";

  public constructor(private readonly client: MobClient) {}

  public async refresh(): Promise<void> {
    try {
      const event = await this.client.query<DevicesData>(["device", "list"]);
      this.devices = event.data?.devices ?? [];
      this.defaultDevice = event.data?.defaultDevice ?? "";
      this.unavailableReason = "";
    } catch (error) {
      this.devices = [];
      this.defaultDevice = "";
      this.unavailableReason = errorMessage(error);
    }
    this.changed.fire();
  }

  public getTreeItem(item: DeviceTreeItem): vscode.TreeItem {
    return item;
  }

  public getChildren(): DeviceTreeItem[] {
    if (this.devices.length === 0) {
      const unavailable = this.unavailableReason !== "";
      const item = new DeviceTreeItem({ id: "none", platform: "", nativeId: "", kind: "", name: unavailable ? "Devices unavailable" : "No devices found", state: "" }, false);
      item.contextValue = undefined;
      item.tooltip = unavailable ? this.unavailableReason : "Connect a physical device, start an emulator, or run Mob without --no-device-create.";
      item.iconPath = new vscode.ThemeIcon(unavailable ? "warning" : "device-mobile", unavailable ? new vscode.ThemeColor("testing.iconFailed") : undefined);
      return [item];
    }
    return this.devices.map((device) => new DeviceTreeItem(device, device.id === this.defaultDevice));
  }

  public readyDevices(): MobDevice[] {
    return this.devices.filter((device) => device.state === "ready");
  }
}

export class DeviceTreeItem extends vscode.TreeItem {
  public constructor(public readonly device: MobDevice, isDefault: boolean) {
    super(device.name || device.id, vscode.TreeItemCollapsibleState.None);
    this.description = [device.kind, device.state, isDefault ? "default" : ""].filter(Boolean).join(" · ");
    this.tooltip = `${device.id}\n${this.description}`;
    this.contextValue = device.kind === "emulator" ? "mob.emulator" : "mob.device";
    this.iconPath = new vscode.ThemeIcon(device.kind === "emulator" ? "vm" : "device-mobile", device.state === "ready" ? new vscode.ThemeColor("testing.iconPassed") : new vscode.ThemeColor("testing.iconFailed"));
    this.command = { command: "mob.openDevice", title: "Open Device Preview", arguments: [this] };
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
