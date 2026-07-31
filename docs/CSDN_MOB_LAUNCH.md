# 在 VS Code 中一键启用移动端开发：Mob 让 Android 先跑起来，iOS 与鸿蒙随后接入

> 移动端开发不该从安装多个 IDE、下载多套 SDK、配置环境变量开始。Mob 希望把 Android、iOS、HarmonyOS 的工具链、设备和运行流程收敛为 VS Code 中的一个入口：当前 Android 已可用，iOS 与鸿蒙正在接入。

[TOC]

## 移动端开发的第一个障碍，往往不是业务代码

很多开发者使用 VS Code 写 Kotlin、Flutter 或 Kotlin Multiplatform。第一次真正运行 Android 项目时，通常要依次处理 Android SDK 路径、`compileSdk`、Build Tools、JDK、ADB、模拟器镜像，以及 Gradle 和环境变量之间的关系。切换到 iOS 或 HarmonyOS 后，又会面对 Xcode/Simulator 或 DevEco/HDC 的另一套入口。

Android Studio、Xcode 和 DevEco 都能覆盖其中的一部分环节，但它们也让“安装和熟悉多个 IDE”变成开发前置条件。对于只需要编辑、构建、运行、调试和看设备画面的日常工作，开发者需要的是一个统一入口，而不是三套不同的环境配置方式。

更现实的问题是 AI 开发。AI 可以生成界面和业务代码，但如果它不知道当前机器有没有 API 34、哪个 JDK 能构建、设备是否在线，最后仍然需要开发者回到终端手动排障。移动端缺的不是又一个代码生成器，而是一套可以查询、执行并返回结构化结果的环境入口。

## Mob 的目标：VS Code + 一个入口完成移动端开发

Mob 是移动开发环境管理 CLI，定位接近 `nvm`，不是新的构建系统。它的长期目标是让开发者以 VS Code 为中心，用一个入口完成 Android、iOS 和 HarmonyOS 的环境准备与日常工作流。它做的是：

1. 发现并复用各平台已有的官方 SDK、工具链、设备桥接与模拟器。
2. 当前项目缺少官方组件时，按需下载、校验并准备它们。
3. 根据项目配置选择工具链，只向本次构建、运行或调试子进程注入环境。
4. 用统一设备模型连接真机、启动模拟器，并把构建、运行、调试与设备检查收敛到同一组命令。
5. 为 VS Code、CI 与 AI 工具提供文本、JSON 和 JSON Lines 三种输出。

目标不是让官方工具消失，而是让开发者不再手工拼装 SDK、设备桥接、模拟器和环境变量。日常流程可以是 **VS Code + Mob + 项目自己的官方构建工具**。

项目仍然是标准项目：Mob 不创建 `mob.yaml`，不改写 `build.gradle`、Flutter 配置或 `.fvmrc`。Android Studio 依然可以用于布局设计、Profiler 和专项诊断；Xcode 与 DevEco 的官方要求也会被保留。Mob 的目标是让手工环境配置不再成为日常构建、运行和调试的必要前提。

## 从一台新电脑到运行项目

先安装 Mob。安装脚本下载经过校验的 Release 二进制，不要求本机有 Go：

```powershell
irm https://raw.githubusercontent.com/xy200303/MobBase/main/scripts/install.ps1 | iex
mob --version
```

在 VS Code 打开一个 Android 项目后，可以从 Mob 扩展点击操作，也可以在集成终端执行：

```powershell
mob doctor
mob run --accept-licenses
```

Mob 识别当前项目后，会调用它原本的官方构建入口。原生 Android 和 KMP Android 目标使用 Gradle Wrapper；Flutter Android 使用 Flutter 或 FVM。若缺少 SDK Platform、Build Tools、NDK、JDK、ADB 或模拟器，Mob 会在开发者明确确认 Android SDK 许可后补齐可自动安装的组件。没有设备时，它可以创建匹配项目 API 的默认 Android 模拟器并继续运行。

第一次下载需要时间，后续项目可以复用已有工具链与下载缓存。不同项目的 API、NDK、JDK 需求也不会靠覆盖全局 `JAVA_HOME` 或 `ANDROID_HOME` 来切换。

## 多个项目不用反复改环境

假设一个旧项目需要 API 27，另一个新项目需要 API 35、NDK 和 JDK 17：

```powershell
cd C:\work\legacy-api-27
mob run --accept-licenses

cd C:\work\modern-api-35
mob debug --accept-licenses
```

Mob 从当前项目推导需求，在启动 Gradle、Flutter 或设备命令时注入相应环境。这样旧项目和新项目可以同时存在，不需要为了运行其中一个去破坏另一个。

## VS Code 不只是编辑器

Mob for VS Code 是 CLI 的图形入口。它把工具链诊断、SDK 安装、模拟器、真机、构建、运行、日志和调试放进 Activity Bar，但不在插件里重复实现 Android 工具管理。CLI 始终是唯一负责发现、下载、环境注入和命令执行的组件。

扩展还可以在编辑器内打开 Android 真机或模拟器的实时预览。预览使用 H.264 视频流，而非截图轮询，支持点击、滑动、文本输入、返回、主页和最近任务操作。会话仅监听本机回环地址，临时 token 和 ADB 转发会在关闭预览后回收。

## AI 如何参与完整开发流程

Mob 的默认输出给人看，机器接口给自动化工具看：

```powershell
mob help run --format json
mob status --json
mob doctor --fix --accept-licenses --json=events
mob run --accept-licenses --json=events
mob device ui-tree android:emulator-5554 --json
```

`--json` 返回一个终态对象，`--json=events` 在长任务中返回 JSON Lines 进度事件。AI 可以读取命令能力、检查当前环境、发现缺失工具链、请求用户确认许可证、执行构建与运行，再通过截图或 UI 树检查结果。它不需要猜一段文本报错，也不应绕过用户对许可证、设备授权和签名的确认。

这也是 Mob 和“给 VS Code 再装一堆扩展”之间的区别：CLI、插件、CI 和 AI 共享同一份工具链状态与命令契约。

## 当前范围：Android 现在可用，跨平台持续接入

Mob 当前完整支持 Android：SDK、NDK、JDK、ADB、Emulator、真机、Flutter/FVM、构建、运行、调试、测试、日志、发布构建，以及 VS Code 内的 Android 设备预览。

iOS 和 HarmonyOS 已有统一的平台命名空间与设备协议设计，完整的工具链、设备、签名和发布工作流正在接入。Mob 不会伪造 Xcode、DevEco、签名、设备控制或发布能力；对应平台会以官方能力真实交付。

## 获取项目

- GitHub：https://github.com/xy200303/MobBase
- Release：https://github.com/xy200303/MobBase/releases
- VS Code Marketplace：https://marketplace.visualstudio.com/items?itemName=xiaoyun.mob-vscode
- 产品规范：https://github.com/xy200303/MobBase/blob/main/docs/PRODUCT_PLAN.md
- 手动验证清单：https://github.com/xy200303/MobBase/blob/main/docs/MANUAL_VALIDATION.md

Mob 从 Android 开始，把“在 VS Code 中一键启用移动端开发环境”做成真实体验；随后让 iOS 与 HarmonyOS 接入同一套工具链、设备和自动化入口。

---

**推荐 CSDN 标签：** `Android`、`VS Code`、`Gradle`、`Flutter`、`ADB`、`Android Studio`、`开发工具`、`AI 编程`
