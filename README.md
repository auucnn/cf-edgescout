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

将 `viz/frontend` 目录作为静态站点部署或直接用浏览器打开 `index.html` 即可体验仪表板。前端默认访问当前来源的 `/results` API，可通过在页面加载前设置 `window.EDGESCOUT_API_BASE` 指向后端服务。

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

前端位于 `viz/frontend`，采用模块化原生 JavaScript，核心组成如下：

- `js/api.js`：负责访问 REST API，并在网络异常时透明切换到 `mock-data.json`。
- `js/state.js`：集中管理筛选条件、加载状态与返回结果。
- `js/ui.js` / `js/charts.js`：更新指标卡片、列表、图表（使用 Chart.js）。

仪表板主要提供以下能力：

- 官方 / 第三方 / 全部数据源切换，支持区域、得分区间与关键字过滤。
- 聚合指标、优选节点、折线趋势、健康雷达图实时渲染，并兼顾键盘导航和 ARIA 辅助文本。
- 一键导出当前视图（包含 summary/timeseries/结果明细）以便排查。
- 后端不可达时自动回落至离线示例数据，方便前端独立开发。

### 离线调试

若后端暂未部署，可直接打开 `index.html`，前端会自动加载 `mock-data.json`。也可以显式指定接口地址：

```html
<script>
  window.EDGESCOUT_API_BASE = 'https://api.example.com';
</script>
```

该脚本需置于 `js/main.js` 之前。保留空字符串或删除脚本即可使用内置 mock。

## 部署建议

1. 使用 `cf-edgescout daemon` 持续写入 JSONL 文件。
2. 通过 `cf-edgescout serve` 将 JSONL 文件暴露为 API，可结合 Systemd、容器或无服务器平台运行。
3. 将 `viz/frontend` 上传至任意静态资源服务（如 Cloudflare Pages、Netlify、Vercel）。
4. 依据部署架构配置 `window.EDGESCOUT_API_BASE` 或让前端与 API 同域提供服务。

欢迎根据业务需求扩展探测指标、持久化后端或前端 UI。若发现问题，欢迎提交 Issue 或 PR。
