# cf-edgescout

EdgeScout 是一个用于探测并可视化 Cloudflare 边缘节点健康度的演示项目，涵盖采集、评分、存储到前端可视化的完整链路。本仓库提供扫描 CLI、数据 API 以及一个原生前端仪表板，帮助你快速部署并观察网络探测结果。

## 快速开始

```bash
# 一次性扫描
cf-edgescout scan -domain example.com -count 32 -jsonl results.jsonl

# 持续运行探测守护进程
cf-edgescout daemon -domain example.com -jsonl edges.jsonl

# 启动 API + 前端
cf-edgescout serve -jsonl edges.jsonl -addr :8080
```

将 `viz/frontend` 目录以静态站点方式部署或直接用浏览器打开 `index.html` 即可体验仪表板。若与 `serve` 子命令同域部署，前端会默认访问 `http://<host>:8080` 提供的 API。

## 数据 API

`cf-edgescout serve` 会暴露以下 HTTP 端点（均为 `GET`）：

| 端点 | 说明 |
| --- | --- |
| `/healthz` | 简单的健康检查，返回 `ok`。 |
| `/results` | 返回符合过滤条件的原始探测记录，支持分页。 |
| `/results/summary` | 返回聚合指标：得分统计、来源/区域分布、组件平均值及最近记录。 |
| `/results/timeseries` | 返回按时间桶聚合的得分趋势，可通过 `bucket`（如 `5m`、`1h`）控制粒度。 |
| `/results/{source}` | 针对指定来源（如 `official` 或 `third-party`）的聚合详情。 |

所有查询端点均支持以下可选 Query 参数：

- `source`: 逗号分隔的来源过滤（`official`、`third-party` 等）。
- `region`: 逗号分隔的地理区域过滤（如 `SJC,LHR`）。
- `score_min` / `score_max`: 限定得分区间。
- `limit` / `offset`: 用于 `/results` 的分页。
- `bucket`: `/results/timeseries` 的时间桶大小，默认为 `1m`。

### 缓存与 CORS

`serve` 子命令支持细化 API 行为：

```bash
cf-edgescout serve \
  -jsonl edges.jsonl \
  -addr :8080 \
  -cache-ttl 45s \
  -default-sources official \
  -default-regions sjc,lhr \
  -default-score-min 0.6 \
  -cors-origins "https://dashboard.example.com,https://viz.example.com"
```

- `-cache-ttl`：控制 GET 请求缓存刷新间隔。
- `-default-sources`/`-default-regions`：为所有请求注入默认过滤条件。
- `-default-score-min` / `-default-score-max`：设置全局得分阈值。
- `-cors-origins`：配置允许跨域的来源（逗号分隔，默认允许 `*`）。

## 前端仪表板

前端位于 `viz/frontend`，完全基于原生 HTML/CSS/JS，并通过 Chart.js 绘制趋势与雷达图。主要能力包括：

- 官方 / 第三方数据源切换，支持键盘方向键导航。
- 区域、得分区间筛选与关键字搜索，实时更新优选列表与事件流。
- 趋势折线、健康雷达、核心指标卡片及可访问性优化（ARIA 标签、aria-live 状态提示）。
- 一键导出当前视图数据（JSON），供分享或排错。
- API 不可用时自动回落至 `mock-data.json`，支持离线调试。

### 离线调试

若后端不可用，可直接打开 `viz/frontend/index.html` 并通过浏览器控制台设置：

```js
// 可选：显式切换为离线模式
window.EDGESCOUT_API_BASE = ''; // 保持为空使用 mock-data
```

前端会自动从 `viz/frontend/mock-data.json` 载入示例数据，展示完整交互流程。

## 部署建议

1. 使用 `cf-edgescout daemon` 持续写入 JSONL 文件。
2. 通过 `cf-edgescout serve` 将 JSONL 文件暴露为 API，可使用 Systemd 或容器编排运行。
3. 将 `viz/frontend` 上传至任意静态资源服务（如 Cloudflare Pages、Netlify）。
4. 配置前端的 `window.EDGESCOUT_API_BASE` 指向 API 地址，或保持同域部署。

欢迎根据业务需求扩展探测指标、持久化后端或前端 UI。若发现问题，欢迎提交 Issue 或 PR。
