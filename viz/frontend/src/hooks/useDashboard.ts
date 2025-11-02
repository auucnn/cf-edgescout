import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  fetchResults,
  fetchSummary,
  fetchTimeseries,
  QueryParams,
  uniqueProviders,
  uniqueSources,
} from "../lib/api";

export interface DashboardFilters {
  source?: string;
  provider?: string;
  success?: boolean;
}

export const useDashboardData = (filters: DashboardFilters) => {
  const queryParams: QueryParams = useMemo(() => {
    const params: QueryParams = {};
    if (filters.source && filters.source !== "all") params.source = filters.source;
    if (filters.provider && filters.provider !== "all") params.provider = filters.provider;
    if (typeof filters.success === "boolean") params.success = filters.success;
    params.limit = 200;
    return params;
  }, [filters]);

  const summaryQuery = useQuery({
    queryKey: ["summary", queryParams],
    queryFn: () => fetchSummary(queryParams),
    staleTime: 30_000,
  });

  const resultsQuery = useQuery({
    queryKey: ["results", queryParams],
    queryFn: () => fetchResults(queryParams),
    staleTime: 15_000,
  });

  const timeseriesQuery = useQuery({
    queryKey: ["timeseries", queryParams],
    queryFn: () => fetchTimeseries(queryParams),
    staleTime: 15_000,
  });

  const providerOptions = useMemo(() => uniqueProviders(summaryQuery.data), [summaryQuery.data]);
  const sourceOptions = useMemo(() => uniqueSources(summaryQuery.data), [summaryQuery.data]);

  return {
    summaryQuery,
    resultsQuery,
    timeseriesQuery,
    providerOptions,
    sourceOptions,
  };
};
