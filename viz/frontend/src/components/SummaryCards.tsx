import { SummaryResponse } from "../lib/api";

interface SummaryCardsProps {
  summary?: SummaryResponse;
}

const SummaryCards = ({ summary }: SummaryCardsProps) => {
  if (!summary || summary.providers.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-slate-700 p-6 text-center text-slate-400">
        暂无可用统计数据，请先运行探测任务。
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-3">
      {summary.providers.map((item) => (
        <div
          key={`${item.provider}-${item.source}`}
          className="flex flex-col gap-2 rounded-xl bg-slate-800/70 p-4 shadow-lg shadow-slate-950/40"
        >
          <div className="text-sm text-slate-400">{item.source || "未标注来源"}</div>
          <div className="text-lg font-semibold text-slate-100">{item.provider || item.source || "未知提供方"}</div>
          <div className="flex flex-wrap gap-3 text-sm text-slate-300">
            <span>样本：{item.count}</span>
            <span>成功率：{(item.successRate * 100).toFixed(1)}%</span>
            <span>平均得分：{item.avgScore.toFixed(3)}</span>
            <span>平均延迟：{item.avgLatencyMs.toFixed(1)}ms</span>
          </div>
        </div>
      ))}
    </div>
  );
};

export default SummaryCards;
