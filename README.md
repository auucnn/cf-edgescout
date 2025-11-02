# cf-edgescout

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
