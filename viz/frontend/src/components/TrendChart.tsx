import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  Legend,
} from "recharts";
import { TimeseriesResponse } from "../lib/api";

interface TrendChartProps {
  data?: TimeseriesResponse;
}

const TrendChart = ({ data }: TrendChartProps) => {
  if (!data || data.points.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-slate-700 p-6 text-center text-slate-400">
        暂无历史趋势数据。
      </div>
    );
  }

  const chartData = data.points.map((point) => ({
    ...point,
    timestampLabel: new Date(point.timestamp).toLocaleTimeString(),
  }));

  return (
    <div className="h-80 rounded-xl bg-slate-800/70 p-4 shadow-lg shadow-slate-950/40">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData} margin={{ top: 16, right: 24, left: 0, bottom: 8 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
          <XAxis dataKey="timestampLabel" stroke="#94a3b8" />
          <YAxis stroke="#94a3b8" domain={[0, 1]} tickFormatter={(value) => value.toFixed(1)} />
          <YAxis yAxisId={1} orientation="right" stroke="#facc15" tickFormatter={(value) => `${value.toFixed(0)}ms`} />
          <Tooltip
            contentStyle={{ backgroundColor: "#0f172a", borderRadius: 8, border: "1px solid #1e293b" }}
            labelStyle={{ color: "#e2e8f0" }}
            formatter={(value: number, name) => {
              if (name === "score") {
                return [value.toFixed(3), "综合得分"];
              }
              if (name === "latencyMs") {
                return [`${value.toFixed(1)} ms`, "探测延迟"];
              }
              return [value, name];
            }}
          />
          <Legend formatter={(value) => (value === "score" ? "综合得分" : "延迟(ms)")} />
          <Line type="monotone" dataKey="score" stroke="#22c55e" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="latencyMs" stroke="#38bdf8" strokeWidth={2} dot={false} yAxisId={1} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
};

export default TrendChart;
