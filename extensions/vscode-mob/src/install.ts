import * as vscode from "vscode";
import { MobClient } from "./mobClient";
import { startWorkflow } from "./workflow";

interface CatalogItem {
  packageId: string;
  version: string;
  displayName: string;
  license: string;
  size: number;
  checksumAlgorithm: string;
}

interface JavaRelease {
  major: number;
  version: string;
  source: string;
}

interface CatalogData {
  source: string;
  androidSdk?: CatalogItem[];
  androidNdk?: CatalogItem[];
  androidSystemImages?: CatalogItem[];
  java?: JavaRelease[];
}

interface FlutterRelease {
  version: string;
  archive: string;
  sha256: string;
  current: boolean;
}

interface FVMRelease {
  version: string;
  archiveUrl: string;
  sha256: string;
  dartSdk: string;
  current: boolean;
}

interface AvailableData<T> {
  source: string;
  releases: T[];
}

interface InstallChoice extends vscode.QuickPickItem {
  command: readonly string[];
  androidLicense: boolean;
  source: string;
}

export async function selectAndInstallToolchain(
  client: MobClient,
  output: vscode.OutputChannel,
  afterInstall: () => Promise<void>,
): Promise<void> {
  const [catalogResult, flutterResult, fvmResult] = await Promise.allSettled([
    client.query<CatalogData>(["catalog"]),
    client.query<AvailableData<FlutterRelease>>(["flutter", "available"]),
    client.query<AvailableData<FVMRelease>>(["fvm", "available"]),
  ]);
  const catalog = catalogResult.status === "fulfilled" ? catalogResult.value.data ?? { source: "" } : { source: "" };
  const flutter = flutterResult.status === "fulfilled" ? flutterResult.value.data : undefined;
  const fvm = fvmResult.status === "fulfilled" ? fvmResult.value.data : undefined;
  const choices = buildChoices(catalog, flutter, fvm);
  if (choices.length === 0) {
    const reason = [catalogResult, flutterResult, fvmResult]
      .filter((result): result is PromiseRejectedResult => result.status === "rejected")
      .map((result) => result.reason instanceof Error ? result.reason.message : String(result.reason))
      .join(" ");
    vscode.window.showErrorMessage(`Mob could not load an installable toolchain catalog. ${reason}`);
    return;
  }
  const choice = await vscode.window.showQuickPick(choices, {
    placeHolder: "Choose an official Mob toolchain component to install",
    matchOnDescription: true,
    matchOnDetail: true,
  });
  if (!choice) {
    return;
  }
  const notice = choice.androidLicense
    ? `Install ${choice.label} into Mob-managed storage and accept the Android SDK license?`
    : `Install ${choice.label} into Mob-managed storage?`;
  const confirmed = await vscode.window.showWarningMessage(notice, { modal: true, detail: choice.detail }, "Install");
  if (confirmed !== "Install") {
    return;
  }
  startWorkflow(client, output, choice.command, {
    title: `Install ${choice.label}`,
    onCompleted: afterInstall,
  });
}

function buildChoices(
  catalog: CatalogData,
  flutter?: AvailableData<FlutterRelease>,
  fvm?: AvailableData<FVMRelease>,
): InstallChoice[] {
  const choices: InstallChoice[] = [];
  const addAndroid = (category: string, item: CatalogItem, command: readonly string[]): void => {
    choices.push({
      label: item.displayName || item.packageId,
      description: `${category} · ${item.version}`,
      detail: `${item.packageId} · ${formatSize(item.size)} · ${item.checksumAlgorithm} · ${catalog.source}`,
      command,
      androidLicense: true,
      source: catalog.source,
    });
  };
  for (const item of catalog.androidSdk ?? []) {
    if (!item.packageId.startsWith("system-images;")) {
      addAndroid("Android SDK", item, ["android", "sdk", "install", "managed", "--package", item.packageId, "--accept-licenses"]);
    }
  }
  for (const item of catalog.androidNdk ?? []) {
    addAndroid("Android NDK", item, ["android", "ndk", "install", item.version, "--sdk", "managed", "--accept-licenses"]);
  }
  for (const item of catalog.androidSystemImages ?? []) {
    addAndroid("Android system image", item, ["android", "emulator", "image", "install", item.packageId, "--sdk", "managed", "--accept-licenses"]);
  }
  for (const release of catalog.java ?? []) {
    choices.push({
      label: `Temurin JDK ${release.major}`,
      description: `JDK · ${release.version}`,
      detail: release.source,
      command: ["java", "install", String(release.major)],
      androidLicense: false,
      source: release.source,
    });
  }
  for (const release of flutter?.releases ?? []) {
    choices.push({
      label: `Flutter ${release.version}`,
      description: release.current ? "Flutter SDK · current stable" : "Flutter SDK · stable",
      detail: `${release.archive} · SHA-256 ${release.sha256} · ${flutter?.source ?? ""}`,
      command: ["flutter", "install", "--version", release.version],
      androidLicense: false,
      source: flutter?.source ?? "",
    });
  }
  for (const release of fvm?.releases ?? []) {
    choices.push({
      label: `FVM ${release.version}`,
      description: release.current ? "FVM launcher · current" : "FVM launcher",
      detail: `${release.archiveUrl} · SHA-256 ${release.sha256} · Dart ${release.dartSdk} · ${fvm?.source ?? ""}`,
      command: ["fvm", "install", "--version", release.version],
      androidLicense: false,
      source: fvm?.source ?? "",
    });
  }
  return choices;
}

function formatSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return "size unavailable";
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
