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
