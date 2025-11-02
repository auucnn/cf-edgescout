import { renderTrend, renderRadar } from './charts.js';

const elements = {
  sourceButtons: Array.from(document.querySelectorAll('.source-switch .pill')),
  regionSelect: document.getElementById('region-filter'),
  scoreMin: document.getElementById('score-min'),
  scoreMax: document.getElementById('score-max'),
  search: document.getElementById('search-input'),
  refresh: document.getElementById('refresh-button'),
  export: document.getElementById('export-button'),
  status: document.getElementById('status-message'),
  error: document.getElementById('error-message'),
  mock: document.getElementById('mock-indicator'),
  metricAvg: document.getElementById('metric-avg'),
  metricMin: document.getElementById('metric-min'),
  metricMax: document.getElementById('metric-max'),
  metricTotal: document.getElementById('metric-total'),
  leaderboard: document.getElementById('leaderboard'),
  leaderboardCount: document.getElementById('leaderboard-count'),
  resultsBody: document.getElementById('results-body'),
  trendCanvas: document.getElementById('trend-chart'),
  radarCanvas: document.getElementById('radar-chart'),
};

export function bindControls({ onRefresh, onFiltersChanged, onExport }) {
  elements.sourceButtons.forEach((button) => {
    button.addEventListener('click', () => {
      elements.sourceButtons.forEach((btn) => {
        btn.classList.toggle('active', btn === button);
        btn.setAttribute('aria-selected', btn === button ? 'true' : 'false');
      });
      onFiltersChanged({ source: button.dataset.source });
    });
  });

  elements.regionSelect.addEventListener('change', (event) => {
    onFiltersChanged({ region: event.target.value });
  });
  elements.scoreMin.addEventListener('input', (event) => {
    onFiltersChanged({ scoreMin: event.target.value });
  });
  elements.scoreMax.addEventListener('input', (event) => {
    onFiltersChanged({ scoreMax: event.target.value });
  });
  elements.search.addEventListener('input', (event) => {
    onFiltersChanged({ search: event.target.value });
  });

  elements.refresh.addEventListener('click', () => {
    onRefresh();
  });
  elements.export.addEventListener('click', () => {
    onExport();
  });
}

export function render(state) {
  updateStatus(state);
  toggleMockIndicator(state.usingMock);
  renderMetrics(state.summary);
  renderLeaderboard(state.summary, state.filters);
  renderResults(state.results, state.filters);
  renderTrend(elements.trendCanvas, state.timeseries);
  renderRadar(elements.radarCanvas, state.summary?.components || []);
}

export function applyRegionOptions(summary) {
  if (!summary || !Array.isArray(summary.regions)) {
    return;
  }
  const existing = new Set();
  Array.from(elements.regionSelect.options).forEach((option) => existing.add(option.value));
  summary.regions.forEach((group) => {
    const value = group.key || '';
    if (!existing.has(value)) {
      const option = document.createElement('option');
      option.value = value;
      option.textContent = value ? value.toUpperCase() : '未知区域';
      elements.regionSelect.append(option);
    }
  });
}

function updateStatus(state) {
  if (state.loading) {
    elements.status.textContent = '加载中…';
    elements.error.classList.add('hidden');
    return;
  }
  if (state.error) {
    elements.error.textContent = state.error;
    elements.error.classList.remove('hidden');
  } else {
    elements.error.classList.add('hidden');
    elements.error.textContent = '';
  }
  if (state.summary?.updated_at) {
    const date = new Date(state.summary.updated_at);
    elements.status.textContent = `最近更新时间：${date.toLocaleString()}`;
  } else {
    elements.status.textContent = '暂无数据';
  }
}

function toggleMockIndicator(usingMock) {
  if (usingMock) {
    elements.mock.hidden = false;
  } else {
    elements.mock.hidden = true;
  }
}

function renderMetrics(summary) {
  if (!summary) {
    elements.metricAvg.textContent = '--';
    elements.metricMax.textContent = '--';
    elements.metricMin.textContent = '--';
    elements.metricTotal.textContent = '--';
    return;
  }
  elements.metricAvg.textContent = formatNumber(summary.score?.average);
  elements.metricMax.textContent = formatNumber(summary.score?.max);
  elements.metricMin.textContent = formatNumber(summary.score?.min);
  elements.metricTotal.textContent = summary.total ?? 0;
}

function renderLeaderboard(summary, filters) {
  const container = elements.leaderboard;
  container.innerHTML = '';
  const records = summary?.recent ? [...summary.recent] : [];
  records.sort((a, b) => b.score - a.score);
  const search = filters.search.trim().toLowerCase();
  const top = records.filter((record) => includeBySearch(record, search)).slice(0, 5);

  top.forEach((record) => {
    const item = document.createElement('li');
    const meta = document.createElement('div');
    meta.className = 'leaderboard-meta';
    const title = document.createElement('strong');
    title.textContent = record.measurement?.ip || record.measurement?.colo || record.measurement?.cfColo || '未知节点';
    const subtitle = document.createElement('span');
    subtitle.textContent = [record.source, record.region || record.measurement?.cfColo]
      .filter(Boolean)
      .map((value) => value.toUpperCase?.() || value)
      .join(' · ');
    meta.append(title, subtitle);

    const score = document.createElement('span');
    score.className = 'leaderboard-score';
    score.textContent = formatNumber(record.score);

    item.append(meta, score);
    container.append(item);
  });
  elements.leaderboardCount.textContent = top.length ? `展示 ${top.length} / ${records.length} 条` : '暂无符合条件的数据';
}

function renderResults(results, filters) {
  const tbody = elements.resultsBody;
  tbody.innerHTML = '';
  const search = filters.search.trim().toLowerCase();
  results
    .filter((record) => includeBySearch(record, search))
    .forEach((record) => {
      const row = document.createElement('tr');
      row.innerHTML = `
        <td>${formatTime(record.timestamp)}</td>
        <td>${formatSource(record.source)}</td>
        <td>${formatRegion(record.region || record.measurement?.cfColo)}</td>
        <td>${formatNumber(record.score)}</td>
        <td>${formatDetails(record)}</td>
      `;
      tbody.append(row);
    });
}

function includeBySearch(record, search) {
  if (!search) {
    return true;
  }
  const haystack = [
    record.source,
    record.region,
    record.measurement?.cfColo,
    record.measurement?.colo,
    record.measurement?.ip,
  ]
    .concat(Object.keys(record.components || {}))
    .join(' ')
    .toLowerCase();
  return haystack.includes(search);
}

function formatNumber(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) {
    return '--';
  }
  return Number(value).toFixed(2);
}

function formatTime(timestamp) {
  if (!timestamp) {
    return '--';
  }
  return new Date(timestamp).toLocaleString();
}

function formatSource(source) {
  if (!source) {
    return '<span class="badge warning">未知</span>';
  }
  const normalized = source.toString().toLowerCase();
  if (normalized === 'official') {
    return '<span class="badge success">官方</span>';
  }
  if (normalized === 'third-party') {
    return '<span class="badge">社区</span>';
  }
  return source;
}

function formatRegion(region) {
  return region ? region.toString().toUpperCase() : '—';
}

function formatDetails(record) {
  const parts = [];
  if (record.measurement?.ip) {
    parts.push(record.measurement.ip);
  }
  if (record.measurement?.rtt_ms || record.measurement?.rttMs) {
    const value = record.measurement.rtt_ms ?? record.measurement.rttMs;
    parts.push(`${value} ms`);
  }
  const components = record.components || {};
  Object.entries(components).forEach(([key, value]) => {
    parts.push(`${key}: ${formatNumber(value)}`);
  });
  return parts.join(' · ') || '—';
}
