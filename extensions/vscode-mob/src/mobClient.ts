import { ChildProcessWithoutNullStreams, spawn } from "child_process";
import * as vscode from "vscode";

export interface MobError {
  code: string;
  message: string;
  remediation?: string;
}

export interface MobEvent<T = unknown> {
  schemaVersion: string;
  event: string;
  command: string;
  sequence: number;
  ok: boolean;
  data?: T;
  error?: MobError;
}

export class MobCommandError extends Error {
  public constructor(public readonly detail: MobError, public readonly stderr = "") {
    super(detail.message);
    this.name = "MobCommandError";
  }
}

export class MobClient {
  public constructor(private readonly output: vscode.OutputChannel) {}

  public async query<T>(args: readonly string[], cwd = workspaceDirectory()): Promise<MobEvent<T>> {
    const { child, command } = this.start([...args, "--json"], cwd);
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk: Buffer) => { stdout += chunk.toString(); });
    child.stderr.on("data", (chunk: Buffer) => { stderr += chunk.toString(); });

    const code = await waitForExit(child);
    if (stderr.trim()) {
      this.output.appendLine(stderr.trimEnd());
    }
    const line = stdout.trim();
    if (!line) {
      throw new Error(`${command} produced no JSON output${code === 0 ? "" : ` (exit ${code})`}.`);
    }
    let event: MobEvent<T>;
    try {
      event = parseEvent<T>(line);
    } catch (error) {
      this.output.appendLine(`[protocol] Unexpected output from ${command}: ${line}`);
      throw new Error(`${command} produced invalid JSON output: ${error instanceof Error ? error.message : String(error)}`);
    }
    if (!event.ok || event.event === "error") {
      throw new MobCommandError(event.error ?? { code: "MOB_COMMAND_FAILED", message: `${command} failed.` }, stderr);
    }
    if (code !== 0) {
      throw new Error(`${command} exited with code ${code}.`);
    }
    return event;
  }

  public stream(
    args: readonly string[],
    onEvent: (event: MobEvent) => void,
    cwd = workspaceDirectory(),
  ): ActiveMobProcess {
    const { child, command } = this.start([...args, "--json=events"], cwd);
    let stdoutBuffer = "";
    const process = new ActiveMobProcess(child, command, this.output);
    child.stdout.on("data", (chunk: Buffer) => {
      stdoutBuffer += chunk.toString();
      const lines = stdoutBuffer.split(/\r?\n/);
      stdoutBuffer = lines.pop() ?? "";
      for (const line of lines) {
        this.handleEventLine(command, line, onEvent, process);
      }
    });
    child.stdout.on("end", () => {
      if (stdoutBuffer.trim()) {
        this.handleEventLine(command, stdoutBuffer, onEvent, process);
      }
    });
    child.stderr.on("data", (chunk: Buffer) => this.output.append(chunk.toString()));
    return process;
  }

  public commandLine(args: readonly string[]): string {
    return [this.binary(), ...args].map(quoteForShell).join(" ");
  }

  private start(args: readonly string[], cwd: string | undefined): { child: ChildProcessWithoutNullStreams; command: string } {
    const command = this.binary();
    this.output.appendLine(`> ${this.commandLine(args)}`);
    const child = spawn(command, [...args], { cwd, windowsHide: true });
    child.on("error", (error) => this.output.appendLine(`[error] ${error.message}`));
    return { child, command: `mob ${args.join(" ")}` };
  }

  private handleEventLine(command: string, line: string, onEvent: (event: MobEvent) => void, process: ActiveMobProcess): void {
    if (!line.trim()) {
      return;
    }
    try {
      onEvent(parseEvent(line));
    } catch (error) {
      const message = `${command} produced an invalid event: ${error instanceof Error ? error.message : String(error)}`;
      process.recordProtocolError(message);
      this.output.appendLine(`[protocol] ${message}: ${line}`);
    }
  }

  private binary(): string {
    return vscode.workspace.getConfiguration("mob").get<string>("path", "mob").trim() || "mob";
  }
}

export class ActiveMobProcess implements vscode.Disposable {
  public readonly completed: Promise<number | null>;
  public protocolError: string | undefined;

  public constructor(
    private readonly child: ChildProcessWithoutNullStreams,
    private readonly command: string,
    private readonly output: vscode.OutputChannel,
  ) {
    this.completed = waitForExit(child).then((code) => {
      this.output.appendLine(`[${this.command}] exited with code ${code ?? "unknown"}.`);
      return code;
    });
  }

  public dispose(): void {
    if (!this.child.killed) {
      this.child.kill();
    }
  }

  public recordProtocolError(message: string): void {
    this.protocolError ??= message;
  }
}

function parseEvent<T = unknown>(line: string): MobEvent<T> {
  const value: unknown = JSON.parse(line);
  if (!value || typeof value !== "object") {
    throw new Error("event must be a JSON object");
  }
  const event = value as Partial<MobEvent<T>>;
  if (typeof event.schemaVersion !== "string" || typeof event.event !== "string" || typeof event.command !== "string" || typeof event.sequence !== "number" || typeof event.ok !== "boolean") {
    throw new Error("event does not match the Mob JSON envelope");
  }
  return event as MobEvent<T>;
}

function waitForExit(child: ChildProcessWithoutNullStreams): Promise<number | null> {
  return new Promise((resolve) => {
    // spawn emits error (rather than close) when the configured executable is absent.
    // The caller can then report the missing JSON result without an unhandled promise.
    child.once("error", () => resolve(null));
    child.once("close", (code) => resolve(code));
  });
}

export function workspaceDirectory(): string | undefined {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

function quoteForShell(value: string): string {
  if (/^[A-Za-z0-9_./:=+-]+$/.test(value)) {
    return value;
  }
  if (process.platform === "win32") {
    return `"${value.replace(/"/g, '\\"')}"`;
  }
  return `'${value.replace(/'/g, "'\\''")}'`;
}
