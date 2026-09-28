# AGENTS.md

本仓库是 Get Oudio 使用的 `apple-music-downloader` fork。适配上游前确认当前工作树、`upstream` 指向及双方提交差异，并核对相邻 `get-oudio` 中的 `AppleMusicDownloadService`、`config.yaml.template` 和 `script/build_apple_music_downloader.sh`。以当前源码和消费端为准，不沿用旧版 wrapper 的端口或解密约定；保留无关未提交改动。

## 集成边界

Get Oudio 从 URL 启动 `darwin/arm64` downloader，以 `--events=jsonl` 消费 stdout。该模式保持非交互、每行一个有效 JSON 事件，保留 `schema_version`、`run_id`、`sequence`、`event`、`item_id` 和数据字段；人类可读输出不能进入 stdout。`run_started`、`item_started`、`progress`、`item_completed`、`item_failed`、`run_completed` 的语义及失败退出码须与消费端一致，错误和事件不得泄露 token、密钥或解密模板。Get Oudio 依靠 URL 中的单曲路径或 `?i=` 识别单曲，不使用旧 `--song` 参数。

`internal/wrapper` 统一处理 wrapper-lite HTTP API。响应须同时检查 HTTP 状态及 `{code,msg,data}`；业务失败可能仍是 HTTP 200。`/status`、`/m3u8`、`/key`、`/webplayback`、`/license` 等接口变更时，连同调用方和测试一起检查，不能只改端口。配置中的 `lite-server` 对应 Get Oudio 管理的本机服务；不要恢复旧四端口、`template-decrypt` 或 `key-server` 契约。

ALAC 解密依赖 `internal/fairplay-rip/runv4.go` 和 Temari；`CGO_ENABLED=0` 的 Go 可执行文件仍需同目录的 `libtemari.dylib`。升级 Temari 或改动加载路径时，同时检查库的架构、定位、签名与加载失败的非零退出码。不要把编译成功当作可运行证明。

## 上游同步与验证

同步上游时先更新指定远端并比较接口、配置、依赖和 Get Oudio 专用改动，再决定合并或选择性移植；不要用上游入口覆盖 JSONL 和错误语义。改动 wrapper 协议、解密或 CLI 时运行 `go test ./...`，并用相邻仓库的 `bash script/build_apple_music_downloader.sh` 验证实际内嵌的 arm64 二进制与 Temari。发布前固定干净的源码提交，核对 `go version -m`、`file`、`otool -L`、签名及配置，并把 downloader、wrapper QEMU 和 Get Oudio 作为同一组合做真实下载验收；按受影响格式分别记录 AAC、ALAC、Atmos 的结果。纯文档改动运行 `git diff --check`。
