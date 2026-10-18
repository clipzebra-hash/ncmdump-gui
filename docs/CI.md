# CI 使用说明

本流程只生成供检查的社区构建，不创建标签，不创建或更新 Release，也不向上游发消息。

## 流程

Actions 是 GitHub 的自动化功能；Workflow 是 `.github/workflows/ci.yml` 里的步骤清单；Runner 是执行清单的临时电脑。每次运行先安装工具、生成 Wails 绑定、检查 TypeScript 并构建前端、执行两个 Go 模块的 race 测试，再编译和检查 Intel Mac 应用。任一步失败，后续应用上传不会执行。Artifact 是这次运行的临时下载附件；Release 是面向用户的正式版本页面，两者独立，本流程没有 Release 权限或发布步骤。

在仓库 Actions → CI - macOS Intel → 对应运行中查看步骤日志。成功运行页面底部有 `ncmdump-gui-macos-intel-<完整提交号>` 和 `ci-logs-macos-intel-<运行号>-<重试号>` 附件。应用附件解压后包含应用 zip、SHA-256、构建来源与许可证。下载通常需要登录 GitHub。附件保留 14 天，过期后可重新运行；GitHub 原生日志另受仓库保留策略约束。失败时尽力上传已有诊断日志，但 runner 未启动或下载 action 失败时可能没有附件。

## 触发与权限

推送 main 或 ci/** 分支、打开/更新 PR、手动运行可触发。手动按钮通常要等 workflow 合并到默认分支后才出现。相同 workflow/ref 的新运行取消旧运行，每次最多 30 分钟。没有标签触发，也没有 pull_request_target。令牌只有 contents:read，检出后不保留凭据，不使用仓库 secrets。

所有 GitHub actions 固定到完整提交 SHA（旁边注明版本）。Go 1.24.13、Node 24.19.0、pnpm 9.15.9、Wails CLI 2.11.0 固定版本；pnpm 使用 frozen-lockfile 和 ignore-scripts，只显式执行已有 esbuild 安装脚本。Go 使用 readonly 模块模式和 go.sum 校验，两套 go.sum 均参与 Go 缓存键；不缓存 node_modules、应用或 Wails CLI。Wails 模块的间接工具依赖由其模块文件与校验服务约束。系统镜像、Xcode、SDK、npm 的工具下载仍是外部输入，因此不承诺逐字节可复现。日志记录系统与工具版本以方便追溯。

## 平台与验证边界

明确选择 macos-15-intel，不使用可能指向 ARM 的 macos-latest。runner 自带 Xcode/clang 和 macOS SDK，Wails 使用系统 WebKit，不需要 Linux 的 GTK/WebKitGTK。Wails 2.11.0 默认另加 10.13 的编译参数，因此同时显式设置 CGO_CFLAGS / CGO_CXXFLAGS / CGO_LDFLAGS 的 -mmacosx-version-min=13.0 与 MACOSX_DEPLOYMENT_TARGET=13.0，并检查实际 Mach-O 的 x86_64 架构、minos 与 plist 最低版本。最后做本地临时签名和签名完整性检查；没有 Apple Developer ID 签名或公证，下载后可能被 Gatekeeper 阻止。不要把编译成功当成 Ventura 13.7.8 实机验收。

CI 自动使用确定性人工 NCM 样本，测试转换、队列、取消、输出碰撞保护与测试临时废纸篓；仍可设置 NCM_TEST_FILE 使用外部样本。真实音乐批量回归与 SMB 测试需要专用环境，默认会显示 SKIP。CI 不接触真实用户废纸篓。不将人工样本称为真实音乐兼容性证明。Windows/Linux 的废纸篓语义尚未验证，因此暂不提供这些应用附件。

## 成本与回退

本仓库为公开仓库。GitHub 官方说明标准托管 runner 在公开仓库免费；GitHub Free 的附件额度为 500 MB（与 Packages 共用），缓存为每仓库 10 GB；实际账户套餐未核实。附件存储与账户配额、保留策略仍应在 Settings → Billing and licensing / Actions 中核对。这里仅使用标准 runner，保留期 14 天；没有启用付费大规格 runner，也未更改账单设置。私有仓库、收费规则变化或额外存储可能影响费用。参考：https://docs.github.com/en/billing/concepts/product-billing/github-actions

在合并前可以直接放弃 CI 分支；合并后可在 Actions 中 Disable workflow 暂停，或通过新提交撤销本次 CI 改动。不要强推改写已有历史。下载 CI 应用不会自动替换本机应用；人工试用前保留旧应用，输出到独立目录，删除源文件选项保持关闭。正式标签发布须先人工检查成功日志、许可证、Ventura 实机行为和附件，再由用户单独确认发布内容。
