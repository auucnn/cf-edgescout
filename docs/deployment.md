# 部署与运维手册

本文提供从环境准备、后端运行到前端构建的全流程指南，帮助你快速落地 Cloudflare CDN 优选平台。

## 先决条件

| 组件 | 最低版本 | 说明 |
| --- | --- | --- |
| Go | 1.22+ | 用于编译 / 运行后端探测与 API 服务 |
| Node.js | 18+ | 用于运行现代化前端（Vite + React） |
| npm / pnpm / yarn | 最新 | 推荐使用 npm 以免额外配置 |

## 项目初始化

```bash
# 克隆代码
git clone <repo-url>
cd cf-edgescout

# 安装前端依赖
cd viz/frontend
npm install
cd ../../
```

## 后端：探测与调度

命令行入口位于 `cmd/edgescout`，包含 `scan`、`daemon`、`serve` 三个子命令。

### 一次性探测

```bash
go run ./cmd/edgescout scan \
  --domain example.com \
  --count 64 \
  --providers official,bestip,uouin \
  --jsonl results.jsonl \
  --csv results.csv
```

- `--providers` 以逗号分隔的提供方键值，可选 `official`、`bestip`、`uouin` 或 `all`。
- 默认会并行抓取所有启用的数据源；若部分第三方失败，程序会记录警告并继续使用成功的来源。
- 结果会被写入内存或 JSONL 文件，且可选导出 CSV。

### 守护式探测

```bash
go run ./cmd/edgescout daemon \
  --domain example.com \
  --count 48 \
  --interval 5m \
  --providers all \
  --jsonl edges.jsonl
```

- 周期性抓取网段并探测，适合长期运行在服务器或容器中。
- 若 `--providers` 中第三方暂时不可用，守护进程会记录日志并继续下一轮。

### API 服务

```bash
go run ./cmd/edgescout serve --jsonl edges.jsonl --addr :8080
```

提供以下端点（均支持 `/api/` 前缀）：

- `GET /results`：分页 + 多条件筛选（`source`、`provider`、`success`、`limit`、`offset`）。
- `GET /results/summary`：按来源/提供方聚合成功率、平均得分、延迟等指标。
- `GET /results/timeseries`：按时间轴返回得分与延迟趋势数据。

## 前端：可视化控制台

```bash
cd viz/frontend
npm run dev   # 启动开发服务器（默认 http://localhost:5173）

npm run build # 生产构建，输出至 dist/
```

- 开发模式下 Vite 会将 `/api/*` 请求代理到 `http://localhost:8080`，确保先启动后端 `serve` 命令。
- 生产构建可部署至任意静态资源托管（例如 Cloudflare Pages），并使用反向代理将 `/api` 转发至后端。

## 数据源管理

- 默认启用：官方 (`official`)、BestIP (`bestip`)、UOUIN (`uouin`)。
- 若需自定义第三方接口，可在 `fetcher.DefaultProviders()` 中新增 `ProviderSpec`，或在后续版本使用配置文件方式注入。
- `ProviderSpec.Weight` 会影响采样概率；官方默认权重 1.0，第三方示例权重 0.7~0.8。

## 常见问题

### 提供方抓取失败怎么办？

- CLI 会打印 `数据源告警` 提示，可暂时忽略或调整 `--providers`。
- 确保服务器具有公网访问权限，并检查第三方文档中的接口路径是否调整。

### 如何调整评分偏好？

- 修改 `scorer.New()` 中的默认权重，或在运行时通过自定义构造注入 `Scorer{Config: ...}`。
- `Config.SourcePreference` 采用 `map[string]float64`，键值为来源（小写）或提供方名称，可用于额外拉高官方或优质第三方的得分。

### 是否支持导出更多维度？

- `exporter.ToCSV` 已包含来源、提供方、HTTP 状态码、响应哈希、地理信息等字段，可根据需要扩展。
- 如需自定义格式，可参考 `exporter` 包实现新的导出器。

更多架构细节请查阅 [架构与原理总览](./overview.md)。
