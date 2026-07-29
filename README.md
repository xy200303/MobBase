# Mob

> 面向 VS Code、终端与 CI 的移动开发环境管理工具。当前以 Android 为完整实现目标。

Mob 像 `nvm` 一样管理移动开发所需的本机工具链：发现已有环境、安装 Mob 托管组件、并为一次构建或运行注入正确环境。它不替代 Android Studio、Gradle、Flutter、FVM 或调试器，而是让它们在同一台机器上更可靠地协作。

## 当前状态

| 平台 | 状态 | 范围 |
| --- | --- | --- |
| Android | 主要实现 | SDK、NDK、JDK、Flutter/FVM、ADB、模拟器、真机、构建、运行、调试、测试、日志与发布构建。 |
| iOS | 规划中 | Xcode、Simulator、真机、签名、构建、调试与发布能力将逐项接入。 |
| HarmonyOS | 规划中 | DevEco、官方 SDK/Native 组件、模拟器、真机与发布工作流将按官方能力接入。 |

Android 是当前的交付重点。iOS 和 HarmonyOS 命名空间用于保持未来 CLI 边界稳定，不应被视为跨平台工作流已经完成。

## 解决的问题

- 多个 Android 项目需要不同 API Level、Build Tools、NDK 或 JDK。
- 新机器缺少 Android SDK、ADB、Emulator 或系统镜像。
- 在 VS Code 中频繁切换真机、模拟器、Flutter/FVM 项目和原生 Gradle 项目。
- 不希望为了一个项目永久修改系统 `PATH`、`JAVA_HOME`、`ANDROID_HOME`。

```text
Mob 管理：SDK、NDK、JDK、Flutter SDK、ADB、Emulator、设备、临时子进程环境
项目工具管理：Gradle Wrapper、FVM 项目版本、Flutter/Android 项目配置
VS Code 插件：可视化状态、设备与工作流入口，所有业务逻辑仍由 Mob CLI 执行
```

## 核心能力

### Android 工具链

- 发现 Android Studio 默认 SDK、`ANDROID_SDK_ROOT`、`ANDROID_HOME` 和已导入 SDK。
- 管理 SDK Platform、Build Tools、platform-tools/ADB、Emulator、系统镜像与 NDK。
- 发现、导入、安装和按项目要求选择 JDK 8/11/17/21。
- 通过 `mob catalog` 与各平台 `available` 命令查看可安装版本和组件。
- 支持仅用于 Android SDK 安装的 HTTP(S) 代理，适用于受限网络环境。
- 只删除 Mob 托管目录中的组件；已发现或导入的外部 SDK 不会被自动覆盖或删除。

### 项目工作流

- 原生 Android：调用项目 Gradle Wrapper 完成 build、run、test、logs、debug 和 release。
- Flutter Android：调用 Flutter/FVM 官方命令，不读取或修改 `.fvmrc`。
- 缺少可自动安装的 Android 组件时，`mob build`、`mob run`、`mob debug` 自动补齐后继续执行；首次 Android SDK 安装需显式传入 `--accept-licenses`。
- 自动识别 Gradle 中的 API、Build Tools、NDK 与 Java 要求；动态表达式不会被猜测。
- 支持 `-- <command> [args...]` 转发官方命令，同时保持项目校验和临时环境注入。

### 设备与调试

- 统一列出 Android 真机和模拟器；支持将可用设备设为默认目标。
- 无设备时可为当前 Android 项目自动创建匹配 API 的 Mob 托管 AVD。
- 支持 ADB USB、Wireless Debugging 配对与无线连接。
- 使用 Android Emulator 官方窗口提供实时预览；真机预览首次使用时自动准备 Mob 内部的镜像运行时，无需手动安装或配置 `scrcpy`。

如网络受限，用户也可将完整的 Windows x64 `scrcpy` 官方压缩包解压到 `MOB_HOME/runtime/scrcpy`（默认为 `~/.mob/runtime/scrcpy`）。目录中必须包含 `scrcpy.exe` 以及同一压缩包中的 DLL、`scrcpy-server` 等全部文件；Mob 发现该目录后会直接使用它，不再下载。
- 支持截图、录屏、项目日志和原生 Android JDWP 调试端点。
- VS Code 插件可将 JDWP 端点交给已安装的 Java/Kotlin 调试扩展，Mob 不伪造调试器。

## 安装与运行

当前仓库提供源码构建方式，需要 Go `1.26` 或更高版本。

Windows PowerShell：

```powershell
go build -o mob.exe ./cmd/mob
.\mob.exe help
```

macOS/Linux：

```bash
go build -o mob ./cmd/mob
./mob help
```

Mob 默认将自身托管的文件放在 `~/.mob`。可用 `MOB_HOME` 临时覆盖，或使用 `mob home set <path>` 迁移 Mob 自己拥有的根目录。

Mob 不会永久修改系统 `PATH`、`JAVA_HOME`、`ANDROID_HOME` 或 `ANDROID_SDK_ROOT`。构建、运行和调试只向其启动的子进程注入需要的环境变量。

### 一键安装脚本

仓库提供跨平台安装脚本。它们将 CLI 安装到 `MOB_INSTALL_DIR`、`MOB_HOME/bin` 或默认的 `~/.mob/bin`，并默认只更新当前用户的 `PATH`。安装器不会请求管理员权限，也不会修改 Android/JDK 环境变量。

Windows PowerShell：

```powershell
.\scripts\install.ps1
```

macOS/Linux：

```bash
chmod +x scripts/install.sh
./scripts/install.sh
```

两者均需 Go `1.26` 或更高版本。脚本在当前源码仓库中执行时构建本地代码；独立脚本可通过 `--version <version>` 从 `github.com/xy200303/MobBase/cmd/mob` 安装版本。常用参数：

```powershell
.\scripts\install.ps1 -InstallDir D:\tools\mob\bin -NoPath
.\scripts\install.ps1 -Version latest
```

```bash
./scripts/install.sh --install-dir "$HOME/.local/bin" --no-path
./scripts/install.sh --version latest
```

## 快速开始

### 使用已有 Android 项目

```powershell
cd C:\work\notes
mob doctor
mob device list
mob run --accept-licenses
```

`mob run` 读取当前 Gradle 项目需求。若没有可用 Android 设备，它会在许可已确认的前提下准备缺失组件、创建匹配项目 API 的默认 AVD，并继续构建、安装和启动应用。

### 导入已有 Android SDK

```powershell
mob android sdk import --path "E:\Android\Sdk" --name shared-sdk
mob android sdk use shared-sdk
mob android sdk inspect shared-sdk
```

导入仅保存引用，不复制、不修改也不删除外部 SDK。

### 从零创建 Android 项目

```powershell
mob android create notes --language kotlin --ui compose --min-sdk 24
cd notes
mob run --accept-licenses
```

### 多版本项目并行

```powershell
cd C:\work\legacy-api-27
mob run --accept-licenses

cd C:\work\modern-api-35
mob debug --accept-licenses
```

Mob 根据每个项目的 Gradle 配置选择并临时注入对应 SDK/JDK/NDK，不会把一个项目的环境永久写入另一个项目或系统环境。

### 连接无线真机

```powershell
mob android device pair 192.168.1.20:37123 --code 123456
mob android device connect 192.168.1.20:5555
mob device list
mob device use android:192.168.1.20:5555
mob run
```

配对地址与连接地址由 Android Wireless Debugging 分别提供，配对完成后仍需使用设备显示的连接地址执行 `connect`。

## 常用命令

| 目标 | 命令 |
| --- | --- |
| 查看总览 | `mob status` |
| 诊断 Android 环境 | `mob doctor` 或 `mob android doctor` |
| 查看可安装组件 | `mob catalog --platform android` |
| 查看或安装 Android SDK 组件 | `mob android sdk list` / `mob android sdk available` / `mob android sdk install managed --api 35 --accept-licenses` |
| 管理 NDK | `mob android ndk list --sdk managed` / `mob android ndk available` |
| 管理模拟器 | `mob android emulator list` / `mob android emulator create` / `mob android emulator start <name>` |
| 运行项目 | `mob run [--platform android] [--device android:<id>]` |
| 构建、测试、调试 | `mob build` / `mob test` / `mob debug` |
| 查看项目日志 | `mob logs --follow` |
| 创建 Android 发布产物 | `mob release --platform android --artifact aab` |
| 获取机器可读帮助 | `mob help <command> --format json` |

运行 `mob help --format markdown` 可获取当前 CLI 的完整命令契约。`--json` 输出单个结构化终态结果，`--json=events` 为长任务输出 JSON Lines 进度事件，适合 VS Code、CI 和 AI 工具调用。

### 终端反馈

普通终端执行安装、自动补齐 SDK 或创建模拟器时，Mob 在标准错误输出统一的阶段提示、官方下载的字节进度条，以及经过筛选的 `sdkmanager` 安装进度。交互式终端会在同一行更新下载进度；重定向到文件或 CI 日志时按阶段输出可读的进度行。Windows 的批处理命令回显和 SDK 许可证全文不会混入终端日志。

`--json` 与 `--json=events` 不输出任何人类 UI 文本：前者只在标准输出产生一个最终 JSON 对象，后者只产生 JSON Lines 事件。因此 VS Code、CI 和 AI 调用方不需要解析进度条或 SDK 工具的文本输出。

## VS Code 扩展

扩展源码位于 [`extensions/vscode-mob`](extensions/vscode-mob)。它提供 Mob Activity Bar、工具链与设备树、Android/Flutter 项目创建、构建、运行、测试、日志、调试、无线设备连接和 Android Emulator 管理入口。

扩展只调用 Mob CLI，不直接扫描 SDK 目录或调用 ADB/Emulator。将 `mob.path` 设置为 `mob` 可执行文件路径或命令名；其余配置包括：

- `mob.autoRefresh`：工作区变更时自动刷新 Mob 状态。
- `mob.autoAttachNativeDebug`：收到 Android JDWP 目标时请求 Java/Kotlin 调试扩展附加。

构建扩展：

```powershell
cd extensions/vscode-mob
npm ci
npm run compile
```

## 安全与边界

- 不创建 `mob.yaml`，不修改项目配置。
- 不读取、解析或改写 Flutter `.fvmrc`。
- 不重分发 Xcode、DevEco、Apple 证书或平台许可证。
- Android SDK 许可证必须由用户通过 `--accept-licenses` 明确确认。
- `mob support bundle` 生成脱敏诊断包，不包含项目文件、凭据、代理设置、环境变量或原始主机路径。

## 路线图

1. 完成 Android 的真实环境兼容性验证、安装包发布、CI 矩阵与 VS Code 端到端测试。
2. 扩展 Android 的企业网络、缓存恢复和发布签名场景验证。
3. 在 macOS 上逐项接入 iOS Xcode、Simulator、真机、签名、调试与 IPA 发布流程。
4. 基于 DevEco 与公开官方 SDK，接入 HarmonyOS 工具链、设备、构建、调试与发布能力。

详细的产品边界、命令契约和用户场景见 [`docs/PRODUCT_PLAN.md`](docs/PRODUCT_PLAN.md)。

## 开发验证

```powershell
go test ./... -count=1 -timeout 60s
go test -race ./... -count=1 -timeout 120s
go vet ./...

cd extensions/vscode-mob
npm run compile
```
