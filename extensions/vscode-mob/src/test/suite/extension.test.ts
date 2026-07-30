import * as assert from "assert";
import * as vscode from "vscode";

suite("Mob VS Code extension", () => {
  test("registers Android workflow and device commands", async () => {
    const extension = vscode.extensions.getExtension("xy200303.mob-vscode");
    assert.ok(extension, "Mob extension was not discovered by the Extension Host");
    await extension.activate();

    const commands = await vscode.commands.getCommands(true);
    for (const command of [
      "mob.build",
      "mob.run",
      "mob.debug",
      "mob.openDevice",
      "mob.captureDeviceScreenshot",
      "mob.inspectDeviceUI",
    ]) {
      assert.ok(commands.includes(command), `Expected ${command} to be registered`);
    }
  });
});
