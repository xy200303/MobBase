<p align="center">
  <img src="docs/assets/logo.svg" width="148" alt="Mob logo: a mobile device with a command prompt" />
</p>

<h1 align="center">Mob</h1>

<p align="center">
  <strong>让移动开发环境随项目就绪。</strong><br />
  面向开发者、VS Code、CI 和 AI Agent 的移动工具链管理 CLI。
</p>

<p align="center">
  <a href="https://github.com/xy200303/MobBase/releases"><img src="https://img.shields.io/github/v/release/xy200303/MobBase?style=flat-square&label=release" alt="GitHub Release" /></a>
  <a href="https://github.com/xy200303/MobBase/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/xy200303/MobBase/release.yml?style=flat-square&label=release%20pipeline" alt="Release pipeline" /></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/xy200303/MobBase?style=flat-square&label=go" alt="Go version" /></a>
  <img src="https://img.shields.io/badge/current%20platform-Android-24C6B5?style=flat-square" alt="Current platform Android" />
  <img src="https://img.shields.io/badge/protocol-text%20%7C%20JSON%20%7C%20JSONL-FFB547?style=flat-square" alt="Text JSON and JSON Lines protocol" />
  <img src="https://img.shields.io/badge/hosts-Windows%20%7C%20macOS%20%7C%20Linux-536DFE?style=flat-square" alt="Windows macOS and Linux hosts" />
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#为-ai-和自动化设计">AI 与自动化</a> ·
  <a href="#vs-code-扩展">VS Code 扩展</a> ·
  <a href="docs/PRODUCT_PLAN.md">产品规范</a> ·
  <a href="docs/TEST_REPORT_2026-07-30.md">测试报告</a>
</p>

---

Mob 管理移动开发所需的本机工具链，并在构建、运行和调试时只向当前子进程注入正确环境。它的角色类似 `nvm`，但面向 Android SDK、NDK、JDK、Flutter/FVM、ADB、模拟器和设备。

Mob 不替代 Gradle、Android Studio、Flutter、FVM、Xcode 或 DevEco。它负责发现、安装、选择和衔接这些官方工具，让一个终端命令、一个 VS Code 操作或一个 AI 调用得到相同的结果。

## 为什么是 Mob

| 常见问题 | Mob 的处理方式 |
| --- | --- |
| 不同项目需要 API 27、34、35 或不同 NDK/JDK | 按项目读取 Gradle 需求，选择并临时注入相应工具链。 |
| 新机器没有 SDK、ADB、Emulator 或系统镜像 | 查询官方可安装目录；工作流可在许可证确认后自动补齐缺失组件。 |
| 系统已有 Android Studio 或外部 SDK | 自动发现；导入仅注册引用，绝不复制、覆盖或删除外部目录。 |
| VS Code、CI、AI 需要稳定接口 | 人类模式提供清晰终端反馈；`--json` 和 `--json=events` 提供机器协议。 |
| 不想污染全局环境变量 | 不持久化改写 `PATH`、`JAVA_HOME`、`ANDROID_HOME` 或 `ANDROID_SDK_ROOT`。 |

## 当前交付范围

| 平台 | 状态 | 当前范围 |
| --- | --- | --- |
| Android | 当前交付 | SDK/NDK/JDK、Flutter/FVM、ADB、模拟器、真机、构建、运行、调试、测试、日志与发布构建。 |
| iOS | 规划中 | 保留命令命名空间；Xcode、Simulator、真机、签名和发布将在 macOS 上逐项接入。 |
| HarmonyOS | 规划中 | 保留命令命名空间；DevEco SDK、设备和发布工作流将在后续版本接入。 |

Android 是当前唯一的完整实现目标。iOS 与 HarmonyOS 的命令边界已经稳定，但不表示跨平台工作流已可交付。

## 安装

安装脚本下载与校验 GitHub Release 中对应当前宿主机的最终二进制，不需要 Go。默认安装位置为 `~/.mob/bin`，仅将该目录加入当前用户的 `PATH`。

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.ps1 | iex
```

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.sh | bash
```

安装特定版本或使用自定义目录时，先下载脚本再传参：

```powershell
irm https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.ps1 -OutFile install-mob.ps1
.\install-mob.ps1 -Version v0.1.0 -InstallDir D:\tools\mob\bin
```

```bash
curl -fsSLO https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.sh
bash install.sh --version v0.1.0 --install-dir "$HOME/.local/bin"
```

安装脚本会校验同名 `.sha256` 文件，并将已验证的 Release 缓存到 `~/.mob/cache/releases`。查看脚本：[PowerShell](scripts/install.ps1) · [Bash](scripts/install.sh)。

从源码构建仅用于开发：

```powershell
go build -o mob.exe ./cmd/mob
.\mob.exe help
```

```bash
go build -o mob ./cmd/mob
./mob help
```

## 快速开始

### 运行已有 Android 项目

```powershell
cd C:\work\notes
mob doctor
mob device list
mob run --accept-licenses
```

`mob run` 识别当前 Gradle 项目的 SDK、Build Tools、NDK 和 JDK 需求。若没有可用设备，在已确认许可证且未传入 `--no-device-create` 时，它会准备缺失组件、创建匹配项目 API 的默认 AVD，并继续运行项目。

### 创建并运行原生 Android 项目

```powershell
mob android create notes --language kotlin --ui compose --min-sdk 24
cd notes
mob run --accept-licenses
```

创建命令生成标准 Gradle 项目，不创建 `mob.yaml`，也不向项目注入 Mob 私有配置。项目应保留自己的 Gradle Wrapper；Mob 始终调用项目官方构建入口。

> 当前首次执行 `mob android create` 需要本机可用的 `gradle` 来生成标准 Gradle Wrapper；缺失时 Mob 会返回 `MOB_TOOLCHAIN_MISSING`。已有项目的 `mob build`、`mob run`、`mob test` 与 `mob release` 使用项目自带的 Wrapper，不依赖全局 Gradle。

### 管理 SDK、NDK 与多版本项目

```powershell
mob catalog --platform android
mob android sdk install managed --api 35 --accept-licenses
mob android ndk available
mob android sdk inspect managed
```

```powershell
cd C:\work\legacy-api-27
mob run --accept-licenses

cd C:\work\modern-api-35
mob debug --accept-licenses
```

Mob 按每个项目的 Gradle 配置选择工具链；一个项目使用的 SDK/JDK/NDK 不会永久写入另一个项目或系统环境。

### 连接真机或模拟器

```powershell
mob device list
mob device use android:emulator-5554
mob run
```

无线调试：

```powershell
mob android device pair 192.168.1.20:37123 --code 123456
mob android device connect 192.168.1.20:5555
mob device use android:192.168.1.20:5555
mob run --mirror
```

Android Emulator 使用官方窗口提供实时预览。真机的 `--mirror` 首次使用时会准备 Mob 内部的预览运行时；也可以将完整的 Windows x64 官方 `scrcpy` 压缩包解压到 `MOB_HOME/runtime/scrcpy`，Mob 检测到 `scrcpy.exe` 和同包依赖后会直接复用。

## 为 AI 和自动化设计

Mob 的人类接口和机器接口来自同一份命令契约。先让 Agent 查询能力，再执行受控工作流：

```powershell
mob help run --format json
mob status --json
mob catalog --platform android --json
mob run --accept-licenses --json=events
```

| 模式 | 适用对象 | stdout 契约 |
| --- | --- | --- |
| 默认文本 | 开发者终端 | 命令结果；进度和阶段信息写入 stderr。 |
| `--json` | VS Code、CI、AI 的查询与有限工作流 | 一个终态 JSON 对象，包含 `schemaVersion`、`event`、`ok` 和 `data` 或 `error`。 |
| `--json=events` | build/run/debug/logs 等长任务 | JSON Lines 事件流，stdout 不混入进度条或 SDK 工具原始文本。 |

机器调用可根据稳定错误码决定下一步，而不是解析面向人的字符串，例如：

```json
{
  "schemaVersion": "1.0",
  "event": "error",
  "ok": false,
  "error": {
    "code": "MOB_LICENSE_REQUIRED",
    "remediation": "Review the Android SDK license, then repeat with --accept-licenses."
  }
}
```

自定义官方命令可通过 `--` 转发，Mob 仍负责平台校验、设备选择和本次环境注入：

```powershell
mob run --device android:emulator-5554 -- .\gradlew.bat installDebug
```

需要在自定义命令中引用目标设备时，使用完整参数占位符 `{{mob.device.nativeId}}`、`{{mob.device.id}}` 或 `{{mob.platform}}`；Mob 同时注入 `MOB_DEVICE_ID` 与 `MOB_DEVICE_NATIVE_ID`。

## 常用命令

| 目标 | 命令 |
| --- | --- |
| 查看本机总览 | `mob status` |
| 诊断 Android 环境 | `mob doctor` 或 `mob android doctor` |
| 查看官方可安装目录 | `mob catalog --platform android` |
| 管理 SDK 组件 | `mob android sdk list` / `available` / `install` / `inspect` |
| 管理 NDK | `mob android ndk list --sdk managed` / `available` / `install` |
| 管理模拟器 | `mob android emulator list` / `create` / `start` |
| 选择或查看设备 | `mob device list` / `mob device use <platform:native-id>` |
| 构建、运行、测试、调试 | `mob build` / `mob run` / `mob test` / `mob debug` |
| 查看项目日志 | `mob logs --follow --json=events` |
| 生成发布产物 | `mob release --platform android --artifact aab` |
| 获取完整命令契约 | `mob help --format markdown` |

## VS Code 扩展

扩展源码位于 [`extensions/vscode-mob`](extensions/vscode-mob)。它在 Activity Bar 中提供工具链与设备视图，并通过 Mob CLI 执行项目创建、SDK 安装、构建、运行、测试、日志、调试、无线设备连接和 Emulator 管理。

扩展不自行扫描 SDK 目录，也不直接调用 ADB 或 Emulator。将 `mob.path` 设置为 `mob` 命令名或可执行文件路径即可；其余行为通过以下设置控制：

- `mob.autoRefresh`：工作区变更时刷新 Mob 状态。
- `mob.autoAttachNativeDebug`：Mob 返回 Android JDWP 目标后，请求已安装的 Java/Kotlin 调试扩展附加。

构建扩展：

```powershell
cd extensions/vscode-mob
npm ci
npm run compile
```

## 边界与安全

- 不创建 `mob.yaml`，不修改 Gradle、Flutter、FVM 或其他项目配置。
- 不读取、解析或改写 Flutter `.fvmrc`。
- SDK 许可必须通过 `--accept-licenses` 显式确认。
- Mob 只删除其托管目录内的组件；外部 SDK/JDK/Flutter 始终只读。
- `mob support bundle` 生成脱敏诊断包，不包含项目文件、凭据、代理设置、环境变量或原始主机路径。
- Mob 默认根目录是 `~/.mob`。`MOB_HOME` 可临时覆盖；`mob home set <path>` 可迁移 Mob 自己拥有的目录。

详细平台边界、命令语义、设备策略和发布行为见 [产品规范](docs/PRODUCT_PLAN.md)。

## 开发与发布

```powershell
go test ./... -count=1 -timeout 60s
go test -race ./... -count=1 -timeout 120s
go vet ./...

cd extensions/vscode-mob
npm run compile
```

推送 `v*` Git tag 会触发 Release 工作流：测试、静态检查、Windows x64/macOS x64/macOS ARM64/Linux x64/Linux ARM64 交叉编译、SHA-256 生成和 GitHub Release 上传。

```bash
git tag v0.1.0
git push origin v0.1.0
```

## 路线图

1. 完成 Android 真机、模拟器、企业网络和发布签名的真实环境验证。
2. 扩展缓存恢复与更多 Android 项目类型的兼容性测试。
3. 在 macOS 上接入 iOS 的 Xcode、Simulator、真机、签名、调试和 IPA 发布流程。
4. 基于 DevEco 与公开官方 SDK 接入 HarmonyOS 工具链、设备、构建、调试和发布能力。
