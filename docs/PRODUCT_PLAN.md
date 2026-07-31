# Mob 产品规范

## 0. 产品目标与交付范围

Mob 的长期产品目标是让开发者使用 VS Code 完成 Android、iOS 与 HarmonyOS 的日常开发，不必为每个平台手工拼装 SDK、NDK、JDK、设备桥接、模拟器和环境变量。平台官方工具仍然存在；Mob 的职责是按项目发现、安装、选择并交给正确的官方构建器。

同一个能力必须同时服务三类调用方：人在终端中的命令、VS Code 插件的可视化操作，以及 AI/CI 的机器调用。三者不得各自维护 SDK 路径、设备状态或环境变量，也不得依赖解析不稳定的人类输出。

当前版本以 Android 为唯一完整交付重点：Android SDK/NDK/JDK、Flutter Android、ADB、模拟器、真机、构建、运行、调试、测试、日志、发布构建和 VS Code 入口均属于本阶段范围。Android 是跨平台产品愿景的第一个完整适配器，而不是产品命名空间的终点。

iOS 与 HarmonyOS 仅保留稳定的平台命名空间和设计边界，不是当前跨平台交付承诺。后续版本分别基于 Xcode 与 DevEco 的官方能力接入，不复用 Android SDK/NDK，不绕过平台许可证、账号、证书或宿主机限制。

## 1. 产品定位

Mob 是移动开发环境管理 CLI，定位类似 `nvm`：发现已有工具链、安装缺失组件、管理多个版本，并把正确环境交给当前项目的官方构建工具。它不是新的 IDE，也不是新的构建系统；它是 VS Code、终端、AI 与官方移动端工具之间的统一入口。

Mob 不替代 Gradle、Flutter、FVM、Xcode、DevEco 或语言调试器，也不要求卸载 Android Studio。项目仍由自身的 Gradle Wrapper、Flutter/FVM 配置和官方构建入口维护。Android Studio 仍适合布局可视化编辑、Profiler 与专项诊断，但不应成为日常 build/run/debug 的前置条件。

```text
Mob：SDK、NDK、JDK、Flutter SDK、ADB、模拟器、设备、版本选择、按次环境注入、JSON 契约
项目：Gradle Wrapper、Flutter/FVM 项目版本、React Native、CocoaPods、DevEco 项目配置
VS Code：编辑、可视化操作、终端、任务与现有调试扩展接入
AI / CI：读取命令契约与结构化状态，执行受控工作流，不猜测环境或文本报错
```

## 2. 核心原则

- 零项目配置：Mob 不创建 `mob.yaml`，不向项目写入配置文件。
- VS Code 优先：每个已交付平台的日常开发路径必须可由 VS Code 集成终端或 Mob 扩展完成。Android 不以 Android Studio 为前置依赖；iOS 与 HarmonyOS 保留 Xcode、DevEco 和宿主机的官方要求。
- 项目优先：每次执行都从 Gradle、`pubspec.yaml`、`package.json`、Xcode、DevEco 等现有文件识别需求。
- 平台隔离：Android、iOS、HarmonyOS 的路径、缓存、工具和子进程环境彼此隔离。
- 按次注入：日常 `build`、`run`、`debug` 只给本次子进程注入工具链，不修改用户全局环境变量。
- 自动补齐：`build`、`run`、`debug` 缺少可自动安装的工具链组件时，补齐后继续执行；首次涉及 Android SDK 许可时，用户以 `--accept-licenses` 明确确认后继续。
- 显式切换：`mob <platform> ... use` 只更新 Mob 自己的默认选择；`build`、`run`、`debug` 再把它按次注入子进程，不改写用户级环境变量。
- 已有环境优先：自动发现已有工具链；仅非标准路径可通过 `mob <platform> ... import`（例如 `mob android sdk import`）手动导入，且不复制、覆盖或删除它们。
- 复用官方能力：编译、模拟器、签名、调试和设备桥接均调用平台官方工具。
- 机器可调用：每个公开工作流必须有可查询帮助、稳定错误码，以及不混入终端装饰内容的 `--json` 或 `--json=events` 契约。

## 3. 根目录与本机状态

默认 Mob 根目录是 `~/.mob`。Windows 上对应 `C:\Users\<用户名>\.mob`；macOS/Linux 上对应 `~/.mob`。

```text
~/.mob/
  toolchains/
    android/managed/sdk/
    flutter/
    harmony/managed/sdk/
  cache/downloads/
  logs/
  config.yaml
  backups/environment-variables.yaml
```

安装器首次安装时允许用户选择根目录；未选择时使用 `~/.mob`。用户选择其他位置时，Mob 保存自己的用户级根目录选择，不改写 `PATH`、`JAVA_HOME` 或 Android 环境变量。Mob 启动时优先读取显式 `MOB_HOME`（适合 CI 和临时覆盖），其次读取该用户选择，最后回退到 `~/.mob`。

```powershell
mob home set D:\mobile-tools
```

该命令只迁移 Mob 自己拥有的根目录，目标必须是空目录；成功后更新 Mob 的用户级根目录选择。`config.yaml` 是 Mob 根目录中唯一维护的工具链配置文件，保存已注册/托管工具链、当前选择、命令偏好和网络设置。

## 4. 项目识别与命令分发

Mob 在 `doctor`、`build`、`run`、`debug`、`test` 中自动识别当前项目：

- 原生 Android：`settings.gradle(.kts)`、`build.gradle(.kts)`、`AndroidManifest.xml`。
- 原生 iOS：项目根目录中包含 `project.pbxproj` 的 `.xcodeproj` 包。
- Flutter：包含 Flutter 配置的 `pubspec.yaml`，以及 `android/`、`ios/` 目录。
- React Native：`package.json` 中的 `react-native` 依赖和原生目录。
- Kotlin Multiplatform：Gradle Kotlin Multiplatform 插件和 Android/iOS target。
- 后续平台：依据官方项目描述文件增加适配器。

`mob build`、`mob run`、`mob debug` 自动选择对应的官方命令：

| 项目类型 | Build | Run | Debug |
| --- | --- | --- | --- |
| 原生 Android | Gradle Wrapper | Gradle + ADB | debug APK + JDWP 目标 |
| 原生 iOS | macOS 上的 Xcode `xcodebuild` | 后续 iOS 设备适配器 | 后续 iOS 调试适配器 |
| Flutter | 已配置的 Flutter/FVM 命令 | Flutter/FVM + 设备 ID | Flutter/FVM 官方交互式 debug runner |
| React Native | 项目官方脚本/CLI | 项目官方命令 + 设备 ID | 官方调试流程 |
| Kotlin Multiplatform | Gradle Wrapper | 对应 target 官方命令 | 对应调试目标 |

Flutter 项目的运行器按以下规则选择：项目存在 `.fvmrc` 时，Mob 不读取文件内容，只确保并调用 `fvm flutter`，具体 Flutter 版本仍由 FVM 决定；没有 `.fvmrc` 时优先使用已选定的 Mob Flutter SDK，其次复用系统 `flutter`；两者都不存在时，Mob 自动安装 Flutter stable 并继续原命令。

当前 Android CLI 已实现 Flutter Android 的官方命令分发：`mob build --platform android` 默认调用 `flutter build apk`，`mob run --platform android` 默认调用 `flutter run -d <Android 设备 ID>`；`.fvmrc` 存在时两者改为 `fvm flutter ...`。Mob 不读取或改写 `.fvmrc`；缺少普通 Flutter 或 FVM 时，会在未传 `--no-install` 的前提下准备对应的 Mob 托管启动器。

当前原生 iOS 构建适配器仅在 macOS 上可用。`mob build --platform ios` 对项目根目录中唯一的 `.xcodeproj` 调用 `xcodebuild -project <path> -configuration Debug build`，并只向该子进程注入已发现的 `DEVELOPER_DIR`；它不修改 Xcode 的全局选择、签名或项目文件。多个 Xcode 项目时，调用方必须使用 `-- xcodebuild ...` 明确传入官方命令。Flutter iOS、workspace、Simulator、真机运行、调试与发布构建仍由后续适配器覆盖。

当前 Android CLI 的 `mob flutter create <name> [--platforms android,ios]` 调用已发现 Flutter CLI 的官方 `flutter create`，仅接受合法的小写 Dart package 名；它生成标准 Flutter 文件，不创建 `mob.yaml` 或 Mob 私有构建配置。

Mob 只识别 `.fvmrc` 是否存在，用它选择 FVM 启动器，不写入、解析或替换其中内容。若系统没有可用 FVM，Mob 从默认官方来源 `pub.dev` 读取版本和 SHA-256，下载并校验对应 FVM 包归档后，再以 Mob 托管 Flutter 所附的 Dart 执行 `dart pub global activate --source path`；启动器和独立 `PUB_CACHE` 位于 `~/.mob/toolchains/fvm/<version>`。若没有可用于引导的 Dart，Mob 可先安装 Mob 托管 Flutter stable，但它只用于启动 FVM，绝不替代 `.fvmrc` 项目由 FVM 决定的 Flutter 版本。`mob fvm update` 才更新该启动器，绝不借更新 FVM 改动项目锁定的 Flutter 版本；系统 FVM 始终优先于 Mob 托管启动器。

`build`、`run`、`debug` 可在选项后以 `-- <command> [args...]` 转发用户指定的官方或框架命令。Mob 仍负责项目/平台校验、按次环境注入、缺失组件补齐和 `run` 的设备选择。没有 `--` 时，项目适配器会向已知官方运行器传入正确的设备参数；有 `--` 时，Mob 不猜测或追加框架私有参数，调用方必须使用下列占位符显式绑定目标：`{{mob.device.nativeId}}`、`{{mob.device.id}}`、`{{mob.platform}}`。Mob 在启动子进程前只替换这些完整参数 token，不经过 shell 展开；同时提供 `MOB_DEVICE_ID` 与 `MOB_DEVICE_NATIVE_ID` 供自定义工具读取。未识别项目或需要特殊命令时，例如：

```powershell
mob run --device android:emulator-5554 -- fvm flutter run -d {{mob.device.nativeId}}
```

## 5. 命令层级

全局命令处理跨平台工作流；平台命名空间处理该平台的 SDK 和专属工具。`mob list` 不存在，避免“列出什么”的歧义。

```text
mob status                              SDK、工具、设备、当前项目总览
mob version | --version | -V             输出当前可执行文件的嵌入版本；支持 --json
mob doctor [--platform <id>]            诊断本机和当前项目需求
mob help [<command>...] [--format text|markdown|json] [--json]
                                        查看命令、参数、示例和副作用；`--json`/`--format json` 供插件和模型调用
mob catalog [--platform <id>] [--component <kind>] [--refresh]
                                        汇总当前宿主机可安装的工具链与版本
mob home show|set <path>                查看或迁移 Mob 根目录；迁移目标必须为空目录
mob support bundle [--output <path>]    生成脱敏的环境与工具链诊断 ZIP，不覆盖已有文件

mob android doctor                      诊断 Android 工具链
mob android sdk list                    列出已发现/已管理的 Android SDK
mob android sdk available [--api <n>] [--refresh]
                                        列出可安装的 Platform、Build Tools、ADB/平台工具等组件
mob android sdk import --path <p> [--name <n>]
                                        高级：导入无法自动发现的既有 Android SDK
mob android sdk install <name> [--api <n>] [--package <id>...] [--accept-licenses] [--allow-external-write --yes]
                                        安装目录中指定的 Android SDK 组件
mob android sdk inspect <name>          查看 SDK、Build Tools、platform-tools/ADB
mob android sdk use <name>              持久化选择 Android SDK
mob android sdk remove <name> --yes     删除 Mob 托管 SDK（显式确认后）
mob android ndk list [--sdk <name>]     查看某 Android SDK 中的 NDK
mob android ndk available [--refresh]   列出可安装的 Android NDK 版本
mob android ndk install <version> --sdk <name> [--allow-external-write]
mob android ndk remove <version> --sdk <name> --yes
mob android emulator list
mob android emulator create [<avd-name>] [--image <package-id>] [--sdk <name>]
mob android emulator start <avd-name>
mob android emulator stop <android:emulator-id>
mob android emulator image available [--api <n>] [--refresh]
                                        列出可安装的 Android 系统镜像
mob android emulator image install <package-id> --sdk <name> --accept-licenses [--allow-external-write --yes]
                                        安装目录中指定的 Android 系统镜像
mob android device connect <host:port>  使用 ADB 连接无线设备
mob android device pair <host:port> --code <6-digits>
                                        配对 Android Wireless debugging；完成后仍使用设备显示的连接地址执行 connect
mob android proxy show|set|clear        配置仅注入 Android sdkmanager 安装子进程的 HTTP(S) 代理

mob java list|import|use                发现、注册并选择具名共享 JDK；构建时按项目选择版本
mob java available [--refresh]          列出官方可校验的 Temurin JDK 8、11、17、21
mob java install <8|11|17|21>           校验并安装 Mob 托管 Temurin JDK
mob java remove <name> --yes             删除指定的 Mob 托管 JDK
mob flutter list|available|install|use  管理 Mob 托管 Flutter SDK；Dart 随 Flutter 提供
mob flutter create <name>               自动准备 Flutter 后调用 flutter create
mob fvm status                          查看系统/托管 FVM 启动器及当前项目 `.fvmrc` 标记，不读取文件内容
mob fvm list|use|remove                 列出、选择或删除 Mob 托管 FVM 版本；删除需 `--yes`
mob fvm available [--refresh]           列出 pub.dev 官方目录中的可校验 FVM 版本
mob fvm install [--version <v>]         校验 FVM 包归档并安装到 Mob 隔离的 PUB_CACHE
mob fvm update                          安装并选择当前官方 FVM 版本，不改动项目 `.fvmrc`

mob android create <name> [--language kotlin|java] [--ui compose|views] [--min-sdk <n>]
                                        创建标准原生 Android Gradle 项目

mob ios doctor                          只读检查活跃 Xcode Developer Directory、Xcode 与 Build 版本；非 macOS 返回 `MOB_HOST_UNSUPPORTED`
mob ios simulator start ios:<udid>      启动已有 Simulator，并打开 Xcode 官方 Simulator 窗口
mob harmony doctor                      保留命名空间：暂返回 `MOB_PLATFORM_NOT_SUPPORTED`

mob device list [--platform android|ios]
                                        统一列出 ADB Android 真机/模拟器，以及 macOS 上的 Xcode iOS Simulator
mob device use <platform:native-id>     选择默认运行设备
mob device open android:<id>            唤醒 Android 模拟器，或自动准备 Mob 真机预览运行时并打开实时窗口
mob device screenshot [android:<id>] [--output|--out <path>]
mob device ui-tree [--device android:<id>] --json
mob device wait --boot|--idle
mob device input tap <x> <y>
mob device input swipe <x1> <y1> <x2> <y2> --duration <ms>
mob device input text <value>
mob device input key <keycode>
                                        通过 ADB 保存真机或模拟器 PNG 截图
mob device record android:<id> [--output <path>] [--seconds <1-180>]
                                        通过 ADB 录制并保存 MP4
mob device mirror <id>                  真机实时镜像（首次自动准备 Mob 内部运行时）
mob device forward remove android:<id> --port <n>
                                        移除 `mob debug` 创建的本机 ADB JDWP 转发

mob env show                            查看 Mob 当前选择及仅对子进程注入的环境变量
mob build [--platform <id>] [--no-install] [--accept-licenses] [-- <command> [args...]]
                                        自动补齐环境并调用官方构建工具，或转发指定命令
mob run [--platform <id>] [--device <id>] [--mirror] [--headless] [--no-install] [--no-device-create] [--accept-licenses] [-- <command> [args...]]
                                        自动补齐环境，启动实时预览并运行项目
mob debug [--platform <id>] [--device <id>] [--mirror] [--headless] [--no-install] [--no-device-create] [--accept-licenses] [-- <command> [args...]]
                                        自动补齐环境并启动调试目标，或转发指定命令
mob test [--platform <id>] [--no-install] [--accept-licenses] [-- <command> [args...]]
                                        调用项目官方测试入口
mob logs [--device <id>] [--follow]     查看当前应用日志；`--follow` 实时输出日志，使用 `--json=events` 输出 JSON Lines 事件
mob release [--platform <id>] [--artifact <type>] [--output <path>] [--no-install] [--accept-licenses]
                                        执行项目已配置的官方签名发布构建
mob release check [--platform <id>]     签名前和发布前检查
```

所有查询和有限工作流命令支持 `--json`，提供一个终态的稳定结构化结果给 VS Code 插件、CI 和 AI 工具；需要持续输出的命令使用 `--json=events` 提供 JSON Lines 事件流。普通用户不需要使用或理解这些参数。

普通终端模式下，Mob 将阶段提示、可确定总大小的下载进度条和外部工具状态写入标准错误；标准输出保留给命令结果。Mob 自身阶段使用产品主色，`sdkmanager`、Gradle、Flutter、Xcode 等官方工具输出统一显示为灰色，并在交互式终端内使用固定三行视窗滚动刷新最近细节，避免长任务刷屏。重定向或 CI 环境自动降级为无颜色、无光标控制的逐行日志。Mob 不转发 Windows 批处理命令回显或完整许可证正文。`--json` 和 `--json=events` 禁用这些人类终端 UI，确保标准输出始终可直接由插件、CI 和 AI 解析。

当前 CLI 的 `mob env show` 是只读诊断：返回 Mob 根目录、当前 Android SDK/JDK/Flutter、默认设备以及 `child-process-only` 范围。Mob 不持久化改写系统 PATH、`JAVA_HOME` 或 Android 环境变量，因此不提供会误导用户的 `env restore`；构建和运行只在被启动的官方子进程中注入所需变量。

### 5.1 可安装目录与帮助

`list` 永远回答“本机已经发现、注册或由 Mob 托管了什么”；`available` 永远回答“按当前宿主机、平台许可和已配置来源，现在可以安装什么”。两者绝不混用。`mob catalog` 是所有 `available` 结果的汇总视图，适合首次安装或 VS Code 的工具链面板；平台命名空间下的 `available` 命令是精确筛选视图，适合脚本和安装前选择。

当前 Android CLI 的 `mob catalog [--refresh]` 汇总 Android Repository 中可安装的 SDK 组件、NDK、系统镜像以及 Eclipse Temurin JDK，并保留来源、刷新时间、缓存状态、包 ID、版本、大小和校验信息；`--json` 为 VS Code 和 AI 提供单一目录入口。

Mob 在 `status`、`doctor`、`build/run/debug` 中自动发现 Android Studio 默认 SDK 目录、`ANDROID_HOME`/`ANDROID_SDK_ROOT`、已安装的 Xcode、DevEco 和常见 Flutter 路径，并在 `list` 中标记为 `discovered`。因此普通用户不需要手动登记 SDK。`mob android sdk import --path <p> [--name <n>]` 仅面向外接磁盘、企业共享目录或其他无法自动发现的非标准 SDK 根目录；它先验证目录，再保存一个不拥有该文件的 `imported` 引用。若未给 `--name`，Mob 从目录和来源自动生成唯一名称。

当前 Android CLI 的 `mob flutter list` 已提供系统 `flutter`、`fvm` 启动器与 Mob 托管 Flutter 的只读发现，供脚本和 VS Code 使用；`mob flutter use <version>` 选择一个已安装的 Mob 托管版本，不修改系统 PATH 或项目文件。

当前 Android CLI 的 `mob flutter available [--refresh]` 已从 Flutter 官方、按当前宿主机区分的 release 目录列出 stable SDK 版本、归档路径与 SHA-256，并缓存已验证目录供离线重用；`mob flutter install [--version <v>]` 负责安装，随后可通过 `mob flutter use` 切换。

当前 Android CLI 的 `mob flutter install [--version <v>]` 从该官方目录选择 current stable 或指定版本，校验归档 SHA-256 后原子发布到 `~/.mob/toolchains/flutter/<version>` 并登记为 Mob 托管 Flutter。已登记且文件完整的相同版本会直接复用，不会重复下载。当前仅接受官方目录为 ZIP 的宿主机归档；其他格式会明确拒绝而不创建不完整安装。

`mob flutter use <version>` 只切换 Mob 配置中的当前托管 Flutter 引用，不修改系统 PATH 或项目文件；后续非 FVM Flutter 项目 build/run/test/release 会优先使用该版本。当这些标准工作流或 `mob flutter create` 找不到普通 Flutter 时，默认自动下载并使用经 SHA-256 校验的 current stable；`--no-install` 会禁止这一步。含 `.fvmrc` 的项目始终交给现有 FVM 启动器，Mob 不读取、猜测或改写其版本配置。

`mob flutter remove <version> --yes` 仅删除严格位于 `~/.mob/toolchains/flutter/<version>` 的托管 SDK 和对应配置；系统 Flutter、FVM 和任何外部目录都不会被 Mob 删除。

可安装项必须提供稳定的组件 ID、版本、平台、来源、宿主机兼容性、所需许可、下载大小、校验值与算法、是否已缓存与目录刷新时间。当前 Android 目录包含 Platform、Build Tools、Command-line Tools、platform-tools/ADB、Emulator 和系统镜像；Android 官方 Repository 当前对部分归档只声明 SHA-1，Mob 必须如实返回 `checksumAlgorithm` 与 `checksum`，不能伪造为 SHA-256；NDK、JDK、Flutter 与 FVM 分别返回其可安装版本。HarmonyOS SDK 和 Xcode 目录属于后续平台适配器范围；Mob 不重分发 Xcode。

默认使用最近一次经过校验的本地目录缓存；传入 `--refresh` 才查询官方目录。离线时返回缓存并标注 `stale` 与刷新时间；没有缓存也无法联网时返回 `MOB_CATALOG_UNAVAILABLE`，不伪造可安装版本。`install` 只接受目录中返回的组件 ID/版本，随后仍会执行许可确认、哈希校验和原子安装。

`mob help` 不触发项目构建、设备操作、下载或环境变量修改。人类默认获得简洁帮助；`--format markdown` 用于可读文档；`--format json` 或通用 `--json` 用于 VS Code、CI 和大模型，二者都使用稳定的 JSON 事件信封。机器帮助返回 CLI 版本、完整命令路径、用途、从 CLI 用法推导的位置参数和选项语法、已知前置条件、副作用、支持平台、示例、相关命令和可能错误码。模型应先调用 `mob help <command> --json` 或 `mob catalog --json` 再生成命令，不应依赖终端文案或自行假设版本。

`android`、`ios`、`harmony` 都是平台命名空间，不是环境代号。工具链环境名称由用户或安装命令确定：

```powershell
mob android sdk import --path "E:\shared-tools\Android\Sdk" --name shared
mob android sdk install managed --api 35
mob android sdk use managed
```

上述两个 Android 环境的完整内部标识分别是 `android:shared` 与 `android:managed`。这样同一平台可注册多套 SDK，而不会把平台名误当作环境名。

## 6. Android 完整能力

### 6.1 创建原生 Android 项目

`mob android create` 面向没有 Android Studio 的 VS Code 用户创建原生 Android 项目：

```powershell
mob android create notes --language kotlin --ui compose --min-sdk 24
```

Android 官方没有覆盖所有工程形态的统一脚手架 CLI，因此 Mob 使用随版本发布、可审计的 Mob 标准 Gradle 模板。当前 Android CLI 生成普通的 Android Gradle 工程、标准目录、Kotlin/Java Activity 与可选最小 SDK，并使用系统 Gradle 或 Mob 安装的、经官方 SHA-256 校验的 Gradle 发行版生成项目 Gradle Wrapper。缺失时 Gradle 安装到 `MOB_HOME/toolchains/gradle` 并注册到用户命令路径，以供其他项目复用；Windows 安全更新用户 PATH，不使用可能截断 PATH 的 `setx`。生成后的项目始终使用自身 Wrapper。不会生成 `mob.yaml`、运行时依赖或私有构建插件。生成后项目由 Gradle 和 Android 工具链正常维护，Mob 仅在后续 `build/run/debug` 时识别并提供环境。

Compose 模板仅接受 Kotlin，且会生成匹配 Kotlin 版本的 Compose 编译插件配置；Java 模板只能使用 Views，避免产生不可构建的项目组合。

### 6.2 SDK、NDK 与 JDK

一个 Android SDK 根目录可同时拥有 API 27、34、35 等 Platform 和多套 Build Tools；不需要为每个 API 复制完整 SDK。

```powershell
mob android sdk install managed --api 27 --package "build-tools;27.0.3"
mob android sdk install managed --api 34 --package "build-tools;34.0.0"
mob android ndk install 27.2.12479018 --sdk managed
```

每次 `build/run/debug` 解析项目 Gradle 配置，匹配 `compileSdk`、`minSdk`、Build Tools、AGP、Gradle、JDK 和 `ndkVersion`。当前 Android 实现读取显式数字 `compileSdk`、`buildToolsVersion`、`ndkVersion` 以及 `sourceCompatibility`、`targetCompatibility`、JVM toolchain Java 版本；动态 Gradle 表达式不会被猜测。缺少 Android SDK 组件时，`mob build/run --accept-licenses` 会安装到 `android:managed`；缺少 JDK 时会安装经 SHA-256 校验的 Temurin 8、11、17 或 21 到 Mob 托管目录；`--no-install` 对两者都会返回精确缺失错误。

`mob android sdk available` 返回的每个组件都有可直接传给 `--package` 的规范包 ID。`--api <n>` 是安装对应 Platform 的简写；需要精确指定 Build Tools、Command-line Tools、platform-tools、Emulator 或多个组件时使用一个或多个 `--package <id>`。系统镜像由 `mob android emulator image install <package-id> --sdk <name>` 安装，包 ID 同样来自 `image available`。不在当前目录中的包 ID 返回 `MOB_PACKAGE_NOT_AVAILABLE`，而不是猜测相近版本。

JDK 8、11、17、21 可以并存。当前 Android CLI 的 `mob java list` 会发现 `JAVA_HOME`、PATH 与已注册 JDK，`mob java import --path <jdk-root> --name <name>` 会验证 `bin/java` 和版本后仅保存外部引用，`mob java available [--refresh]` 从 Eclipse Temurin 官方目录返回可校验的宿主机包，`mob java install <8|11|17|21>` 校验 SHA-256 后原子安装到 `~/.mob/toolchains/java`，`mob java remove <name> --yes` 仅删除该目录内的受管 JDK，`mob java use <name>` 只选择 Mob 的默认 JDK。构建、运行、测试、调试和发布会将选中的 JDK 以本次子进程的 `JAVA_HOME`/PATH 注入，不修改系统环境变量；显式 `sourceCompatibility`、`targetCompatibility` 或 JVM toolchain 版本会优先选择同版本 JDK。当前安装器支持官方 ZIP 归档的宿主机。

Gradle Wrapper 是 Android 项目的权威入口，Mob 优先调用项目中的 `gradlew`/`gradlew.bat`，不强制安装全局 Gradle。Maven 由 Gradle 仓库配置管理；Mob 只诊断代理、镜像、缓存和下载故障。

Android SDK、Android NDK、platform-tools、Emulator 和系统镜像都通过同一个 Android Repository 源适配器检索；Android NDK 的规范包 ID 是 `ndk;<version>`，不是独立于 Android SDK 的通用 NDK。`mob android ndk available/install` 必须使用当前选择的 Android 源，安装到指定 Android SDK 根目录，并校验该仓库声明的包校验和。

自动发现或 `import` 的外部 SDK 一律只读：Mob 不会在 `run/build/debug` 自动向 Android Studio、外接盘或企业共享 SDK 写入组件。当前项目缺失组件时，Mob 创建或复用 `android:managed` 并仅为本次子进程选择它；已发现 SDK 完整兼容时才直接复用。只有用户在针对外部名称的 SDK、NDK 或系统镜像安装命令中显式传入 `--allow-external-write`，且在交互式确认目标目录后，Mob 才会修改外部 SDK。这样自动补齐不污染用户已有环境，也保留高级用户集中维护既有 SDK 的选择。

### 6.3 设备、模拟器与 ADB

`mob device list` 提供统一设备模型：

```text
ID:       android:emulator-5554
Platform: android
Kind:     emulator
Name:     Pixel 8 API 35
State:    ready
```

ID 始终是 `<platform>:<native-id>`，避免跨平台冲突。对多 target 项目，所有平台敏感工作流先决定目标平台：显式 `--platform` 优先；`run/debug/test` 在未显式指定时可使用显式 `--device` 的平台，再使用兼容的全局默认设备平台；若项目只声明一个平台则使用该平台。`build/release` 不从设备推断平台，多 target 项目未传 `--platform` 时返回 `MOB_PLATFORM_REQUIRED`。`run/debug/test` 既未指定平台/设备、也没有兼容默认设备时同样返回该错误，不任意选取另一平台的设备。显式 `--platform` 与 `--device` 的平台前缀不一致时返回 `MOB_PLATFORM_DEVICE_MISMATCH`。平台确定后，`mob device use` 保存的全局默认设备和 `mob run/debug/test` 按以下顺序选择具体目标：

1. 使用命令显式传入的 `--device <id>`，并校验它已连接、状态为 ready 且与请求平台兼容。
2. 未显式指定时，使用全局默认设备；默认设备不在线或不兼容时输出原因并继续查找。
3. 未配置可用默认设备时，使用第一台状态为 ready 且与请求平台兼容的设备。
4. 没有任何可用设备且未显式指定 `--device` 时，创建并启动一个与目标平台和项目要求匹配的 Mob 托管默认模拟器。
5. 显式指定的设备不存在、未连接或不兼容时立即报错，避免替用户猜测其他设备。

```text
Error: MOB_DEVICE_UNAVAILABLE
Requested device: android:emulator-5554
Remediation: mob device list
```

用户可随时通过 `mob device use <id>` 更换全局默认设备，或在单次命令中覆盖：

```powershell
mob device use android:emulator-5554
mob run
mob run --device android:R58N123456A
```

当前实现仅为 Android 自动创建默认 AVD：它使用项目 `compileSdk` 对应的系统镜像、与宿主机匹配的 ABI 和 Google APIs 镜像，名称形如 `mob-android-api-35`。Mob 只会复用同名、同 API 的默认 AVD，不会自动选择用户已有的其他 AVD；缺少的 Emulator、系统镜像和 ADB 会先自动安装。创建成功后设备被保存为 Mob 全局默认设备并启动预览窗口。iOS 与 HarmonyOS 的自动设备创建属于后续平台适配器范围；当前保留诊断命令会明确返回宿主系统或平台未支持错误。

`--no-device-create` 禁用自动创建设备，适用于 CI 或只允许使用已连接真机的流程。

Android 通过 ADB 管理 USB 真机、无线真机和 Emulator。ADB 是 Android SDK 的 `platform-tools` 组件，在 `mob android sdk inspect` 中展示，不作为独立 SDK 管理。

Mob 启动 Android Emulator 的官方原生窗口，直接提供实时画面、触控、键盘、定位、网络、摄像头和录屏等能力。Mob 不在 VS Code Webview 中重做模拟器。对于 Android 真机，`mob device mirror` 是 Mob 内置的预览能力；首次调用时会自动准备所需运行时。iOS/HarmonyOS 仅在官方或受支持能力可用时提供镜像。

当前 Android CLI 的 `mob device mirror <android:native-id>` 要求目标是真实、ready 的 ADB 设备。首次使用时 Mob 从 Genymobile 官方 GitHub Release 获取 Windows x64 预览运行时，校验 Release 元数据提供的 SHA-256，并解压到 `~/.mob/runtime/scrcpy`；它不会依赖或修改系统 PATH，也不要求用户单独安装或管理 `scrcpy`。运行时由 Mob 以 `--serial <native-id>` 启动低延迟镜像和输入窗口。它不对 Emulator 启动镜像，避免替代官方 Emulator 窗口。

`mob run --mirror --device android:<native-id>` 复用相同规则：在构建与启动应用前打开真机镜像窗口，并在 `--json` 事件流中发送 `preview` 事件。对 Emulator 使用该选项会直接报错，用户应使用官方 Emulator 窗口。

当前 Android CLI 的 `mob run --headless`（以及 `mob debug --headless`）在没有可用设备而需要自动启动 AVD 时，将官方 Emulator 以 `-no-window` 无窗口模式启动；若设备已经连接或 Emulator 已经运行，该选项不改变现有设备窗口。

当前 Android CLI 的 `mob doctor` 会诊断选定 SDK 的 `platform-tools`/ADB 与 Emulator 包（它们是运行能力的可选检查，真机流程不要求 Emulator）；原生 Android 项目还会检查 Gradle Wrapper。它在原生 Android 或 Flutter Android 项目中额外读取显式数字 `compileSdk`、`buildToolsVersion` 和 `ndkVersion`，并判断已发现 SDK 是否满足这些要求；缺少匹配组件时给出精确安装建议，不会修改项目文件。

### 6.4 构建、运行、调试、日志与发布

原生 Android 的 `mob build` 调用 Gradle Wrapper；`mob run` 构建、ADB 安装并启动 Activity；`mob debug` 执行 debug 构建，通过 `adb shell am set-debug-app -w` 启动应用，找到 JDWP PID 后创建随机本机回环 ADB 转发，并返回设备 ID、包名、PID 与 `127.0.0.1:<port>` 调试端点。`mob device forward remove android:<id> --port <n>` 只移除指定的 ADB 转发，不影响应用或设备。VS Code 读取该结构后交给 Java/Kotlin 调试扩展建立调试会话，Mob 不自行实现调试器。

`mob run` 默认是交互式实时预览流程：

```text
解析项目和设备
  -> 未运行的目标模拟器自动启动并等待 boot complete
  -> 将官方模拟器窗口前台展示
  -> 补齐缺失工具链，构建、安装并启动应用
  -> 保持运行会话，报告应用和设备状态给 VS Code 插件
```

当前 Android 实现使用 ADB 和系统启动完成状态等待刚启动的 Emulator，不会把应用安装到另一台已经就绪的模拟器；iOS 与 HarmonyOS 的对应启动和 boot 状态接口属于后续平台适配器范围。若目标是真机，Mob 直接安装运行，真机本身就是实时预览载体。

目标是模拟器时，`mob run` 默认弹出并聚焦官方原生窗口，用户可即时操作。目标是真机时，`mob run` 默认在手机上启动应用；传入 `--mirror` 后会弹出电脑端低延迟镜像窗口，并转发鼠标/键盘输入。VS Code 插件只聚焦这些原生窗口，不在 Webview 内重做视频/触控协议。`--headless` 关闭窗口展示，供 CI 或后台任务使用。

Flutter 的热重载由其官方交互式 `flutter run` 会话提供；Mob 不为原生 Android 声明 `--watch` 或 Flutter 式热重载。原生 Android 的后续文件监听能力必须先实现增量 Gradle 构建、ADB 安装和 Activity 重启，再公开为 CLI 参数。

Mob 不自行实现 Kotlin、Java、Dart 或 JavaScript 调试器。当前原生 Android `mob debug --json` 返回可附加的回环 JDWP 目标，VS Code 插件默认使用该目标请求已安装的 Java/Kotlin 调试扩展附加；用户可通过 `mob.autoAttachNativeDebug=false` 关闭自动附加，并从 Mob Output 手动使用该 endpoint。Flutter `mob debug` 保持官方 `flutter run`/`fvm flutter run` 的交互式调试会话和热重载；`mob debug --json=events` 会使用官方 `flutter run --machine`，从其 `app.debugPort` 事件返回 Dart VM Service WebSocket 目标，供 VS Code Flutter 调试扩展接入。该机器模式不能与 `-- <command>` 组合，避免 Mob 猜测或修改用户转发的官方命令。

`mob logs` 按当前设备和应用包名筛选平台日志。`mob release check` 检查版本号、签名配置、构建类型、平台要求与产物元数据；`mob release` 先执行这些检查，再调用项目已经配置的官方签名发布流程。它不生成、导入或保管密钥、keystore、Apple 证书、令牌或密码。

当前 Android CLI 的 `mob release` 默认执行原生 Gradle `bundleRelease`（或 Flutter `flutter build appbundle --release`），`--artifact apk` 切换到 APK 发布任务。Mob 只读取构建生成的产物，返回绝对路径、字节大小与 SHA-256；`--output` 仅复制最终产物。签名始终由项目已有的 Gradle 或 Flutter Android 配置负责，未产出对应 release 文件时返回错误。

当前 Android CLI 的 release 结果还包含版本：原生项目读取静态 `versionName`，Flutter 项目读取 `pubspec.yaml` 的 `version`。无法静态解析的 Gradle 动态表达式返回空版本值，Mob 不猜测或修改项目配置。


当前 Android CLI 的 `mob release check` 是只读诊断：原生项目检查兼容 SDK、Gradle Wrapper、显式 `applicationId` 与 Gradle release signingConfig；Flutter 项目检查兼容 SDK 和所需 Flutter/FVM 启动器。检查不读取密钥内容、不执行构建，也不会写入项目或系统环境。

当前 Android CLI 的 `mob logs [--device <id>]` 通过 ADB 获取当前项目正在运行应用的 PID，并以 `logcat -d --pid=<pid>` 返回该进程当前日志缓冲；它不会读取其他应用日志，也不改变设备状态。

`mob android sdk remove <name> --yes` 只接受路径严格位于 `~/.mob/toolchains/android/managed/sdk` 的 Mob 托管 SDK；它删除该目录和对应 Mob 配置。任何已发现或导入的 Android Studio、企业或外接 SDK 都会被拒绝，Mob 从不删除外部工具链。

`mob android ndk remove <version> --sdk managed --yes` 只删除 Mob 托管 Android SDK 中对应的 `ndk/<version>` 目录；版本参数不能包含路径分隔符，外部 SDK 或未安装版本都会被拒绝。

当前 Android CLI 的 `mob test` 在原生 Android 项目默认调用 Gradle Wrapper 的 `test` 任务，在 Flutter 项目默认调用 `flutter test` 或 `fvm flutter test`；和其他工作流一样，它只向本次子进程注入 Android SDK 环境，缺失组件仅会进入 `android:managed`。

`--artifact` 限制发布产物类型，`--output` 指定最终产物复制目录；未指定时采用项目官方默认发布产物。当前已实现 Android Gradle release 构建，默认产出已签名 AAB，可显式请求已签名 APK。iOS 的 Archive/Export IPA 与 HarmonyOS 的官方发布构建属于后续适配器能力，在实现前 `mob release` 明确返回 `MOB_PLATFORM_NOT_SUPPORTED`，不伪造产物或猜测签名。成功结果包含平台、产物绝对路径、大小、SHA-256 和项目版本；`--json` 还返回这些字段给 CI。

若项目没有声明请求平台 target，或平台不支持请求的产物类型，`mob release` 与 `mob build/run/debug` 一样返回 `MOB_PLATFORM_NOT_SUPPORTED`，不下载其他平台 SDK，也不猜测签名方式。

## 7. iOS 与 HarmonyOS

### iOS

iOS 构建、Simulator、签名和真机调试必须在 macOS/Xcode 环境中运行。Mob 只能发现、注册、选择和调用 Xcode；不尝试重分发或绕过 Xcode、Apple 账号、证书或许可证。

`mob ios doctor` 是当前可用的只读诊断入口：在 macOS 上调用 `xcode-select -p` 和 `xcodebuild -version`，返回活跃 Developer Directory、Xcode 版本和 Build 版本；它不会下载、选择、安装或接受 Xcode 许可证。Windows/Linux 明确返回 `MOB_HOST_UNSUPPORTED`。若 macOS 未安装 Xcode、未选中 Developer Directory，或 Xcode 许可证尚未接受，则返回 `MOB_TOOLCHAIN_MISSING` 并提示通过 Apple 官方渠道完成安装、选择与许可证流程。

原生 iOS 的 `mob build --platform ios` 已调用 Xcode；Simulator、真机、签名发布和调试适配器仍按后续平台能力逐项实现。

`mob device list --platform ios` 在 macOS 上通过 `xcrun simctl list devices available -j` 只读列出可用 iOS Simulator；Booted 设备统一标为 `ready`，其余状态保留为小写 Xcode 状态。未指定平台时，Mob 在可用时合并 Android ADB 和 iOS Simulator 列表。`mob ios simulator start ios:<udid>` 通过 `xcrun simctl boot`、`bootstatus -b` 和 macOS `open -a Simulator` 启动已有 Simulator 并展示官方实时窗口；它不创建或删除设备。Simulator 的关闭、设为默认 `mob device use`、截图、录屏、真机和 `mob run` 仍由后续 iOS 设备适配器实现。

### HarmonyOS

Mob 后续仅通过官方公开的 SDK、DevEco 和设备桥接能力发现、安装、调用和诊断 HarmonyOS 工具链。设备、模拟器、签名和构建能力根据安装版本及官方许可决定，不伪造跨系统支持。当前 `mob harmony doctor` 是保留入口，返回 `MOB_PLATFORM_NOT_SUPPORTED`。

HarmonyOS 的 Native/NDK 是 HarmonyOS SDK/DevEco 官方组件，不得复用 Android NDK。Mob 从项目现有 Native 配置推导所需组件，并通过当前选择的 HarmonyOS 源适配器获取；组件名称、版本约束和宿主机要求以该官方工具链目录为准。iOS 没有 Android/HarmonyOS 这种独立 NDK 概念，始终使用 Xcode 随附的 Apple 工具链。

## 8. 下载源、镜像、CI 与安全

- 未来若增加企业离线仓库或第三方镜像，必须使用“源适配器”而非一个通用下载 URL。每个适配器需要理解对应平台的目录格式、包 ID、主机约束、许可和完整性信息；这不是当前 CLI 命令：

| 组件 | 默认官方源适配器 | 镜像规则 |
| --- | --- | --- |
| Android SDK、NDK、ADB、Emulator、系统镜像 | Android Repository / `sdkmanager` 元数据 | 企业镜像必须保持 Android Repository 元数据和每个包校验和。 |
| Flutter SDK | Flutter 官方 release archive 目录 | 可镜像 release 归档与发布元数据，包版本和哈希不得改写。 |
| FVM | `pub.dev` | 仅接受与已固定 FVM 包版本及 SHA-256 相符的镜像包。 |
| JDK | Mob 支持的 JDK 发行版官方目录 | 企业镜像必须声明发行商、版本、平台和哈希。 |
| HarmonyOS SDK、Native/NDK、模拟器 | DevEco/HarmonyOS 官方 SDK 目录 | 仅使用与 DevEco 版本匹配的官方目录格式和签名。 |
| Xcode/iOS 工具链 | Apple 官方渠道与本机 Xcode | 不重分发 Xcode；只发现、诊断和调用官方安装结果。 |

- Android SDK、NDK、ADB、Emulator 和系统镜像默认只使用 Android 官方 Repository 与 `sdkmanager` 元数据；Flutter、FVM 和 JDK 也只使用各自的官方可校验目录。当前 Android CLI 通过 `mob android proxy set <http(s)://host:port>` 将 HTTP(S) 代理仅注入 Android 安装子进程和 command-line-tools 引导下载，不改写系统代理变量。代理 URL 不允许包含用户名、密码或 token。
- Mob 不提供泛化的“镜像 URL 替换”或下载源优先级命令。Android 的 `sdkmanager` 不支持将任意 Repository URL 当作安全的覆盖入口；仅记录一个镜像地址不会真正改变它的下载行为。企业离线仓库和第三方镜像需要各平台适配器、包哈希/签名校验和凭据存储后才会单独加入，不能伪装成当前可用能力。
- 下载使用 HTTPS、临时目录、校验和和原子写入；失败不得破坏可用 SDK。默认支持官方目录、可选代理和离线缓存。
- `mob <platform> ... import` 导入的外部工具链永远不被 Mob 删除；`remove` 只处理 `MOB_HOME` 下 Mob 托管内容。
- 任何安装、删除、持久化环境变量、录屏或镜像操作都明确显示目标与副作用。
- 同一 `mob doctor/build/test/release/release check` 可在本机和 CI 执行，CI 使用 `--json` 获取诊断与产物信息。
- `mob support bundle` 收集脱敏后的 Mob 版本、宿主机信息和已注册工具链清单；不读取项目文件，不收集环境变量、设备标识、日志、代理设置、密钥、token、密码或原始主机路径。当前支持包仅包含 `manifest.json` 与 `toolchains.json`，并且不会覆盖已有 ZIP。

## 9. `--json` 插件与自动化协议

`--json` 是 CLI 对 VS Code 插件、CI 和 AI 工具的机器接口，不要求消费者解析人类文本。无论命令是否为长任务，普通 `--json` 在标准输出都只输出一个最终 `completed` 或 `error` JSON 对象；只有显式传入 `--json=events` 时才输出 JSON Lines（一行一个完整 JSON 对象）的生命周期事件。支持该模式的长任务会在 `mob help <command> --json` 中以 `supportsEventStream: true` 和 `--json|--json=events` 用法标出。普通终端和 `--json=events` 都逐行实时转发官方 Gradle、Flutter 等子进程输出；事件模式以带 `stream` 和 `output` 的 `log` 事件提供。JSON 模式下 Mob 自己不向标准输出写入非 JSON 文本。`mob logs --follow` 的机器调用必须使用 `--json=events`，避免无界日志流伪装成单个结果。

每个事件采用以下稳定信封；字段可以新增，已有字段在同一 `schemaVersion` 主版本中不可重命名或改变含义：

```json
{
  "schemaVersion": "1.0",
  "event": "started | progress | log | device | preview | debugTarget | completed | error",
  "command": "mob run",
  "sequence": 1,
  "ok": true,
  "data": {},
  "error": null
}
```

`sequence` 从 1 开始，在一次命令执行内严格递增。有限命令发送一个 `completed`；长任务先发送一个 `started`，可发送任意多个 `progress`、`log`、`device`、`preview` 或 `debugTarget`，成功时以一个 `completed` 结束。`device` 报告选择、创建和就绪状态；`preview` 报告模拟器窗口或真机镜像状态；`debugTarget` 报告可交给调试扩展的 Android JDWP 或 Flutter Dart VM Service 端点。`completed.data` 放最终结果，例如设备 ID、调试端点或发布产物；`progress.data` 至少含 `phase`，可含 `percent`、`tool` 和可展示的 `message`。失败时发送一个 `error` 事件，并使用以下 `error` 结构：

```json
{
  "schemaVersion": "1.0",
  "event": "error",
  "command": "mob release",
  "sequence": 4,
  "ok": false,
  "data": {},
  "error": {
    "code": "MOB_PLATFORM_NOT_SUPPORTED",
    "message": "The current project does not declare an Android target.",
    "remediation": "Run mob status, or choose a platform declared by the project."
  }
}
```

错误码是面向自动化的稳定标识，文案和修复建议可本地化：

| 错误码 | 含义 |
| --- | --- |
| `MOB_PLATFORM_NOT_SUPPORTED` | 项目未声明请求的平台或该平台不支持请求动作/产物。 |
| `MOB_PLATFORM_REQUIRED` | 多 target 项目未能从参数、设备或默认设备确定运行平台。 |
| `MOB_PLATFORM_DEVICE_MISMATCH` | `--platform` 与 `--device` 指向不同平台。 |
| `MOB_DEVICE_UNAVAILABLE` | 显式请求的设备不存在、不在线或不兼容。 |
| `MOB_HOST_UNSUPPORTED` | 当前宿主系统不能运行该平台能力，例如非 macOS 的 iOS 构建。 |
| `MOB_TOOLCHAIN_MISSING` | 必需工具链不可用，且不能自动安装或用户传入 `--no-install`。 |
| `MOB_LICENSE_REQUIRED` | 安装继续前需要接受官方许可。 |
| `MOB_CATALOG_UNAVAILABLE` | 无法刷新可安装目录，且没有可用的本地缓存。 |
| `MOB_PACKAGE_NOT_AVAILABLE` | 请求安装的组件包 ID 不在当前可用目录中。 |
| `MOB_SOURCE_INVALID` | 下载源不符合对应平台目录格式，或包映射、哈希、签名校验失败。 |
| `MOB_SOURCE_AUTH_REQUIRED` | 所选企业镜像需要凭据，但操作系统凭据存储中没有可用凭据。 |
| `MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED` | 试图修改发现/导入的外部工具链，但未显式允许并确认。 |
| `MOB_RUNNER_UNAVAILABLE` | 识别到项目，但对应官方运行器不可用，例如 Flutter/FVM 或项目 CLI 缺失。 |
| `MOB_PROJECT_UNRECOGNIZED` | 无法从当前目录识别受支持项目。 |
| `MOB_COMMAND_FAILED` | 官方构建、设备或框架命令执行失败。 |
| `MOB_RELEASE_CONFIGURATION_INVALID` | 发布所需签名、版本或项目配置不完整。 |
| `MOB_INVALID_COMMAND` | 命令路径、选项组合或位置参数不符合 CLI 契约。 |
| `MOB_INVALID_ARGUMENT` | 选项值格式或取值无效。 |
| `MOB_INTERNAL_ERROR` | Mob 自身遇到未分类的内部错误；调用方应保留事件并报告。 |

插件根据 `type`、`data` 和 `error.code` 渲染状态、进度与修复操作，不能依赖 `message` 内容、控制台颜色或特定日志行。人类可读模式仍保持简洁的终端输出。

## 10. VS Code 插件边界

插件使用 TypeScript 和 VS Code Extension API，只负责：

- 展示 `mob status`、`mob doctor`、SDK 与设备状态。
- 提供安装、设备选择、构建、运行、调试、日志的可视化入口。
- 读取 `--json` 输出，将调试目标交给现有调试扩展。
- 展示可执行修复建议和受控命令输出。

所有 SDK 安装、环境变量修改、ADB/模拟器调用、项目识别和构建分发逻辑均在 Go CLI 内实现，插件不复制业务逻辑。

当前仓库中的 `extensions/vscode-mob` 已提供首个可编译的插件实现。它在 Mob Activity Bar 中分组展示 `mob doctor --json` 的工具链检查、`mob status --json` 的 Android SDK/JDK/Flutter 状态与 `mob device list --json` 的统一设备列表；可在当前已打开的父目录中调用 `mob android create` 或 `mob flutter create` 创建标准项目，完成后由用户确认是否切换工作区。插件还可从 `mob android sdk list --json` 的已安装系统镜像创建 AVD，并从 `mob android emulator list --json` 选择启动的 AVD；运行中的 Emulator 可在设备树中停止。无线真机可先通过 Android Wireless debugging 的配对地址和六位码执行 `mob android device pair`，再使用设备显示的独立连接地址执行 `connect`。插件不会自行扫描 SDK 目录或直接调用 AVD/Emulator/ADB 二进制。它还可选择默认设备、通过 ADB 连接无线设备、打开原生设备窗口、显示 CLI 输出，并在工作区切换时按 `mob.autoRefresh` 刷新；若 ADB/设备查询不可用，设备面板显示具体失败原因而不误报为“无设备”。状态栏区分“环境已就绪”“需要配置”和 CLI 不可用，避免把找不到 `mob` 误显示为环境缺失。插件还会读取 `mob catalog --json`、`mob flutter available --json` 与 `mob fvm available --json`，只让用户选择 CLI 返回的官方目录组件；安装前明确显示来源、版本、体积（若目录提供）和校验算法/哈希，且 Android SDK/NDK/系统镜像安装必须经用户确认后才传入 `--accept-licenses`。`mob.path` 指向 CLI 可执行文件或命令名，默认 `mob`。

“Run Project”和“Follow Project Logs”在 VS Code 集成终端分别执行 `mob run` 与 `mob logs --follow`，保留 Gradle/Flutter/日志的交互式实时输出。Build、Test、Create Release 和工具链安装消费 `--json=events`，将 `started`、`progress`、`log`、`completed` 与结构化错误写入 Mob Output。“Start Debug Session”消费 `mob debug --json=events`，将 `log` 事件写入 Mob Output，并显示 `debugTarget` 返回的 JDWP 或 Dart VM Service 端点；对原生 Android 回环 JDWP endpoint，插件默认请求已安装的 Java/Kotlin 调试扩展附加，可用 `mob.autoAttachNativeDebug=false` 关闭。插件会在它启动的 Java 调试会话结束后调用 `mob device forward remove` 清理对应 ADB 转发；附加失败时同样清理。插件不伪造调试器，Java/Kotlin 或 Flutter 扩展仍拥有实际断点、变量和附加过程。停止命令会停止插件启动的事件流，并结束由 Mob 请求的原生 Java 调试会话。

插件还注册 VS Code 原生 `type: "mob"` Task Provider，因此用户可从 “Run Task” 或工作区 `.vscode/tasks.json` 调用 `build`、`run`、`test`、`release` 与 `logs`。任务定义不是任意命令转发：`command` 必须是上述工作流之一，且只接受 CLI 已有的受控字段。`platform`、`noInstall` 和 `acceptLicenses` 可用于构建类任务；`run` 额外支持 `device`、`mirror`、`headless`、`noDeviceCreate`；`release` 额外支持 `artifact` 和 `output`；`logs` 只支持 `device` 和 `follow`。例如：

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Mob: Run Android",
      "type": "mob",
      "command": "run",
      "platform": "android",
      "device": "android:emulator-5554"
    },
    {
      "label": "Mob: Release Android AAB",
      "type": "mob",
      "command": "release",
      "platform": "android",
      "artifact": "aab"
    }
  ]
}
```

`debug` 不是 Task Provider 工作流：它必须消费 `debugTarget` JSON 事件，并交由 Java/Kotlin 或 Flutter 调试扩展继续附加，用户应使用插件的 “Start Debug Session” 命令。

## 11. 典型用户场景

### 场景 A：已有 Android 环境，打开现有项目开发

用户已安装 Android Studio、Android SDK 和一台 USB 真机，在 VS Code 打开原生 Android 项目：

```powershell
mob doctor
mob device list
mob device use android:R58N123456A
mob run
```

1. `mob doctor` 读取项目 Gradle，并发现系统已有 JDK、SDK、ADB，报告项目所需 API/Build Tools。
2. `mob device list` 通过 ADB 列出真机，`mob device use` 选择默认运行设备。
3. `mob run` 临时注入正确的 SDK/JDK，调用项目 Gradle Wrapper 构建 debug APK，通过 ADB 安装并启动应用。

### 场景 B：拿到旧项目，机器没有 Android 环境

用户在 Windows/VS Code 克隆一个要求 API 27、JDK 8 的旧项目：

```powershell
cd legacy-android-app
mob run
```

1. `mob run` 发现没有可用设备，也没有 Android SDK、API 27 系统镜像、Build Tools 和 JDK 8。
2. Mob 将这些组件从官方来源安装到 `android:managed` 和 Mob 托管 JDK；首次只要求确认 Android SDK 官方许可。
3. Mob 创建并启动 `mob-android-api-27`，将其设为默认设备并弹出实时预览窗口。
4. 环境和设备就绪后，原命令自动继续构建、安装并启动。用户不必手动设置 `JAVA_HOME`、`ANDROID_HOME` 或运行 ADB。

首次安装时用户执行 `mob run --accept-licenses`；之后 Mob 仍只向 `android:managed` 补齐缺失组件，已发现或导入的 SDK 永远不会被自动写入。

### 场景 C：从零创建 Flutter 项目

用户尚未安装 Flutter：

```powershell
mob flutter create travel_app
cd travel_app
mob run
```

1. `mob flutter create` 发现没有 Flutter，自动安装 Mob 托管 Flutter stable；Dart 随 Flutter 一起提供。
2. Mob 调用官方 `flutter create travel_app`，产物仍是标准 Flutter 项目，不额外创建 Mob 项目配置。
3. `mob run` 没有发现可用设备时，自动创建并启动匹配 API 35 的 Android 模拟器，同时补齐 Android SDK、ADB、系统镜像和 JDK。
4. Mob 识别 `pubspec.yaml`，调用 `flutter run` 并传入自动创建的设备；Flutter 负责 Dart 编译和 Android Gradle 构建。

### 场景 D：Flutter/FVM 项目在 VS Code 调试

用户打开已有 Flutter/FVM 项目，并在 VS Code 设备列表选中 Android 模拟器：

```powershell
mob device use android:emulator-5554
mob debug
```

1. Mob 发现 `.fvmrc`，选择 `fvm flutter` 作为启动器，但不读取或改写该文件。
2. 如果 FVM 启动器或 Android 环境缺失，Mob 自动准备所需组件；具体 Flutter 版本仍由 FVM 的项目配置决定。
3. Mob 启动 `fvm flutter run`（Flutter 官方默认 debug 模式），并复用 `mob run` 的设备选择、自动补齐和子进程环境。
4. VS Code 插件从该受控 Flutter 进程接入已安装的 Flutter 调试扩展；断点、变量和热重载仍由官方 Flutter 工具提供。

### 场景 E：新旧 Android 项目并行开发

同一机器同时存在 API 27/JDK 8 项目和 API 35/JDK 17 项目：

```powershell
cd C:\work\legacy-app
mob run

cd C:\work\modern-app
mob debug
```

1. Mob 分别解析两个项目的 Gradle，得出 API、AGP、Gradle、JDK、NDK 要求。
2. 缺少的 SDK Platform、Build Tools、NDK 或 JDK 自动补齐，并与已有版本共存。
3. 每个项目只在自己的子进程使用匹配 JDK/SDK，不改写另一个项目的执行环境。
4. 用户不需要在两个项目之间反复切换 SDK，也不需要手工维护多套环境变量。

## 12. VS Code 插件与设备预览计划

VS Code 插件是 Mob CLI 的图形入口，不应自行管理 ADB、SDK 或 scrcpy。所有设备连接、工具安装、权限校验和进程生命周期仍由 CLI 负责；插件只消费稳定 JSON 事件，并呈现结果。

### 第一阶段：当前 Android 交付

- 工具链、诊断、设备、模拟器、项目创建、build/run/test/release/logs 和调试入口。
- “Open Device Preview” 调用 `mob device preview serve android:<native-id> --json=events`，在 VS Code Webview 中打开同一套低延迟视频与控制会话；CLI 的 `mob device open` 仍保留给独立的官方窗口工作流。
- “Capture Device Screenshot” 调用 `mob device screenshot` 后在 VS Code 中打开 PNG。
- “Inspect Device UI” 调用 `mob device ui-tree --json`，在只读 Webview 中展示 UI Automator 层级；临时设备文件不暴露给插件。
- Task Provider 只声明当前已交付的 Android 工作流，不将 iOS/HarmonyOS 的预留命名空间伪装成可用功能。

### Mob Device Session Protocol

Mob 不将 scrcpy 作为跨平台协议。CLI、VS Code 和未来 IDE 统一采用版本化的 `mob.device.session.v1`：适配器返回平台、设备 ID、视频编码、短期本地 endpoint、一次性 token 和可用控制能力，客户端根据 `controls` 协商 UI。协议详细规定见 [MOB_DEVICE_SESSION_PROTOCOL.md](MOB_DEVICE_SESSION_PROTOCOL.md)。

Android 以 scrcpy/ADB 作为首个完整适配器；iOS 必须在 macOS 上通过 Apple 官方设备或 Simulator 服务实现，HarmonyOS 必须通过 HDC/DevEco 官方能力实现。平台可以只提供显示，此时仅声明 `close`，插件不会伪造可操作性。

### 已交付：CLI 本地预览服务

内嵌实时预览不使用 ADB 截图轮询。`mob device preview serve android:<native-id> --json=events` 会创建一次性的 Android 预览会话：

- 服务只监听 `127.0.0.1` 的随机端口，并使用加密随机令牌认证；令牌仅出现在发起命令的 JSON `preview` 事件中，绝不写入日志或配置。
- CLI 将 Mob 内置的 `scrcpy-server` 推送到设备，通过临时 ADB reverse 通道接收连续 Annex B H.264/AVC 帧；视频不经过截图轮询，也不受 `screenrecord` 的单段录制限制。Mob 只启用视频编码，设备输入仍由受管 ADB 执行。
- `GET /video?token=<token>` 是视频 WebSocket。先发送包含 `codec` 和 `format` 的 JSON 配置消息，随后发送二进制包：1 字节关键帧标志、8 字节大端微秒时间戳、H.264 Annex B access unit。该端点属于 `mob.device.session.v1`，而不是 Android 专属公开协议。
- `GET /control?token=<token>` 是控制 WebSocket。它接受 `tap`、`swipe`、`text`、`key` 和 `close` JSON 指令；前四种由 CLI 通过当前受管 SDK 的 ADB 执行，`close` 用于主动回收预览会话。不向 Webview 暴露 ADB 路径或进程。
- 服务缓存最近的 H.264 关键帧。客户端在视频已开始后加入时会立即得到可解码画面，不必等待下一轮 IDR 帧。
- 预览服务随 CLI 进程或上层客户端结束而回收；设备断开和流错误以标准 JSON 错误事件返回。协议独立于 VS Code，可供终端、自动化和未来 IDE 复用。

### 已交付：VS Code 内嵌预览

插件的 `Mob: Open Device Preview` 启动上述服务，在 `Mob Preview` Webview 中用 WebCodecs 解码 H.264，并以设备实际视频分辨率渲染。鼠标或触摸的点击与滑动会转换为同一会话的控制消息；面板同时提供文本输入、返回、主页和最近任务控制。令牌只在 Webview 的内存连接过程中存在，不写入工作区、设置或磁盘。

真机和 Android 模拟器使用同一协议。iOS Simulator 与 HarmonyOS 只有在其官方工具提供等价、稳定的流和输入能力后接入；在此之前保持官方窗口或截图模式，不承诺内嵌控制。

## 13. 实施顺序

1. Go CLI 基座：`.mob` 状态、配置、稳定错误码/JSON、命令路由、日志和安全子进程执行。
2. Android：SDK/NDK/JDK 发现和注册、受控安装、版本兼容诊断、环境注入。
3. Android：ADB、模拟器、统一设备模型、Gradle 构建/运行/测试/日志、JDWP 调试目标。
4. 项目分发器：原生 Android、Flutter、React Native、Kotlin Multiplatform 的自动识别和官方命令调用。
5. VS Code 插件：状态、设备、任务、日志和调试扩展接入。
6. iOS 与 HarmonyOS 适配器、签名发布构建、CI、远程 macOS 构建和企业镜像能力。
