# Onix

[English](README.md) | [简体中文](README.zh-CN.md)

Onix 是使用 Go 编写的游戏与应用引擎，提供类型化 ECS、2D/3D 渲染与物理、
HTML/CSS UI、音频、资源打包和项目编辑器。

Go 模块路径为 `github.com/diiyw/onix`，根包名为 `onix`。

## 环境要求

- **Go 1.26 或更高版本**，以及用于克隆仓库的 Git。
- **原生构建所需的 C/C++ 工具链**，启用 CGO，并安装宿主平台需要的开发库。
  Linux 窗口系统依赖 X11、Xext、Xrender、Xi、Wayland、wayland-cursor 和 xkbcommon；
  macOS 原生开发需要 Xcode Command Line Tools。
- **打包或交叉编译所需的目标平台工具**。工具链与打包要求见
  [桌面构建](docs/desktop-builds.md)和 [Android 构建](docs/android-builds.md)。

## 快速开始

在引擎仓库根目录执行以下命令。游戏与应用项目应放在本仓库之外：

```sh
go run ./editor/cmd/onix new ../my-game
go run ./editor/cmd/onix check ../my-game
go run ./editor/cmd/onix ../my-game
```

最后一条命令会打开新项目的编辑器。使用 **Play** 构建并运行项目，使用
**Build Project** 打包项目。不带参数执行 `go run ./editor/cmd/onix`，
则进入编辑器的新建／打开项目流程。

如果目标目录及其父目录没有 Go 模块，创建项目时会生成 `go.mod`，通过本地
`replace` 指向当前引擎仓库。也可以显式设置应用模块路径和初始视口尺寸：

```sh
go run ./editor/cmd/onix new -module example.com/my-game -width 1280 -height 720 ../my-game
```

### 项目结构

```text
my-game/
  go.mod
  go.sum
  main.go
  onix_generated.go
  resources/
    project.json
    scenes/
      main.json
```

`main.go` 由应用开发者维护。`onix_generated.go` 包含生成的注册与引擎初始化代码，
入口包括 `OnixRegistry`、`OnixInstallSystems`、`OnixModuleOptions` 和
`OnixOpenWindow`；需要更新时重新生成此文件。新项目初始包含一个空场景。
项目 JSON 和资源文件应放在 `resources/` 中，模块定义与行为的编写方式见
[模块开发指南](docs/module-system-authoring.md)。

### 常用命令

以下命令均在引擎仓库根目录执行，选项放在项目目录参数之前。

| 操作 | 命令 |
| --- | --- |
| 创建项目 | `go run ./editor/cmd/onix new ../my-game` |
| 校验项目 | `go run ./editor/cmd/onix check ../my-game` |
| 重新生成注册代码 | `go run ./editor/cmd/onix generate ../my-game` |
| 在编辑器中打开项目 | `go run ./editor/cmd/onix ../my-game` |
| 为宿主桌面平台构建发布版本 | `go run ./editor/cmd/onix build -mode release ../my-game` |
| 构建 Web 发布版本 | `go run ./editor/cmd/onix build -platform web -mode release ../my-game` |

`generate` 保留应用的 `main.go`。构建流程会收集、预处理资源，并将平台产物输出到
项目的 `dist/` 下。发布构建使用 `onix_release` 标签和二进制资源包，应通过构建命令
或编辑器完成准备。构建目标包括 `windows`、`macos`、`linux`、`web`、`android`、
`ios` 和 `wechat`，各目标需满足相应的宿主平台与工具链要求。

## 源码导航

| 路径 | 职责 |
| --- | --- |
| 根包 `onix` | 应用生命周期、调度和引擎集成 |
| [`ecs/`](ecs/) | 类型化组件、查询、系统和结构变更命令 |
| [`module/`](module/) | 模块生命周期、类型化消息、槽位和场景管理 |
| [`project/`](project/) | 项目校验、编译和资源预处理 |
| [`editor/`](editor/) | 项目编辑器、代码生成、构建流程和命令行入口 |
| [`scene/`](scene/)、[`camera/`](camera/)、[`physics/`](physics/) | 场景组件、相机和 2D/3D 物理 |
| [`render/`](render/)、[`graphics/`](graphics/) | 渲染后端、图形数据和图像编解码 |
| [`ui/`](ui/)、[`text/`](text/)、[`font/`](font/) | HTML/Markdown UI、文本布局和字体 |
| [`resource/`](resource/)、[`fs/`](fs/) | 资源包和文件访问 |
| [`window/`](window/)、[`input/`](input/)、[`audio/`](audio/) | 平台窗口、输入和音频 |
| [`tests/`](tests/)、[`testdata/`](testdata/) | 集成检查和测试资源 |

平台桥接名称使用 `Onix` 前缀；构建标签和环境变量分别使用 `onix_` 和 `ONIX_` 前缀。

## 开发文档

从[文档索引](docs/README.zh-CN.md)进入使用、设计和性能总览；每类总览都有
英文和简体中文版本。API 与平台专题参考集中列在[使用指南](docs/usage.zh-CN.md)。

## 引擎开发约定

修改引擎前请阅读 [AGENTS.md](AGENTS.md)。优先使用简洁的泛型实现，尽量减少
热路径分配，并合并可复用的遍历。HTML 渲染修改应遵循 W3C 标准并与 Chromium 对齐。

仅运行改动涉及的包测试。例如修改 ECS 时：

```sh
go test ./ecs
go test ./ecs -run TestName -count=1
```

将 `TestName` 替换为相关测试名称或匹配表达式。**禁止运行 `go test ./...`**，
全仓库测试可能导致机器卡死。视觉验收由人工完成，不要启动编辑器进行自动截图测试。

## 许可证

Onix 使用 [MIT 许可证](LICENSE)。在遵守许可证声明保留要求的前提下，
可以使用、修改、编译和分发本引擎，包括用于商业游戏和软件。
第三方代码与资源保留各自的许可证，详见[第三方声明](THIRD_PARTY_NOTICES.md)。
