# Mob Android 手动验证清单

本清单验证 Mob 跨平台产品愿景的首个完整适配器：在不安装完整 Android Studio、也不手动配置 VS Code Android 环境的前提下，开发者可从终端或 VS Code 使用标准 Android 工具链完成项目创建、构建、运行、调试和设备操作。iOS 与 HarmonyOS 的等价清单会在各自官方适配器交付后补充。

它用于在 Windows PowerShell 中验证一个实际构建的 `mob.exe`。所有 Android 下载均来自官方 `sdkmanager`；首次安装会要求显式传入 `--accept-licenses`。命令的 JSON 输出也应可被 VS Code、CI 与 AI 工具直接消费，而无需解析终端 UI。

## 准备

- [ ] 使用 Windows 10/11 x64，并预留至少 12 GB 磁盘空间（Android Emulator 与一个系统镜像通常需要数 GB）。
- [ ] 确认网络可访问 Android 官方 Repository；受限网络可先运行 `mob android proxy set <http(s)-proxy>`。
- [ ] 将以下路径替换为实际构建产物：

  ```powershell
  $mob = "C:\path\to\mob-windows-amd64.exe"
  ```

- [ ] 如需验证首次安装的下载进度，在新的 PowerShell 会话中指定一个不存在的空目录，而不是复用已有 `MOB_HOME`：

  ```powershell
  $env:MOB_HOME = "D:\mob-manual-validation"
  ```

## 1. CLI 与只读诊断

- [ ] 运行帮助；预期退出码为 `0`，显示命令分组：

  ```powershell
  & $mob help
  ```

- [ ] 运行诊断；预期明确列出 Android SDK、JDK、ADB 和 Emulator 的状态，而不是把缺失组件报成未知错误：

  ```powershell
  & $mob doctor --platform android
  & $mob status
  ```

- [ ] 验证机器接口；预期标准输出是一个完整 JSON 对象：

  ```powershell
  & $mob help android sdk install --format json
  & $mob android sdk list --json
  ```

## 2. 首次 SDK 下载与终端 UI

- [ ] 安装 `platform-tools`；预期 Mob 阶段使用主色，Command-line Tools 下载显示真实字节进度条（空 SDK 时），Android SDK 官方输出以灰色三行视窗滚动刷新；不应刷屏，也不应显示 Windows 批处理回显或完整许可证正文：

  ```powershell
  & $mob android sdk install managed --package platform-tools --accept-licenses
  ```

- [ ] 安装项目所需 API 与 Build Tools；预期最终显示已安装的包和 SDK 名称：

  ```powershell
  & $mob android sdk install managed --api 35 --package "build-tools;35.0.0" --accept-licenses
  & $mob android sdk list --json
  ```

- [ ] 验证 JSON 不混入终端 UI；预期 `stdout` 只有 JSON Lines，`stderr` 为空：

  ```powershell
  & $mob android sdk install managed --package platform-tools --accept-licenses --json=events 1> events.jsonl 2> events.stderr
  Get-Content events.jsonl
  Get-Content events.stderr
  ```

## 3. Emulator 与系统镜像

- [ ] 查看真实可安装镜像；预期 `source` 为 `sdkmanager`，且能列出 `system-images;android-35;google_apis;x86_64`：

  ```powershell
  & $mob android emulator image available --api 35 --json
  ```

- [ ] 验证普通终端表格；预期显示紧凑的 `API`、`IMAGE`、`ABI`、`REV` 列，不显示会换行的长 Package ID 与描述。安装时可用表格字段按页尾模板拼装 Package ID：

  ```powershell
  & $mob android emulator image available --api 35
  ```

- [ ] 安装 Emulator 和一个 API 35 Google APIs x86_64 镜像；预期显示阶段和受节流的安装进度，完成后 `sdk list` 包含 `emulator` 与该系统镜像：

  ```powershell
  & $mob android sdk install managed --package emulator --accept-licenses
  & $mob android emulator image install "system-images;android-35;google_apis;x86_64" --sdk managed --accept-licenses
  & $mob android sdk list --json
  ```

- [ ] 创建并启动 AVD；预期 Emulator 窗口启动，并在设备列表中显示 `android:emulator-*`：

  ```powershell
  & $mob android emulator create mob-api-35 --sdk managed --image "system-images;android-35;google_apis;x86_64"
  & $mob android emulator start mob-api-35
  & $mob device list --platform android
  ```

## 4. 真机

- [ ] 使用 USB 连接且已开启开发者选项与 USB 调试；预期设备显示为 `android:<serial>` 且状态为可用：

  ```powershell
  & $mob device list --platform android
  ```

- [ ] 启动真机实时预览；首次调用预期自动下载并校验 Mob 内部预览运行时，在 `MOB_HOME/runtime/scrcpy` 写入文件，然后打开可鼠标和键盘操作的窗口。后续调用不应再次下载，也不要求系统 PATH 存在 `scrcpy`：

  ```powershell
  & $mob device mirror android:<serial>
  & $mob device open android:<serial>
  ```

  该能力在当前版本需要 Windows x64、网络访问 Genymobile 官方 GitHub Release、少量本地磁盘空间，以及已经授权且状态为 `ready` 的 ADB 真机；不依赖 JDK、Gradle、Android Studio 或手工安装 `scrcpy`。

- [ ] 验证离线运行时放置：在网络断开的情况下，将完整的 Windows x64 `scrcpy` 官方压缩包解压到以下目录（可通过 `mob home show` 确认 `MOB_HOME`），然后重复预览命令。预期 Mob 直接启动窗口且不显示下载进度：

  ```text
  <MOB_HOME>\runtime\scrcpy\scrcpy.exe
  ```

  不要只复制 `scrcpy.exe`；必须保留压缩包内同级的 DLL、`scrcpy-server` 和其他运行文件。

- [ ] 可选：验证无线调试。配对地址和连接地址由 Android 的 Wireless debugging 页面分别提供：

  ```powershell
  & $mob android device pair 192.168.1.20:37123 --code 123456
  & $mob android device connect 192.168.1.20:5555
  & $mob device use android:192.168.1.20:5555
  ```

## 5. 原生 Android 项目周期

- [ ] 确认系统中已有 Java 17+ 和 Gradle（当前 `mob android create` 使用系统 Gradle 生成 Gradle Wrapper）：

  ```powershell
  java -version
  gradle --version
  ```

- [ ] 创建 Kotlin/Compose 项目并进入目录：

  ```powershell
  & $mob android create notes --language kotlin --ui compose --min-sdk 24
  Set-Location .\notes
  ```

- [ ] 在已启动的模拟器或已连接的真机上验证构建、运行、调试。预期 `mob run` 自动选择默认或第一台可用设备；可用 `--device` 明确指定：

  ```powershell
  & $mob build --accept-licenses
  & $mob run --device android:emulator-5554 --accept-licenses
  & $mob debug --device android:emulator-5554 --accept-licenses --json
  ```

- [ ] 验证无设备自动创建路径：先停止或断开所有设备，再在项目根目录运行。预期 Mob 自动准备所需 Emulator/镜像、创建匹配 API 的 AVD 并继续运行：

  ```powershell
  & $mob run --accept-licenses
  ```

## 6. Flutter Android 项目周期

- [ ] 在 Flutter 项目根目录运行。预期 Mob 不读取或修改 `.fvmrc`，仅选择并调用项目现有 Flutter/FVM 工作流：

  ```powershell
  & $mob status
  & $mob build --platform android --accept-licenses
  & $mob run --platform android --device android:emulator-5554 --accept-licenses
  & $mob debug --platform android --device android:emulator-5554 --json=events
  ```

## 7. 回归与记录

- [ ] 对每个失败结果记录命令、`--json` 输出、操作系统、Mob 版本、SDK 路径和 `mob support bundle` 产物；不要提交 Android keystore、设备日志中的敏感数据或代理凭据：

  ```powershell
  & $mob support bundle --output .\mob-support.zip
  ```

- [ ] 确认 Mob 不会永久修改 `JAVA_HOME`、`ANDROID_HOME` 或 `ANDROID_SDK_ROOT`；这些 Android 环境变量只应在 Mob 启动的子进程中注入。`mob android create` 为生成 Gradle Wrapper 而显式安装的托管 Gradle，属于单独的、可见的用户级命令路径注册行为。
