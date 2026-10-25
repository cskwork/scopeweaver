[English](../VERIFICATION.md) · [한국어](../ko/VERIFICATION.md) · [简体中文](../zh-Hans/VERIFICATION.md) · 繁體中文 · [日本語](../ja/VERIFICATION.md) · [Español](../es/VERIFICATION.md)

<a id="verification-and-known-limitations"></a>

# 驗證與已知限制

各語言版本請見本頁上方導覽。

<a id="shared-runtime-fixes-from-standalone-cli-verification-2026-10-17"></a>

## 獨立 CLI 驗證發現的共用執行時修正（2026-10-17）

候選分支 `fix/cli-discovered-runtime-bugs`，基於 `dev` 提交 `3f0ef25`。以下為本機結果；PR 確切提交的代管檢查另計。此變更未將獨立 CLI 或訂閱供應商加入原始應用程式。

- HTTP 自訂工具現在使用 `Manager.ProxyAddr` 傳回的完整記錄代理 URL。先前 `"http://" + addr` 將測試的 `http://127.0.0.1:<port>` 變成 `http://http://127.0.0.1:<port>`，尚未抵達代理便失敗；現在 `proxyStr = addr` 保留 URL。真實直接、明確設定與自動代理路由皆通過；自動路由修正前失敗。
- 啟動時先確認既有對話記錄路徑是目錄再讀取。Windows 一般檔案迴歸修正前失敗，現會拒絕路徑並保留檔案；不存在的目錄仍有效。新診斷提供英文、韓文、簡體中文。
- 關閉時先取消工作、排空 HTTP，再等待已准入對話、側問與快照發布，最後關閉儲存區。先前僅排空 HTTP，可能讓背景結束處理在資料庫關閉後寫入。最終快照／上下文寫入錯誤現會向上傳遞、保留待寫項目，並使處理程序明確關閉失敗；已刪／衝突記錄仍視為無害。此有界排空不宣稱涵蓋所有排程器或引擎生產者。
- 取消用量測試資料現建立既有選用用量表，符合正式初始化。修正前重現缺表失敗；修正後全新資料庫測試保留精確用量斷言。

真實 PostgreSQL 關閉檢查通過 11 個測試／子測試，含關閉重開持久化與注入最終寫入失敗。主處理程序通過 4 項，涵蓋正常結束 0、重啟 75、有界失敗及真實連接埠佔用服務失敗的清理。聚焦代理／啟動／語系迴歸通過。完整 Windows 後端為 1,975 通過、2 失敗、15 略過；一項為未改變的符號連結權限限制，另一項揭露目錄預期數，因新增兩診斷由 1,980 改為 1,982，未移除完整性檢查。修正目錄另行驗證；其餘平台結果須由 Linux／代管檢查建立。

前端 65 測試與靜態匯出通過，既有 POSIX npm 指令碼使用 Git Bash。配接器 7 測試與語法檢查通過。聚焦後端靜態分析通過。未使用使用者資料庫、真實線上目標或通知；各寫入測試均建立並刪除獨立 `scopeweaver_test_*`。不宣稱合併或發行。

<a id="chat-controls-and-context-2026-10-16"></a>

## 對話控制與上下文（2026-10-16）

[PR #29](https://github.com/cskwork/scopeweaver/pull/29) 作者針對上游 `6366429` 回報以下檢查。這是 PR #29／#30 合併審查前的貢獻者歷史 Linux 結果，不證明後續候選狀態或 Windows 附件複製行為。

| 檢查 | 結果 |
| --- | --- |
| 可拋棄 PostgreSQL 17 競態套件（`db`、`server`、`agent`） | 667 頂層測試通過，無競態報告 |
| Go 套件與靜態分析 | `go test ./... -count=1`、`go vet ./...` 通過 |
| 前端測試、翻譯、稽核、靜態匯出 | 65 測試；3,263 EN／KO／zh-Hans 鍵對等；0 稽核問題；TypeScript 與 31 頁通過 |
| 後端翻譯 | 1,978 正式鍵在三語通過完整性、對等與 printf |
| 對話瀏覽器閘門 | EN／KO／zh-Hans、1440／390 px、獨立／任務對話共 60 情境通過，無未捕捉頁面錯誤 |
| 內嵌應用基本測試與儀表板 | Smoke E2E 與 14 儀表板情境通過，無未捕捉瀏覽器錯誤 |
| 代理程式配接器 | 7 單元、語法及 3 MCP／Pi 情境通過 |
| 安裝與封裝 | 8 安裝資料、11 Compose 案例、每語 EN／KO 各 5 封存檔、6 發行聲明測試通過 |

對話瀏覽器案例涵蓋佇列編輯／排序、重啟確認、重試／編輯／分支、上下文檢查／壓縮、工具引導與文件貼上。後端迴歸涵蓋已接受語言持久化、預檢錯誤、中斷工具交換、放棄分支準備與文件讀取歸屬。

可將模型目前上下文與計費對話用量分開檢視。壓縮保留您選擇的近期完整交換；完整顯示歷史與之前嘗試用量仍保留。

貢獻者回報一次審查找出六個整合缺陷，之後修復生成輸入准入與複測復原、壓縮後附件路徑、舊版顯示歷史及引導投遞控制。一項附件測試在子分支下次提示前刪父分支，另一項檢查 rebase 保留 manifest 字面文字。各修復前均回報失敗迴歸，之後完整本機重跑。儀表板推理總數斷言限縮至摘要列，因懸停提示有相同標籤；重跑回報 14 儀表板情境通過。

回報推論測試使用回送模型與可拋棄資料庫。Windows 執行、付費供應商、遠端 CI 需另驗證。聲明測試使用 Python 3，符合 CI 直譯器需求。

<a id="native-hardening-candidate--2026-10-15"></a>

## 原生強化候選版 — 2026-10-15

在基於 main `33b131e` 的 `feat/ollama-zero-egress-hardening` 進行本機與獨立檢查。這是候選結果，確切提交的遠端 CI 與 PR 交付仍待完成。以下歷史段落保留原始結果。

| 實際執行檢查 | 結果 |
| --- | --- |
| 獨立 Go 執行，20 套件各用隔離可拋棄 PostgreSQL | 1,366 測試／子測試通過、0 失敗；`enrich` 無測試；2 選擇性略過（`TestAgentAdapterE2E`、`TestLiveContextReview`） |
| 另測代理程式配接器 | 7 單元及 3 真實 MCP／Pi 情境通過；刻意不執行外部真實供應商審查 |
| 聚焦後端 `-race` | 傳輸、wire／用量、上下文、來源選擇／失敗、搜尋／更新、准入皆通過，無競態 |
| `go vet ./...` | 通過 |
| 前端單元／本地化與靜態匯出 | 45 通過；3,168 EN／KO／zh-Hans 鍵保留對等、預留位置、rich tag；TypeScript 與全部 31 頁通過 |
| 後端目錄 | 1,943 正式鍵通過原始三語完整性／對等與 printf |
| 安裝、Compose、聲明 | 8 安裝、11 Compose 憑證、EN／KO 每語 5 封裝測試封存的聲明檢查、Python 3.12 的 6 發行聲明測試通過 |
| 靜態建置與相依檢查 | macOS 內嵌與 Windows amd64 交叉建置通過；`go.mod`／`go.sum` 無漂移；未執行原生 Windows |
| 內嵌瀏覽器基本測試 | 最終 UI 重建後 60 通過（48 smoke + 12 儀表板）、0 失敗。EN／KO／zh-Hans 穩定桌面／行動畫面及水平邊界通過：390px sheet [0,390]，1440px [864,1440]，全部 20 項關鍵內容可容納。已重現並修正先前 420px 最小寬裁切 |

[原生 HTTP 優先級迴歸](../../server/ollama_priority_test.go) 將 worker 1 留在 `/api/chat`，排隊 worker 2／3，再呼叫真實接線裁決，不由呼叫端標記緊急。`-race` 通過，HTTP 順序 **worker 1 → judge → worker 2 → worker 3**，峰值並行 1，原生裁決 schema／verdict 有效，全部完成且無名額洩漏。既有佇列測試涵蓋 FIFO 角色、取消與降低上限。

[原生來源失敗迴歸](../../server/provider_origin_failure_test.go) 修正前失敗：原生工具待執行後串流出錯，可能繼承全域雲端 `allow`。修正後 Stream／Complete 與終止路由錯誤保留具體原生設定檔、90 秒／`ask` 模型政策、300 秒／`deny` 人工核准到期，不呼叫審查者。實際雲端容錯仍清除原生偏好。迴歸與受影響競態套件通過。

已驗證 API 對不轉送請求的本機代理切換嚴格模式 **false → true → false**。四種搜尋後端 `ddgs`、`brave-free`、`tavily`、`deepseek` 僅在關閉時到達該接收端，嚴格模式各新增請求為零。嚴格更新檢查／套用遭封鎖，本機狀態可用。[更新迴歸](../../server/update_policy_test.go) 亦通過快取、重新導向、即時本文讀取，以及已准入非同步下載被後續切換阻擋後的清理／失敗完成。這不代表整台主機出口控制。

主機 Ollama **0.32.15** 使用既有 `qwen3:4b`、**4,096 token 輸出預算**，通過原生診斷、decision／comment schema、結構工具呼叫／結果重播、用量與要求 32K（`/api/ps` **32,768**）。先前 256／512 預算失敗仍保留證據，未丟棄或算成功。載入／速度／卸載數值只是一次熱執行快照，非基準或駐留保證。未下載模型或改主機設定，不宣稱自主任務完成或安全評估品質。

瀏覽器涵蓋三語診斷／警告、嚴格更新狀態、裁決載入／失敗保護及既有儀表板／用量。視覺 QA 重現固定 420px 最小寬編輯器在 390px 視窗左溢 30px。最小寬現符合視窗；重建應用通過全部 60 項，六張桌面／行動畫面標籤可讀，穩定內容在視窗內。獨立內嵌重建與瀏覽器執行檔逐位元組一致。確切提交遠端 CI 與 PR 讀回另為交付閘門。不宣稱合併、發行、付費供應商呼叫、外部目標掃描或真實通知。

<a id="provider-authentication-follow-up--2026-10-15"></a>

## 供應商驗證後續 — 2026-10-15

基於已合併 main `38fde23` 的 `fix/provider-env-auth`，本機迴歸重現並修正兩個需驗證的本機閘道失敗。舊版 Anthropic、OpenAI Chat Completions、OpenAI Responses 的啟動供應商金鑰備援與空金鑰連線測試皆回 HTTP 401；相同資料在修正後通過。

| 檢查 | 結果 |
| --- | --- |
| [`FromEnv → NewProvider` 本機 HTTP 驗證](../../agent/provider_env_test.go) | 21 情境通過：18 一般格式涵蓋供應商金鑰、檔案金鑰、現代／舊版覆寫、明確空白；3 原生涵蓋雲端金鑰隔離、Ollama 權杖、明確空白 |
| [透過本機 HTTP 資料的連線測試 handler](../../server/llm_gateway_test.go) | 16 情境通過：15 一般格式涵蓋相符正規化服務、不同端點／格式／代理、保留非本機備援；1 原生拒絕繼承啟用雲端金鑰 |
| 兩聚焦組 `-race` | 通過，無競態 |

上述 **37 個案例**為葉情境，不含父測試。各修正前重現驗證失敗，之後同測試通過。非本機 handler 案例中的公開文件位址經明確本機測試代理，不聯絡公開供應商。

無 profile ID 的空金鑰連線測試，僅在一般格式、正規化基底 URL、代理全相符時重用目前本機服務金鑰，含新草稿。不同本機服務與原生 Ollama 保持隔離。空金鑰新設定檔不匯入環境憑證。

這些檢查建立已測協定與憑證邊界，不代表付費帳號可用或全部歷史功能。修正後完整套件、瀏覽器、CI、已合併 main、發行成品結果另為閘門。未呼叫付費供應商。

<a id="installed-model-selection-follow-up--2026-10-15"></a>

## 已安裝模型選擇後續 — 2026-10-15

在 `feat/ollama-native-flexible-providers` 本機驗證新設定檔 Ollama 預設選擇，取代先前範本行為：選 Ollama 現執行唯讀模型探索。下方歷史 37 檢查瀏覽器快照早於此行為；「無隱含請求」為歷史斷言，不是目前契約。

| 檢查 | 結果 |
| --- | --- |
| 前端單元與本地化（`npm --prefix web test`） | 45 通過、0 失敗 |
| 正式靜態匯出（`npm --prefix web run build:static`） | TypeScript 與全部 31 頁通過 |
| 瀏覽器與已驗證後端真實主機模型探索 | 未存新設定檔、同機 Ollama、空代理／金鑰通過 |
| 內嵌執行檔與全新 `scopeweaver_test_default_final` 完整擴展瀏覽器 smoke | 43 通過、0 失敗（31 smoke + 12 儀表板）；涵蓋 EN／KO 三範本安裝預設、直接格式選擇、雲端相容、僅中繼資料選擇、失敗／空探索、手改／拒絕延遲回應、重開草稿／既有設定檔，以及既有設定、登入、本地化、SSE、儀表板迴歸 |

主機 `/api/tags` 傳回 `qwen2.5-coder:7b`、`qwen3:4b`、`embeddinggemma:latest`。瀏覽器選 **`qwen2.5-coder:7b`**，第一個宣告 `tools` 的已安裝模型。中繼資料為 `completion`、`tools`、`insert`、7.6B、Q4_K_M；`qwen3:4b` 亦宣告 `tools`，`embeddinggemma:latest` 僅 `embedding`。

瀏覽器恰好送一次 LLM 操作 `POST /api/llm/models`，主機端點、無金鑰／代理。新草稿關閉，未儲存、啟用、測連線或推論。模型／機器設定不變。這僅證明安裝模型選擇與中繼資料探索，不代表工具執行或自主可靠性。

<a id="historical-native-ollama-branch--2026-10-15-branch-local-not-a-release"></a>

## 歷史原生 Ollama 分支 — 2026-10-15（僅分支，非發行）

`feat/ollama-native-flexible-providers` 開 PR 前的本機檢查。遠端 CI 另於 GitHub Actions 追蹤；本節不代表發行、合併或 CI 結果。

| 檢查 | 結果 |
| --- | --- |
| 20 個 Go 套件各用全新可拋棄 PostgreSQL 16 | 1,197 測試／子測試通過；2 選擇性略過（`TestAgentAdapterE2E`、`TestLiveContextReview`） |
| 另測配接器 | 3 真實 MCP／Pi 情境通過 |
| `go vet ./...` | 通過 |
| 前端單元／本地化 | 43 通過；英／韓／簡體中文各 3,159 唯一鍵 |
| 後端訊息目錄 | 三語各 1,938 唯一鍵；printf／對等通過 |
| TypeScript／正式靜態匯出 | 通過，最終 health UI 後重建 |
| `npm audit` | 0 漏洞 |
| 建置 | 最終 health UI 後重建 `CGO_ENABLED=0` 內嵌執行檔；Windows amd64 `CGO_ENABLED=0` 交叉建置通過，未執行原生 Windows |
| 真實內嵌執行檔與全新 `scopeweaver_test_ollama_final_health` 的 `npm run smoke` | 37 通過、0 失敗（25 smoke + 12 儀表板）。含 EN／KO 三 Ollama 範本無隱含請求、免金鑰儲存／啟用、本機免金鑰相容 health 與缺金鑰公開雲端對比、雲端格式／可編輯 GLM、模型中繼資料／工具警告、缺模型精確 pull 指令、工具／上下文／載入／速度診斷、預設關閉本機政策，以及既有三語設定、登入、語言、草稿、SSE、核准用量迴歸 |
| Compose 憑證 | 真實 `network_mode: none` 容器經環境與 `FromEnv`，11 案例通過 |
| 安裝與封裝 | 8 安裝資料共通過，含英／韓選擇；英／韓各 5 封存檔通過 |
| 發行聲明（Python 3.12） | 6 測試通過 |
| 獨立後端安全／正確性審查 | 通過，含 DeepSeek 本機政策、即時 CIDR、404 分類 |

<a id="real-ollama-run"></a>

### 真實 Ollama 執行

主機 Ollama 0.32.15、`qwen3:4b`，經原生串流 API：

- SDK 層級串流檢查的合成工具資料中，模型呼叫 `record_fact`，ScopeWeaver 在本機執行並重播結果，回合完成；這不是平台資料庫的 `record_fact`。
- 要求 32K，`/api/ps` 回報分配 32,768。
- 精確用量：串流回合輸入 186／輸出 750；完成回合輸入 826／輸出 193。
- 真實 API 免金鑰儲存／啟用通過。診斷 `tool_call_verified`、`context_verified` 皆 true，上下文 32,768、載入 2.225375 ms、29.231 token/s、延遲 18,958 ms。**僅一次熱執行，非冷啟基準或硬體泛化。**
- 缺模型的精確韓文 pull 提示及嚴格政策切換封鎖公開 IP 通過。
- **平台任務 Worker Bash 證據**：task 1 的 `qwen3:4b`（要求 32K、分配 32,768；並行 1；keep-alive 30m；思考關閉；上限 4,096）發出原生 Bash `call_219yv72x`。活動 9（call）、10（`tool_result`）同 ID、無錯誤，輸出含 `LOCAL_TASK_OLLAMA_OK`。帳本精確記 6,704 輸入／4,096 輸出；下一原生呼叫重播相同 `tool_call_id`。意圖透過既有 `db.AddIntent` 手動植入，非 LLM 建立。輸出亦含測試資料的字面 `\n` 跳脫，不據此推斷換行處理或模型錯誤。執行檔來自最終 UI／DTO 前的未提交分支原始碼。

<a id="autonomous-task-reliability-is-not-validated"></a>

### 自主任務可靠性未驗證

工具診斷通過僅證明傳輸路徑，不代表模型可完成任務。

- `qwen2.5-coder:7b` 宣告 `tools`，卻將工具形狀 JSON 寫入 `assistant.content`，不是傳輸 `tool_calls`，因此無 Bash／事實工具執行；ScopeWeaver 正確保留純文字。
- `qwen3:4b` 作主要代理時，長提示下反覆思考並以文字寫出預期標記。作任務 Worker 確實執行上述 Bash，但 360 秒時任務暫停，未 `record_fact` 或自主完成；一次成功工具呼叫不代表自主完成。

**不對任何本機模型宣稱端對端自主完成或安全評估品質，7B 級模型亦未驗證可作完整代理程式。** [硬體分級](llm-providers.md#hardware-tiers) 為估算，非基準測試。

<a id="not-exercised"></a>

### 未執行項目

無付費雲端呼叫、外部目標掃描、真實通知、發布、部署、發行或合併。Anthropic、OpenAI Chat／Responses、自訂閘道經傳輸契約測試、迴歸與可編輯瀏覽器流程驗證，不使用真實供應商帳號。測試資料庫皆可拋棄，主機 Ollama 設定未改。選用 `local_llm_only` 預設關閉；伺服器仍需另設 `OLLAMA_NO_CLOUD`。

<a id="running-database-backed-tests"></a>

## 執行資料庫測試

`db`、`server`、`agent` 套件需要 PostgreSQL，連線字串**只**從 `ARTEX_TEST_DSN` 取得，絕不備援應用程式的 `ARTEX_PG_DSN`／`config.json`。此設計防止 `go test` 修改執行中應用的即時／正式資料庫。

- **未設 `ARTEX_TEST_DSN`**：全部資料庫測試略過，不接觸資料庫。
- **`ARTEX_TEST_DSN` 設為可拋棄資料庫**：套件對該庫執行。
- **解析至與應用相同資料庫**（同主機、連接埠、庫名）：任何測試連線前即快速失敗，錯誤 `refusing to run tests against the live database; point ARTEX_TEST_DSN at a throwaway DB`。

最簡單的執行方式：



```bash
make test-db
```



`make test-db`（透過 `scripts/test-db.sh`）建立由 `sw_user_test` 角色擁有的可拋棄 `scopeweaver_test_<pid>`，將 `ARTEX_TEST_DSN` 指向該資料庫，再由 `ARTEX_TEST_DSN` 指定測試連線，執行 `go test ./db/ ./server/ ./agent/ -count=1`，並在結束時刪除資料庫，即使測試失敗亦同。可覆寫 `TEST_DB_ROLE`、`TEST_DB_HOST`、`TEST_DB_NAME` 配合本機 PostgreSQL。庫名必須以 `scopeweaver_test_` 開頭，配接器 E2E 可用 `scopeweaver_adapter_`。

若要針對自行管理資料庫執行部分測試，匯出 `ARTEX_TEST_DSN` 並直接呼叫 `go test`：



```bash
export ARTEX_TEST_DSN='postgres://sw_user_test@/scopeweaver_test_1?host=/tmp&sslmode=disable'
go test ./server/ -run TestSomething -count=1
```



<a id="v021-candidate--2026-10-11"></a>

## v0.2.1 候選版 — 2026-10-11

Windows 候選版以真實靜態前端建置，針對可拋棄 PostgreSQL 16.15 測試。結果取代下方歷史相依與 Windows 執行限制；舊記錄保留供追溯。

| 檢查 | 結果 |
| --- | --- |
| 前端相依稽核 | 全嚴重度 0 漏洞，含開發相依 |
| 前端迴歸／本地化 | 26 通過、無略過 |
| 正式靜態匯出 | TypeScript 通過，生成 31 頁 |
| 原生 Windows Go、18 套件 | 1,016 測試／子測試通過、0 失敗、13 既有平台／選擇性略過 |
| 原生 Windows 完整 server | 311 測試／子測試通過、3 既有選擇性／平台略過 |
| 配接器 | 7 單元／語法通過；真實已驗證後端與新庫的 3 MCP／Pi 情境通過 |
| 本機安裝 | 4 隔離資料皆通過：新安裝、舊檔替換、相依失敗、前端建置失敗 |
| 後端翻譯目錄 | 1,868 正式鍵通過英／韓完整性與 printf 參數 |
| 通知隔離 | 確定性舊 Worker 迴歸修正前失敗、之後通過；管理／通知選擇連續 5 次通過 |
| Windows 證據 | 完整 PostgreSQL 套件通過，含並行寫入、提交失敗、長路徑 |
| Windows Python／技能檔 | 真實 Python 與無法使用啟動器的備援通過；巢狀 Unicode 路徑通過清單／讀取 handler |
| 內嵌 Windows 瀏覽器 E2E | 8 通過：兩語設定服務中斷保護、設定／登入、同源 SSE、用量 API 隔離、兩語用量卡、無未捕捉錯誤 |

Go 數量含父測試與子測試。13 原生略過為十個 Unix 自我更新資料、一個缺符號連結權限、選擇性配接器整合及真實供應商。配接器另驗證，Unix 資料由 Linux CI 涵蓋。未為產生結果停用測試。

迴歸重現原 Windows 目錄同步失敗、本地化分隔符不符、不可用 Python 啟動器、通知 Worker 洩漏與設定檔清理順序。E2E SQL 輔助現將選項置連線 URL 前；直接中斷／還原檢查確認 Windows psql 執行陳述式。

發行閘門另要求合併前後的 `frontend`、`windows`、`backend`、`agent-adapter`、瀏覽器 `smoke`。發布結果、平台封存與校驗值見 [v0.2.1](https://github.com/cskwork/scopeweaver/releases/tag/v0.2.1)；此來源記錄不宣稱後續工作或下載資產已通過。

保留既有 Python 直譯器路徑。若舊安裝存入不可用啟動器別名，請在設定選可執行直譯器。原生 Windows 測試不證明 Windows 自我更新或 macOS 執行。未使用付費供應商、外部目標或真實通知收件人。

<a id="historical-verification--2026-10-03"></a>

## 歷史驗證 — 2026-10-03

**2026-10-03** 記錄的英／韓 ScopeWeaver 驗證，基於上游 ARTEX `160fe13`，使用可拋棄 PostgreSQL 與本機資料。此為原始碼交付，當時未發布二進位或容器映像。

| 檢查 | 結果 |
| --- | --- |
| 前端迴歸／本地化 | 21 通過，TypeScript 通過 |
| 正式靜態匯出／Go 內嵌執行檔 | 建置成功 |
| Go 迴歸、18 套件 | 986 測試／子測試通過，1 繼承失敗，1 外部供應商略過 |
| 完整 server | 285 測試／子測試通過，僅選擇性真實供應商略過 |
| 後端翻譯 | 1,865 正式鍵通過英／韓完整性與 printf |
| 自撰中文稽核 | 無未翻譯自撰來源／文件／Go 註解，保留刻意相容／資料例外 |
| 桌面瀏覽器 | 每語 24 路由，無缺標籤或頁面水平溢出 |
| 行動瀏覽器 | 兩語儀表板、任務、發現、設定，390px 無頁面水平溢出 |
| 真實應用畫面 | 兩語登入／通知表單檢查；設定驗證／初始化通過 |
| 語言切換 | 導覽／狀態更新，保留選擇與未送草稿 |
| HTTP 本地化 | Markdown／CSV 匯出、協商、偏好儲存、無效語言拒絕通過 |
| GLM-5.3 契約 | 本機 HTTP／SSE 21 聚焦測試／子測試通過，含兩端點、推理、工具重播 |
| GLM 範本瀏覽器 | 兩語兩範本，保留名稱／金鑰／代理；無儲存、啟用或供應商請求 |
| 封裝／啟動指令碼 | Shell 語法、雙語封裝資料、10 監督案例通過 |
| 來源／相依 | 原授權、相依版本、Go 模組不變；文件連結與差異檢查通過 |

Go 數量含父測試與子測試。各套件用獨立資料庫，初始化與一般啟動相同的補充計量表。原始測試與檢查保持啟用。

<a id="inherited-issues-at-the-time"></a>

### 當時繼承問題

`TestGraphOverviewExpandsAssociatedCompanyScope` 在 ScopeWeaver 與未改上游皆因 `task scope missing: <nil>` 失敗。測試預期圖概覽含詳細範圍，但上游 `06a43f3` 刻意精簡。`75a706d` 改為 `TestGraphOverviewKeepsAssociatedCompanyScopeCompact`，檢查精簡概覽並直接驗證儲存的公司範圍。此後完整後端 CI 已通過。

上游驗證曾在 `TestTaskMetadataPatchReturnsRenameAndPin` 出現間歇暫存目錄清理失敗；最終 ScopeWeaver server 執行通過，未以無關生命週期變更隱藏。

上游 mock 預覽無通知中繼資料 mock，因此通知頁報錯。**真實後端的通知頁與新增通道表單在兩語皆通過**，驗證未傳送通知。

v0.1.0 記錄 **14 個 npm audit 問題：1 嚴重、9 高、4 中**。框架／工具更新及移除未用 shadcn CLI 相依，已於上方 v0.2.1 候選解決。執行 `cd web && npm audit` 查看 npm 套件登錄庫的最新稽核報告；新公告可能改變結果。

<a id="historical-scope-and-limits"></a>

### 歷史範圍與限制

- 選擇性 `TestLiveContextReview` 需私人外部供應商設定，未執行。無付費模型、外部目標掃描或真實通知。
- GLM 測試僅證明本機契約，不代表帳號存取／供應商可用。Coding Plan 僅支援官方工具，ScopeWeaver 未列入，見[供應商設定](llm-providers.md)。
- 未驗證 Docker 映像／Windows 批次執行；封裝資料不代表真實 Windows 測試。
- 設定進入已驗證路由，但一次等待 window load 逾時；之後以真實 API 確認初始化與新密碼。
- 使用者文字、儲存證據、編輯提示詞、傳輸識別碼、舊剖析標記、Unicode 資料與上游原畫面均保留。英／韓應用不翻譯任意使用者資料或第三方工具輸出。
- `sidequestion/` 歷史驗證描述上游工作，不是新的 ScopeWeaver 執行。

目前遠端檢查見 [GitHub Actions](https://github.com/cskwork/scopeweaver/actions/workflows/ci.yml)。

<a id="chat-document-paste-2026-10-16"></a>

## 對話文件貼上（2026-10-16）

PR #29 記錄對可拋棄 PostgreSQL 17 與本機後端的以下貼上檢查。這是貢獻者於 2026-10-16 回報結果；目前整合證據另記：

- 前端 52 測試與正式靜態匯出通過。
- 剪貼簿瀏覽器 12 組通過：獨立／任務對話、EN／KO／zh-Hans、1440／390 px。
- 涵蓋單一草稿擁有者的快速文件貼上、伴隨文字、原生 3,000 字元插入／復原、Unicode 預覽、草稿還原、上傳重試、導覽、mention、IME 與兩種送出偏好。貼上未提交任何訊息。
- 貢獻者停用 React paste handler，確認因缺第二附件標籤而失敗，再還原重跑。
- 後端 smoke 通過剪貼簿與 Ollama 回送資料，無未捕捉頁面錯誤。

檔案案例透過 paste event 提供瀏覽器 File 物件；文字與復原案例使用 Chromium 剪貼簿與鍵盤貼上。
