# Mob for VS Code

Mob for VS Code is the one-click entry point for mobile development environments inside the editor. Mob's direction is to let developers use VS Code for Android, iOS, and HarmonyOS projects without separately assembling SDKs, device bridges, simulators, environment variables, and platform-specific command chains.

Android is the current complete workflow: use VS Code without installing Android Studio solely to set up SDK paths, ADB, JDK, Build Tools, or an emulator. iOS and HarmonyOS have the shared platform boundary and protocol in place, but their complete toolchain and device workflows are still being implemented.

The extension is a visual client, not a second environment manager. The Go CLI discovers or prepares the official Android tools, selects the environment required by the open project, invokes its Gradle Wrapper or Flutter/FVM command, and exposes structured results. Future iOS and HarmonyOS adapters follow the same command contract while using their official tooling. This keeps the terminal, VS Code, CI, and AI-assisted workflows on one toolchain and device model.

Android Studio remains useful for layout tooling, Profiler, and specialised diagnostics. It is not required for the normal Android build, run, debug, device, and environment-preparation path that Mob supports. Future Apple and HarmonyOS workflows will retain the same honest boundary around Xcode and DevEco requirements.

## Before You Start

Install the Mob CLI first. The extension runs `mob` from `PATH` by default; set `mob.path` to an absolute executable path when it is installed elsewhere. The extension reports a missing CLI directly instead of treating it as a missing Android SDK.

The current complete workflow is Android. iOS and HarmonyOS are not presented as ready-to-use toolchains or device workflows.

## What the Extension Does

- Shows the same Android SDK, JDK, Flutter, and device diagnostics returned by the CLI.
- Creates standard Android or Flutter projects in a selected parent directory.
- Installs CLI-listed toolchain components after the required licence confirmation.
- Creates, starts, and stops Android virtual devices; pairs and connects wireless ADB devices.
- Runs build, run, test, release, logs, and debug workflows from the active workspace.
- Opens an Android device preview, captures screenshots, and inspects the UI Automator tree.

`Mob: Run Project` and `Mob: Follow Project Logs` use an integrated terminal so interactive framework output remains available. Build, test, release, and installation consume `--json=events` and write structured progress to the Mob Output channel.

For native Android, `Mob: Start Debug Session` consumes `mob debug --json=events`, receives a loopback ADB JDWP endpoint, and asks an installed Java/Kotlin debug extension to attach. The extension removes its ADB forward when that debug session ends. Set `mob.autoAttachNativeDebug` to `false` to attach manually. Flutter debug continues to expose its Dart VM Service target to the installed Flutter tooling.

## Devices and Preview

The Devices view lists the same Android devices returned by `mob device list`. Context actions operate on ready devices:

- **Open Device** opens `Mob Preview` inside VS Code.
- **Capture Device Screenshot** saves a PNG through ADB and opens it in VS Code.
- **Inspect Device UI** requests `mob device ui-tree --json` and shows the current UI Automator hierarchy in a read-only editor. Android temporary paths are not exposed to the extension.

`Mob Preview` is an H.264 stream, not screenshot polling. The CLI starts the Android preview service, creates a temporary ADB reverse tunnel, and binds the local endpoint to `127.0.0.1`. The extension receives a temporary token through `mob device preview serve <device> --json=events` and keeps it in memory only. Click or touch to tap and swipe; the footer controls send text, Back, Home, and Recent Apps actions. Closing the preview cleans up the CLI service, video stream, and reverse tunnel.

The preview uses the versioned `mob.device.session.v1` contract. Android is the current complete adapter; a future platform adapter must declare its own supported video and controls rather than assuming Android semantics. See `docs/MOB_DEVICE_SESSION_PROTOCOL.md` in the repository.

## Toolchains View

The Toolchains view is a compact view of the environment used by the current workspace. Counts are inventory values, not error indicators.

- **Diagnostics**: checks from `mob doctor` and their remediation.
- **Android SDK**: SDK installations discovered or registered by Mob.
- **JDK**: Java installations available to Android and Gradle builds.
- **Flutter**: discovered Flutter SDK installations. A value of `0` does not affect native Android projects.

Use the toolbar to install a component, create a project, build, run, or refresh. The extension does not scan SDK folders or invoke ADB and Emulator binaries itself, so the result matches the CLI used by an AI tool or CI job.

When creating an Android or Flutter project, the extension asks for a parent directory, defaults to the current workspace, and creates a new `<project-name>` directory. After creation, choose **Open Project** in the confirmation message to switch VS Code to that directory.

## Tasks

The extension provides a `mob` task type for Run Task and `tasks.json`. It accepts only declared Mob workflows and fields, rather than forwarding arbitrary shell text:

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

Supported task commands are `build`, `run`, `test`, `release`, and `logs`. `device`, `mirror`, `headless`, and `noDeviceCreate` apply only to `run`; `artifact` and `output` apply only to `release`; `follow` applies only to `logs`. Use `Mob: Start Debug Session` for debugging because it consumes Mob's structured JDWP or Dart VM target event.
