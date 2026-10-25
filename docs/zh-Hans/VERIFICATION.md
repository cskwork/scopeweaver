[English](../VERIFICATION.md) · [한국어](../ko/VERIFICATION.md) · 简体中文 · [繁體中文](../zh-Hant/VERIFICATION.md) · [日本語](../ja/VERIFICATION.md) · [Español](../es/VERIFICATION.md)

<a id="verification-and-known-limitations"></a>

# 验证与已知限制

各语言版本请见本页上方导览。

<a id="shared-runtime-fixes-from-standalone-cli-verification-2026-10-17"></a>

## 独立 CLI 验证发现的共用运行时修正（2026-10-17）

候选分支 `fix/cli-discovered-runtime-bugs`，基于 `dev` 提交 `3f0ef25`。以下为本地结果；PR 确切提交的托管检查另计。此变更未将独立 CLI 或订阅供应商加入原始应用程序。

- HTTP 自定义工具现在使用 `Manager.ProxyAddr` 传回的完整记录代理 URL。先前 `"http://" + addr` 将测试的 `http://127.0.0.1:<port>` 变成 `http://http://127.0.0.1:<port>`，尚未抵达代理便失败；现在 `proxyStr = addr` 保留 URL。真实直接、明确配置与自动代理路由皆通过；自动路由修正前失败。
- 启动时先确认既有对话记录路径是目录再读取。Windows 一般文件回归修正前失败，现会拒绝路径并保留文件；不存在的目录仍有效。新诊断提供英文、韩文、简体中文。
- 关闭时先取消工作、排空 HTTP，再等待已准入对话、侧问与快照发布，最后关闭保存区。先前仅排空 HTTP，可能让背景结束处理在数据库关闭后写入。最终快照／上下文写入错误现会向上传递、保留待写项目，并使进程明确关闭失败；已删／冲突记录仍视为无害。此有界排空不宣称涵盖所有排程器或引擎生产者。
- 取消用量测试数据现创建既有选用用量表，符合正式初始化。修正前重现缺表失败；修正后全新数据库测试保留精确用量断言。

真实 PostgreSQL 关闭检查通过 11 个测试／子测试，含关闭重开持久化与注入最终写入失败。主进程通过 4 项，涵盖正常结束 0、重启 75、有界失败及真实连接埠占用服务失败的清理。聚焦代理／启动／语言区域回归通过。完整 Windows 后端为 1,975 通过、2 失败、15 略过；一项为未改变的符号链接权限限制，另一项揭露目录预期数，因新增两诊断由 1,980 改为 1,982，未移除完整性检查。修正目录另行验证；其余平台结果须由 Linux／托管检查创建。

前端 65 测试与静态导出通过，既有 POSIX npm 脚本使用 Git Bash。适配器 7 测试与语法检查通过。聚焦后端静态分析通过。未使用用户数据库、真实线上目标或通知；各写入测试均创建并删除独立 `scopeweaver_test_*`。不宣称合并或发行。

<a id="chat-controls-and-context-2026-10-16"></a>

## 对话控制与上下文（2026-10-16）

[PR #29](https://github.com/cskwork/scopeweaver/pull/29) 作者针对上游 `6366429` 回报以下检查。这是 PR #29／#30 合并审查前的贡献者历史 Linux 结果，不证明后续候选状态或 Windows 附件复制行为。

| 检查 | 结果 |
| --- | --- |
| 可抛弃 PostgreSQL 17 竞态包（`db`、`server`、`agent`） | 667 顶层测试通过，无竞态报告 |
| Go 包与静态分析 | `go test ./... -count=1`、`go vet ./...` 通过 |
| 前端测试、翻译、审计、静态导出 | 65 测试；3,263 EN／KO／zh-Hans 键对等；0 审计问题；TypeScript 与 31 页通过 |
| 后端翻译 | 1,978 正式键在三语通过完整性、对等与 printf |
| 对话浏览器关卡 | EN／KO／zh-Hans、1440／390 px、独立／任务对话共 60 情境通过，无未捕捉页面错误 |
| 内嵌应用基本测试与仪表板 | Smoke E2E 与 14 仪表板情境通过，无未捕捉浏览器错误 |
| 智能体适配器 | 7 单元、语法及 3 MCP／Pi 情境通过 |
| 安装与封装 | 8 安装数据、11 Compose 案例、每语 EN／KO 各 5 封存档、6 发行声明测试通过 |

对话浏览器案例涵盖队列编辑／排序、重启确认、重试／编辑／分支、上下文检查／压缩、工具引导与文件贴上。后端回归涵盖已接受语言持久化、预检错误、中断工具交换、放弃分支准备与文件读取归属。

可将模型目前上下文与计费对话用量分开检视。压缩保留您选择的近期完整交换；完整显示历史与之前尝试用量仍保留。

贡献者回报一次审查找出六个整合缺陷，之后修复生成输入准入与复测回滚、压缩后附件路径、旧版显示历史及引导投递控制。一项附件测试在子分支下次提示前删父分支，另一项检查 rebase 保留 manifest 字面文字。各修复前均回报失败回归，之后完整本地重跑。仪表板推理总数断言限缩至摘要列，因悬停提示有相同标签；重跑回报 14 仪表板情境通过。

回报推论测试使用环回模型与可抛弃数据库。Windows 运行、付费供应商、远程 CI 需另验证。声明测试使用 Python 3，符合 CI 解释器需求。

<a id="native-hardening-candidate--2026-10-15"></a>

## 原生强化候选版 — 2026-10-15

在基于 main `33b131e` 的 `feat/ollama-zero-egress-hardening` 进行本地与独立检查。这是候选结果，确切提交的远程 CI 与 PR 交付仍待完成。以下历史段落保留原始结果。

| 实际运行检查 | 结果 |
| --- | --- |
| 独立 Go 运行，20 包各用隔离可抛弃 PostgreSQL | 1,366 测试／子测试通过、0 失败；`enrich` 无测试；2 选择性略过（`TestAgentAdapterE2E`、`TestLiveContextReview`） |
| 另测智能体适配器 | 7 单元及 3 真实 MCP／Pi 情境通过；刻意不运行外部真实供应商审查 |
| 聚焦后端 `-race` | 传输、wire／用量、上下文、来源选择／失败、搜寻／更新、准入皆通过，无竞态 |
| `go vet ./...` | 通过 |
| 前端单元／本地化与静态导出 | 45 通过；3,168 EN／KO／zh-Hans 键保留对等、占位符、rich tag；TypeScript 与全部 31 页通过 |
| 后端目录 | 1,943 正式键通过原始三语完整性／对等与 printf |
| 安装、Compose、声明 | 8 安装、11 Compose 凭证、EN／KO 每语 5 封装测试封存的声明检查、Python 3.12 的 6 发行声明测试通过 |
| 静态构建与依赖检查 | macOS 内嵌与 Windows amd64 交叉构建通过；`go.mod`／`go.sum` 无漂移；未运行原生 Windows |
| 内嵌浏览器基本测试 | 最终 UI 重建后 60 通过（48 smoke + 12 仪表板）、0 失败。EN／KO／zh-Hans 稳定桌面／行动画面及水平边界通过：390px sheet [0,390]，1440px [864,1440]，全部 20 项关键内容可容纳。已重现并修正先前 420px 最小宽裁切 |

[原生 HTTP 优先级回归](../../server/ollama_priority_test.go) 将 worker 1 留在 `/api/chat`，排队 worker 2／3，再呼叫真实接线裁决，不由呼叫端标记紧急。`-race` 通过，HTTP 顺序 **worker 1 → judge → worker 2 → worker 3**，峰值并行 1，原生裁决 schema／verdict 有效，全部完成且无名额泄漏。既有队列测试涵盖 FIFO 角色、取消与降低上限。

[原生来源失败回归](../../server/provider_origin_failure_test.go) 修正前失败：原生工具待运行后流式出错，可能继承全域云端 `allow`。修正后 Stream／Complete 与终止路由错误保留具体原生配置、90 秒／`ask` 模型政策、300 秒／`deny` 人工审批到期，不呼叫审查者。实际云端容错仍清除原生偏好。回归与受影响竞态包通过。

已验证 API 对不转发请求的本地代理切换严格模式 **false → true → false**。四种搜寻后端 `ddgs`、`brave-free`、`tavily`、`deepseek` 仅在关闭时到达该接收端，严格模式各新增请求为零。严格更新检查／应用遭封锁，本地状态可用。[更新回归](../../server/update_policy_test.go) 亦通过缓存、重新导向、即时本文读取，以及已准入异步下载被后续切换阻挡后的清理／失败完成。这不代表整台主机出口控制。

主机 Ollama **0.32.15** 使用既有 `qwen3:4b`、**4,096 token 输出预算**，通过原生诊断、decision／comment schema、结构工具呼叫／结果重放、用量与要求 32K（`/api/ps` **32,768**）。先前 256／512 预算失败仍保留证据，未丢弃或算成功。加载／速度／卸载数值只是一次热运行快照，非基准或驻留保证。未下载模型或改主机配置，不宣称自主任务完成或安全评估品质。

浏览器涵盖三语诊断／警告、严格更新状态、裁决加载／失败保护及既有仪表板／用量。视觉 QA 重现固定 420px 最小宽编辑器在 390px 视窗左溢 30px。最小宽现符合视窗；重建应用通过全部 60 项，六张桌面／行动画面标签可读，稳定内容在视窗内。独立内嵌重建与浏览器可执行文件逐字节一致。确切提交远程 CI 与 PR 读回另为交付关卡。不宣称合并、发行、付费供应商呼叫、外部目标扫描或真实通知。

<a id="provider-authentication-follow-up--2026-10-15"></a>

## 供应商验证后续 — 2026-10-15

基于已合并 main `38fde23` 的 `fix/provider-env-auth`，本地回归重现并修正两个需验证的本地网关失败。旧版 Anthropic、OpenAI Chat Completions、OpenAI Responses 的启动供应商密钥回退与空密钥连接测试皆回 HTTP 401；相同数据在修正后通过。

| 检查 | 结果 |
| --- | --- |
| [`FromEnv → NewProvider` 本地 HTTP 验证](../../agent/provider_env_test.go) | 21 情境通过：18 一般格式涵盖供应商密钥、文件密钥、现代／旧版覆写、明确空白；3 原生涵盖云端密钥隔离、Ollama 令牌、明确空白 |
| [通过本地 HTTP 数据的连接测试 handler](../../server/llm_gateway_test.go) | 16 情境通过：15 一般格式涵盖相符正规化服务、不同端点／格式／代理、保留非本地回退；1 原生拒绝继承启用云端密钥 |
| 两聚焦组 `-race` | 通过，无竞态 |

上述 **37 个案例**为叶情境，不含父测试。各修正前重现验证失败，之后同测试通过。非本地 handler 案例中的公开文件地址经明确本地测试代理，不联络公开供应商。

无 profile ID 的空密钥连接测试，仅在一般格式、正规化基础 URL、代理全相符时重用目前本地服务密钥，含新草稿。不同本地服务与原生 Ollama 保持隔离。空密钥新配置不导入环境凭证。

这些检查创建已测协定与凭证边界，不代表付费帐号可用或全部历史功能。修正后完整包、浏览器、CI、已合并 main、发行产物结果另为关卡。未呼叫付费供应商。

<a id="installed-model-selection-follow-up--2026-10-15"></a>

## 已安装模型选择后续 — 2026-10-15

在 `feat/ollama-native-flexible-providers` 本地验证新配置 Ollama 默认选择，取代先前模板行为：选 Ollama 现运行只读模型探索。下方历史 37 检查浏览器快照早于此行为；「无隐含请求」为历史断言，不是目前契约。

| 检查 | 结果 |
| --- | --- |
| 前端单元与本地化（`npm --prefix web test`） | 45 通过、0 失败 |
| 正式静态导出（`npm --prefix web run build:static`） | TypeScript 与全部 31 页通过 |
| 浏览器与已验证后端真实主机模型探索 | 未存新配置、同机 Ollama、空代理／密钥通过 |
| 内嵌可执行文件与全新 `scopeweaver_test_default_final` 完整扩展浏览器 smoke | 43 通过、0 失败（31 smoke + 12 仪表板）；涵盖 EN／KO 三模板安装默认、直接格式选择、云端兼容、仅元数据选择、失败／空探索、手改／拒绝延迟响应、重开草稿／既有配置，以及既有配置、登录、本地化、SSE、仪表板回归 |

主机 `/api/tags` 传回 `qwen2.5-coder:7b`、`qwen3:4b`、`embeddinggemma:latest`。浏览器选 **`qwen2.5-coder:7b`**，第一个声明 `tools` 的已安装模型。元数据为 `completion`、`tools`、`insert`、7.6B、Q4_K_M；`qwen3:4b` 亦声明 `tools`，`embeddinggemma:latest` 仅 `embedding`。

浏览器恰好送一次 LLM 操作 `POST /api/llm/models`，主机端点、无密钥／代理。新草稿关闭，未保存、启用、测连接或推论。模型／机器配置不变。这仅证明安装模型选择与元数据探索，不代表工具运行或自主可靠性。

<a id="historical-native-ollama-branch--2026-10-15-branch-local-not-a-release"></a>

## 历史原生 Ollama 分支 — 2026-10-15（仅分支，非发行）

`feat/ollama-native-flexible-providers` 开 PR 前的本地检查。远程 CI 另于 GitHub Actions 追踪；本节不代表发行、合并或 CI 结果。

| 检查 | 结果 |
| --- | --- |
| 20 个 Go 包各用全新可抛弃 PostgreSQL 16 | 1,197 测试／子测试通过；2 选择性略过（`TestAgentAdapterE2E`、`TestLiveContextReview`） |
| 另测适配器 | 3 真实 MCP／Pi 情境通过 |
| `go vet ./...` | 通过 |
| 前端单元／本地化 | 43 通过；英／韩／简体中文各 3,159 唯一键 |
| 后端消息目录 | 三语各 1,938 唯一键；printf／对等通过 |
| TypeScript／正式静态导出 | 通过，最终 health UI 后重建 |
| `npm audit` | 0 漏洞 |
| 构建 | 最终 health UI 后重建 `CGO_ENABLED=0` 内嵌可执行文件；Windows amd64 `CGO_ENABLED=0` 交叉构建通过，未运行原生 Windows |
| 真实内嵌可执行文件与全新 `scopeweaver_test_ollama_final_health` 的 `npm run smoke` | 37 通过、0 失败（25 smoke + 12 仪表板）。含 EN／KO 三 Ollama 模板无隐含请求、免密钥保存／启用、本地免密钥兼容 health 与缺密钥公开云端对比、云端格式／可编辑 GLM、模型元数据／工具警告、缺模型精确 pull 命令、工具／上下文／加载／速度诊断、默认关闭本地政策，以及既有三语配置、登录、语言、草稿、SSE、审批用量回归 |
| Compose 凭证 | 真实 `network_mode: none` 容器经环境与 `FromEnv`，11 案例通过 |
| 安装与封装 | 8 安装数据共通过，含英／韩选择；英／韩各 5 封存档通过 |
| 发行声明（Python 3.12） | 6 测试通过 |
| 独立后端安全／正确性审查 | 通过，含 DeepSeek 本地政策、即时 CIDR、404 分类 |

<a id="real-ollama-run"></a>

### 真实 Ollama 运行

主机 Ollama 0.32.15、`qwen3:4b`，经原生流式 API：

- SDK 层级流式检查的合成工具数据中，模型呼叫 `record_fact`，ScopeWeaver 在本地运行并重放结果，回合完成；这不是平台数据库的 `record_fact`。
- 要求 32K，`/api/ps` 回报分配 32,768。
- 精确用量：流式回合输入 186／输出 750；完成回合输入 826／输出 193。
- 真实 API 免密钥保存／启用通过。诊断 `tool_call_verified`、`context_verified` 皆 true，上下文 32,768、加载 2.225375 ms、29.231 token/s、延迟 18,958 ms。**仅一次热运行，非冷启基准或硬体泛化。**
- 缺模型的精确韩文 pull 提示及严格政策切换封锁公开 IP 通过。
- **平台任务 Worker Bash 证据**：task 1 的 `qwen3:4b`（要求 32K、分配 32,768；并行 1；keep-alive 30m；思考关闭；上限 4,096）发出原生 Bash `call_219yv72x`。活动 9（call）、10（`tool_result`）同 ID、无错误，输出含 `LOCAL_TASK_OLLAMA_OK`。帐本精确记 6,704 输入／4,096 输出；下一原生呼叫重放相同 `tool_call_id`。意图通过既有 `db.AddIntent` 手动植入，非 LLM 创建。输出亦含测试数据的字面 `\n` 跳脱，不据此推断换行处理或模型错误。可执行文件来自最终 UI／DTO 前的未提交分支源代码。

<a id="autonomous-task-reliability-is-not-validated"></a>

### 自主任务可靠性未验证

工具诊断通过仅证明传输路径，不代表模型可完成任务。

- `qwen2.5-coder:7b` 声明 `tools`，却将工具形状 JSON 写入 `assistant.content`，不是传输 `tool_calls`，因此无 Bash／事实工具运行；ScopeWeaver 正确保留纯文字。
- `qwen3:4b` 作主要代理时，长提示下反复思考并以文字写出预期标记。作任务 Worker 确实运行上述 Bash，但 360 秒时任务暂停，未 `record_fact` 或自主完成；一次成功工具呼叫不代表自主完成。

**不对任何本地模型宣称端对端自主完成或安全评估品质，7B 级模型亦未验证可作完整智能体。** [硬体分级](llm-providers.md#hardware-tiers) 为估算，非基准测试。

<a id="not-exercised"></a>

### 未运行项目

无付费云端呼叫、外部目标扫描、真实通知、发布、部署、发行或合并。Anthropic、OpenAI Chat／Responses、自定义网关经传输契约测试、回归与可编辑浏览器流程验证，不使用真实供应商帐号。测试数据库皆可抛弃，主机 Ollama 配置未改。选用 `local_llm_only` 默认关闭；服务器仍需另设 `OLLAMA_NO_CLOUD`。

<a id="running-database-backed-tests"></a>

## 运行数据库测试

`db`、`server`、`agent` 包需要 PostgreSQL，连接字串**只**从 `ARTEX_TEST_DSN` 取得，绝不回退应用程序的 `ARTEX_PG_DSN`／`config.json`。此设计防止 `go test` 修改运行中应用的即时／正式数据库。

- **未设 `ARTEX_TEST_DSN`**：全部数据库测试略过，不接触数据库。
- **`ARTEX_TEST_DSN` 设为可抛弃数据库**：包对该库运行。
- **解析至与应用相同数据库**（同主机、连接埠、库名）：任何测试连接前即快速失败，错误 `refusing to run tests against the live database; point ARTEX_TEST_DSN at a throwaway DB`。

最简单的运行方式：



```bash
make test-db
```



`make test-db`（通过 `scripts/test-db.sh`）创建由 `sw_user_test` 角色拥有的可抛弃 `scopeweaver_test_<pid>`，将 `ARTEX_TEST_DSN` 指向该数据库，再由 `ARTEX_TEST_DSN` 指定测试连接，运行 `go test ./db/ ./server/ ./agent/ -count=1`，并在结束时删除数据库，即使测试失败亦同。可覆写 `TEST_DB_ROLE`、`TEST_DB_HOST`、`TEST_DB_NAME` 配合本地 PostgreSQL。库名必须以 `scopeweaver_test_` 开头，适配器 E2E 可用 `scopeweaver_adapter_`。

若要针对自行管理数据库运行部分测试，导出 `ARTEX_TEST_DSN` 并直接呼叫 `go test`：



```bash
export ARTEX_TEST_DSN='postgres://sw_user_test@/scopeweaver_test_1?host=/tmp&sslmode=disable'
go test ./server/ -run TestSomething -count=1
```



<a id="v021-candidate--2026-10-11"></a>

## v0.2.1 候选版 — 2026-10-11

Windows 候选版以真实静态前端构建，针对可抛弃 PostgreSQL 16.15 测试。结果取代下方历史依赖与 Windows 运行限制；旧记录保留供追溯。

| 检查 | 结果 |
| --- | --- |
| 前端依赖审计 | 全严重度 0 漏洞，含开发依赖 |
| 前端回归／本地化 | 26 通过、无略过 |
| 正式静态导出 | TypeScript 通过，生成 31 页 |
| 原生 Windows Go、18 包 | 1,016 测试／子测试通过、0 失败、13 既有平台／选择性略过 |
| 原生 Windows 完整 server | 311 测试／子测试通过、3 既有选择性／平台略过 |
| 适配器 | 7 单元／语法通过；真实已验证后端与新库的 3 MCP／Pi 情境通过 |
| 本地安装 | 4 隔离数据皆通过：新安装、旧档替换、依赖失败、前端构建失败 |
| 后端翻译目录 | 1,868 正式键通过英／韩完整性与 printf 参数 |
| 通知隔离 | 确定性旧 Worker 回归修正前失败、之后通过；管理／通知选择连续 5 次通过 |
| Windows 证据 | 完整 PostgreSQL 包通过，含并行写入、提交失败、长路径 |
| Windows Python／技能档 | 真实 Python 与无法使用启动器的回退通过；巢状 Unicode 路径通过清单／读取 handler |
| 内嵌 Windows 浏览器 E2E | 8 通过：两语配置服务中断保护、配置／登录、同源 SSE、用量 API 隔离、两语用量卡、无未捕捉错误 |

Go 数量含父测试与子测试。13 原生略过为十个 Unix 自更新数据、一个缺符号链接权限、选择性适配器整合及真实供应商。适配器另验证，Unix 数据由 Linux CI 涵盖。未为产生结果停用测试。

回归重现原 Windows 目录同步失败、本地化分隔符不符、不可用 Python 启动器、通知 Worker 泄漏与配置清理顺序。E2E SQL 辅助现将选项置连接 URL 前；直接中断／恢复检查确认 Windows psql 运行陈述式。

发行关卡另要求合并前后的 `frontend`、`windows`、`backend`、`agent-adapter`、浏览器 `smoke`。发布结果、平台封存与校验值见 [v0.2.1](https://github.com/cskwork/scopeweaver/releases/tag/v0.2.1)；此来源记录不宣称后续工作或下载资产已通过。

保留既有 Python 解释器路径。若旧安装存入不可用启动器别名，请在配置选可运行解释器。原生 Windows 测试不证明 Windows 自更新或 macOS 运行。未使用付费供应商、外部目标或真实通知收件人。

<a id="historical-verification--2026-10-03"></a>

## 历史验证 — 2026-10-03

**2026-10-03** 记录的英／韩 ScopeWeaver 验证，基于上游 ARTEX `160fe13`，使用可抛弃 PostgreSQL 与本地数据。此为源代码交付，当时未发布二进位或容器映像。

| 检查 | 结果 |
| --- | --- |
| 前端回归／本地化 | 21 通过，TypeScript 通过 |
| 正式静态导出／Go 内嵌可执行文件 | 构建成功 |
| Go 回归、18 包 | 986 测试／子测试通过，1 继承失败，1 外部供应商略过 |
| 完整 server | 285 测试／子测试通过，仅选择性真实供应商略过 |
| 后端翻译 | 1,865 正式键通过英／韩完整性与 printf |
| 自撰中文审计 | 无未翻译自撰来源／文件／Go 注解，保留刻意兼容／数据例外 |
| 桌面浏览器 | 每语 24 路由，无缺标签或页面水平溢出 |
| 行动浏览器 | 两语仪表板、任务、发现、配置，390px 无页面水平溢出 |
| 真实应用画面 | 两语登录／通知表单检查；配置验证／初始化通过 |
| 语言切换 | 导览／状态更新，保留选择与未送草稿 |
| HTTP 本地化 | Markdown／CSV 导出、协商、偏好保存、无效语言拒绝通过 |
| GLM-5.3 契约 | 本地 HTTP／SSE 21 聚焦测试／子测试通过，含两端点、推理、工具重放 |
| GLM 模板浏览器 | 两语两模板，保留名称／密钥／代理；无保存、启用或供应商请求 |
| 封装／启动脚本 | Shell 语法、双语封装数据、10 监督案例通过 |
| 来源／依赖 | 原授权、依赖版本、Go 模组不变；文件链接与差异检查通过 |

Go 数量含父测试与子测试。各包用独立数据库，初始化与一般启动相同的补充计量表。原始测试与检查保持启用。

<a id="inherited-issues-at-the-time"></a>

### 当时继承问题

`TestGraphOverviewExpandsAssociatedCompanyScope` 在 ScopeWeaver 与未改上游皆因 `task scope missing: <nil>` 失败。测试预期图概览含详细范围，但上游 `06a43f3` 刻意精简。`75a706d` 改为 `TestGraphOverviewKeepsAssociatedCompanyScopeCompact`，检查精简概览并直接验证保存的公司范围。此后完整后端 CI 已通过。

上游验证曾在 `TestTaskMetadataPatchReturnsRenameAndPin` 出现间歇暂存目录清理失败；最终 ScopeWeaver server 运行通过，未以无关生命周期变更隐藏。

上游 mock 预览无通知元数据 mock，因此通知页报错。**真实后端的通知页与新增通道表单在两语皆通过**，验证未传送通知。

v0.1.0 记录 **14 个 npm audit 问题：1 严重、9 高、4 中**。框架／工具更新及移除未用 shadcn CLI 依赖，已于上方 v0.2.1 候选解决。运行 `cd web && npm audit` 查看 npm 软件包仓库的最新审计报告；新公告可能改变结果。

<a id="historical-scope-and-limits"></a>

### 历史范围与限制

- 选择性 `TestLiveContextReview` 需私人外部供应商配置，未运行。无付费模型、外部目标扫描或真实通知。
- GLM 测试仅证明本地契约，不代表帐号存取／供应商可用。Coding Plan 仅支持官方工具，ScopeWeaver 未列入，见[供应商配置](llm-providers.md)。
- 未验证 Docker 映像／Windows 批次运行；封装数据不代表真实 Windows 测试。
- 配置进入已验证路由，但一次等待 window load 逾时；之后以真实 API 确认初始化与新密码。
- 用户文字、保存证据、编辑提示词、传输识别码、旧剖析标记、Unicode 数据与上游原画面均保留。英／韩应用不翻译任意用户数据或第三方工具输出。
- `sidequestion/` 历史验证描述上游工作，不是新的 ScopeWeaver 运行。

目前远程检查见 [GitHub Actions](https://github.com/cskwork/scopeweaver/actions/workflows/ci.yml)。

<a id="chat-document-paste-2026-10-16"></a>

## 对话文件贴上（2026-10-16）

PR #29 记录对可抛弃 PostgreSQL 17 与本地后端的以下贴上检查。这是贡献者于 2026-10-16 回报结果；目前整合证据另记：

- 前端 52 测试与正式静态导出通过。
- 剪贴簿浏览器 12 组通过：独立／任务对话、EN／KO／zh-Hans、1440／390 px。
- 涵盖单一草稿拥有者的快速文件贴上、伴随文字、原生 3,000 字符插入／撤销、Unicode 预览、草稿恢复、上传重试、导览、mention、IME 与两种送出偏好。贴上未提交任何消息。
- 贡献者停用 React paste handler，确认因缺第二附件标签而失败，再恢复重跑。
- 后端 smoke 通过剪贴簿与 Ollama 环回数据，无未捕捉页面错误。

文件案例通过 paste event 提供浏览器 File 对象；文字与撤销案例使用 Chromium 剪贴簿与键盘贴上。
