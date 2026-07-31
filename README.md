<p align="center">
  <img src="docs/assets/logo.svg" width="148" alt="Mob logo" />
</p>

<h1 align="center">Mob</h1>

<p align="center">
  <strong>在 VS Code 中完成 Android 开发，不必安装完整 Android Studio，也不必手工配置环境。</strong><br />
  Mob 按项目准备 Android 工具链，并提供适合终端、VS Code、CI 与 AI 工具调用的统一入口。
</p>

<p align="center">
  <a href="https://github.com/xy200303/MobBase/releases"><img src="https://img.shields.io/github/v/release/xy200303/MobBase?style=flat-square&label=release" alt="GitHub Release" /></a>
  <a href="https://github.com/xy200303/MobBase/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/xy200303/MobBase/release.yml?style=flat-square&label=release%20pipeline" alt="Release pipeline" /></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/xy200303/MobBase?style=flat-square&label=go" alt="Go version" /></a>
  <img src="https://img.shields.io/badge/current%20platform-Android-24C6B5?style=flat-square" alt="Current platform Android" />
  <img src="https://img.shields.io/badge/interface-text%20%7C%20JSON%20%7C%20JSONL-FFB547?style=flat-square" alt="Text JSON and JSON Lines interface" />
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#为什么需要-mob">为什么需要 Mob</a> ·
  <a href="#ai-与自动化接口">AI 与自动化接口</a> ·
  <a href="#vs-code-扩展">VS Code 扩展</a> ·
  <a href="docs/PRODUCT_PLAN.md">产品规范</a> ·
  <a href="docs/MANUAL_VALIDATION.md">手动验证</a>
</p>

---

## 目标

Android 开发的难点经常不在代码，而在开发机：Android SDK、Build Tools、NDK、JDK、ADB、Emulator、系统镜像、Gradle 和环境变量相互影响。Android Studio 能集中处理其中一部分，但对习惯使用 VS Code 的开发者来说，安装完整 IDE 和反复配置本机环境成本很高。

Mob 的目标很明确：**让 VS Code 成为 Android 日常开发入口，让环境准备变成一个命令或一次插件操作。**

Mob 使用 Android 官方 command-line tools、Gradle Wrapper、ADB 与 Emulator。它不伪造或替代这些工具，而是负责发现、下载、版本选择、设备选择与本次进程的环境注入。项目仍是标准 Android、Flutter 或 Kotlin Multiplatform 项目，可以随时使用 Gradle、Flutter 或 Android Studio 打开。

Android Studio 仍适合布局可视化编辑、Profiler 和少数专项诊断；它不再是构建、运行、调试 Android 项目的必要前提。

## 为什么需要 Mob

| 常见问题 | Mob 的处理方式 |
| --- | --- |
| 新电脑上没有 SDK、ADB、JDK 或模拟器 | 从官方目录按需准备缺失组件，并在许可确认后继续工作流。 |
| VS Code 需要手动设置 SDK 路径、PATH 与环境变量 | CLI 与扩展共用 Mob 管理的工具链；构建时只向子进程注入环境。 |
| 一个项目用 API 27，另一个项目用 API 35 或不同 NDK/JDK | 从当前项目的 Gradle 配置推导需求，按项目选择工具链。 |
| 真机、模拟器与无线调试入口分散 | 使用统一设备模型列出、选择、连接和启动设备。 |
| AI 编程工具只能猜测本机环境和命令结果 | 使用稳定的 `--json`、`--json=events`、错误码和 UI 自动化命令。 |

Mob 不会创建 `mob.yaml`，不会改写 `build.gradle`、Flutter 配置或 `.fvmrc`，也不会永久设置 `JAVA_HOME`、`ANDROID_HOME`、`ANDROID_SDK_ROOT`。它管理自己的目录，默认位于 `~/.mob`。

## 当前范围

| 平台 | 状态 | 当前能力 |
| --- | --- | --- |
| Android | 已交付 | SDK、NDK、JDK、ADB、Emulator、真机、Flutter/FVM、构建、运行、调试、测试、日志、发布构建与 VS Code 设备预览。 |
| iOS | 规划中 | 保留平台边界；Xcode、Simulator、真机、签名和发布尚未作为完整工作流交付。 |
| HarmonyOS | 规划中 | 保留平台边界；DevEco SDK、设备和构建工作流尚未交付。 |

本仓库当前只承诺完整的 Android 开发体验。

## 安装

安装脚本下载并校验对应宿主机的 Release 二进制，不要求先安装 Go。默认安装到 `~/.mob/bin`，并将该目录加入当前用户的 `PATH`。

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.ps1 | iex
```

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.sh | bash
```

确认安装：

```powershell
mob --version
mob help
```

安装特定 Release 或自定义目录时，先下载脚本再传参：

```powershell
irm https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.ps1 -OutFile install-mob.ps1
.\install-mob.ps1 -Version latest -InstallDir D:\tools\mob\bin
```

```bash
curl -fsSLO https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.sh
bash install.sh --version latest --install-dir "$HOME/.local/bin"
```

已验证的下载会缓存到 `~/.mob/cache/releases`。安装脚本见 [PowerShell](scripts/install.ps1) 与 [Bash](scripts/install.sh)。

## 快速开始

### 在 VS Code 打开已有 Android 项目

安装 Mob 后，在 VS Code 打开项目目录。可以使用 Mob 扩展的工具链与设备视图，也可以直接在集成终端运行：

```powershell
mob doctor
mob run --accept-licenses
```

`mob run` 会识别当前项目。对于原生 Android 与 Kotlin Multiplatform Android 目标，它调用项目 Gradle Wrapper；对于 Flutter Android 目标，它调用项目的 Flutter 或 FVM 工作流。缺少 SDK Platform、Build Tools、NDK、JDK、ADB 或 Emulator 时，Mob 会在许可确认后准备可自动安装的组件。没有可用设备时，默认可创建与项目 API 匹配的 Mob 托管 AVD。

### 创建并运行原生 Android 项目

```powershell
mob android create notes --language kotlin --ui compose --min-sdk 24
cd notes
mob run --accept-licenses
```

生成的是标准 Gradle 工程。Mob 创建项目时会优先复用系统 Gradle；没有时会下载经过校验的 Gradle 发行版以生成 Wrapper。之后 `mob build`、`mob run`、`mob test` 与 `mob release` 使用项目自己的 Gradle Wrapper。

### 多版本项目并存

```powershell
cd C:\work\legacy-api-27
mob run --accept-licenses

cd C:\work\modern-api-35
mob debug --accept-licenses
```

Mob 按当前项目选择 SDK、Build Tools、NDK 和 JDK，仅对本次子进程注入环境。运行旧项目不会破坏新项目的环境，反之亦然。

### 连接真机与模拟器

```powershell
mob device list
mob device use android:emulator-5554
mob run
```

无线 Android 设备：

```powershell
mob android device pair 192.168.1.20:37123 --code 123456
mob android device connect 192.168.1.20:5555
mob device use android:192.168.1.20:5555
mob run --mirror
```

## VS Code 扩展

[Mob for VS Code](https://marketplace.visualstudio.com/items?itemName=xiaoyun.mob-vscode) 是 CLI 的可视化入口。扩展不会自行扫描 SDK 或直接调用 ADB、Emulator；所有环境、设备和工作流仍由同一个 `mob` CLI 处理。

在 Activity Bar 的 Mob 视图中可以：

- 查看 Android SDK、JDK、Flutter 与设备诊断结果。
- 创建标准 Android 或 Flutter 项目。
- 安装 SDK/NDK/系统镜像，创建、启动与停止 Android 模拟器。
- 运行、构建、测试、调试、查看日志与创建发布产物。
- 配对无线真机，选择默认设备，并在编辑器中打开设备预览。

设备预览是 H.264 实时视频流，不是截图轮询。Android 真机与模拟器都可以在 VS Code 中显示，并支持点击、滑动、文本输入、返回、主页和最近任务。会话只监听 `127.0.0.1`，使用短期 token；关闭面板会回收临时 ADB 转发和辅助进程。

扩展安装后未找到 CLI 时，设置 `mob.path` 为 `mob` 命令或可执行文件的绝对路径。详细行为见 [插件文档](extensions/vscode-mob/README.md)。

## AI 与自动化接口

Mob 面向人提供可读的终端输出，也面向 AI、编辑器扩展和 CI 提供机器接口。工具无需解析一段不稳定的报错文字，即可判断当前环境、进度与下一步动作：

```powershell
mob help run --format json
mob status --json
mob catalog --platform android --json
mob run --accept-licenses --json=events
```

| 模式 | 用途 | stdout 契约 |
| --- | --- | --- |
| 默认文本 | 人在终端中执行 | 人类可读结果；阶段与进度输出到 stderr。 |
| `--json` | 查询或有限工作流 | 一个终态 JSON 对象，含 `schemaVersion`、`event`、`ok` 与 `data` 或 `error`。 |
| `--json=events` | build、run、debug、logs 等长任务 | JSON Lines 事件流；不混入进度条与外部工具原始输出。 |

AI 开发 Android 项目时，可以先读取帮助契约和当前状态，再准备环境、构建、运行、调试与检查设备 UI：

```powershell
mob doctor --fix --accept-licenses --json=events
mob run --accept-licenses --json=events
mob device screenshot android:emulator-5554
mob device ui-tree android:emulator-5554 --json
```

错误对象包含稳定错误码和修复建议。例如 `MOB_LICENSE_REQUIRED` 表示需要开发者显式同意 Android SDK 许可，AI 或插件可以据此请求确认，而不是擅自接受许可证。

如需调用项目的特殊官方命令，可用 `--` 转发。Mob 仍负责平台检查、设备选择和本次环境注入：

```powershell
mob run --device android:emulator-5554 -- .\gradlew.bat installDebug
```

完整的事件与错误码契约见 [产品规范](docs/PRODUCT_PLAN.md)。设备预览协议见 [设备会话协议](docs/MOB_DEVICE_SESSION_PROTOCOL.md)。

## 常用命令

| 目标 | 命令 |
| --- | --- |
| 查看版本和帮助 | `mob --version`、`mob help`、`mob help <command> --format json` |
| 查看状态与诊断 | `mob status`、`mob doctor`、`mob doctor --fix --accept-licenses` |
| 查看可安装组件 | `mob catalog --platform android`、`mob android sdk available`、`mob android ndk available` |
| 管理 Android SDK | `mob android sdk list`、`mob android sdk install managed --api 35 --accept-licenses` |
| 管理模拟器 | `mob android emulator image available`、`mob android emulator create`、`mob android emulator start` |
| 管理设备 | `mob device list`、`mob device use <platform:native-id>` |
| 日常项目工作流 | `mob build`、`mob run`、`mob debug`、`mob test`、`mob logs --follow` |
| 发布 Android 产物 | `mob release --platform android --artifact aab` |
| 设备检查与自动化 | `mob device screenshot`、`mob device ui-tree --json`、`mob device wait --idle` |

## 安全与边界

- Android SDK 许可证必须通过 `--accept-licenses` 显式确认。
- Mob 只删除 `MOB_HOME` 内由自己托管的组件；发现或导入的外部工具链始终只读。
- `mob support bundle` 只生成脱敏的 Mob 诊断信息，不包含项目文件、密钥、环境变量、代理设置或原始主机路径。
- iOS 与 HarmonyOS 不是当前完整交付能力，Mob 不会伪造跨平台构建、签名、设备控制或发布结果。

## 开发与验证

```powershell
go test ./... -count=1 -timeout 60s
go test -race ./... -count=1 -timeout 120s
go vet ./...

cd extensions/vscode-mob
npm ci
npm run compile
```

真实 Android 环境的验证步骤见 [手动验证清单](docs/MANUAL_VALIDATION.md)，已执行的验证和未覆盖风险见 [测试报告](docs/TEST_REPORT_2026-07-30.md)。

## 路线图

1. 扩充真实 Android 项目、模拟器、真机、企业网络与签名发布验证。
2. 持续完善 Android 项目类型的识别与工具链兼容性。
3. 在 macOS 上基于 Xcode 接入 iOS 工具链和设备工作流。
4. 基于 DevEco 和公开官方 SDK 接入 HarmonyOS 工具链和设备工作流。
