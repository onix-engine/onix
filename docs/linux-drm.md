# Linux DRM/KMS direct display

`onix_drm` renders directly to a Linux display with GLES 3, GBM/EGL and DRM page
flips. Add `onix_drm_vulkan` to use Vulkan display surfaces and a swapchain. Both
builds exclude X11, Wayland and WebView. Cross-compilation needs a target C compiler
and the selected backend's development libraries.

## Build

```sh
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -tags onix_drm -o app ./path/to/app
CGO_ENABLED=1 go build -tags onix_drm,onix_drm_vulkan -o app ./path/to/app
```

GLES requires EGL/GLES3 headers and link libraries at build time, and
`libdrm.so.2`, `libgbm.so.1`, `libEGL.so.1`, `libGLESv2.so.2` on the device.
Vulkan links `libvulkan.so.1` and requires `libdrm.so.2` and the target GPU's ICD;
it does not link or initialize EGL, GLES or GBM. Vulkan headers are bundled.

Vulkan requires instance extensions `VK_KHR_surface`, `VK_KHR_display`,
`VK_EXT_direct_mode_display`, `VK_EXT_acquire_drm_display`, and device extension
`VK_KHR_swapchain`. Missing extensions or display ownership cause an initialization
error. There is no automatic GLES fallback. Desktop Vulkan or software rendering
support alone does not establish direct-display support. `onix_drm_vulkan` must
always be combined with `onix_drm`.

## Output selection

Scan `/dev/dri/card0` through `card15` and choose the first connected connector
with an available CRTC. Pin a device in the application's `resources/project.json`:

```json
{
  "window": {
    "width": 1280,
    "height": 720,
    "drmDevice": "/dev/dri/card1",
    "drmConnectorId": 43
  }
}
```

Width and height specify the logical viewport; physical scanout uses the connector's
preferred DRM mode. Vulkan matches the physical device by DRM fd and connector ID,
selects the closest refresh rate at the preferred resolution, then a compatible
plane. Use `drmDevice` to select a capable GPU on multi-GPU systems.

Inspect connectors without modesetting:

```sh
for connector in /sys/class/drm/card*-*; do
  printf '%s: ' "$connector"
  cat "$connector/status"
  cat "$connector/connector_id" 2>/dev/null
done
```

`modetest -c` also lists connector IDs and modes.

## Run from a text console

A GPU has one DRM master at a time. A desktop compositor normally owns it; GLES
then fails at modesetting with Permission denied, or Vulkan fails acquiring display
ownership. Switch to a text virtual terminal or SSH, stop the compositor/display
manager, and grant read/write access to the primary node through the video group
or logind ACL. Ctrl+C exits normally and restores the saved CRTC. SIGKILL cannot
run cleanup. Vulkan destroys the swapchain/surface, releases display ownership,
restores the CRTC and connector routing, then closes the DRM fd.

At startup the backend scans `/dev/input/event*`, identifies keyboards, mice and
touch devices by evdev capabilities, and polls all readable matches into the shared
input API. Device numbering does not affect selection. Fixed buffers and queues
avoid Go heap allocation in stable frames.

Use `libinput list-devices` or `/proc/bus/input/devices` to identify required input
nodes. Grant access before startup through the input group, restricted udev rules
or temporary ACLs. A temporary test command is:

```sh
sudo setfacl -m u:"$USER":r /dev/input/event*
```

Grant only needed devices in production. Temporary ACLs disappear when nodes are
recreated. Visual output and real input require manual testing on a device with
DRM master access. Mock-based tests verify device/mode/plane selection, failed
initialization cleanup and output restoration without modesetting:

```sh
go test -tags onix_drm,onix_drm_vulkan ./render ./window
go test -tags onix_drm ./render ./window
```

## Driver inspection

```sh
ls -l /dev/dri/card* /dev/dri/renderD*
eglinfo | grep -A4 -i gbm
ldconfig -p | grep -E 'libdrm\.so\.2|libgbm\.so\.1|libEGL\.so\.1|libGLESv2\.so\.2'
```

`kmscube -D /dev/dri/card1` independently checks DRM/GBM/EGL under the same console
conditions. `vulkaninfo --summary` checks the ICD and instance extensions, but the
selected physical device must also support direct display. See
[VK_EXT_acquire_drm_display](https://docs.vulkan.org/refpages/latest/refpages/source/VK_EXT_acquire_drm_display.html)
and [VK_KHR_display](https://docs.vulkan.org/refpages/latest/refpages/source/VK_KHR_display.html).
