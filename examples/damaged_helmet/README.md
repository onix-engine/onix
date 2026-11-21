# DamagedHelmet：Onix / three.js 渲染对照

使用 [three.js glTF 示例](https://threejs.org/examples/#webgl_loader_gltf) 当前默认的 DamagedHelmet 模型。模型、五张内嵌 JPEG 贴图、Royal Esplanade 原始 UltraHDR JPEG 均已下载到 `resources`，Onix 运行时无需联网。抓取日期：2026-10-12；官网 three.js revision：186。

在仓库根目录运行：

```sh
go run ./editor/cmd/builder run ./examples/damaged_helmet
```

按住鼠标左键拖动可绕模型水平、垂直旋转，松开即停止；方向键也可旋转。Q 拉近、E 拉远，R 恢复参考视角。窗口初始大小 1280×800。也可以在编辑器中打开本项目，由人工检查画面。

## 人工对照

```sh
python3 -m http.server 8080 --directory examples/damaged_helmet
```

打开 [本地 three.js 对照页](http://localhost:8080/resources/reference/comparison.html)。它使用本地同一个 GLB 和同一个原始 Ultra HDR JPG，three.js 库从官网加载，需要联网。Onix 直接读取 JPG 中的 SDR 图像、增益图与元数据，不生成中间 HDR 文件。页面显示实际 three.js 版本与绘制像素尺寸；官网库可能更新，原始页面与 UltraHDRLoader 源码另存于 `resources/reference`。

将浏览器绘制区域设为与 Onix 窗口内容相同的宽高，注意显示缩放/DPR。两边重置相机后，对照头盔轮廓、金属划痕、高光位置与强度、粗糙表面反射、法线细节、遮蔽、自发光和背景。背景旋转/镜像、高光方向、UV 接缝也需人工检查。测试没有启动编辑器或生成截图，也没有计算像素误差。

## 对齐的场景输入

- 原始 GLB 不修改，保留 glTF 节点的 X 轴旋转和材质参数；14,556 顶点、15,452 三角形，五张 2048×2048 贴图。
- 相机 FOV 45°，按照官网当前 `fitCameraToSelection` 的包围盒、1.3 fitOffset 和初始观察方向计算中心、距离与近裁剪面。Onix 使用无限远反向 Z，three.js 使用有限远裁剪面。
- 仅使用环境照明，环境强度 1、曝光乘数 1、背景开启。显式添加亮度为 0 的环境光，关闭 Onix 默认的 800 亮度补光。
- Onix 的曝光公式为 `1 / (1.2 × 2^EV100)`，因此设置 `EV100 = -log2(1.2)`，而非默认 EV100 13；保留原始自发光贴图和强度。
- 两边启用抗锯齿，关闭额外 SSAO、SSR、雾、灯光和材质覆盖；纹理压缩关闭。
- 两边使用 ACES 输入/输出颜色矩阵、Stephen Hill RRT/ODT 拟合及 `1/0.6` 曝光缩放。Onix 背景保留原始 2048×1024 分辨率，独立于光照 atlas。

## 已知差异与验证范围

目前不能认定 Onix 的渲染结果与 three.js 完全一样，源码已确认存在以下差异：

- Onix 将环境预过滤为 256×1024 的 RGBE 图集，包含六档 GGX 反射、漫反射和 BRDF LUT。预过滤按球面面积构建线性 mip，并按样本概率密度选择采样面积；漫反射与最粗糙反射采用全景积分的九项球谐近似。光照包含多次散射补偿和粗糙度方向修正。three.js 使用 PMREM，锐利高光和粗糙反射仍不应假定完全一致。
- Onix 按 [Ultra HDR 规范](https://developer.android.com/media/platform/hdr-image-format)在线性颜色空间还原完整 HDR，支持 XMP/ISO 元数据、灰度/RGB 增益图和缩小增益图的双线性采样。three.js r186 的 UltraHDRLoader 在非线性 JPEG 数值上施加增益，并使用 `1.8 ** (HDRCapacityMax * 0.5)` 的显示 boost、sRGB 查表和半精度截断，因此环境解码的亮度和颜色仍可能不同。
- HDR 图集的每个像素拥有自己的 RGBE 指数；GPU 先解码四个邻近像素再插值，避免强高光导致整张环境暗部归零或出现色块。资源打包会保留环境 JPG，普通纹理压缩不会移除增益图。

自动检查加载真实资产、原始分辨率背景、贴图与材质完整性、HDR 动态范围、相机拟合、输入路由、左键拖动与松开、缩放上限、重置及活动控制器的零分配。引擎数值回归另外检查细小 HDR 光源积分；GLES/Vulkan 像素回归检查背景细节、旋转、经度接缝与 ACES 色彩。通过这些检查不代表完整场景的 GPU 图像已经与 three.js 一致，仍需人工对照。

```sh
GOMAXPROCS=2 GOMEMLIMIT=768MiB go run -p 2 ./editor/cmd/builder test ./examples/damaged_helmet -p 2 -count=1 -v
```

## 资源来源

- [DamagedHelmet GLB](https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Assets/main/Models/DamagedHelmet/glTF-Binary/DamagedHelmet.glb)，Khronos glTF Sample Assets；作者及许可说明保存在 `resources/models/SOURCE.md` 和 `LICENSE.md`。
- [原始环境 JPEG](https://threejs.org/examples/textures/equirectangular/royal_esplanade_2k.hdr.jpg)，[Royal Esplanade](https://polyhaven.com/a/royal_esplanade)，Greg Zaal，CC0。
- three.js 页面/加载器源码采用 MIT，保留在 `resources/reference/THREE-LICENSE.txt`。
- 文件 SHA-256 与来源记录保存在 `resources/reference/assets.json`。
