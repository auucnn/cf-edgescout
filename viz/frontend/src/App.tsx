import { useState } from "react";
import FilterBar from "./components/FilterBar";
import SummaryCards from "./components/SummaryCards";
import TrendChart from "./components/TrendChart";
import ResultsTable from "./components/ResultsTable";
import LoadingState from "./components/LoadingState";
import ErrorState from "./components/ErrorState";
import { useDashboardData } from "./hooks/useDashboard";

const App = () => {
  const [source, setSource] = useState<string>("all");
  const [provider, setProvider] = useState<string>("all");
  const [successOnly, setSuccessOnly] = useState<boolean | undefined>(undefined);

  const { summaryQuery, resultsQuery, timeseriesQuery, providerOptions, sourceOptions } = useDashboardData({
    source,
    provider,
    success: successOnly,
  });

  const isLoading = summaryQuery.isLoading || resultsQuery.isLoading || timeseriesQuery.isLoading;
  const errorCandidate = summaryQuery.error || resultsQuery.error || timeseriesQuery.error;
  const typedError = errorCandidate instanceof Error ? errorCandidate : null;

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 bg-slate-900 px-4 py-8 text-slate-100">
      <header className="flex flex-col gap-2">
        <h1 className="text-3xl font-semibold">Cloudflare CDN 多源探测与优选控制台</h1>
        <p className="text-slate-300">
          实时聚合官方与社区提供的 Cloudflare 节点资源，自动探测与多维评分，帮助你快速定位最优边缘节点。
        </p>
      </header>
      <FilterBar
        sources={sourceOptions}
        providers={providerOptions}
        selectedSource={source}
        selectedProvider={provider}
        successOnly={successOnly}
        onSourceChange={setSource}
        onProviderChange={setProvider}
        onSuccessToggle={setSuccessOnly}
      />
      {isLoading && <LoadingState />}
      {typedError && !isLoading && (
        <ErrorState error={typedError} retry={() => {
          summaryQuery.refetch();
          resultsQuery.refetch();
          timeseriesQuery.refetch();
        }} />
      )}
      {!isLoading && !typedError && (
        <div className="flex flex-col gap-6">
          <SummaryCards summary={summaryQuery.data} />
          <TrendChart data={timeseriesQuery.data} />
          <ResultsTable data={resultsQuery.data} />
        </div>
      )}
    </main>
  );
};

export default App;
