# Usage Guide

[English](usage.md) | [简体中文](usage.zh-CN.md)

## Start with a project

Create a project with the editor or the project tooling, keep project JSON and
source assets under `resources/`, then validate the project before running it.
The [module authoring guide](module-system-authoring.md) covers project layout,
module lifecycle, typed messages, scenes and validation.

## Add resources and behavior

Use [resource packages](resource-packages.md) to understand preprocessing,
package configuration and runtime ownership. ECS components and queries are
described in the [ECS API reference](ecs-api-design.md). For built-in behavior,
see [2D physics](physics2d.md), [3D physics](physics3d.md),
[virtual joystick](virtual-joystick.md), [system fonts](system-fonts.md) and
[scrollbar modes](scrollbar-modes.md).

## Render and integrate

Use [custom shaders](custom-shaders.md) and [lighting](render-lighting.md) for
graphics configuration. HTML behavior is covered by the focused guides for
[CSS animations](html-css-animations.md), [DOM parsing](html-native-dom-parser.md),
[visibility](html-visibility.md), [paint culling](html-paint-culling.md) and
[sticky positioning](html-position-sticky.md). Use [WebView](webview.md) when
the host browser control is required.

## Build and deploy

Choose [desktop builds](desktop-builds.md) or [Android builds](android-builds.md).
Linux applications can also use [DRM/KMS direct display](linux-drm.md).

For engine changes, read the [design guide](design.md) and
[performance guide](performance.md), then run focused package checks. Repository
development constraints are listed in [`AGENTS.md`](../AGENTS.md).
