import * as vscode from "vscode";
import { ActiveMobProcess, MobClient, MobCommandError, MobEvent } from "./mobClient";

export interface WorkflowOptions {
  title: string;
  cwd?: string;
  onCompleted?: () => Promise<void> | void;
}

// Long-running CLI actions use the event protocol so VS Code never has to
// infer progress or failure from Gradle, Flutter, or sdkmanager text output.
export function startWorkflow(client: MobClient, output: vscode.OutputChannel, args: readonly string[], options: WorkflowOptions): ActiveMobProcess {
  output.show(true);
  let receivedFailure = false;
  const process = client.stream(args, (event) => {
    if (event.event === "log") {
      const data = event.data as { output?: string } | undefined;
      if (data?.output) {
        output.appendLine(data.output);
      }
      return;
    }
    if (event.event === "error") {
      receivedFailure = true;
      showMobError(new MobCommandError(event.error ?? { code: "MOB_COMMAND_FAILED", message: `${options.title} failed.` }));
      return;
    }
    if (event.event === "started" || event.event === "progress") {
      output.appendLine(`[${event.event}] ${describeData(event.data)}`);
      return;
    }
    if (event.event === "completed") {
      output.appendLine(`[completed] ${describeData(event.data)}`);
      vscode.window.showInformationMessage(`Mob: ${options.title} completed.`);
    }
  }, options.cwd);
  void process.completed.then(async (code) => {
    if (process.protocolError) {
      showMobError(new Error(process.protocolError));
      return;
    }
    if (code !== 0 && !receivedFailure) {
      showMobError(new Error(`${options.title} stopped with exit code ${code ?? "unknown"}.`));
      return;
    }
    if (code === 0) {
      await options.onCompleted?.();
    }
  }).catch((error: unknown) => showMobError(error));
  return process;
}

export function showMobError(error: unknown): void {
  if (error instanceof MobCommandError) {
    const suffix = error.detail.remediation ? ` ${error.detail.remediation}` : "";
    vscode.window.showErrorMessage(`Mob (${error.detail.code}): ${error.message}${suffix}`);
    return;
  }
  vscode.window.showErrorMessage(`Mob: ${error instanceof Error ? error.message : String(error)}`);
}

function describeData(data: unknown): string {
  if (!data || typeof data !== "object") {
    return "";
  }
  const record = data as Record<string, unknown>;
  const phase = typeof record.phase === "string" ? record.phase : "";
  const message = typeof record.message === "string" ? record.message : "";
  if (phase || message) {
    return [phase, message].filter(Boolean).join(": ");
  }
  return JSON.stringify(data);
}
