import { ChangeEvent } from "react";

interface FilterBarProps {
  sources: string[];
  providers: string[];
  selectedSource: string;
  selectedProvider: string;
  successOnly: boolean | undefined;
  onSourceChange: (value: string) => void;
  onProviderChange: (value: string) => void;
  onSuccessToggle: (value: boolean | undefined) => void;
}

const FilterBar = ({
  sources,
  providers,
  selectedSource,
  selectedProvider,
  successOnly,
  onSourceChange,
  onProviderChange,
  onSuccessToggle,
}: FilterBarProps) => {
  const handleSuccessChange = (event: ChangeEvent<HTMLSelectElement>) => {
    const value = event.target.value;
    if (value === "all") {
      onSuccessToggle(undefined);
    } else {
      onSuccessToggle(value === "success");
    }
  };

  return (
    <div className="flex flex-col gap-4 rounded-lg bg-slate-800/60 p-4 shadow-lg md:flex-row md:items-end md:justify-between">
      <div className="flex flex-1 flex-wrap gap-4">
        <label className="flex flex-col text-sm">
          <span className="mb-1 text-slate-300">来源</span>
          <select
            className="min-w-[160px] rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-slate-100 focus:border-primary focus:outline-none"
            value={selectedSource}
            onChange={(event) => onSourceChange(event.target.value)}
          >
            <option value="all">全部来源</option>
            {sources.map((source) => (
              <option key={source} value={source}>
                {source}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col text-sm">
          <span className="mb-1 text-slate-300">提供方</span>
          <select
            className="min-w-[160px] rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-slate-100 focus:border-primary focus:outline-none"
            value={selectedProvider}
            onChange={(event) => onProviderChange(event.target.value)}
          >
            <option value="all">全部提供方</option>
            {providers.map((provider) => (
              <option key={provider} value={provider}>
                {provider}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col text-sm">
          <span className="mb-1 text-slate-300">结果状态</span>
          <select
            className="min-w-[160px] rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-slate-100 focus:border-primary focus:outline-none"
            value={successOnly === undefined ? "all" : successOnly ? "success" : "failed"}
            onChange={handleSuccessChange}
          >
            <option value="all">全部</option>
            <option value="success">仅成功</option>
            <option value="failed">仅失败</option>
          </select>
        </label>
      </div>
    </div>
  );
};

export default FilterBar;
