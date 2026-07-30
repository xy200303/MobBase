import { spawn } from "child_process";
import { access, mkdtemp, readdir, rm } from "fs/promises";
import { tmpdir } from "os";
import * as path from "path";
import { downloadAndUnzipVSCode } from "@vscode/test-electron";

async function resolveWindowsAppRoot(executable: string): Promise<string | undefined> {
  if (process.platform !== "win32") {
    return undefined;
  }

  const installRoot = path.dirname(executable);
  const directAppRoot = path.join(installRoot, "resources", "app");
  try {
    await access(directAppRoot);
    return directAppRoot;
  } catch {
    // Current Windows archive builds place the Electron application under the
    // commit directory, while Code.exe remains in the archive root.
  }

  for (const entry of await readdir(installRoot, { withFileTypes: true })) {
    if (!entry.isDirectory()) {
      continue;
    }
    const appRoot = path.join(installRoot, entry.name, "resources", "app");
    try {
      await access(appRoot);
      return appRoot;
    } catch {
      // Continue until the archive's application directory is found.
    }
  }
  throw new Error(`Could not locate the VS Code application files beside ${executable}`);
}

function createTestEnvironment(): NodeJS.ProcessEnv {
  const environment = { ...process.env };
  delete environment.ELECTRON_RUN_AS_NODE;
  for (const key of Object.keys(environment)) {
    if (key.startsWith("VSCODE_")) {
      delete environment[key];
    }
  }
  return environment;
}

function launchVSCode(executable: string, appRoot: string | undefined, args: string[]): Promise<void> {
  return new Promise((resolve, reject) => {
    // @vscode/test-electron 2.5.x launches Code.exe through cmd.exe on Windows.
    // Node 24 changes that quoting path, so launch the downloaded executable directly.
    const child = spawn(executable, appRoot ? [appRoot, ...args] : args, {
      env: createTestEnvironment(),
      shell: false,
      stdio: "inherit",
      windowsHide: true,
    });
    child.on("error", reject);
    child.on("exit", (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`VS Code extension tests exited with ${signal ?? code ?? "an unknown error"}`));
    });
  });
}

async function main(): Promise<void> {
  const extensionDevelopmentPath = path.resolve(__dirname, "../..");
  const extensionTestsPath = path.resolve(__dirname, "./suite/index");
  const vscodeExecutablePath = await downloadAndUnzipVSCode({
    version: "1.131.0",
    extensionDevelopmentPath,
  });
  const profilePath = await mkdtemp(path.join(tmpdir(), "mob-vscode-test-"));

  try {
    await launchVSCode(vscodeExecutablePath, await resolveWindowsAppRoot(vscodeExecutablePath), [
      "--disable-extensions",
      "--no-sandbox",
      "--disable-gpu-sandbox",
      "--disable-updates",
      "--skip-welcome",
      "--skip-release-notes",
      "--disable-workspace-trust",
      `--extensionDevelopmentPath=${extensionDevelopmentPath}`,
      `--extensionTestsPath=${extensionTestsPath}`,
      `--extensions-dir=${path.join(profilePath, "extensions")}`,
      `--user-data-dir=${path.join(profilePath, "user-data")}`,
    ]);
  } finally {
    await rm(profilePath, { recursive: true, force: true });
  }
}

void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
