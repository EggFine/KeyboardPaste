# KeyboardPaste

一个使用 **Go + Fyne** 编写的跨平台桌面 GUI 工具：将文本粘贴进窗口，点击**「开始模拟输入」**，经过可配置的倒计时延迟后自动模拟系统键盘逐字输入，轻松绕过禁止粘贴（`Ctrl+V` / 右键粘贴失效、网页禁粘贴、远程桌面 / KVM 控制台、在线答题 / 代码编辑器禁粘贴等）的场景。

---

## 功能特性

- **直观易用的原生 GUI**：支持一键读取剪贴板、追加粘贴、清空文本，实时显示字符数、行数与预计输入耗时。
- **可自定义延迟与打字速率**：
  - **启动延迟（秒）**：自由调整倒计时（默认 `3` 秒），留出充足时间将鼠标焦点切入目标输入框。
  - **单字间隔（毫秒）**：支持 `1ms ~ 5000ms` 速率调节，开启**拟人随机速率抖动**可模拟真实人类打字节奏。
- **双输入引擎模式**：
  - **通用 Unicode 模式（默认）**：直接注入系统级 Unicode 字符事件，**完整支持中文、英文、全角符号与 Emoji**，且不受本地中文输入法候选框干扰。
  - **硬件按键模式**：模拟真实物理虚拟键码（Virtual-Key / ScanCode）与 `Shift` 组合按键，专为 **VMware / Proxmox noVNC / IPMI KVM 控制台** 及严格监听物理键码的密码框设计。
- **针对代码编辑器 / 聊天框的贴心选项**：
  - **去除每行行首缩进**：避免在线 IDE / OJ 自动缩进导致缩进翻倍。
  - **将 Tab 转为 4 个空格**：兼容把 `Tab` 当作焦点切换键的表单页面。
  - **多种换行策略**：支持发送 `Enter`、`Shift+Enter`（聊天软件换行不直接发送）或忽略换行符。
- **随时暂停 / 继续 & 全局 `ESC` 紧急中止**：即使窗口焦点在目标程序中，按下物理 **`ESC`** 键即可立即终止自动输入，防止误操作失控。
- **全平台原生实现（无外部臃肿依赖）**：
  - **Windows**：基于 Win32 `user32.dll` `SendInput` 原生注入（编译隐藏黑框控制台）。
  - **macOS**：基于 `ApplicationServices` / `CoreGraphics` `CGEvent` 原生注入，自动检测并引导开启辅助功能权限，打包为高清 Retina `.app` 应用包（支持 Apple Silicon `arm64`、Intel `amd64` 与 `Universal` 通用包）。
  - **Linux**：基于 `X11` + `XTest` 扩展（支持动态 KeySym 映射输入中文与任意 Unicode），并在纯 Wayland 会话下支持自动回退 `wtype`。
- **纯绿色单文件（零注册表、零数据残留）**：无需安装、无附带 DLL，全程纯内存运行，**不写任何注册表项，也不落盘保存任何配置或输入文本数据**，用完即删无残留。

---

## 快速使用

1. 前往本仓库的 **[Releases](../../releases)** 页面下载对应系统的单文件或压缩包运行：
   - **Windows (`amd64`)**：直接下载单文件 `KeyboardPaste_*_windows_amd64.exe`（或解压 `.zip`），双击运行即可。
   - **macOS (`arm64` / `amd64` / `universal`)**：解压 `KeyboardPaste_*_darwin_*.zip`，直接打开 `KeyboardPaste.app`（或拖入「应用程序」）。
   - **Linux (`amd64` / `arm64`)**：下载单文件 `KeyboardPaste_*_linux_*`（`chmod +x` 后运行）或解压 `.tar.gz` 执行 `./KeyboardPaste`。
2. 将需要输入的内容粘贴进文本框（或点击左上角 **「粘贴剪贴板」**）。
3. 设置 **启动延迟**（如 `3` 秒）与 **单字间隔**（如 `30` ms），点击 **「开始模拟输入」**。
4. 在倒计时结束前，**用鼠标点击目标软件或网页中的输入框**，倒计时归零后即会自动逐字输入。
5. 如需中途暂停或终止，可点击界面上的 **「暂停 / 停止」**，或直接按下键盘 **`ESC`** 键紧急停止。

---

## 各平台权限与注意事项

### Windows
- 如果目标窗口（例如任务管理器、管理员终端、部分安全控件客户端）是以**管理员身份**运行的，受 Windows UIPI 隔离机制限制，请右键 `KeyboardPaste.exe` 选择 **「以管理员身份运行」**。

### macOS
- 首次点击「开始模拟输入」时，macOS 会提示需要辅助功能权限。请前往 **「系统设置 -> 隐私与安全性 -> 辅助功能」**，勾选允许 `KeyboardPaste` 控制电脑。
- 若首次打开提示「无法验证开发者」或隔离属性限制，可在终端执行：
  ```bash
  xattr -cr /Applications/KeyboardPaste.app
  ```

### Linux
- X11 及 XWayland 应用开箱即用。
- 若在纯 Wayland 原生应用下使用，请安装 `wtype`（例如 `sudo apt install wtype`）。

---

## 本地编译指南

环境要求：**Go 1.22+** 与 C 编译器（CGO 开启）。

### Linux (Ubuntu / Debian)
```bash
sudo apt-get update
sudo apt-get install -y gcc pkg-config libgl1-mesa-dev xorg-dev libxtst-dev libwayland-dev libxkbcommon-dev
go build -trimpath -ldflags="-s -w" -o KeyboardPaste .
```

### macOS
```bash
xcode-select --install
go build -trimpath -ldflags="-s -w" -o KeyboardPaste .
```

### Windows (PowerShell + MinGW-w64)
```powershell
go build -trimpath -ldflags="-s -w -H=windowsgui" -o KeyboardPaste.exe .
```

---

## GitHub Actions 自动全平台编译发布

项目已内置全自动发布流水线 [`.github/workflows/release.yml`](.github/workflows/release.yml)，支持以下两种触发方式：

### 方式一：推送版本标签自动发布（推荐）
```bash
git tag v1.0.0
git push origin v1.0.0
```
推送 `v*` 标签后，GitHub Actions 将自动并行编译以下全平台产物并发布到 **GitHub Releases**（附带 `SHA256SUMS.txt` 校验文件）：
- `KeyboardPaste_v1.0.0_windows_amd64.zip`
- `KeyboardPaste_v1.0.0_darwin_arm64.zip`（Apple Silicon `.app`）
- `KeyboardPaste_v1.0.0_darwin_amd64.zip`（Intel Mac `.app`）
- `KeyboardPaste_v1.0.0_darwin_universal.zip`（macOS 双架构通用 `.app`）
- `KeyboardPaste_v1.0.0_linux_amd64.tar.gz`
- `KeyboardPaste_v1.0.0_linux_arm64.tar.gz`

### 方式二：网页端手动一键触发
进入 GitHub 仓库的 **Actions -> Release All Platforms -> Run workflow**，输入版本号（如 `v1.0.0`）点击运行即可自动打包并创建 Release。
