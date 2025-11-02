import { ApiClient } from './api.js';
import { bindControls, render, applyRegionOptions } from './ui.js';
import { getState, subscribe, update, setError, setLoading, setUsingMock } from './state.js';

const client = new ApiClient();
const unsubscribe = subscribe(render);
render(getState());

bindControls({
  onRefresh: refresh,
  onFiltersChanged: (changes) => {
    update({ filters: changes });
    refresh();
  },
  onExport: exportCurrentView,
});

refresh();

async function refresh() {
  const state = getState();
  const params = buildParams(state.filters);
  const base = params.toString();
  const summaryParams = new URLSearchParams(base);
  const timeseriesParams = new URLSearchParams(base);
  const detailParams = new URLSearchParams(base);
  const listParams = new URLSearchParams(base);
  listParams.set('limit', '100');

  setLoading(true);
  setError('');
  try {
    const [summary, timeseries, list, sourceDetail] = await Promise.all([
      client.get('/results/summary', summaryParams),
      client.get('/results/timeseries', timeseriesParams),
      client.get('/results', listParams),
      fetchSourceDetail(state.filters, detailParams),
    ]);

    update({
      summary,
      timeseries,
      results: normalizeResults(list.results || []),
      total: list.total ?? 0,
      sourceDetail,
    });
    applyRegionOptions(summary);
    setUsingMock(client.isOffline());
  } catch (error) {
    console.error(error);
    setError('加载数据失败，请稍后重试。');
  } finally {
    setLoading(false);
  }
}

function buildParams(filters) {
  const params = new URLSearchParams();
  if (filters.source && filters.source !== 'all') {
    params.set('source', filters.source);
  }
  if (filters.region) {
    params.set('region', filters.region);
  }
  if (filters.scoreMin) {
    params.set('score_min', filters.scoreMin);
  }
  if (filters.scoreMax) {
    params.set('score_max', filters.scoreMax);
  }
  return params;
}

async function fetchSourceDetail(filters, params) {
  if (!filters.source || filters.source === 'all') {
    return null;
  }
  return client.get(`/results/${encodeURIComponent(filters.source)}`, params);
}

function normalizeResults(results) {
  return results.map((record) => ({
    ...record,
    timestamp: record.timestamp,
    components: record.components || {},
  }));
}

function exportCurrentView() {
  const state = getState();
  const payload = {
    exported_at: new Date().toISOString(),
    filters: state.filters,
    summary: state.summary,
    timeseries: state.timeseries,
    results: state.results,
    source_detail: state.sourceDetail || null,
    using_mock: state.usingMock,
  };

  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `edgescout-${Date.now()}.json`;
  anchor.click();
  URL.revokeObjectURL(url);
}

window.addEventListener('beforeunload', () => {
  unsubscribe();
});
