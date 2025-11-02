# cf-edgescout

## Project Overview

`cf-edgescout` 是一套針對 Cloudflare 網路邊緣進行探測與基準測試的工具集，可協助網路工程師與 SRE 評估特定網域在不同資料中心（colo）節點上的連線品質、回應延遲以及服務可用性。工具支援一次性掃描、常駐守護行為與 HTTP 服務模式，並可輸出多種報告格式，方便整合進既有的監控與容量規劃流程。

主要能力包括：

- 對 Cloudflare 管理的測試網域進行並行探測，記錄 TCP/TLS/HTTP 往返時間與回應內容。
- 評估節點可用性與成功率，找出延遲異常或錯誤率高的資料中心。
- 提供標準化輸出格式（JSONL、CSV、純文字），便於後續分析或外部系統匯入。
- 可選擇輸出 Prometheus metrics 及 HTML 概覽，整合進監控儀表板。

## Prerequisites

在開始使用 `cf-edgescout` 前，請先確認以下條件：

- **作業系統**：Linux 或 macOS（需支援 `bash` 與 `curl`）。
- **依賴套件**：`python >= 3.10`、`pip`、`virtualenv`（建議使用虛擬環境）、`make`（非必須但有助於執行常用命令）。
- **網路權限**：可以對 Cloudflare 邊緣節點發送 HTTP/HTTPS 請求，並遵循內部或 Cloudflare 的測試限制。
- **Cloudflare API Token（選用）**：若要自動建立測試資產或查詢帳號資訊，需要具備擁有 `Zone.Cache Purge` 與 `Zone.Zone Settings` 讀取權限的 token。

## Configuration & Environment Schema

`cf-edgescout` 透過設定檔或環境變數定義掃描參數。支援 YAML、TOML 與 JSON 格式；本專案提供的範例使用 YAML。主要欄位如下：

| 欄位 | 說明 | 預設值 |
| ---- | ---- | ------ |
| `target.host` | 要掃描的 Cloudflare 管理網域或完整 URL | 無（必填） |
| `target.health_path` | 健康檢查路徑，例如 `/cdn-cgi/trace` 或自訂健康物件 | `/cdn-cgi/trace` |
| `probe.modes` | 啟用的模式列表：`scan`、`daemon`、`serve` | `["scan"]` |
| `probe.timeout_ms` | HTTP/TLS timeout（毫秒） | 3000 |
| `probe.retry` | 失敗後重試次數 | 1 |
| `concurrency.workers` | 同時測試的 worker 數量 | `min(4, CPU 核心數)` |
| `outputs.formats` | 輸出格式列表：`results.jsonl`、`results.csv`、`top_k.txt` | `["results.jsonl"]` |
| `outputs.directory` | 儲存輸出檔案的資料夾 | `./outputs` |
| `report.prometheus.enabled` | 是否啟用 Prometheus 匯出 | `false` |
| `report.prometheus.listen_addr` | Prometheus 監聽位址 | `127.0.0.1:9797` |
| `report.html.enabled` | 是否生成 HTML 報告 | `false` |
| `report.html.path` | HTML 報告輸出路徑 | `./outputs/report.html` |

環境變數優先級高於設定檔，可透過以下變數覆寫：

| 變數 | 對應欄位 |
| ---- | -------- |
| `CF_EDGESCOUT_TARGET` | `target.host` |
| `CF_EDGESCOUT_HEALTH_PATH` | `target.health_path` |
| `CF_EDGESCOUT_PROMETHEUS_ADDR` | `report.prometheus.listen_addr` |
| `CF_EDGESCOUT_OUTPUT_DIR` | `outputs.directory` |

若同時提供設定檔與環境變數，工具會先載入設定檔，再覆寫同名欄位。

## Scan Modes

`cf-edgescout` 支援三種執行模式：

### `scan`
- **用途**：一次性掃描選定的資料中心列表，輸出結果後立即結束。
- **啟動方式**：`./edgescout scan --config configs/quickstart.yaml`
- **特性**：預設會生成 JSONL 與 CSV 報告，可搭配 `--top-k` 參數輸出最低延遲的節點列表。

### `daemon`
- **用途**：長時間持續探測，定期刷新結果並將最新資料寫入輸出檔案或 Prometheus。
- **啟動方式**：`./edgescout daemon --config configs/benchmark.yaml`
- **特性**：可透過 `--interval <秒>` 控制探測頻率，適合與監控系統整合。

### `serve`
- **用途**：啟動 HTTP API，對外提供即時探測結果與 Prometheus metrics。
- **啟動方式**：`./edgescout serve --config configs/benchmark.yaml`
- **特性**：預設監聽 `0.0.0.0:8080`，可透過設定檔中的 `serve.listen_addr` 自訂。

## Output Formats

每次掃描會依設定輸出以下檔案：

- **`results.jsonl`**：每行一筆探測資料，包含 `colo_code`、`latency_ms`、`status_code`、`success` 等欄位，適合後續以 `jq` 或資料處理工具分析。
- **`results.csv`**：與 JSONL 欄位相同，但為逗號分隔格式，可直接匯入試算表或資料倉儲。
- **`top_k.txt`**：依延遲排序的前 K 個資料中心代碼，每行包含 `colo_code latency_ms`。啟動命令需搭配 `--top-k` 或在設定檔中設定 `outputs.top_k`。

所有輸出檔案將儲存在 `outputs.directory` 指定的位置；若目錄不存在，工具會自動建立。

## Cloudflare 測試網域準備

為避免影響正式流量，建議建立專用的 Cloudflare 管理網域作為測試標的：

1. **建立測試子網域**：於 Cloudflare 儀表板中建立 `edgescout.example.com` 或任一子網域，指向對應的原始伺服器或 Cloudflare Workers。
2. **部署健康檢查端點**：
   - **選項 A：`/cdn-cgi/trace`** —— Cloudflare 內建的 debug 端點，會回傳請求來源、colo 代碼等資訊。
   - **選項 B：健康物件** —— 於原始伺服器部署靜態 JSON，例如 `{"status":"ok"}`，並確保回應時間可代表實際服務行為。
3. **權限設定**：確認 API Token 或服務使用者具備讀取該網域設定的權限。
4. **遵守探測限制**：
   - 遵循 Cloudflare 對單 IP 每秒請求數的限制，建議在設定檔中調整 `concurrency.workers` 與 `daemon.interval`。
   - 若與 Cloudflare 支援協調測試，請先行告知預計的請求量與持續時間，避免觸發自動防護。

## 基準測試與報告

### 執行基準測試

1. 建立或修改設定檔，指定欲測試的資料中心清單與輸出設定（可參考 `configs/benchmark.yaml`）。
2. 啟動掃描或守護模式，例如：
   ```bash
   ./edgescout scan --config configs/benchmark.yaml --duration 300s
   ```
3. 若要長期監測，可使用 `daemon` 模式並搭配 `--interval` 控制頻率。

### 指標解讀

- **Latency (ms)**：測量從發出請求到收到完整回應的往返時間，通常取平均、P95 或 P99 作為指標。
- **Success Rate (%)**：成功回應（HTTP 2xx/3xx 或符合自訂判斷）的比例；若低於 100%，需檢查失敗原因欄位。
- **Colo Codes**：Cloudflare 資料中心代碼，常見如 `SJC`、`HKG`；可透過 `/cdn-cgi/trace` 回應中的 `colo=` 欄位取得。
- **Error Breakdown**：`results.jsonl` 中的 `error` 欄位會標示超時、TLS 錯誤或 HTTP 狀態碼，協助定位問題。

### Prometheus 與 HTML 報告

- 在設定檔啟用 `report.prometheus.enabled: true` 後，`serve` 或 `daemon` 模式會於 `report.prometheus.listen_addr` 提供 `/metrics` 端點，可匯入現有的 Prometheus + Grafana 儀表板。
- 啟用 `report.html.enabled: true` 時，掃描結束會在 `report.html.path` 生成互動式 HTML 報告，包含延遲分布、成功率趨勢等視覺化資訊。

## 快速入門流程

1. 下載或複製本專案程式碼。
2. 建立 Python 虛擬環境並安裝依賴：
   ```bash
   python -m venv .venv
   source .venv/bin/activate
   pip install -r requirements.txt
   ```
3. 修改 `configs/quickstart.yaml`，將 `target.host` 改為你的測試網域。
4. 執行首次掃描：
   ```bash
   ./edgescout scan --config configs/quickstart.yaml
   ```
5. 檢視 `outputs/` 目錄中的結果檔案，並依需求整合 Prometheus 或 HTML 報告。

## 常用命令

```bash
# 一次性掃描並輸出 CSV
./edgescout scan --config configs/quickstart.yaml --output results.csv

# 常駐探測並曝光 Prometheus metrics
./edgescout daemon --config configs/benchmark.yaml --prometheus

# 啟動 HTTP 服務並提供最新結果
./edgescout serve --config configs/benchmark.yaml --listen 0.0.0.0:8080
```

更多參數說明請執行 `./edgescout --help` 取得。

## 附錄：檔案結構

```
.
├── README.md
└── configs/
    ├── benchmark.yaml
    └── quickstart.yaml
```
cf-edgescout 是一个用于评估 Cloudflare 边缘网络触达质量的主动探测工具。它可以从多个 Cloudflare 边缘数据中心发起 HTTP/HTTPS 探测，对比各个出口的可达性、延迟以及路由稳定性，并将结果保存为结构化报告，帮助 SRE、性能工程师以及网络团队快速定位问题。项目提供一次性扫描、持续守护进程以及交互式服务三种模式，并支持将指标输出至 Prometheus 或生成 HTML 报表。

## Prerequisites
在开始之前，请确保具备以下条件：

- Python 3.11 及以上版本，或容器化运行环境（Docker 24+）。
- 已安装 Poetry 或 pip 以管理依赖：
  ```bash
  pip install -r requirements.txt
  ```
- 可选：用于 Prometheus 导出或 HTML 报告的额外依赖（例如 `prometheus-client`、`jinja2`）。
- 一个可控的 Cloudflare 管理域名，以及 API Token（具有 Zone.Zone、Zone.Cache Purge、Account.Analytics 查看权限）。
- (可选) Redis 或其他缓存后端，用于高频守护任务的速率限制。

## Configuration Schema
cf-edgescout 通过 YAML 配置文件与环境变量联合定义运行时行为。常见字段如下：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `mode` | `scan \| daemon \| serve` | 运行模式。 |
| `targets` | 列表 | 每个对象包含 `host`、`protocols`、`ports`、`paths` 与可选的 `description`。 |
| `probe.timeout_ms` | 数值 | 单次请求超时时间（毫秒）。 |
| `probe.retries` | 数值 | 每个数据中心的重试次数。 |
| `probe.max_parallel` | 数值 | 并行探测的最大数量。 |
| `region_filter.include`/`exclude` | 列表 | 控制使用哪些 Cloudflare 数据中心（colo 代码）。 |
| `output.directory` | 字符串 | 结果输出目录。 |
| `output.formats` | 列表 | 启用的输出格式：`jsonl`、`csv`、`top_k`。 |
| `cloudflare.account_id` | 字符串 | Cloudflare Account ID。 |
| `cloudflare.api_token_env` | 字符串 | 保存 API Token 的环境变量名称。 |
| `prometheus.enabled` | 布尔值 | 是否开启 Prometheus 指标导出（守护/服务模式）。 |
| `notifications` | 对象 | 守护模式下的告警阈值与通知目标。 |

### 环境变量
- `CF_API_TOKEN`：Cloudflare API Token。
- `CF_ACCOUNT_ID`：Cloudflare Account ID。
- `EDGE_SCOUT_TOKENS`：服务模式访问令牌列表，逗号分隔。
- `REDIS_URL`：可选的 Redis 连接串，用于缓存或速率限制。
- `SLACK_WEBHOOK_URL`：守护模式下的 Slack 通知入口。

## Scan Modes
cf-edgescout 支持三种运行模式：

### `scan`
一次性批量扫描指定目标，并输出指标文件。适用于验证变更或周期性基线测试。

### `daemon`
以守护进程方式运行，按照预设频率循环执行任务，并支持告警通知、指标聚合与历史记录保留。

### `serve`
提供 HTTP API、实时仪表盘与可选的 Prometheus 指标导出。可作为团队内部的网络可观测性服务。

## Output Formats
运行结束后，默认会在 `output.directory` 下生成以下文件：

- `results.jsonl`：逐探测结果，包含时间戳、colo、延迟、HTTP 状态码等字段。
- `results.csv`：适合导入数据仓库或表格工具的聚合视图。
- `top_k.txt`：按延迟或成功率排序的前 K 个数据中心列表，便于快速定位最佳出口。

## Cloudflare 管理域名准备
为了获得准确的测量数据，请在 Cloudflare 管理的测试域名上完成以下准备：

1. 创建专用子域（如 `probe.example.com`），并将其指向可用的源站或 Cloudflare Workers/Pages 服务。
2. 部署可返回诊断信息的端点，例如开启默认的 `/cdn-cgi/trace`，或上传一个健康检查对象（返回 200 与简单正文）。
3. 在 Cloudflare 防火墙与速率限制中为探测源 IP 设置白名单，确保不会被意外阻断。
4. 遵守探测频率限制：建议单个目标每分钟不超过 60 次请求，如需更高频率，请与 Cloudflare 支持沟通以避免被视为滥用。
5. 记录 Cloudflare colo 代码与期望覆盖范围，以便在 `region_filter` 中配置。

## Benchmarking & Reporting

1. 使用 `scan` 模式执行多轮基准测试：
   ```bash
   edgescout scan --config configs/scan.yaml --repeat 5 --cooldown 10s
   ```
2. 输出的 `results.jsonl` 可配合以下指标解释：
   - **Latency**：`latency_ms` 字段，关注 p50/p95；可利用 `jq` 或 Pandas 聚合。
   - **Success Rate**：按 `status_code` 分组，统计 2xx/3xx 占比。
   - **Colo Codes**：`colo` 字段提供 Cloudflare 数据中心三字码，用于地理分析。
3. 若启用了 Prometheus：
   - 在配置文件中设置 `prometheus.enabled: true` 与监听端口。
   - 运行 `serve` 或 `daemon` 模式后，抓取 `/metrics` 暴露的指标并接入现有监控系统。
4. HTML 报告：
   - 在配置中指定模板目录（例如 `report.template_dir`）。
   - 运行 `edgescout scan --render-html reports/latest.html` 生成可视化报告。
5. 为长时间基准测试记录元数据（Git commit、配置版本），确保指标可追溯。

## Sample Configurations & Commands

仓库的 [`configs/`](configs) 目录包含三个示例：

- `configs/scan.yaml`：一次性扫描多个数据中心。
  ```bash
  CF_API_TOKEN=... CF_ACCOUNT_ID=... edgescout scan --config configs/scan.yaml
  ```
- `configs/daemon.yaml`：持续守护并推送通知。
  ```bash
  CF_API_TOKEN=... CF_ACCOUNT_ID=... SLACK_WEBHOOK_URL=... edgescout daemon --config configs/daemon.yaml
  ```
- `configs/serve.yaml`：启动 API/仪表盘服务并导出 Prometheus 指标。
  ```bash
  CF_API_TOKEN=... CF_ACCOUNT_ID=... EDGE_SCOUT_TOKENS="token1,token2" edgescout serve --config configs/serve.yaml
  ```

如需自定义，可复制这些文件并根据环境调整目标、网络限制与输出格式。

## Benchmark Workflow Example

1. **准备环境**：设置 Cloudflare API Token、Prometheus 抓取目标以及 HTML 模板。
2. **执行扫描**：使用 `scan` 模式进行多轮探测，记录 `results.jsonl` 与 `top_k.txt`。
3. **分析结果**：
   - 使用 `edgescout report stats --input outputs/results.jsonl` 生成概览。
   - 导入 `results.csv` 到数据仓库，构建历史延迟与成功率趋势图。
4. **持续监控**：将关键目标迁移至守护模式，启用通知与 Prometheus 指标。
5. **回归验证**：在网络变更前后复用同一配置文件进行对比，确保性能无回退。

## Additional Resources
- [Cloudflare Data Center Locations](https://www.cloudflare.com/network/) – 获取最新的 colo 代码。
- [Prometheus Documentation](https://prometheus.io/docs/introduction/overview/) – 指标抓取与可视化参考。
- [jq Manual](https://stedolan.github.io/jq/manual/) – JSONL 数据分析。

## Project Goals
- **Edge reconnaissance automation**：自動探索並驗證 Cloudflare 邊緣節點的可用性、安全性與性能，協助基礎設施團隊及時發現異常。
- **多模態結果聚合**：整合掃描、被動監控與服務模式的輸出，提供統一的評估管道。
- **合規與倫理優先**：在任何測試與部署過程中遵循企業與法律合規框架，避免對使用者與第三方造成影響。

## Architecture Overview
```
+-----------------------+
|       CLI Entrypoint  |
|  (scan / daemon / serve)
+-----------+-----------+
            |
   +--------+--------+
   |                 |
Scan Engine     Daemon Supervisor
   |                 |
Result Writers  Metrics Emitters
   |                 |
   +--------+--------+
            |
    Storage & Reporting
 (JSONL, CSV, HTML, Prometheus)
```
- **CLI Entrypoint**：使用 `cf-edgescout` 指令進入三種模式。
- **Scan Engine**：並發探測 Cloudflare 節點，產出結構化結果。
- **Daemon Supervisor**：長時間運行的排程器，調度掃描任務並監控健康狀態。
- **Serve 模式**：提供 HTTP API 與前端視覺化頁面。
- **Storage & Reporting**：將結果寫入多種格式，並可輸出監控指標。

## Module Overview
- `edgescout/cli.py`：解析命令列參數，載入設定並啟動對應模式。
- `edgescout/config.py`：處理 YAML 與環境變數設定。
- `edgescout/scanner/`：包含探測器、併發控制、結果序列化。
- `edgescout/daemon/`：排程器、任務佇列與重試邏輯。
- `edgescout/server/`：FastAPI/Starlette 應用，提供 `/results`, `/metrics` 等端點。
- `edgescout/reporting/`：Prometheus exporter、HTML 報表產生器。

## Compliance Constraints
- 僅可針對已授權之 Cloudflare zone、子域名或 IP 範圍操作。
- 請遵循組織安全政策、GDPR、CCPA 等資料保護規範。
- 設定 `targets.allowlist` 明確列出允許掃描的主機，並使用 `--dry-run` 驗證設定。
- 遵循 Cloudflare 自動化流量與 API 使用條款，避免過度負載。

## Prerequisites
- Python 3.10+
- pip / virtualenv
- 建議：Redis（若使用佇列式 daemon）、Prometheus（若啟用 metrics）。
- Cloudflare API Token（可選，若需檢索 zone metadata）。

### Installation
```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
pip install -e .
```

## Configuration
支援 YAML 檔與環境變數。

### YAML 範例 (`config.yaml`)
```yaml
runtime:
  concurrency: 32
  timeout_s: 5
  retries: 1

outputs:
  jsonl_path: ./artifacts/results.jsonl
  csv_path: ./artifacts/results.csv
  top_k_path: ./artifacts/top_k.txt

reporting:
  prometheus:
    enabled: true
    bind: 0.0.0.0:9300
  html:
    enabled: true
    output_dir: ./artifacts/html

targets:
  allowlist:
    - https://example.com
    - https://sub.example.com/cdn-cgi/trace

cloudflare:
  zone_id: ZONEID
  api_token: ${CF_API_TOKEN}
```

### 環境變數範例
```bash
export CF_EDGESCOUT_CONCURRENCY=32
export CF_EDGESCOUT_TIMEOUT_S=5
export CF_EDGESCOUT_OUTPUT_JSONL=./artifacts/results.jsonl
export CF_EDGESCOUT_OUTPUT_CSV=./artifacts/results.csv
export CF_EDGESCOUT_OUTPUT_TOPK=./artifacts/top_k.txt
export CF_EDGESCOUT_PROMETHEUS_ENABLED=true
export CF_EDGESCOUT_PROMETHEUS_BIND=0.0.0.0:9300
export CF_EDGESCOUT_HTML_ENABLED=true
export CF_EDGESCOUT_HTML_DIR=./artifacts/html
export CF_EDGESCOUT_TARGETS_ALLOWLIST=https://example.com,https://sub.example.com/cdn-cgi/trace
```

### 優先順序
1. 命令列旗標
2. 環境變數
3. YAML 設定檔

## Command-Line Usage

### `scan`
執行一次性掃描，輸出結果至 JSONL/CSV。
```bash
cf-edgescout scan \
  --config config.yaml \
  --concurrency 32 \
  --output-jsonl artifacts/results.jsonl \
  --output-csv artifacts/results.csv
```
常用旗標：
- `--targets-file`：包含目標列表的檔案。
- `--dry-run`：驗證設定與權限而不觸發掃描。
- `--rate-limit`：指定每秒請求數。

### `daemon`
長時間運行，定期執行掃描。
```bash
cf-edgescout daemon \
  --config config.yaml \
  --interval 15m \
  --queue redis://localhost:6379/0
```
常用旗標：
- `--interval`：掃描週期。
- `--max-backoff`：連續失敗時的最大退避。
- `--health-endpoint`：暴露健康檢查端點供探針。

### `serve`
啟動 API 與儀表板。
```bash
cf-edgescout serve \
  --config config.yaml \
  --host 0.0.0.0 \
  --port 8080 \
  --enable-metrics
```
常用旗標：
- `--enable-metrics`：暴露 `/metrics` 端點。
- `--enable-html`：提供 HTML 報表頁面。
- `--auth-token`：啟用簡單 API token 保護。

## Interpreting Outputs
- **`results.jsonl`**：每行一個探測結果，包含 `target`, `status`, `latency_ms`, `colo`, `timestamp` 等欄位。適合批次處理與資料湖整合。
- **`results.csv`**：摘要欄位的表格版本，可直接匯入試算表或 BI 工具。
- **`top_k.txt`**：根據延遲或成功率排序的最佳邊緣節點列表。
- 使用 `edgescout.reporting.html.generate` 可產生靜態 HTML 報表。
- Prometheus metrics 於 `/metrics`，例如 `edgescout_scan_latency_ms_bucket`、`edgescout_scan_success_total`。

## Prometheus 與 HTML Reporting
1. 在設定中啟用 `reporting.prometheus.enabled`，並在 `serve` 或 `daemon` 模式啟動 metrics。
2. 配置 Prometheus `scrape_config`：
   ```yaml
   scrape_configs:
     - job_name: edgescout
       static_configs:
         - targets: ['edgescout.internal:9300']
   ```
3. 啟用 HTML 報表後，`serve` 模式會提供 `/reports/index.html`，或在掃描完成後於 `output_dir` 生成靜態檔。

## Cloudflare 部署建議
- 將 `/cdn-cgi/trace` 或自定義健康檢查端點加入 `targets.allowlist`，確保掃描到 Cloudflare 回應資訊。
- 對使用者的 Cloudflare zone，配置 Workers / Pages 以回傳簡易 JSON `{ "status": "ok" }`，供 daemon 健康探針使用。
- 使用 Cloudflare Access 或 API Token 控制內部儀表板的存取。

## Rate Limiting
- 設定 `--rate-limit` 或 `runtime.rate_limit_per_s` 以限制請求速率。
- 配合 Cloudflare API 的原生 rate limit (例如 1200 req/5min)，在設定中預留安全緩衝。
- Daemon 模式可開啟平滑器（token bucket），避免尖峰突發。

## Ethical Considerations
- 僅對明確授權的網域與資源進行掃描。
- 先通知利害關係人，避免被誤判為惡意流量。
- 避免蒐集使用者個資，對任何敏感資料採匿名化與最小化原則。
- 在報告中標記測試時段與測試來源 IP，供稽核追蹤。

## Benchmarks 與 實驗重現
1. 取得官方 `benchmarks/` 腳本：
   ```bash
   git clone https://github.com/cloudflare-labs/cf-edgescout-benchmarks.git benchmarks
   ```
2. 準備樣本設定：
   ```bash
   cp benchmarks/configs/sample.yaml config-bench.yaml
   ```
3. 執行多輪測試：
   ```bash
   cf-edgescout scan --config config-bench.yaml --output-jsonl runs/run1.jsonl
   cf-edgescout scan --config config-bench.yaml --output-jsonl runs/run2.jsonl
   ```
4. 聚合結果：
   ```bash
   python benchmarks/aggregate.py runs/*.jsonl --out runs/aggregate.csv
   ```
5. 產出比較圖表：
   ```bash
   python benchmarks/plot_latency.py runs/aggregate.csv --output runs/latency.png
   ```
6. 將報告與數據存放於版本控制下，確保可重現性。

## Troubleshooting
- 使用 `--debug` 查看詳細日誌。
- 若 Prometheus 指標缺失，確認 `serve` 模式是否啟用 metrics。
- 檢查 `artifacts/` 權限以確保能寫入輸出檔。
- 運行 `cf-edgescout doctor`（若有）自動檢測環境設定。

Cloudflare CDN 多源探测与优选工具，后端基于 Go 1.22，前端采用 React 18 + Vite + TypeScript + Tailwind + Recharts。项目支持官方与第三方节点源的聚合探测、综合评分与可视化展示。

## 功能亮点

- ✅ **多数据源**：同时消费 Cloudflare 官方、BestIP、UOUIN 等社区数据源，自动去重与权重分配。
- ✅ **多维测量**：覆盖 TCP/TLS/HTTP 延迟、吞吐、证书完整性、HTTP 状态码、响应哈希与地理信息。
- ✅ **可视化控制台**：现代化前端提供来源筛选、提供方对比、趋势折线与榜单表格。
- ✅ **模块化设计**：fetcher、sampler、prober、scorer、store、API 互相解耦，便于扩展和二次开发。

## 快速开始

1. 准备环境：Go ≥ 1.22、Node.js ≥ 18。
2. 运行一次性探测：

   ```bash
   go run ./cmd/edgescout scan --domain example.com --providers official,bestip,uouin --count 64 --jsonl results.jsonl
   ```

3. 启动 API 服务并连接前端：

   ```bash
   go run ./cmd/edgescout serve --jsonl results.jsonl --addr :8080

   cd viz/frontend
   npm install
   npm run dev
   ```

## 文档

- [架构与原理总览](docs/overview.md)：核心流程、模块拆解与数据模型。
- [部署与运维手册](docs/deployment.md)：环境准备、命令行用法、前端构建与常见问题。

如需进一步自定义（新增提供方、调整评分策略、整合外部告警等），可参考上述文档中的扩展建议。
