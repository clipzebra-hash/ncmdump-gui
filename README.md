# ncmdump-gui 离线优化分支

基于 [TaurusXin/ncmdump-gui](https://git.taurusxin.com/taurusxin/ncmdump-gui) v1.2.1 的社区修改版，非官方发行。原作者为 TaurusXin and contributors，保留原 MIT 许可证；本地修改沿用 MIT。完整上游 Git 历史保留。

增加最多四任务的并发队列、真实批量进度、取消、错误详情；优化解密读写，支持 SMB 不覆盖保存。移除远程封面下载，只保留内嵌封面。新增默认关闭的成功后移至废纸篓选项。

构建、测试、使用限制见 [LOCAL_CHANGES.md](LOCAL_CHANGES.md)。当前主要验证目标是 Intel Mac / macOS Ventura；尚未完成原生窗口人工验收，未验证 Windows/Linux。此仓库不包含音乐文件、个人配置、性能原始数据或应用二进制。

第三方转换库来自 ncmdump-go v1.7.5，保存在 third_party/ncmdump-go，保留其 MIT 许可证；字体许可证见 frontend/src/assets/fonts/OFL.txt。

上游当前要求在 [taurusxin/ncmdump 的 GitHub Issues](https://github.com/taurusxin/ncmdump/issues) 反馈 GUI 问题。这里的 Go/Wails GUI 与该 C++ 仓库代码不同，不能直接跨仓库提交可合并的 PR。

## 自动构建检查

Intel Mac CI 的触发、附件下载、测试边界与回退方法见 [CI 使用说明](docs/CI.md)。CI 附件是供验收的社区构建，不是正式 Release。
