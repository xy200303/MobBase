# Mob CLI 测试报告

测试日期：2026-07-30  
版本范围：`main`（基线提交 `b751444`，包含本次测试发现的 Android 目录与 JSON 输出修复）  
宿主机：Windows 10 `10.0.26200`，Go `1.26.0 windows/amd64`，Docker `29.2.1`

## 验证目标

本报告记录 2026-07-30 的验证基线。它验证的是 Mob 最核心的产品路径：把 Android 工具链管理从 Android Studio 和人工环境配置中拆出，使 VS Code、终端与 AI/CI 可以通过同一个 CLI 准备环境并执行工作流。

这是一份历史测试记录，不代表之后版本的全部能力，也不把未执行的真机、模拟器或真实项目周期测试写成已验证结论。

## 结论

Mob 的 Android 环境管理主路径、帮助契约、查询类 JSON、事件流 JSON Lines、真实 Android SDK/ADB 安装以及跨平台构建均通过验证。CLI 的标准输出在 `--json` 和 `--json=events` 模式下可由 VS Code 插件、CI 和 AI 直接解析；普通终端模式保留面向人的文本输出和进度提示。这些结果支持“统一入口管理环境”的方向，但不等于 Android 全生命周期已在所有真实设备上完成验证。

本轮测试发现并修复两项会影响 AI/插件使用的实现问题：Android 系统镜像目录未加载，以及 Windows `sdkmanager.bat` 的批处理回显进入成功 JSON 结果。当前仍未在本机完成真机授权、模拟器启动和真实 Android 项目构建/运行/调试/发布，这些项目明确列为环境限制，不能视为已验证。

## 范围与方法

测试覆盖以下层次：

| 层次 | 方法 | 结果 |
| --- | --- | --- |
| 单元与包集成 | Go 测试、竞态检查、静态检查 | 通过 |
| CLI 契约 | 遍历全部公开帮助命令，比较文本 Usage 与 JSON Usage | 85/85 通过 |
| 隔离功能测试 | 临时 `MOB_HOME` 和临时项目目录执行状态、SDK、设备、代理、支持包、错误分支 | 23/23 通过 |
| 官方目录 | 请求 Android 官方 Repository 与系统镜像目录 | 通过，系统镜像 60 项 |
| 真实工具链 | 下载 Android Command-line Tools 与 `platform-tools`，调用实际 ADB | 通过 |
| 机器协议 | 校验 `--json` 终态对象和 `--json=events` NDJSON，分离 stdout/stderr | 通过 |
| 交叉发布构建 | Windows、macOS、Linux 共 5 个目标 | 通过 |

所有会写入状态或下载内容的测试均使用临时 `MOB_HOME`，不修改用户的实际 Mob 根目录、系统 `PATH`、`JAVA_HOME` 或 Android 环境变量。

## 自动化检查

以下命令均成功退出：

```powershell
go test ./... -count=1 -timeout 180s
go vet ./...
go test -race -p 1 ./... -count=1 -timeout 300s
docker run --rm -v "${PWD}:/src" -w /src golang:1.26 bash -c 'go test ./... -count=1 -timeout 180s && go vet ./... && bash -n scripts/install.sh'
```

主要覆盖率如下，比例只反映现有自动化用例，不代表端到端功能完成度：

| 包 | 覆盖率 |
| --- | ---: |
| `internal/project` | 79.8% |
| `internal/state` | 64.0% |
| `internal/system` | 40.4% |
| `internal/app` | 30.0% |
| `internal/platform/android` | 27.6% |
| `internal/platform/flutter` | 14.2% |
| `internal/platform/fvm` | 11.4% |
| `internal/platform/scrcpy` | 8.0% |
| `internal/platform/java` | 3.1% |

## 人类 CLI 与 JSON 契约

使用临时构建的 Windows 可执行文件，遍历了 85 条已注册的公开 `mob help <command>` 命令路径。每条命令同时验证：

- 默认文本帮助中存在 `Usage: ...`；
- `--json` 输出可解析为 JSON；
- JSON 包含 `schemaVersion: "1.0"`、`event: "completed"`、`ok: true`；
- 文本中的 Usage 与 JSON 的 `data.usage` 完全一致。

结果：

```text
HELP_CONTRACT_COMMANDS=85
HELP_CONTRACT_FAILURES=0
HELP_CONTRACT_INVALID_COMMAND=PASSED
```

无效命令 `mob help unknown --json` 正确返回非成功结构化错误，错误码为 `MOB_INVALID_COMMAND`。

代表性的人类输出：

```text
Mob home: <temporary-mob-home>
Android SDKs: 0
JDKs: 0
Flutter SDKs: 0
Default device:
```

代表性 JSON 终态输出：

```json
{
  "schemaVersion": "1.0",
  "event": "completed",
  "command": "mob android sdk inspect",
  "sequence": 1,
  "ok": true,
  "data": {
    "sdk": {
      "name": "shared",
      "ownership": "imported"
    }
  }
}
```

## 隔离功能端到端测试

在独立状态目录中执行 23 条命令，命令退出码、文本/JSON 模式和预期错误码均一致：

| 领域 | 已验证行为 |
| --- | --- |
| 总览 | `status` 文本和 JSON、`doctor --json` |
| SDK/NDK | Android SDK list/import/inspect/use、NDK list |
| 语言与框架 | Java、Flutter、FVM 的 list/status JSON |
| 设备与平台边界 | 无 ADB 设备查询错误、Windows 上 iOS 宿主机限制、HarmonyOS 保留命名空间 |
| 配置 | Android 代理 set/show/clear |
| 诊断 | support bundle 生成 |
| 所有权保护 | 拒绝删除导入 SDK，返回 `MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED` |
| 项目识别 | 未识别目录执行 run，返回 `MOB_PROJECT_UNRECOGNIZED` |
| 创建项目 | 无 Gradle 时 `android create` 返回 `MOB_TOOLCHAIN_MISSING` |

结果：

```text
STATE_E2E_COMMANDS=23
STATE_E2E_PASSED=23
STATE_E2E_FAILED=0
```

这里验证的是预期的可恢复错误，而不是把缺失环境误判为成功。例如删除外部导入 SDK 的结构化输出包含：

```json
{
  "schemaVersion": "1.0",
  "event": "error",
  "ok": false,
  "error": {
    "code": "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED"
  }
}
```

## 官方目录与可安装组件

实际访问 Android 官方目录后，发现原实现只加载了通用 SDK Repository：

```text
https://dl.google.com/android/repository/repository2-1.xml
```

Android 系统镜像实际在独立目录：

```text
https://dl.google.com/android/repository/sys-img/android/sys-img2-1.xml
```

已修正目录加载逻辑，同时合并两个来源并缓存系统镜像 XML。修复后的真实复测：

```text
SYSTEM_IMAGE_EXIT=0
SYSTEM_IMAGE_OK=True
SYSTEM_IMAGE_COUNT=60
SYSTEM_IMAGE_SOURCE=https://dl.google.com/android/repository/repository2-1.xml; https://dl.google.com/android/repository/sys-img/android/sys-img2-1.xml
```

`mob android emulator image available --api 35` 的文本输出也正确列出 API 35 对应系统镜像。

## 真实 Android SDK 与 ADB 验证

在隔离的真实 Mob 根目录中运行：

```powershell
mob android sdk install managed --package platform-tools --accept-licenses --json=events
```

Mob 从 Android 官方源下载 Android Command-line Tools（约 148 MiB），再由官方 `sdkmanager` 安装 `platform-tools`。安装完成后，实际 ADB 可运行：

```text
Android Debug Bridge version 1.0.41
Version 37.0.0-14910828
```

无已连接设备时，文本输出和 JSON 均符合预期：

```text
No mobile devices found.
```

```json
{
  "schemaVersion": "1.0",
  "event": "completed",
  "command": "mob device list",
  "sequence": 1,
  "ok": true,
  "data": {
    "defaultDevice": "",
    "devices": []
  }
}
```

`mob status` 和 `mob android sdk inspect managed` 均显示托管 SDK 的 `platformTools: true` 与 `commandLineTools: true`。

## 事件流与输出对齐

机器模式的规则是：`stdout` 只输出 JSON；`stderr` 不混入机器事件；长期工作流使用一行一个 JSON 对象的 NDJSON。

验证结果：

| 场景 | 结果 |
| --- | --- |
| 空项目 `mob run --no-install --json=events` | 一条合法 error 事件，`MOB_PROJECT_UNRECOGNIZED` |
| 空项目 `mob logs --json=events` | 一条合法 error 事件，`MOB_PROJECT_UNRECOGNIZED` |
| 已安装 `platform-tools` 再次安装 | 两条合法事件（started、completed），退出码 0，stderr 0 字节 |

真实 SDK 安装复测摘要：

```text
ExitCode=0
EventCount=2
EventsValid=True
ContainsBatchEcho=False
StdErrBytes=0
CompletedOutputPresent=False
```

完整事件形态如下，字段顺序不作为协议要求，但字段语义固定：

```json
{"schemaVersion":"1.0","event":"started","command":"mob android sdk install","sequence":1,"ok":true,"data":{"packages":["platform-tools"],"phase":"install","sdk":"managed","tool":"android-sdkmanager"}}
{"schemaVersion":"1.0","event":"completed","command":"mob android sdk install","sequence":2,"ok":true,"data":{"installation":{"sdkManager":"<mob-home>\\toolchains\\android\\managed\\sdk\\cmdline-tools\\latest\\bin\\sdkmanager.bat","packages":["platform-tools"]},"sdk":{"name":"managed","ownership":"managed","components":{"platformTools":true,"commandLineTools":true}}}}
```

修复前，第二条事件的 `installation.output` 会包含 Windows 批处理内部命令，例如 `if "Windows_NT"` 和 `set DIRNAME`。现已在成功结果中移除该字段；人类模式下的进度仍通过 stderr 实时显示。失败场景会过滤批处理噪声后保留 SDK 管理器的真实诊断。

## 跨平台可执行文件

以下发行目标均编译成功：

| 目标 | 结果 |
| --- | --- |
| Windows amd64 | 8,312,320 bytes |
| macOS amd64 | 8,221,440 bytes |
| macOS arm64 | 7,550,546 bytes |
| Linux amd64 | 8,061,090 bytes |
| Linux arm64 | 7,405,730 bytes |

Linux amd64 产物在 `golang:1.26` Linux 容器中实际执行 `mob help --json` 成功。

## 未完成的真实验证与风险

以下项目因当前测试机或项目条件不具备而未执行，不能据此宣称 Android 全生命周期已完成：

| 项目 | 原因与后续验证 |
| --- | --- |
| Android 真机 run/debug/mirror | 没有已连接并完成 USB/Wireless debugging 授权的实体设备；需连接真机后验证 install、启动、logcat、JDWP 和 scrcpy。 |
| Android 模拟器 run/debug | 尚未安装系统镜像、创建并启动 AVD；需安装一项 API 镜像，验证自动创建、启动、选择和运行。 |
| 原生 Android build/run/test/release | 本机没有 Gradle，`mob android create` 正确报 `MOB_TOOLCHAIN_MISSING`；需以含 Gradle Wrapper 和签名配置的真实项目验证。 |
| Flutter/FVM 生命周期 | 本轮未下载 Flutter/FVM 或运行真实 Flutter 项目；需分别验证普通 Flutter 和 `.fvmrc` 项目。 |
| iOS | Windows 不是支持 Xcode 的宿主机；当前错误边界已验证，需在 macOS + Xcode + Simulator/真机测试。 |
| HarmonyOS | 当前版本只保留稳定命名空间，尚未实现 DevEco SDK 工作流。 |

## 后续建议

1. 增加 CI 中的命令矩阵测试：所有公共命令至少覆盖文本帮助、`--json` 成功/失败契约和 `--json=events` 的 stdout 纯 NDJSON 断言。
2. 增加可选的 Android 集成测试 Job：安装 API 镜像、创建 AVD、启动后对样例 Gradle 项目执行 build/run/test/logs；该 Job 应使用缓存并允许手工触发，以控制下载和运行时间。
3. 为 `mob android create` 决定首用策略。当前要求本机 Gradle 才能生成 Wrapper，错误清晰但第一次使用仍有门槛；可评估托管 Wrapper 模板或明确提供 Gradle 安装入口。
4. 在 macOS 和 HarmonyOS 宿主机补充同等层级的真实工具链与设备验证后，再将对应平台标记为可交付。
