# EasyPackage · Go 版 v1.0.0

将任意项目目录打包为 ZIP，并可附加 JPG 封面。Windows amd64 在独立桌面窗口中嵌入 WebView2，使用项目内的网页 UI，不会打开外部浏览器；Windows 其他架构保留 Win32/GDI 界面。macOS 使用 AppKit，Linux 使用 GTK 3。界面只通过随机本机端口与程序内的服务通信，文件处理仍在本机完成。压缩和还原逻辑以 Go 标准库实现。

## 启动

Windows amd64：双击 `dist/v1.0.0/windows-amd64/EasyPackage.exe`。程序包含 Microsoft WebView2Loader，首次启动需要系统已安装 **Microsoft Edge WebView2 Runtime**；未安装时会显示下载提示。浏览器 Runtime 不包含在 exe 中。安装 Runtime 后，EasyPackage 的文件打包和还原可离线使用。本机也提供根目录的 `EasyPackage.lnk` 快捷方式。升级时退出旧程序，再打开新版。

Windows ARM 使用同级 `windows-arm64` 目录中的 Win32 原生界面版本。该版本不包含 WebView2 界面。

macOS 和 Linux 必须在目标系统上构建。macOS 可执行文件使用系统 AppKit；Linux 可执行文件使用 GTK 3，并要求目标机器安装 GTK 3 运行库。对应系统构建后，从 `dist/v1.0.0/<系统>-<架构>/EasyPackage` 启动：

```sh
chmod +x EasyPackage
./EasyPackage
```

macOS 产物目前是未签名、未公证的原生可执行文件，系统安全策略可能要求用户确认后才能启动。Linux 文件选择和提示框由 GTK 3 提供；完成后可调用桌面环境的默认文件管理器打开输出目录。

## 功能

1. 在界面中浏览选择本机项目文件夹，也支持直接输入完整路径。
2. 指定保存目录和文件名称；已有文件需要确认覆盖。
3. 默认过滤大小写不敏感的 `.class` 文件，只从压缩包排除，不删除源文件。
4. 可启用大文件过滤，手动输入 KB、MB、GB 阈值及小数。1 MB = 1024 × 1024 字节；严格大于阈值的文件排除，阈值四舍五入至整数个字节。
5. 默认递归排除 `.git`、`.idea`、`.codegraph`、`.vscode`、`.vs`、`.svn`、`.hg`、`.fleet` 目录及全部内部内容，大小写不敏感。预览分别显示过滤文件数和排除目录数。
6. 可选择 JPG/JPEG 封面，生成仍可查看的图片包。「JPG / ZIP 还原」是**独立菜单页**，选择文件和保存位置后，直接解压到新文件夹，不生成中间 ZIP。支持普通 ZIP、AES ZIP 和旧 Java 版未加密 JPG 包。切换页面会保留已填写内容。
7. 可开启压缩密码，只需输入一次，不需要确认。支持手动输入，或点击「生成密码」生成 20 位密码后继续编辑。压缩和解压密码默认脱敏显示，点击「显示 / 隐藏」切换；重新随机生成、切换菜单后会恢复隐藏。随机密码使用 `crypto/rand`。
   使用 WinZip AES-256（AE-2）加密，ZIP 和 JPG 都适用；还原时输入相同密码。密码不会写入历史记录、配置或日志。文件名、目录结构仍可见，请妥善保存密码。
8. 原生窗口提供用户目录、桌面、文档、下载和最近目录的快速选择，并记录最近使用的 **5 个文件夹**，去重并按最近使用排序，关闭重启后仍保留。Windows 使用 Known Folder API 识别重定向和 OneDrive 目录；Windows、macOS 使用系统文件选择器，Linux 使用 GTK 文件选择器。
9. 支持文件清单预览、原始体积统计、后台进度、取消任务；完成后可浏览实际保存位置。
10. 点击「退出」或关闭窗口即可退出；任务运行时可取消或确认退出。

源项目直接以相对路径存入 ZIP，保留空目录、中文路径和普通文件权限。除上述目录、`.class` 和可选大小过滤外，保留业务文件、`.github`、`.env`、依赖包和构建目录，不笼统排除所有隐藏目录。

最近目录保存到系统用户配置目录下的 `EasyPackage/preferences.json`（Windows 通常为 `%APPDATA%\EasyPackage\preferences.json`），只记录路径。选择文件时记录其父目录；直接输入路径并成功打包/还原时也会更新记录。失效路径不会展示。

加密包采用标准 AES ZIP 格式，可由支持该格式的解压工具打开；某些系统内置 ZIP 解压不支持 AES，可直接用本软件还原，无需额外安装工具。算法由 Go 标准库 `crypto/aes`、`crypto/pbkdf2`、`crypto/hmac` 实现；按 WinZip 格式规定使用 PBKDF2-HMAC-SHA1/1000 次、AES-CTR 和 10 字节认证码。加密时按单个文件使用磁盘暂存，避免大文件占满内存。未加密时仍是普通标准 ZIP。

符号链接和特殊文件跳过，避免意外读取项目外内容。当前输出文件自动排除。临时数据写入保存目录，任务成功后才发布结果，读取/压缩/校验失败或正常取消不会替换已有输出。保存目录必须存在且有足够空间。不使用硬链接，因此也可保存到 FAT/exFAT 等目录。磁盘故障、强制终止或断电不在正常清理保证范围内。

打包期间请避免修改项目。程序会检测文件身份、大小和修改时间变化并提示重试，但不提供文件系统快照。JPG 是否加密取决于是否开启密码；图片编辑、重压、转码可能丢失附件，传输时请按原文件发送。不兼容 `packageProject` 参考项目的自定义加密图片包或传统 ZipCrypto 密码格式。

还原时先在目标父目录下的临时文件夹校验、解密和解压，再发布到新文件夹。已有文件夹不会合并或覆盖，错误密码、损坏附件、正常取消会清理临时内容。拒绝路径穿越、绝对路径、符号链接、特殊文件、重复项和跨平台不兼容的保留文件名；不会执行解压出的文件。

## 构建

开发需要 Go 1.25 或更高版本。使用者不需要安装 Go。项目不依赖第三方 Go 模块。macOS 使用 Xcode Command Line Tools 提供的 clang 和 AppKit；Linux 构建需要 C 编译器、pkg-config、GTK 3 开发文件，运行需要 GTK 3 运行库。

当前构建默认输出到 `dist/v1.0.0/`。Windows 可用 `-OutputRoot <目录>` 指定其他目录；macOS/Linux 可通过 `EASYPACKAGE_DIST` 环境变量指定。如果目标程序正在运行，请退出该程序或改用新的输出目录。

Windows 当前构建：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build.ps1
```

一次构建 Windows amd64 和 arm64 两个版本：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -All
```

macOS / Linux 原生构建（在目标系统本机执行；Linux 请先确认 `pkg-config --modversion gtk+-3.0` 可用）：

```sh
sh scripts/build.sh --native
```

在 macOS 或 Linux 上，也可用 `sh scripts/build.sh --all` 构建两个 Windows 版本和当前系统架构的原生版本。跨系统 GUI 产物要分别在对应系统构建。当前脚本按运行主机架构构建原生程序；arm64 版本应在 arm64 主机上构建，或配置相应 Cgo 交叉编译器和 SDK。

调试启动：

```sh
go run .
```

macOS/Linux 本机调试同样需要 CGO；Linux 需要已安装的 GTK 3 开发库。Windows amd64 使用内嵌 WebView2 窗口，并通过仅监听 `127.0.0.1` 的临时 HTTP 服务渲染页面；不会自动打开外部浏览器。

## 验证

```sh
go test ./...
go vet ./...
```

覆盖中文路径、大小边界、class 和嵌套目录过滤、空目录、输出自排除、JPG 往返及旧格式、直接解压、AES 密码与完整性、CRC 校验、错误密码、路径穿越、重复路径、取消与覆盖保护、源文件变化、临时清理、最近 5 个目录的持久化，以及 HTTP 本机访问限制。`testdata` 包含独立实现生成的 AES-128/192/256 ZIP 固定样本，普通 Go 测试无需第三方依赖。

Windows amd64 在 Windows 上完成了构建和 Go 测试。完整界面点击流程仍待验收；macOS 和 Linux 由 GitHub Actions 在各自系统上原生构建。

## 自动发布

推送 `v*` 标签后，GitHub Actions 会分别在 Windows amd64、macOS arm64、macOS amd64、Linux amd64 上构建，生成 `EasyPackage-<标签>-<系统>-<架构>.<格式>` 命名的 ZIP 或 tar.gz 文件，并附加到同名 GitHub Release。发布需要仓库允许 Actions 使用写入权限创建 Release；工作流已单独声明 `contents: write`。

## 结构

- `main.go`：平台窗口入口、共享后台任务及遗留本机 HTTP 处理器。
- `native_windows.go`：Windows 其他架构使用的 Win32/GDI 桌面界面。
- `webview2_windows.go`：Windows amd64 的 WebView2 桌面窗口与本地 UI 启动流程。
- `assets/windows/amd64/WebView2Loader.dll`：Microsoft WebView2Loader 随程序嵌入；系统 WebView2 Runtime 由用户另行安装。
- `native_darwin.go`、`native_darwin.m`：macOS AppKit 界面与系统对话框。
- `native_linux.go`、`native_linux.c`：Linux GTK 3 界面与系统对话框。
- `native_desktop.go`：macOS/Linux 界面共享的任务和控件逻辑。
- `places.go`、`folders_*.go`：最近目录持久化与跨平台常用目录识别。
- `internal/archive/`：压缩、过滤、JPG 封装与还原，核心测试。
- `web/`：内嵌界面，无外部资源依赖。
- `scripts/`：跨平台构建脚本。
- `dist/`：Go 单文件发行版本。
- `legacy/java/`：迁移前 Java 工程及产物备份；Go 版不引用它，可单独归档。
