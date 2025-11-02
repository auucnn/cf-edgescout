import { ListResponse } from "../lib/api";

interface ResultsTableProps {
  data?: ListResponse;
}

const ResultsTable = ({ data }: ResultsTableProps) => {
  if (!data || data.items.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-slate-700 p-6 text-center text-slate-400">
        暂无探测记录。
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-xl bg-slate-800/70 shadow-lg shadow-slate-950/40">
      <table className="min-w-full divide-y divide-slate-700 text-sm">
        <thead className="bg-slate-900/80 text-slate-300">
          <tr>
            <th className="px-4 py-3 text-left">时间</th>
            <th className="px-4 py-3 text-left">来源</th>
            <th className="px-4 py-3 text-left">提供方</th>
            <th className="px-4 py-3 text-left">网络</th>
            <th className="px-4 py-3 text-right">得分</th>
            <th className="px-4 py-3 text-right">延迟(ms)</th>
            <th className="px-4 py-3 text-right">吞吐(bit/s)</th>
            <th className="px-4 py-3 text-center">状态</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-700 text-slate-200">
          {data.items.map((item) => {
            const latency =
              (item.measurement.tcpDuration + item.measurement.tlsDuration + item.measurement.httpDuration) /
              1_000_000;
            return (
              <tr key={`${item.measurement.network}-${item.timestamp}`} className="hover:bg-slate-700/40">
                <td className="px-4 py-2">{new Date(item.timestamp).toLocaleString()}</td>
                <td className="px-4 py-2">{item.measurement.source || "--"}</td>
                <td className="px-4 py-2">{item.measurement.provider || "--"}</td>
                <td className="px-4 py-2 font-mono text-xs text-slate-300">{item.measurement.network || "--"}</td>
                <td className="px-4 py-2 text-right">{item.score.toFixed(3)}</td>
                <td className="px-4 py-2 text-right">{latency.toFixed(1)}</td>
                <td className="px-4 py-2 text-right">{item.measurement.throughput.toFixed(0)}</td>
                <td className="px-4 py-2 text-center">
                  {item.measurement.success ? (
                    <span className="rounded-full bg-green-500/20 px-3 py-1 text-xs text-green-300">成功</span>
                  ) : (
                    <span className="rounded-full bg-red-500/20 px-3 py-1 text-xs text-red-300">失败</span>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
};

export default ResultsTable;
