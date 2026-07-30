import * as vscode from "vscode";
import * as path from "path";
import { ActiveMobProcess, MobClient, MobCommandError, MobEvent, workspaceDirectory } from "./mobClient";
import { captureDeviceScreenshot, inspectDeviceUI, selectReadyAndroidDevice } from "./deviceTools";
import { createAndroidEmulator, startAndroidEmulator, stopAndroidEmulator } from "./emulators";
import { selectAndInstallToolchain } from "./install";
import { MobTaskProvider } from "./tasks";
import { DeviceTreeItem, DevicesTreeDataProvider, MobDevice, ToolchainsTreeDataProvider } from "./views";
import { showMobError, startWorkflow } from "./workflow";

let activeDebug: ActiveMobProcess | undefined;
let activeNativeDebug: vscode.DebugSession | undefined;
const nativeDebugForwards = new Map<string, { device: string; port: number }>();

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Mob");
  const client = new MobClient(output);
  const toolchains = new ToolchainsTreeDataProvider(client);
  const devices = new DevicesTreeDataProvider(client);
  const status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  status.command = "mob.doctor";
  status.show();

  context.subscriptions.push(
    output,
    status,
    vscode.tasks.registerTaskProvider("mob", new MobTaskProvider(client)),
    vscode.debug.onDidStartDebugSession((session) => {
      const device = session.configuration.__mobDeviceID;
      const port = session.configuration.__mobJDWPPort;
      if (session.configuration.__mobNativeDebug === true && typeof device === "string" && typeof port === "number") {
        activeNativeDebug = session;
        nativeDebugForwards.set(session.id, { device, port });
      }
    }),
    vscode.debug.onDidTerminateDebugSession((session) => {
      if (activeNativeDebug?.id === session.id) {
        activeNativeDebug = undefined;
      }
      const forward = nativeDebugForwards.get(session.id);
      nativeDebugForwards.delete(session.id);
      if (forward) {
        void removeNativeDebugForward(client, forward, output);
      }
    }),
    vscode.window.registerTreeDataProvider("mob.toolchains", toolchains),
    vscode.window.registerTreeDataProvider("mob.devices", devices),
    vscode.commands.registerCommand("mob.refresh", () => refresh(toolchains, devices, status)),
    vscode.commands.registerCommand("mob.doctor", async () => {
      output.show(true);
      await refresh(toolchains, devices, status);
    }),
    vscode.commands.registerCommand("mob.openOutput", () => output.show()),
    vscode.commands.registerCommand("mob.createAndroidProject", () => createAndroidProject(client, output)),
    vscode.commands.registerCommand("mob.createFlutterProject", () => createFlutterProject(client, output)),
    vscode.commands.registerCommand("mob.run", () => runInTerminal(client, ["run"])),
    vscode.commands.registerCommand("mob.build", () => startWorkflow(client, output, ["build"], { title: "Build Project" })),
    vscode.commands.registerCommand("mob.test", () => startWorkflow(client, output, ["test"], { title: "Test Project" })),
    vscode.commands.registerCommand("mob.release", () => startWorkflow(client, output, ["release"], { title: "Create Release" })),
    vscode.commands.registerCommand("mob.logs", () => runInTerminal(client, ["logs", "--follow"])),
    vscode.commands.registerCommand("mob.installToolchain", () => selectAndInstallToolchain(client, output, async () => refresh(toolchains, devices, status))),
    vscode.commands.registerCommand("mob.debug", () => startDebug(client, output)),
    vscode.commands.registerCommand("mob.stopDebug", () => stopDebug()),
    vscode.commands.registerCommand("mob.selectDevice", (item?: DeviceTreeItem) => selectDevice(client, devices, status, item?.device)),
    vscode.commands.registerCommand("mob.openDevice", (item?: DeviceTreeItem) => openDevice(client, item?.device)),
    vscode.commands.registerCommand("mob.captureDeviceScreenshot", (item?: DeviceTreeItem) => captureDeviceScreenshot(client, item)),
    vscode.commands.registerCommand("mob.inspectDeviceUI", (item?: DeviceTreeItem) => inspectDeviceUI(client, item)),
    vscode.commands.registerCommand("mob.connectDevice", () => connectDevice(client, devices)),
    vscode.commands.registerCommand("mob.pairDevice", () => pairDevice(client, devices)),
    vscode.commands.registerCommand("mob.createAndroidEmulator", () => createAndroidEmulator(client, output, () => devices.refresh())),
    vscode.commands.registerCommand("mob.startAndroidEmulator", () => startAndroidEmulator(client, output, () => devices.refresh())),
    vscode.commands.registerCommand("mob.stopAndroidEmulator", (item?: DeviceTreeItem) => stopAndroidEmulator(client, output, () => devices.refresh(), item?.device)),
    vscode.workspace.onDidChangeWorkspaceFolders(() => {
      if (vscode.workspace.getConfiguration("mob").get<boolean>("autoRefresh", true)) {
        void refresh(toolchains, devices, status);
      }
    }),
  );

  void refresh(toolchains, devices, status);
}

export function deactivate(): void {
  stopDebug();
}

async function refresh(toolchains: ToolchainsTreeDataProvider, devices: DevicesTreeDataProvider, status: vscode.StatusBarItem): Promise<void> {
  const health = await toolchains.refresh();
  await devices.refresh();
  status.text = !health.available ? "$(error) Mob: unavailable" : health.ready ? "$(pass-filled) Mob: ready" : "$(warning) Mob: needs setup";
  status.tooltip = "Mob: Diagnose Environment";
}

function runInTerminal(client: MobClient, args: readonly string[]): void {
  const terminal = vscode.window.createTerminal({ name: "Mob", cwd: workspaceDirectory() });
  terminal.show();
  terminal.sendText(client.commandLine(args));
}

async function createAndroidProject(client: MobClient, output: vscode.OutputChannel): Promise<void> {
  const root = workspaceDirectory();
  if (!root) {
    vscode.window.showErrorMessage("Open a parent folder before creating an Android project.");
    return;
  }
  const name = await vscode.window.showInputBox({
    title: "Create Android project",
    prompt: "Project directory name",
    validateInput: (value) => /^[A-Za-z][A-Za-z0-9_-]*$/.test(value) ? undefined : "Use a letter first, then letters, numbers, hyphens, or underscores.",
  });
  if (!name) {
    return;
  }
  const parent = await selectProjectParent(root);
  if (!parent) {
    return;
  }
  const template = await vscode.window.showQuickPick([
    { label: "Kotlin + Compose", language: "kotlin", ui: "compose" },
    { label: "Kotlin + Views", language: "kotlin", ui: "views" },
    { label: "Java + Views", language: "java", ui: "views" },
  ], { title: "Android project template" });
  if (!template) {
    return;
  }
  const minSdk = await vscode.window.showInputBox({
    title: "Minimum Android SDK",
    value: "24",
    validateInput: (value) => /^(2[1-9]|[3-9]\d)$/.test(value) ? undefined : "Enter an integer of at least 21.",
  });
  if (!minSdk) {
    return;
  }
  startWorkflow(client, output, ["android", "create", name, "--language", template.language, "--ui", template.ui, "--min-sdk", minSdk], {
    title: "Create Android Project",
    cwd: parent,
    onCompleted: () => offerOpenProject(path.join(parent, name)),
  });
}

async function createFlutterProject(client: MobClient, output: vscode.OutputChannel): Promise<void> {
  const root = workspaceDirectory();
  if (!root) {
    vscode.window.showErrorMessage("Open a parent folder before creating a Flutter project.");
    return;
  }
  const name = await vscode.window.showInputBox({
    title: "Create Flutter project",
    prompt: "Dart package name",
    validateInput: (value) => /^[a-z][a-z0-9_]*$/.test(value) ? undefined : "Use a lowercase Dart package name.",
  });
  if (!name) {
    return;
  }
  const parent = await selectProjectParent(root);
  if (!parent) {
    return;
  }
  const target = await vscode.window.showQuickPick([
    { label: "Android", platforms: "android" },
    { label: "Android and iOS", platforms: "android,ios" },
  ], { title: "Flutter targets" });
  if (!target) {
    return;
  }
  startWorkflow(client, output, ["flutter", "create", name, "--platforms", target.platforms], {
    title: "Create Flutter Project",
    cwd: parent,
    onCompleted: () => offerOpenProject(path.join(parent, name)),
  });
}

async function selectProjectParent(workspaceRoot: string): Promise<string | undefined> {
  const choice = await vscode.window.showQuickPick([
    { label: "Current workspace", description: workspaceRoot, folder: workspaceRoot },
    { label: "Choose another folder...", description: "Select the parent directory for the new project" },
  ], {
    title: "Project location",
    placeHolder: "Choose the parent directory for the new project",
  });
  if (!choice) {
    return undefined;
  }
  if (choice.folder) {
    return choice.folder;
  }
  const folder = await vscode.window.showOpenDialog({
    canSelectFiles: false,
    canSelectFolders: true,
    canSelectMany: false,
    openLabel: "Use This Folder",
    title: "Choose project location",
  });
  return folder?.[0]?.fsPath;
}

function offerOpenProject(projectPath: string): void {
  vscode.window.showInformationMessage(`Mob created ${projectPath}.`, "Open Project").then((choice) => {
    if (choice === "Open Project") {
      void vscode.commands.executeCommand("vscode.openFolder", vscode.Uri.file(projectPath), false);
    }
  });
}

async function selectDevice(client: MobClient, devices: DevicesTreeDataProvider, status: vscode.StatusBarItem, requested?: MobDevice): Promise<void> {
  let device = requested;
  if (!device) {
    await devices.refresh();
    const selected = await vscode.window.showQuickPick(
      devices.readyDevices().map((candidate) => ({ label: candidate.name || candidate.id, description: `${candidate.kind} · ${candidate.id}`, device: candidate })),
      { placeHolder: "Select the default Mob device" },
    );
    device = selected?.device;
  }
  if (!device) {
    return;
  }
  if (device.state !== "ready") {
    vscode.window.showWarningMessage(`Mob device ${device.name || device.id} is not ready.`);
    return;
  }
  try {
    await client.query(["device", "use", device.id]);
    vscode.window.showInformationMessage(`Mob default device: ${device.name || device.id}`);
    await devices.refresh();
    status.text = "$(pass-filled) Mob: ready";
  } catch (error) {
    showError(error);
  }
}

async function connectDevice(client: MobClient, devices: DevicesTreeDataProvider): Promise<void> {
  const address = await vscode.window.showInputBox({
    title: "Connect Android device",
    prompt: "ADB wireless debugging address",
    placeHolder: "192.168.1.20:5555",
    validateInput: (value) => /^[^\s:]+:\d{1,5}$/.test(value.trim()) ? undefined : "Enter an address in host:port form.",
  });
  if (!address) {
    return;
  }
  try {
    await client.query(["android", "device", "connect", address.trim()]);
    await devices.refresh();
    vscode.window.showInformationMessage(`Mob connected to ${address.trim()}.`);
  } catch (error) {
    showError(error);
  }
}

async function pairDevice(client: MobClient, devices: DevicesTreeDataProvider): Promise<void> {
  const address = await vscode.window.showInputBox({
    title: "Pair Android device",
    prompt: "Wireless debugging pairing address",
    placeHolder: "192.168.1.20:37123",
    validateInput: (value) => /^[^\s:]+:\d{1,5}$/.test(value.trim()) ? undefined : "Enter the pairing address in host:port form.",
  });
  if (!address) {
    return;
  }
  const code = await vscode.window.showInputBox({
    title: "Android pairing code",
    password: true,
    prompt: "Six-digit code shown by Android Wireless debugging",
    validateInput: (value) => /^\d{6}$/.test(value.trim()) ? undefined : "Enter exactly six digits.",
  });
  if (!code) {
    return;
  }
  try {
    await client.query(["android", "device", "pair", address.trim(), "--code", code.trim()]);
    vscode.window.showInformationMessage("Mob paired the Android device. Use its separate Wireless debugging connection address to connect it.", "Connect Device").then((choice) => {
      if (choice === "Connect Device") {
        void connectDevice(client, devices);
      }
    });
  } catch (error) {
    showError(error);
  }
}

async function openDevice(client: MobClient, device?: MobDevice): Promise<void> {
  const selected = await selectReadyAndroidDevice(client, device);
  if (!selected) {
    return;
  }
  try {
    await client.query(["device", "open", selected.id]);
  } catch (error) {
    showError(error);
  }
}

function startDebug(client: MobClient, output: vscode.OutputChannel): void {
  stopDebug();
  output.show(true);
  const process = client.stream(["debug"], (event) => handleDebugEvent(event, output, client));
  activeDebug = process;
  void process.completed.then((code) => {
    if (activeDebug === process) {
      activeDebug = undefined;
      void vscode.commands.executeCommand("setContext", "mob.hasDebugTarget", false);
      if (process.protocolError) {
        showError(new Error(process.protocolError));
      } else if (code !== 0) {
        showError(new Error(`Debug session stopped with exit code ${code ?? "unknown"}.`));
      }
    }
  });
}

function stopDebug(): void {
  activeDebug?.dispose();
  activeDebug = undefined;
  if (activeNativeDebug) {
    void vscode.debug.stopDebugging(activeNativeDebug);
    activeNativeDebug = undefined;
  }
  void vscode.commands.executeCommand("setContext", "mob.hasDebugTarget", false);
}

function handleDebugEvent(event: MobEvent, output: vscode.OutputChannel, client: MobClient): void {
  if (event.event === "log") {
    const data = event.data as { output?: string } | undefined;
    if (data?.output) {
      output.appendLine(data.output);
    }
    return;
  }
  if (event.event === "debugTarget") {
    const data = event.data as { transport?: string; host?: string; wsUri?: string; port?: number; package?: string } | undefined;
    const target = data?.wsUri ?? (data?.port ? `localhost:${data.port}` : "available");
    void vscode.commands.executeCommand("setContext", "mob.hasDebugTarget", true);
    vscode.window.showInformationMessage(`Mob debug target: ${target}`, "Show Output").then((choice) => {
      if (choice) {
        output.show(true);
      }
    });
    output.appendLine(`[debugTarget] ${JSON.stringify(data)}`);
    if (data?.transport === "jdwp" && typeof data.port === "number") {
      const device = (event.data as { device?: { id?: string } } | undefined)?.device?.id;
      if (typeof device === "string") {
        void attachNativeAndroidDebugger({ host: data.host, port: data.port, package: data.package, device }, output, client);
      }
    }
    return;
  }
  if (event.event === "error") {
    showError(new MobCommandError(event.error ?? { code: "MOB_COMMAND_FAILED", message: "Mob debug failed." }));
  }
}

async function attachNativeAndroidDebugger(target: { host?: string; port: number; package?: string; device: string }, output: vscode.OutputChannel, client: MobClient): Promise<void> {
  if (!vscode.workspace.getConfiguration("mob").get<boolean>("autoAttachNativeDebug", true)) {
    return;
  }
  const host = target.host || "127.0.0.1";
  const name = `Mob Android: ${target.package || "debug"}`;
  try {
    const attached = await vscode.debug.startDebugging(undefined, {
      type: "java",
      request: "attach",
      name,
      hostName: host,
      port: target.port,
      __mobNativeDebug: true,
      __mobDeviceID: target.device,
      __mobJDWPPort: target.port,
    });
    if (attached) {
      output.appendLine(`[debug] Requested Java attach at ${host}:${target.port}.`);
      return;
    }
  } catch (error) {
    output.appendLine(`[debug] Java attach failed: ${error instanceof Error ? error.message : String(error)}`);
  }
  await removeNativeDebugForward(client, target, output);
  vscode.window.showWarningMessage("Mob prepared an Android JDWP target, but VS Code could not start a Java debugger. Install or enable the Java debugging extension, then attach to the endpoint shown in Mob Output.");
}

async function removeNativeDebugForward(client: MobClient, forward: { device: string; port: number }, output: vscode.OutputChannel): Promise<void> {
  try {
    await client.query(["device", "forward", "remove", forward.device, "--port", String(forward.port)]);
    output.appendLine(`[debug] Removed JDWP forward at 127.0.0.1:${forward.port}.`);
  } catch (error) {
    output.appendLine(`[debug] Could not remove JDWP forward at 127.0.0.1:${forward.port}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function showError(error: unknown): void {
  showMobError(error);
}
