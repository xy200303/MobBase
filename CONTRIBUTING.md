# Contributing to Mob / 参与贡献

Thanks for helping improve Mob. The project is building one VS Code-centered entry point for Android, iOS, and HarmonyOS development environments. Android is the current complete platform; contributions must not present future iOS or HarmonyOS work as delivered capability.

感谢你参与改进 Mob。本项目的目标是为 Android、iOS 和 HarmonyOS 提供以 VS Code 为中心的统一开发环境入口。Android 是当前完整交付的平台；贡献内容不得将未来的 iOS 或 HarmonyOS 工作宣传为已交付能力。

## Before You Start / 开始前

- Search existing issues and pull requests before opening a new one.
- Use the current `main` branch as the base for changes.
- Keep each pull request focused on one problem or capability.
- Do not add SDK archives, emulator images, built executables, VSIX packages, recordings, credentials, or local caches to Git.

- 新建 Issue 或 Pull Request 前，请先搜索已有讨论。
- 请基于当前 `main` 分支开发。
- 每个 Pull Request 应聚焦一个问题或能力。
- 不要提交 SDK 压缩包、系统镜像、构建产物、VSIX、录屏、凭据或本地缓存。

## Report an Issue / 提交问题

For environment or project-compatibility issues, include:

- Mob version and host operating system.
- Project type and relevant build-tool versions.
- The exact Mob command and a redacted `--json` or `--json=events` result when available.
- Expected behavior, actual behavior, and reproducible steps.
- A redacted `mob support bundle` when it is safe and useful.

对于环境或项目兼容性问题，请提供：

- Mob 版本和宿主操作系统。
- 项目类型及相关构建工具版本。
- 完整 Mob 命令；可提供时附脱敏后的 `--json` 或 `--json=events` 结果。
- 期望行为、实际行为和可复现步骤。
- 在安全且有帮助时附上脱敏后的 `mob support bundle`。

Never include keystores, signing certificates, passwords, tokens, device serial numbers, proxy credentials, or private source code.

请勿提交 keystore、签名证书、密码、token、设备序列号、代理凭据或私有源代码。

## Development / 开发

Use Go for the CLI and TypeScript for the VS Code extension. Preserve the separation of responsibilities:

- The CLI owns toolchain discovery, installation, process environment injection, device operations, and JSON contracts.
- The extension consumes the CLI contract and must not duplicate SDK scanning or direct ADB/Emulator management.
- Project files remain owned by Gradle, Flutter/FVM, Xcode, DevEco, and their official tooling.

CLI 使用 Go，VS Code 扩展使用 TypeScript。请保持职责边界：

- CLI 负责工具链发现与安装、子进程环境注入、设备操作和 JSON 契约。
- 扩展消费 CLI 契约，不重复扫描 SDK，也不直接管理 ADB 或 Emulator。
- 项目文件仍由 Gradle、Flutter/FVM、Xcode、DevEco 及其官方工具维护。

## Validate Changes / 验证改动

Run checks appropriate to the files you change:

```powershell
go test ./... -count=1 -timeout 60s
go vet ./...

cd extensions/vscode-mob
npm ci
npm run compile
```

For Android toolchain, device, or workflow changes, use the [manual validation checklist](docs/MANUAL_VALIDATION.md) and describe what was actually run. For documentation-only changes, run `git diff --check`.

对于 Android 工具链、设备或工作流修改，请使用[手动验证清单](docs/MANUAL_VALIDATION.md)，并在 PR 中说明实际执行的验证。仅文档改动请运行 `git diff --check`。

## Pull Requests / Pull Request 要求

- Explain the user problem and the behavior change.
- List validation commands and results.
- Update Chinese and English README content together when public behavior changes.
- Update command help, JSON contracts, error codes, and tests when changing a public CLI contract.
- State platform scope explicitly. Do not imply iOS or HarmonyOS support before its adapter is implemented and verified.

- 说明用户问题和行为变化。
- 列出验证命令和结果。
- 改变公开行为时，同时更新中英文 README。
- 修改公开 CLI 契约时，同时更新命令帮助、JSON 契约、错误码和测试。
- 明确平台范围。在适配器实现并验证前，不得暗示 iOS 或 HarmonyOS 已被支持。

By contributing, you agree that your contributions are licensed under the repository's [MIT License](LICENSE).

提交贡献即表示你同意将贡献内容按仓库的 [MIT License](LICENSE) 授权。
