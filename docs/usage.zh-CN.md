# 使用指南

[English](usage.md) | [简体中文](usage.zh-CN.md)

## 从项目开始

使用编辑器或项目工具创建项目，将项目 JSON 与源资源放在 `resources/` 中，
运行前先验证项目。[模块开发指南](module-system-authoring.md)介绍项目结构、
模块生命周期、类型化消息、场景与验证流程。

## 添加资源与行为

[资源包](resource-packages.md)说明资源预处理、包配置和运行时所有权。ECS
组件与查询参见 [ECS API 参考](ecs-api-design.md)。内置功能指南包括
[2D 物理](physics2d.md)、[3D 物理](physics3d.md)、[虚拟摇杆](virtual-joystick.md)、
[系统字体](system-fonts.md)和[滚动条模式](scrollbar-modes.md)。

## 渲染与平台集成

图形配置参见[自定义着色器](custom-shaders.md)和[光照](render-lighting.md)。
HTML 专题指南介绍 [CSS 动画](html-css-animations.md)、[DOM 解析](html-native-dom-parser.md)、
[可见性](html-visibility.md)、[绘制裁剪](html-paint-culling.md)和
[粘性定位](html-position-sticky.md)。需要宿主浏览器控件时使用 [WebView](webview.md)。

## 构建与部署

根据目标平台选择[桌面构建](desktop-builds.md)或 [Android 构建](android-builds.md)。
Linux 应用也可使用 [DRM/KMS 直接显示](linux-drm.md)。

修改引擎前先阅读[设计指南](design.zh-CN.md)与[性能指南](performance.zh-CN.md)，
然后运行受影响包的定向检查。仓库开发约定见 [`AGENTS.md`](../AGENTS.md)。
