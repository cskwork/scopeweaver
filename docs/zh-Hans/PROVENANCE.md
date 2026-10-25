[English](../PROVENANCE.md) · [한국어](../ko/PROVENANCE.md) · 简体中文 · [繁體中文](../zh-Hant/PROVENANCE.md) · [日本語](../ja/PROVENANCE.md) · [Español](../es/PROVENANCE.md)

<a id="scopeweaver-provenance-and-modifications"></a>

# ScopeWeaver 来源与修改

各语言版本请见本页上方导览。

ScopeWeaver 为 [Autumn-27/ARTEX](https://github.com/Autumn-27/ARTEX) 的独立衍生专案，基于提交 `160fe13`，另行维护于 [cskwork/scopeweaver](https://github.com/cskwork/scopeweaver)，不具 GitHub fork 网络关系。

**2026-10-02** 的 ScopeWeaver 修改包括产品名称、英文默认／韩文可选的界面与消息、文件与内附智能体指引、配置／构建／启动／更新／重设密码提示、报告与智能体语言处理及发行命名。可执行文件为 `scopeweaver`（Windows 为 `scopeweaver.exe`）；封存档命名为 `scopeweaver-<version>-<os>-<arch>.zip`。Docker Compose 在本地构建签出的源代码。发行工作流以本仓库与 GHCR 为目标。

原始 `LICENSE` 保持不变。原作者、贡献者致谢与上游变更历史仍归属 ARTEX。Go 模组 `github.com/Autumn-27/artex`、来源入口 `./cmd/artex`、`ARTEX_*` 配置契约、`artex` 数据库默认值与 `ARTEX` 登录名称均为兼容性而保留。

此修改日期并非版本标签，也不表示已发布二进位版本或容器镜像。`sidequestion/` 下的历史验证文件描述上游运行，并非新的 ScopeWeaver 验证收据。历史 JSON 翻译会保留技术识别码与测量值，并标示叙述文字为译文。

**2026-10-15** 新增可选的原生 Ollama 配置格式（`/api/chat`，搭配 `num_ctx`、`keep_alive` 与 `num_predict`）、新增表单的三种 Ollama 模板、免密钥本地配置、各配置并行数、Ollama 感知的预热、压缩与审批裁决默认值，以及选择性启用的 `local_llm_only` LLM 网络政策。新增 `SCOPEWEAVER_LLM_*` 与 `SCOPEWEAVER_LOCAL_LLM_*` 别名，优先于保留的 `ARTEX_*` 契约。既有 Anthropic、OpenAI、OpenAI Responses、自定义 URL、代理与 GLM 模板行为保持不变，政策默认关闭。

**2026-10-15** 后续原生强化候选版新增不抢占的紧急下一名额准入、依实际原生来源选择裁决模型、结构约束输出与 90 秒／ask 默认值，以及 64K 以内的 72% 压缩门槛与 16/32 KiB 工具输出上限。选择性严格目的地缩限为环回地址与明确推论 CIDR（包括 LAN／Docker 迁移），拒绝重新导向／代理，封锁所有智能体搜寻与更新网络操作，但保留本地状态／回滚。另提供只读 GPU 内存估算、共用模型警告、安全加载裁决配置，以及英文／韩文／简体中文界面文字。这些是应用程序控制，不保证整台主机离线或模型品质；未新增依赖包、迁移、自动拉取模型或发行。详见[供应商指南](llm-providers.md)与[候选版验证](VERIFICATION.md#native-hardening-candidate--2026-10-15)。

**2026-10-03** 额外修改完成服务器翻译审计，并新增 GLM-5.3 配置模板及其必需的常启推理兼容性。模板使用既有供应商适配器，不会自动创建或启用配置。

另已回移 ARTEX 提交 `e2c8a23`、`86729b6`、`a951e4a` 的修正：正式环境同源 SSE、独立审批模型 token 计量，以及失败时拒绝存取的验证、原子化初始密码创建与有界数据库连接池。新消息与部署指南提供英文及韩文。回移提交已注明原作者，其他 ScopeWeaver 修改保持不变。
