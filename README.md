# cf-edgescout

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
