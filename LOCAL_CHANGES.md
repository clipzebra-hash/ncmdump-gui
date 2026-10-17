# v1.2.1 本地离线优化版

目标：Intel x86_64 / macOS Ventura 13.7.8。保留 Go + Wails + React 技术栈，不升级依赖版本。

- 完全移除封面下载。仅保留 NCM 内嵌封面和文字标签；无内嵌封面时不补图。
- 固定最多 4 个后台转换任务。批量进度以实际终态计数，成功须完成解密、元数据写入、最终文件保存；失败也计入已完成数。取消的未完成项不计成功或失败，可继续处理。
- 显示完成/总数、当前文件（并发时多个）、成功/失败、真实耗时和取消状态；每个错误行保留具体原因，写入本机缓存目录 ncmdump-gui/conversion-errors.jsonl 便于诊断。每 250ms 读取后台快照。开始按钮立即锁定，转换期间禁止增删列表或变更输出设置。
- 前端合并添加与去重，后台解析绝对路径及符号链接后再次去重；重复路径的所有列表项同步状态。重复启动由后台拒绝。
- 音频 XOR 的 256 字节周期查表，缓冲区从 32 KiB 调为 256 KiB，检查读写错误。
- 默认只读源文件；解密和元数据写入本机临时目录，避免在 SMB 上反复重写。目标卷支持硬链接时一步发布；不支持或跨卷时使用独占创建与分块复制、Sync/Close，绝不替换已有文件。失败或取消时清理本次创建的未完成输出。SMB 兼容路径复制期间最终文件名会暂时可见，强制杀进程或断网造成清理失败时可能留下未完成文件；清理失败会报告具体路径。
- 修复源文件句柄泄漏、损坏头长度校验、部分 FLAC 无注释块时的空指针问题。

本地库分支：`third_party/ncmdump-go` 来自 ncmdump-go v1.7.5，保留许可证，通过 go.mod replace 使用。未修改全局 Go 模块缓存。

## 构建

已验证 Go 1.24.13、Wails CLI 2.11.0、Node 24.19.0、Apple Command Line Tools / clang 14.0.3。前端使用原 pnpm 锁文件。Go 与 Node 原本不在系统 PATH，本次工具链隔离下载/复用；未升级 macOS。

Git 仓库不提交生成的 Wails 绑定与前端 dist。首次构建先创建 `frontend/dist/.placeholder`，再生成绑定。

```sh
# 项目根目录；要求 go、node、pnpm 可用
go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
mkdir -p frontend/dist
touch frontend/dist/.placeholder
"$(go env GOPATH)/bin/wails" generate module
cd frontend
pnpm install --frozen-lockfile --ignore-scripts
node node_modules/esbuild/install.js
node node_modules/typescript/bin/tsc
node node_modules/vite/bin/vite.js build
cd ..
MACOSX_DEPLOYMENT_TARGET=13.0 "$(go env GOPATH)/bin/wails" build -s -m -platform darwin/amd64 -trimpath
codesign --force --deep --sign - build/bin/ncmdump-gui.app
```

`--ignore-scripts` 配合显式运行既有 esbuild 安装脚本，避免当前 pnpm 对旧项目安装脚本策略的差异，不增加依赖。

## 测试

```sh
NCM_TEST_FILE='/绝对路径/测试.ncm' go test -race -v ./...
(cd third_party/ncmdump-go && go test -race -v ./ncmcrypt)
```

可选完整批量回归：设置 NCM_TEST_DIR 为包含最多前 12 个测试 NCM 的目录；NCM_BASELINE_DIR 可指向对应原版输出，比较 FLAC 全字节及 MP3 的全部 ID3 帧内容和音频字节（忽略帧排列顺序）。基准工具在 audit/，输出仅能指向空的独立测试目录：默认串行；PARALLEL=1、WORKERS=4 启用四任务上限。

## 使用与回退

本仓库发布源码；此前本地安装与备份状态不属于公开仓库。自行构建后从 build/bin 打开应用。当前应用是本地临时签名版本，未经过 Apple 公证。应用偏好沿用原 ncmdump-gui 的保存目录配置。

取消采用协作式检查：解密每个缓冲区检查一次；正在执行的本地标签写入完成后再取消，可能需要额外等待。取消会保留已经完整保存的文件。退出应用仍受进程终止行为约束，不等同于点击取消。

源码回退：先保存未提交改动，再用 `git switch --detach v1.2.1` 查看原版；使用 `git switch improvements/offline-batch-progress` 返回改进分支。应用回退需自行保留原版应用。生成的前端绑定和 dist 可重新生成。

SMB 修复回归：NCM_SMB_TEST_DIR 指定目标共享测试目录，测试仅创建独立临时子目录并自动清理。当前共享已实测不支持硬链接和独占重命名；已通过独占复制、读回一致性、同名文件保护及真实 NCM 转换回归。网络 I/O 正阻塞时，取消需等待当前系统调用返回。

## 成功后删除源 NCM 选项

新增“转换 / 选项”页面；输出目录与删除开关位于选项页。删除默认关闭，并保存在配置的 delete_source 字段；旧配置缺少该字段时为 false。转换中禁止改变选项，批次使用启动时的选择。

启用后，仅对本批新转换成功的文件执行清理：成品保存后重新读取目标文件，用 SHA-256 与本地成品比较，再将源 NCM 保存到用户废纸篓并校验，确认源文件身份、内容和修改时间未变，最后移除原路径。废纸篓中的文件有唯一后缀，从废纸篓移出并改回原文件名即可恢复；清空废纸篓才释放其占用空间。

已有同名成品、转换/保存/校验失败或取消时保留源文件。删除阶段失败也保留成品并报告具体原因。符号链接不支持开启此选项，避免误删链接目标。开启时会额外读回文件，增加本机与 SMB 的校验耗时。不会追溯删除以前已转换的源文件。

删除回归使用自建 NCM 夹具的可丢弃副本与测试专用“废纸篓”目录，不会清理用户真实 NCM 或写入用户实际废纸篓。已验证成功清理、失败/取消/碰撞保留、源文件变化拒绝删除、废纸篓不可写时保留源文件、SMB 成品读回，以及旧配置默认关闭。

## 发布准备验证（2026-10-17）

从官方仓库完整 clone，v1.2.1 对应 cd14acf43782fbbdb960f45c7b6fa3ef8cb5e40b。此前下载的原始 tar.gz 内 41 个文件与该提交逐字节一致。上游 main 另有两项 README/工作区提交，尚未合并至此 v1.2.1 改进分支；没有将该分支称为最新上游。

本轮以自建 NCM 夹具重跑后台 race 回归及库测试，并通过 TypeScript、Vite 与 Intel 原生构建。不打包测试输入，不提交音乐或测量原始记录。真实音乐的 12 文件回归、SMB 测量属于此前记录，本轮未重跑；相关可选测试会显示 SKIP。原生窗口人工验收仍待完成。删除功能使用测试专用临时废纸篓，未删除用户真实文件。

公开包的目标限于经验证的 macOS Intel 分支；自动清理使用用户目录下 .Trash，未验证 Windows/Linux 回收站集成。SMB 复制期间会临时显示最终文件名，断网或强制杀进程可能留下未完成输出。发布前应检查这些已知限制。
