# Mob for VS Code

Mob for VS Code is the visual entry point for the `mob` CLI. It shows Android toolchain diagnostics, installed SDK/JDK/Flutter state, and connected devices; it can create standard Android or Flutter projects, create/start/stop Android virtual devices, pair and connect an ADB wireless device, select the default device, open native device windows, and start project build/run/test/release/debug workflows. The toolchain installer only offers components from CLI-provided official Android/JDK/Flutter/FVM directories and asks for explicit confirmation before accepting Android SDK licenses.

The extension intentionally keeps SDK installation, Android Emulator, ADB, project detection, and framework invocation inside the Go CLI. Set `mob.path` when the `mob` executable is not available on `PATH`.

`Mob: Run Project` and `Mob: Follow Project Logs` use an integrated terminal so interactive output remains available. Build, test, release, and installation consume `--json=events` and write structured progress to the Mob Output channel. For native Android, `Mob: Start Debug Session` consumes `mob debug --json=events`, receives a loopback ADB JDWP endpoint, and by default asks VS Code's installed Java debugging extension to attach. The extension removes its own ADB forward when that debug session ends. Set `mob.autoAttachNativeDebug` to `false` to attach manually. Flutter debug continues to expose its Dart VM Service target to the installed Flutter tooling.

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
