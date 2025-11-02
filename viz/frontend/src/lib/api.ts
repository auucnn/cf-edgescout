import axios from "axios";

export interface RecordMeasurement {
  timestamp: string;
  score: number;
  components: Record<string, number>;
  measurement: {
    domain: string;
    source: string;
    provider: string;
    sourceType: string;
    network: string;
    family: string;
    bytesRead: number;
    integrity: {
      httpStatus: number;
      matchesSni: boolean;
      responseHash: string;
    };
    location: {
      colo: string;
      city: string;
      country: string;
    };
    tcpDuration: number;
    tlsDuration: number;
    httpDuration: number;
    throughput: number;
    success: boolean;
  };
}

export interface ListResponse {
  total: number;
  items: RecordMeasurement[];
}

export interface SummaryItem {
  source: string;
  provider: string;
  count: number;
  successRate: number;
  avgScore: number;
  avgLatencyMs: number;
}

export interface SummaryResponse {
  generatedAt: string;
  providers: SummaryItem[];
}

export interface TimeseriesPoint {
  timestamp: string;
  source: string;
  provider: string;
  score: number;
  latencyMs: number;
  success: boolean;
}

export interface TimeseriesResponse {
  points: TimeseriesPoint[];
}

const client = axios.create({
  baseURL: "/api",
  timeout: 10000,
});

export interface QueryParams {
  source?: string;
  provider?: string;
  success?: boolean;
  limit?: number;
  offset?: number;
}

const serializeParams = (params?: QueryParams) => {
  const search = new URLSearchParams();
  if (!params) return search.toString();
  if (params.source) search.append("source", params.source);
  if (params.provider) search.append("provider", params.provider);
  if (typeof params.success === "boolean") {
    search.append("success", params.success ? "true" : "false");
  }
  if (params.limit) search.append("limit", String(params.limit));
  if (params.offset) search.append("offset", String(params.offset));
  return search.toString();
};

export async function fetchResults(params?: QueryParams): Promise<ListResponse> {
  const query = serializeParams(params);
  const { data } = await client.get<ListResponse>(`/results${query ? `?${query}` : ""}`);
  return data;
}

export async function fetchSummary(params?: QueryParams): Promise<SummaryResponse> {
  const query = serializeParams(params);
  const { data } = await client.get<SummaryResponse>(`/results/summary${query ? `?${query}` : ""}`);
  return data;
}

export async function fetchTimeseries(params?: QueryParams): Promise<TimeseriesResponse> {
  const query = serializeParams(params);
  const { data } = await client.get<TimeseriesResponse>(`/results/timeseries${query ? `?${query}` : ""}`);
  return data;
}

export const uniqueProviders = (summary?: SummaryResponse) => {
  if (!summary) return [] as string[];
  const providers = new Set<string>();
  summary.providers.forEach((item) => {
    if (item.provider) providers.add(item.provider);
    else if (item.source) providers.add(item.source);
  });
  return Array.from(providers.values());
};

export const uniqueSources = (summary?: SummaryResponse) => {
  if (!summary) return [] as string[];
  const sources = new Set<string>();
  summary.providers.forEach((item) => {
    if (item.source) sources.add(item.source);
  });
  return Array.from(sources.values());
};
