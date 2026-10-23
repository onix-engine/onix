# 3D viewport

The module editor has a grid, world axes, gradient sky, studio environment and
orientation widget. Its independent camera keeps navigation and display settings
separate from module data and scene cameras.

- Middle or right drag: orbit. Shift + middle drag: pan. Wheel: zoom.
- F: focus the selected module. O: toggle perspective and orthographic projection.
- Click the positive or negative X/Y/Z widget to align an orthographic view.
  Clicking the facing direction flips it. Drag to orbit; Esc cancels the rotation.
- Click modules or local elements to select them; drag the selection to move it.
- Cameras show a wireframe frustum and an up-direction triangle. Lights show rings,
  sun rays, spot cones or area outlines. Selected outlines turn orange and support
  dragging, snapping, undo and focus. Local editing moves individual lights/cameras.

Open View at the upper left:

| Tab | Settings |
| --- | --- |
| Display | Gradient sky, gray, environment or source-default background; grid; 1x/2x supersampling; projection; preview ground; automatic/XZ/XY/YZ grid plane |
| Lighting | Studio/scene lighting; environment source camera; environment intensity, rotation and exposure; shadows off/1024/2048 |
| Tools | Grid snapping; automatic/fixed snap step; focus selection |

A four-vertex grid computes antialiasing, decimal scale transitions, distance and
view-angle fading in the shader. Infinite positive/negative world axes use a fixed
pixel width and share scene depth. Automatic orthographic grids align to the view.

The studio environment uses prebaked `resources/environment/studio.png`. Empty
canvases skip module resource initialization. Vulkan pipelines are created lazily;
grid, axes, sky and unlit content use lightweight shaders. Switching canvases retains
resources; stable draws reuse caches and fixed arrays.

Visual acceptance is manual. Automated tests cover switching, orientation,
orthographic dragging, depth, resource isolation and zero allocations in warm frames.
