import * as vscode from "vscode";
import { MobClient, workspaceDirectory } from "./mobClient";

const commands = ["build", "run", "test", "release", "logs"] as const;
type MobTaskCommand = typeof commands[number];

export interface MobTaskDefinition extends vscode.TaskDefinition {
  type: "mob";
  command: MobTaskCommand;
  platform?: "android";
  device?: string;
  artifact?: "apk" | "aab" | "ipa" | "hap";
  output?: string;
  noInstall?: boolean;
  acceptLicenses?: boolean;
  mirror?: boolean;
  headless?: boolean;
  noDeviceCreate?: boolean;
  follow?: boolean;
}

export class MobTaskProvider implements vscode.TaskProvider {
  public constructor(private readonly client: MobClient) {}

  public provideTasks(): vscode.ProviderResult<vscode.Task[]> {
    return commands.map((command) => this.createTask({ type: "mob", command }));
  }

  public resolveTask(task: vscode.Task): vscode.ProviderResult<vscode.Task> {
    const definition = task.definition as Partial<MobTaskDefinition>;
    if (!isValidDefinition(definition)) {
      return undefined;
    }
    const resolved = this.createTask(definition, task.name);
    resolved.group = task.group;
    return resolved;
  }

  private createTask(definition: MobTaskDefinition, name = `Mob: ${displayName(definition.command)}`): vscode.Task {
    const args = taskArgs(definition);
    const task = new vscode.Task(
      definition,
      vscode.TaskScope.Workspace,
      name,
      "mob",
      new vscode.ShellExecution(this.client.commandLine(args), { cwd: workspaceDirectory() }),
    );
    task.isBackground = definition.command === "logs" && definition.follow === true;
    return task;
  }
}

function taskArgs(definition: MobTaskDefinition): string[] {
  const args = [definition.command];
  addOption(args, "--platform", definition.platform);
  addOption(args, "--device", definition.device);
  addOption(args, "--artifact", definition.artifact);
  addOption(args, "--output", definition.output);
  addFlag(args, "--no-install", definition.noInstall);
  addFlag(args, "--accept-licenses", definition.acceptLicenses);

  if (definition.command === "run") {
    addFlag(args, "--mirror", definition.mirror);
    addFlag(args, "--headless", definition.headless);
    addFlag(args, "--no-device-create", definition.noDeviceCreate);
  }
  if (definition.command === "logs") {
    addFlag(args, "--follow", definition.follow);
  }
  return args;
}

function addOption(args: string[], flag: string, value: string | undefined): void {
  if (value) {
    args.push(flag, value);
  }
}

function addFlag(args: string[], flag: string, enabled: boolean | undefined): void {
  if (enabled) {
    args.push(flag);
  }
}

function isValidDefinition(definition: Partial<MobTaskDefinition>): definition is MobTaskDefinition {
  if (definition.type !== "mob" || !commands.includes(definition.command as MobTaskCommand)) {
    return false;
  }
  return isOptionalString(definition.device)
    && isOptionalString(definition.output)
    && isOptionalBoolean(definition.noInstall)
    && isOptionalBoolean(definition.acceptLicenses)
    && isOptionalBoolean(definition.mirror)
    && isOptionalBoolean(definition.headless)
    && isOptionalBoolean(definition.noDeviceCreate)
    && isOptionalBoolean(definition.follow)
    && isOneOf(definition.platform, ["android"])
    && isOneOf(definition.artifact, ["apk", "aab"])
    && optionsFitCommand(definition);
}

function optionsFitCommand(definition: Partial<MobTaskDefinition>): boolean {
  if (definition.command !== "run" && (definition.mirror !== undefined || definition.headless !== undefined || definition.noDeviceCreate !== undefined)) {
    return false;
  }
  if (definition.command !== "logs" && definition.follow !== undefined) {
    return false;
  }
  if (definition.command !== "release" && (definition.artifact !== undefined || definition.output !== undefined)) {
    return false;
  }
  if (definition.command === "logs" && (definition.platform !== undefined || definition.noInstall !== undefined || definition.acceptLicenses !== undefined)) {
    return false;
  }
  return definition.command === "run" || definition.command === "logs" || definition.device === undefined;
}

function isOptionalString(value: unknown): value is string | undefined {
  return value === undefined || (typeof value === "string" && value.trim().length > 0);
}

function isOptionalBoolean(value: unknown): value is boolean | undefined {
  return value === undefined || typeof value === "boolean";
}

function isOneOf<T extends string>(value: unknown, values: readonly T[]): value is T | undefined {
  return value === undefined || (typeof value === "string" && values.includes(value as T));
}

function displayName(command: MobTaskCommand): string {
  return command.charAt(0).toUpperCase() + command.slice(1);
}
