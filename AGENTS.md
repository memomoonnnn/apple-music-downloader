# AGENTS.md

本仓库是 `apple-music-downloader` 的 Get Oudio 专用 fork。修改前先读本文，再读 `go.mod`、`main.go`、`utils/runv2`、`utils/runv3`、`utils/task` 和 `config.yaml.example`；不要把本文当成上游通用说明，它记录的是 Get Oudio 当前消费这个 fork 的约束。

## Integration Contract

Get Oudio 不把本仓库源码 vendoring 到 App 工程里，只消费构建后的 macOS 可执行文件。默认本地布局是 Get Oudio 与本仓库并列：`/Users/shengjiacheng/Desktop/项目/Software/get-oudio` 和 `/Users/shengjiacheng/Desktop/项目/Software/apple-music-downloader-get-oudio`。Get Oudio 端的同步入口是 `get-oudio/script/build_apple_music_downloader.sh`，它从本仓库当前工作树构建 `darwin/arm64`、`CGO_ENABLED=0`、`go build -trimpath -ldflags="-s -w"` 产物，并复制到 `GetOudio/Resources/ThirdParty/apple-music-downloader/apple-music-downloader`。

Get Oudio 当前只通过 `AppleMusicDownloadService.downloaderArguments` 调用本工具：ALAC 使用默认空参数，AAC 使用 `--aac`，Atmos 使用 `--atmos`，单曲 URL 追加 `--song`。它不调用 `--search`、`--select`、`--all-album` 或交互式 artist 选择；但运行时仍依赖 `config.yaml` 的兼容性、退出码、stdout/stderr 进度文本、下载完成行为、歌词/tag/封面相关默认行为和 runv2/runv3 解密链路。不要为了缩小体积无声移除这些兼容面。

## Structured Event Mode

Get Oudio 调用时追加 `--events=jsonl`，此模式的 stdout 是唯一的机器接口：UTF-8 JSONL、每行一个 schema version 1 事件，不得混入终端文本、进度条或 ANSI 控制字符。旧的 stdout 文本在该模式下丢弃，默认模式保持原样。事件至少覆盖 `run_started`、`item_started`、`progress`、`item_completed`、`item_failed`、`diagnostic` 和 `run_completed`；每个事件有单调 `sequence` 与 `run_id`。`item_started.data.content.playlist_title` 仅在播放列表曲目时携带播放列表名称。下载、解密和 tag 阶段只由 30 秒心跳发送最新字节数，不在阶段开始时立即发送；总字节数未知时不要伪造百分比。只有目标文件、封装和 tag 均完成后才能发送 `item_completed`，失败事件与 diagnostic 必须先做凭据脱敏。

## Size Research

此前对 Get Oudio 内嵌二进制的调查结论是：18 MB 体积不是因为把全部 Go module 源码或第三方 dylib 原样打包，而是因为 Go 单文件可执行会携带 Go runtime、可达依赖代码、类型/反射元数据、符号表和 DWARF 调试信息；`otool -L` 只显示 macOS 系统库依赖。低风险优化是链接期剥离：同一上游提交上，普通 `go build -trimpath` 产物约 `18,445,362` 字节，`go build -trimpath -ldflags="-s -w"` 产物约 `12,753,922` 字节，节省约 31%。`strip` 作为后处理效果明显更弱，不应作为主要瘦身方式。

更进一步的体积优化必须通过功能级裁剪或专用入口完成，不能只看 `go.mod` 就删除依赖。Go 只链接 import 图中实际可达的包；当前已裁掉只服务交互搜索、表格输出和终端颜色的 `survey/v2`、`tablewriter` 与 `color`，但 `go-mp4tag`、`resty`、`protobuf`、`mp4ff`、`progressbar` 和 `m3u8` 仍服务下载、解密、tag、进度或 MV/Widevine 路径，不能按体积单独删除。

## Development Direction

优先级最高的是保持行为稳定。小改动应先围绕构建、日志、错误信息、可测试性和 Get Oudio 实际调用路径做，不要先大规模重排上游结构。当前 fork 已移除 `--search`、`--select`、`--all-album`、`--debug` 及其 survey、表格、终端颜色和 artist 交互实现，也移除了下载后格式转换；专辑与播放列表固定遍历全部曲目，M3U8 解析只保留下载所需的质量选择。上游同步若重新引入这些路径，不要无意恢复它们。

候选裁剪区按风险从低到高评估：交互搜索与选择 UI、表格输出和终端颜色通常最容易与 Get Oudio 解耦；MV 下载、Widevine/protobuf、`mp4decrypt`、歌词、MP4 tag、封面写入、`alacfix` 和 runv2/runv3 解密路径必须先确认 Get Oudio 产品面是否真的不用，且要有替代测试或真实下载验证。尤其不要破坏 `--aac`、`--atmos`、`--song`、默认 ALAC、`config.yaml` 字段、进度输出和失败信息格式，因为 Get Oudio 会解析这些行为并向用户展示。

## Build And Sync

日常修改在本仓库完成；需要生成 App 内嵌产物时回到 Get Oudio 根目录运行：

```bash
bash script/build_apple_music_downloader.sh
```

如果本仓库不在默认相邻目录，使用：

```bash
APPLE_MUSIC_DOWNLOADER_SOURCE=/path/to/apple-music-downloader bash script/build_apple_music_downloader.sh
```

同步脚本会使用 Get Oudio 仓库下的 `build/apple-music-downloader/go-build-cache` 和 `build/apple-music-downloader/go-mod-cache`，避免复用可能污染的 `/private/tmp/go-mod-cache`。如果直接在本仓库手动构建，也应使用等价参数：

```bash
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o apple-music-downloader main.go
```

本仓库提供只输出到临时目录的本地基线检查：

```bash
bash script/verify_get_oudio_build.sh
```

该脚本会拒绝把验证产物写进本仓库，并检查 `go version -m` 中的 `darwin/arm64`、`CGO_ENABLED=0`、`-trimpath=true`，在 macOS 上还会用 `otool -L` 拦截 Homebrew 或 `/usr/local` 动态库依赖。CI 也应跑同一脚本，而不是恢复上游三平台 artifact 打包流程。

不要提交本地构建出的二进制、Go module cache、`build/` 中间产物或 `.DS_Store` 到本仓库；Get Oudio 仓库才保存要随 App 分发的内嵌可执行文件。

## Verification

本仓库内先做 Go 侧检查：至少确保 `go build` 成功；若新增或调整 Go 测试，运行 `go test ./...`。同步到 Get Oudio 后，至少检查：

```bash
go version -m GetOudio/Resources/ThirdParty/apple-music-downloader/apple-music-downloader
otool -L GetOudio/Resources/ThirdParty/apple-music-downloader/apple-music-downloader
xcodebuild -project GetOudio.xcodeproj -scheme GetOudioCoreTests -configuration Debug -derivedDataPath build/DerivedData test
```

`go version -m` 应显示目标为 `darwin/arm64`、`CGO_ENABLED=0` 和 `-trimpath=true`；`otool -L` 不应出现 Homebrew 或其他第三方动态库。Get Oudio 核心测试重点关注 `AppleMusicDownloadFormat`、`AppleMusicDownloadService.downloaderArguments`、Apple Music 进度解析、runtime 状态和 wrapper 初始化相关测试。若沙箱内 XCTest 因 `com.apple.testmanagerd.control` 被拒绝而失败，按同一命令在非沙箱环境重跑后再判断代码是否真的失败。

真实功能验证应至少覆盖 ALAC 默认下载、`--aac`、`--atmos`、单曲 URL `--song`、专辑/播放列表 URL 不加 `--song`、wrapper 已登录和未登录两类错误路径、下载中断后的失败信息。涉及歌词、tag、封面、MV 或 Widevine 的裁剪时，还必须用对应真实内容验证输出文件、元数据和失败提示。

## Upstream Sync

保留两个远端：`origin` 指向 `https://github.com/memomoonnnn/apple-music-downloader`，`upstream` 指向 `https://github.com/zhaarey/apple-music-downloader`。同步上游时先运行 `git fetch upstream`，再按当前分支策略选择 merge 或 rebase；冲突解决要优先保留 Get Oudio 专用构建入口、本文档和兼容契约。上游变更如果影响 `go.mod`、`main.go`、`utils/runv2`、`utils/runv3`、`config.yaml.example` 或输出格式，同步后必须重新运行 Build And Sync 与 Verification 中的检查。

## Security And Logging

不要把 Apple ID、密码、验证码、media-user-token、authorization-token、wrapper 登录参数或代理凭据写入日志、默认配置、测试夹具或错误输出。Get Oudio 的 wrapper 登录由 AM Runtime Agent 管理，本工具只应按现有配置和 wrapper 端口交互；修改错误处理或命令输出时，应确保凭据不会出现在 stdout/stderr，因为 Get Oudio 会记录并展示这些信息。
