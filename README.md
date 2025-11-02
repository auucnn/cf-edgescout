# cf-edgescout

## Project Overview
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

