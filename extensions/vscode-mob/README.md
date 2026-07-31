# Mob for VS Code

Mob for VS Code is the visual entry point for the `mob` CLI. It shows Android toolchain diagnostics, installed SDK/JDK/Flutter state, and connected devices; it can create standard Android or Flutter projects, create/start/stop Android virtual devices, pair and connect an ADB wireless device, select the default device, open device previews, capture screenshots, inspect the current Android UI hierarchy, and start project build/run/test/release/debug workflows. The toolchain installer only offers components from CLI-provided official Android/JDK/Flutter/FVM directories and asks for explicit confirmation before accepting Android SDK licenses.

The extension intentionally keeps SDK installation, Android Emulator, ADB, project detection, and framework invocation inside the Go CLI. Set `mob.path` when the `mob` executable is not available on `PATH`.

`Mob: Run Project` and `Mob: Follow Project Logs` use an integrated terminal so interactive output remains available. Build, test, release, and installation consume `--json=events` and write structured progress to the Mob Output channel. For native Android, `Mob: Start Debug Session` consumes `mob debug --json=events`, receives a loopback ADB JDWP endpoint, and by default asks VS Code's installed Java debugging extension to attach. The extension removes its own ADB forward when that debug session ends. Set `mob.autoAttachNativeDebug` to `false` to attach manually. Flutter debug continues to expose its Dart VM Service target to the installed Flutter tooling.

## Devices

The Devices view lists the same Android devices that `mob device list` returns. Its context actions operate only on ready Android devices:

- **Open Device** opens `Mob Preview` inside VS Code. It starts a private local H.264 video/control session through the Mob CLI and works with both Android virtual devices and physical devices.
- **Capture Device Screenshot** saves a PNG through ADB and opens it in VS Code.
- **Inspect Device UI** requests `mob device ui-tree --json` and displays the current UI Automator hierarchy in a read-only editor panel. No Android temporary paths are passed to the extension.

`Mob Preview` is a real H.264 video stream, not screenshot polling. The CLI starts its internal scrcpy server on the selected device, connects it through a temporary ADB reverse tunnel, then binds the local service to `127.0.0.1`. A fresh session token is sent only to the extension through `mob device preview serve <device> --json=events`; the Webview keeps it in memory only. Click or touch the screen to tap and swipe; the footer controls send text, Back, Home, and Recent Apps commands through the same authenticated session. Closing the preview stops the CLI service, scrcpy stream, and temporary reverse tunnel.

The extension consumes Mob's versioned `mob.device.session.v1` contract rather than an Android-only protocol. The session declares its platform, video format, and supported controls, so later iOS and HarmonyOS adapters can use the same preview surface without pretending every platform supports Android-level device control. The complete protocol is documented in the repository at `docs/MOB_DEVICE_SESSION_PROTOCOL.md`.

## Toolchains view

The Toolchains view is a compact summary of the environments Mob can use in the current workspace. The number beside each item is a count, not an error indicator:

- **Diagnostics** shows the checks returned by `mob doctor`. Expand it to see which checks are ready, missing, or need attention.
- **Android SDK** shows Android SDK installations discovered or registered by Mob.
- **JDK** shows Java installations available to Android and Gradle builds.
- **Flutter** shows discovered Flutter SDK installations. A value of `0` simply means no Flutter SDK has been found; it does not affect native Android projects.

Use the view toolbar to install a toolchain component, create an Android project, build the open project, run it on a device, or refresh the information. The extension reads the status from the Mob CLI, so the displayed result is the same environment state used by command-line workflows.

When creating an Android or Flutter project, the extension asks for its parent directory. It defaults to the current workspace and creates a new `<project-name>` folder there. After a successful creation, select **Open Project** in the confirmation message to open the new folder in VS Code.

The extension also provides a `mob` task type for VS Code's Run Task command and `tasks.json`. It only accepts supported Mob workflows, so task definitions remain declarative rather than passing arbitrary shell text through the extension:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Run Android app",
      "type": "mob",
      "command": "run",
      "platform": "android",
      "device": "android:emulator-5554",
      "group": "build"
    },
    {
      "label": "Release AAB",
      "type": "mob",
      "command": "release",
      "platform": "android",
      "artifact": "aab",
      "acceptLicenses": true
    }
  ]
}
```

Supported task commands are `build`, `run`, `test`, `release`, and `logs`. `device`, `mirror`, `headless`, and `noDeviceCreate` apply only to `run`; `artifact` and `output` apply only to `release`; `follow` applies only to `logs`. Use `Mob: Start Debug Session` for debugging because it consumes Mob's structured JDWP/Dart VM target event.
