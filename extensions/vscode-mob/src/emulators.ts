import * as vscode from "vscode";
import { MobClient } from "./mobClient";
import { MobDevice } from "./views";
import { showMobError, startWorkflow } from "./workflow";

interface AndroidSDK {
  name: string;
  components?: { systemImages?: string[] };
}

interface SDKListData {
  sdks: AndroidSDK[];
}

interface Emulator {
  name: string;
}

interface EmulatorListData {
  emulators: Emulator[];
}

interface SystemImageChoice extends vscode.QuickPickItem {
  sdk: string;
  image: string;
}

export async function createAndroidEmulator(
  client: MobClient,
  output: vscode.OutputChannel,
  refreshDevices: () => Promise<void>,
): Promise<void> {
  let result;
  try {
    result = await client.query<SDKListData>(["android", "sdk", "list"]);
  } catch (error) {
    showMobError(error);
    return;
  }
  const images = installedSystemImages(result.data?.sdks ?? []);
  if (images.length === 0) {
    const choice = await vscode.window.showWarningMessage(
      "No installed Android system image is available to create an emulator.",
      "Open Toolchain Installer",
    );
    if (choice === "Open Toolchain Installer") {
      await vscode.commands.executeCommand("mob.installToolchain");
    }
    return;
  }
  const image = await vscode.window.showQuickPick(images, {
    title: "Android system image",
    placeHolder: "Choose an installed image for the new emulator",
    matchOnDescription: true,
  });
  if (!image) {
    return;
  }
  const suggestedName = defaultEmulatorName(image.image);
  const name = await vscode.window.showInputBox({
    title: "Android virtual device name",
    value: suggestedName,
    prompt: "Unique AVD name",
    validateInput: (value) => /^[A-Za-z0-9_.-]+$/.test(value) ? undefined : "Use letters, numbers, dots, hyphens, or underscores.",
  });
  if (!name) {
    return;
  }
  startWorkflow(client, output, ["android", "emulator", "create", name, "--image", image.image, "--sdk", image.sdk], {
    title: "Create Android Emulator",
    onCompleted: async () => {
      await refreshDevices();
      const choice = await vscode.window.showInformationMessage(`Mob created ${name}.`, "Start Emulator");
      if (choice === "Start Emulator") {
        startNamedAndroidEmulator(client, output, name, refreshDevices);
      }
    },
  });
}

export async function startAndroidEmulator(
  client: MobClient,
  output: vscode.OutputChannel,
  refreshDevices: () => Promise<void>,
): Promise<void> {
  let result;
  try {
    result = await client.query<EmulatorListData>(["android", "emulator", "list"]);
  } catch (error) {
    showMobError(error);
    return;
  }
  const emulators = result.data?.emulators ?? [];
  if (emulators.length === 0) {
    const choice = await vscode.window.showInformationMessage("No Android virtual devices are available.", "Create Emulator");
    if (choice === "Create Emulator") {
      await createAndroidEmulator(client, output, refreshDevices);
    }
    return;
  }
  const selected = await vscode.window.showQuickPick(
    emulators.map((emulator) => ({ label: emulator.name, emulator })),
    { title: "Start Android emulator", placeHolder: "Choose an Android virtual device" },
  );
  if (!selected) {
    return;
  }
  startNamedAndroidEmulator(client, output, selected.emulator.name, refreshDevices);
}

export async function stopAndroidEmulator(
  client: MobClient,
  output: vscode.OutputChannel,
  refreshDevices: () => Promise<void>,
  device?: MobDevice,
): Promise<void> {
  if (!device || device.kind !== "emulator") {
    vscode.window.showWarningMessage("Select a running Android emulator to stop it.");
    return;
  }
  const confirmed = await vscode.window.showWarningMessage(`Stop Android emulator ${device.name || device.id}?`, { modal: true }, "Stop Emulator");
  if (confirmed !== "Stop Emulator") {
    return;
  }
  startWorkflow(client, output, ["android", "emulator", "stop", device.id], {
    title: "Stop Android Emulator",
    onCompleted: refreshDevices,
  });
}

function startNamedAndroidEmulator(
  client: MobClient,
  output: vscode.OutputChannel,
  name: string,
  refreshDevices: () => Promise<void>,
): void {
  startWorkflow(client, output, ["android", "emulator", "start", name], {
    title: "Start Android Emulator",
    onCompleted: refreshDevices,
  });
}

function installedSystemImages(sdks: AndroidSDK[]): SystemImageChoice[] {
  const images: SystemImageChoice[] = [];
  for (const sdk of sdks) {
    for (const image of sdk.components?.systemImages ?? []) {
      images.push({ label: image, description: `Android SDK: ${sdk.name}`, sdk: sdk.name, image });
    }
  }
  return images.sort((left, right) => left.label.localeCompare(right.label));
}

function defaultEmulatorName(image: string): string {
  const api = image.split(";", 3)[1];
  return api?.startsWith("android-") ? `mob-android-api-${api.slice("android-".length)}` : "mob-android";
}
