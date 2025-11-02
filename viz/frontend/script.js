const navToggle = document.querySelector('.nav-toggle');
const navLinks = document.querySelector('.nav-links');
const switches = document.querySelectorAll('.switch');
const frontendPanel = document.getElementById('frontend-panel');
const backendPanel = document.getElementById('backend-panel');
const experienceCanvas = document.getElementById('experience-canvas');

const sourcePills = Array.from(document.querySelectorAll('.source-pill'));
const regionFilter = document.getElementById('region-filter');
const scoreMinInput = document.getElementById('score-min');
const scoreMaxInput = document.getElementById('score-max');
const searchInput = document.getElementById('search-input');
const refreshButton = document.getElementById('refresh-button');
const exportButton = document.getElementById('export-button');
const statusMessage = document.getElementById('status-message');
const errorMessage = document.getElementById('error-message');
const leaderboard = document.getElementById('leaderboard');
const leaderboardCount = document.getElementById('leaderboard-count');
const logStream = document.getElementById('log-stream');
const trendCanvas = document.getElementById('trend-chart');
const radarCanvas = document.getElementById('radar-chart');
const metricAvg = document.getElementById('metric-avg');
const metricMax = document.getElementById('metric-max');
const metricMin = document.getElementById('metric-min');
const metricTotal = document.getElementById('metric-total');

const API_BASE = window.EDGESCOUT_API_BASE || '';
const COLOR_PALETTE = [
  '#6d79ff',
  '#15d1d1',
  '#f6c945',
  '#ff6f91',
  '#21a179',
  '#ffa94d',
];

const state = {
  selectedSource: 'official',
  filters: {
    sources: new Set(['official']),
    regions: new Set(),
    scoreMin: null,
    scoreMax: null,
  },
  searchTerm: '',
  summary: null,
  timeseries: [],
  results: [],
  sourceDetail: null,
  loading: false,
  usingMock: false,
};

const charts = {
  trend: null,
  radar: null,
};

const apiClient = (() => {
  let offline = false;
  let mockData;

  function composeURL(path, params) {
    const base = API_BASE || window.location.origin;
    const url = new URL(path, base);
    if (params && params instanceof URLSearchParams) {
      params.forEach((value, key) => {
        if (!url.searchParams.has(key)) {
          url.searchParams.set(key, value);
        }
      });
    }
    return url.toString();
  }

  async function fetchJSON(path, params) {
    if (offline) {
      return mockResponse(path, params);
    }
    try {
      const response = await fetch(composeURL(path, params), {
        headers: { Accept: 'application/json' },
      });
      if (!response.ok) {
        throw new Error(`请求失败: ${response.status}`);
      }
      const data = await response.json();
      offline = false;
      return data;
    } catch (error) {
      console.warn('API 请求失败，切换至离线 mock。', error);
      offline = true;
      state.usingMock = true;
      return mockResponse(path, params);
    }
  }

  async function ensureMock() {
    if (mockData) {
      return mockData;
    }
    const response = await fetch('mock-data.json');
    mockData = await response.json();
    return mockData;
  }

  async function mockResponse(path, params = new URLSearchParams()) {
    const dataset = await ensureMock();
    const records = Array.isArray(dataset.records) ? dataset.records.map(deserializeRecord) : [];
    const filtered = filterRecords(records, params);
    const bucket = params.get('bucket') || '1m';
    if (path.endsWith('/summary')) {
      return buildSummary(filtered);
    }
    if (path.endsWith('/timeseries')) {
      return buildTimeseries(filtered, bucket);
    }
    if (path.startsWith('/results/')) {
      const source = decodeURIComponent(path.split('/').pop() || '');
      const scoped = filtered.filter((record) => sourceOf(record).toLowerCase() === source.toLowerCase());
      return buildSourceDetail(source, scoped);
    }
    // default to /results
    const limit = Number(params.get('limit') || 50);
    const offset = Number(params.get('offset') || 0);
    const slice = limit > 0 ? filtered.slice(offset, offset + limit) : filtered.slice(offset);
    return {
      total: filtered.length,
      offset,
      limit,
      results: slice,
    };
  }

  function deserializeRecord(record) {
    const copy = { ...record };
    if (copy.timestamp) {
      copy.timestamp = new Date(copy.timestamp).toISOString();
    }
    return copy;
  }

  return {
    get: fetchJSON,
    isOffline: () => offline,
  };
})();

function initNavigation() {
  if (navToggle && navLinks) {
    navToggle.addEventListener('click', () => {
      navLinks.classList.toggle('open');
      navToggle.setAttribute('aria-expanded', navLinks.classList.contains('open'));
    });
    navLinks.addEventListener('click', (event) => {
      if (event.target.tagName === 'A') {
        navLinks.classList.remove('open');
        navToggle.setAttribute('aria-expanded', 'false');
      }
    });
  }

  const sections = document.querySelectorAll('section');
  const navAnchors = document.querySelectorAll('.nav-links a');
  const observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          navAnchors.forEach((anchor) => anchor.classList.remove('active'));
          const active = document.querySelector(`.nav-links a[href="#${entry.target.id}"]`);
          if (active) {
            active.classList.add('active');
          }
        }
      });
    },
    { threshold: 0.4 }
  );
  sections.forEach((section) => observer.observe(section));
}

function initExperienceCanvas() {
  if (!experienceCanvas) return;

  function paintExperience(role) {
    const ctx = experienceCanvas.getContext('2d');
    const gradient = ctx.createLinearGradient(0, 0, experienceCanvas.width, experienceCanvas.height);
    if (role === 'frontend') {
      gradient.addColorStop(0, 'rgba(109, 121, 255, 0.8)');
      gradient.addColorStop(1, 'rgba(21, 209, 209, 0.4)');
    } else {
      gradient.addColorStop(0, 'rgba(21, 209, 209, 0.8)');
      gradient.addColorStop(1, 'rgba(246, 201, 69, 0.4)');
    }

    ctx.clearRect(0, 0, experienceCanvas.width, experienceCanvas.height);
    ctx.fillStyle = gradient;
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';

    const peaks = role === 'frontend' ? [40, 120, 80, 180, 140, 220, 160] : [160, 80, 200, 120, 220, 100, 240];
    const step = experienceCanvas.width / (peaks.length - 1);

    ctx.beginPath();
    ctx.moveTo(0, experienceCanvas.height);
    peaks.forEach((value, index) => {
      const x = index * step;
      const y = experienceCanvas.height - value;
      ctx.lineTo(x, y);
    });
    ctx.lineTo(experienceCanvas.width, experienceCanvas.height);
    ctx.closePath();
    ctx.fill();

    ctx.strokeStyle = 'rgba(255, 255, 255, 0.4)';
    ctx.lineWidth = 2;
    ctx.beginPath();
    peaks.forEach((value, index) => {
      const x = index * step;
      const y = experienceCanvas.height - value;
      if (index === 0) {
        ctx.moveTo(x, y);
      } else {
        ctx.lineTo(x, y);
      }
    });
    ctx.stroke();
  }

  switches.forEach((switchButton) => {
    switchButton.setAttribute('role', 'tab');
    switchButton.addEventListener('click', () => {
      switches.forEach((btn) => {
        btn.classList.remove('active');
        btn.setAttribute('aria-selected', 'false');
      });
      switchButton.classList.add('active');
      switchButton.setAttribute('aria-selected', 'true');
      const role = switchButton.dataset.role;
      if (role === 'frontend') {
        frontendPanel.classList.remove('hidden');
        backendPanel.classList.add('hidden');
      } else {
        backendPanel.classList.remove('hidden');
        frontendPanel.classList.add('hidden');
      }
      paintExperience(role);
    });
  });

  paintExperience('frontend');
}

function initSourceSwitch() {
  sourcePills.forEach((pill, index) => {
    pill.addEventListener('click', () => {
      setActiveSource(pill.dataset.source);
    });
    pill.addEventListener('keydown', (event) => {
      if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
        event.preventDefault();
        const dir = event.key === 'ArrowRight' ? 1 : -1;
        const nextIndex = (index + dir + sourcePills.length) % sourcePills.length;
        sourcePills[nextIndex].focus();
      }
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        setActiveSource(pill.dataset.source);
      }
    });
  });
}

function setActiveSource(source) {
  state.selectedSource = source;
  state.filters.sources.clear();
  if (source === 'official' || source === 'third-party') {
    state.filters.sources.add(source);
  }
  sourcePills.forEach((pill) => {
    const isActive = pill.dataset.source === source;
    pill.classList.toggle('active', isActive);
    pill.setAttribute('aria-selected', isActive ? 'true' : 'false');
  });
  loadData();
}

function initFilters() {
  regionFilter.addEventListener('change', () => {
    state.filters.regions.clear();
    if (regionFilter.value) {
      state.filters.regions.add(regionFilter.value.toLowerCase());
    }
    loadData();
  });

  scoreMinInput.addEventListener('change', () => {
    const value = parseFloat(scoreMinInput.value);
    state.filters.scoreMin = Number.isFinite(value) ? value : null;
    loadData();
  });

  scoreMaxInput.addEventListener('change', () => {
    const value = parseFloat(scoreMaxInput.value);
    state.filters.scoreMax = Number.isFinite(value) ? value : null;
    loadData();
  });

  searchInput.addEventListener('input', () => {
    state.searchTerm = searchInput.value.trim().toLowerCase();
    renderLeaderboard();
    renderLog();
  });

  refreshButton.addEventListener('click', () => {
    loadData();
  });

  exportButton.addEventListener('click', () => {
    exportData();
  });
}

function buildParams(extra = {}) {
  const params = new URLSearchParams();
  if (state.filters.sources.size > 0) {
    params.set('source', Array.from(state.filters.sources).join(','));
  }
  if (state.filters.regions.size > 0) {
    params.set('region', Array.from(state.filters.regions).join(','));
  }
  if (state.filters.scoreMin !== null && !Number.isNaN(state.filters.scoreMin)) {
    params.set('score_min', state.filters.scoreMin);
  }
  if (state.filters.scoreMax !== null && !Number.isNaN(state.filters.scoreMax)) {
    params.set('score_max', state.filters.scoreMax);
  }
  Object.entries(extra).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, value);
    }
  });
  return params;
}

async function loadData() {
  if (state.loading) return;
  state.loading = true;
  setStatus('正在拉取最新数据…');
  hideError();
  try {
    const baseParams = buildParams();
    const [summary, timeseries, results] = await Promise.all([
      apiClient.get('/results/summary', baseParams),
      apiClient.get('/results/timeseries', buildParams({ bucket: '5m' })),
      apiClient.get('/results', buildParams({ limit: '200' })),
    ]);
    let sourceDetail = null;
    if (state.selectedSource !== 'all') {
      sourceDetail = await apiClient.get(`/results/${state.selectedSource}`, baseParams);
    }
    state.summary = summary;
    state.timeseries = Array.isArray(timeseries) ? timeseries : [];
    state.results = Array.isArray(results?.results) ? results.results : [];
    state.sourceDetail = sourceDetail;
    state.usingMock = apiClient.isOffline();
    populateRegions(summary?.regions || []);
    renderAll();
    const latest = summary?.score?.latest || summary?.updated_at;
    if (state.usingMock) {
      setStatus('离线模式：已加载本地 mock-data.json');
    } else if (latest) {
      setStatus(`更新于 ${formatTime(latest)}`);
    } else {
      setStatus('数据已刷新');
    }
  } catch (error) {
    console.error('加载数据失败', error);
    showError('获取数据失败，已尝试使用离线 mock。');
  } finally {
    state.loading = false;
  }
}

function populateRegions(groups) {
  const current = regionFilter.value;
  const options = groups
    .map((group) => group.key)
    .filter((key) => key && key !== 'unknown')
    .sort();
  const unique = Array.from(new Set(options));
  regionFilter.innerHTML = '<option value="">全部</option>';
  unique.forEach((region) => {
    const option = document.createElement('option');
    option.value = region.toLowerCase();
    option.textContent = region.toUpperCase();
    regionFilter.appendChild(option);
  });
  if (unique.includes(current.toUpperCase())) {
    regionFilter.value = current;
  }
}

function renderAll() {
  renderMetrics();
  renderLeaderboard();
  renderLog();
  renderTrendChart();
  renderRadarChart();
}

function renderMetrics() {
  if (!state.summary) {
    metricAvg.textContent = metricMax.textContent = metricMin.textContent = metricTotal.textContent = '--';
    return;
  }
  metricAvg.textContent = formatScore(state.summary.score?.average);
  metricMax.textContent = formatScore(state.summary.score?.max);
  metricMin.textContent = formatScore(state.summary.score?.min);
  metricTotal.textContent = String(state.summary.total ?? '--');
}

function renderLeaderboard() {
  leaderboard.innerHTML = '';
  if (!state.results.length) {
    leaderboardCount.textContent = '0';
    return;
  }
  const normalizedSearch = state.searchTerm;
  const items = state.results
    .map((record) => ({
      score: record.score,
      timestamp: record.timestamp,
      region: record.region || record.measurement?.cfColo || record.measurement?.CFColo,
      domain: record.measurement?.domain || record.measurement?.Domain,
      ip: record.measurement?.ip || record.measurement?.IP,
      source: sourceOf(record),
      raw: record,
    }))
    .filter((item) => {
      if (!normalizedSearch) return true;
      const haystack = [item.ip, item.region, item.domain, item.source]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      return haystack.includes(normalizedSearch);
    })
    .sort((a, b) => b.score - a.score)
    .slice(0, 10);

  leaderboardCount.textContent = String(items.length);

  items.forEach((item, index) => {
    const li = document.createElement('li');
    li.setAttribute('tabindex', '0');
    li.innerHTML = `
      <strong>${index + 1}. ${item.ip || '未记录 IP'}</strong>
      <span>${item.region ? item.region.toUpperCase() : 'N/A'} · ${formatScore(item.score)} · ${item.domain || '未知域名'}</span>
    `;
    leaderboard.appendChild(li);
  });
}

function renderLog() {
  logStream.innerHTML = '';
  logStream.setAttribute('aria-busy', 'true');
  const source = state.sourceDetail || state.summary;
  const recent = source?.recent || [];
  const filtered = recent.filter((record) => {
    if (!state.searchTerm) return true;
    const value = [
      record.measurement?.ip,
      record.measurement?.cfColo || record.measurement?.CFColo,
      record.measurement?.domain,
      sourceOf(record),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase();
    return value.includes(state.searchTerm);
  });
  filtered.forEach((record) => {
    const li = document.createElement('li');
    const time = formatTime(record.timestamp);
    const region = (record.region || record.measurement?.cfColo || record.measurement?.CFColo || '').toUpperCase();
    const message = `${time} · ${region || 'GLOBAL'} · ${formatScore(record.score)} · ${record.measurement?.domain || '未知域名'}`;
    li.textContent = message;
    logStream.appendChild(li);
  });
  logStream.setAttribute('aria-busy', 'false');
}

function renderTrendChart() {
  if (!trendCanvas || !window.Chart) return;
  const ctx = trendCanvas.getContext('2d');
  const points = state.timeseries || [];
  const labels = points.map((point) => formatTime(point.timestamp, { hour: '2-digit', minute: '2-digit' }));
  const datasets = [];
  if (points.length) {
    datasets.push({
      label: '整体',
      data: points.map((point) => point.average || 0),
      borderColor: COLOR_PALETTE[0],
      backgroundColor: 'rgba(109, 121, 255, 0.2)',
      tension: 0.35,
      borderWidth: 2,
    });
    const regionKeys = new Set();
    points.forEach((point) => {
      Object.keys(point.regions || {}).forEach((key) => {
        if (key) regionKeys.add(key);
      });
    });
    Array.from(regionKeys)
      .slice(0, COLOR_PALETTE.length - 1)
      .forEach((regionKey, idx) => {
        datasets.push({
          label: regionKey.toUpperCase(),
          data: points.map((point) => point.regions?.[regionKey] || null),
          borderColor: COLOR_PALETTE[idx + 1],
          backgroundColor: 'transparent',
          tension: 0.25,
          spanGaps: true,
        });
      });
  }

  if (!charts.trend) {
    charts.trend = new Chart(ctx, {
      type: 'line',
      data: { labels, datasets },
      options: {
        responsive: true,
        plugins: {
          legend: { labels: { color: '#eef1ff' } },
          tooltip: { mode: 'index', intersect: false },
        },
        scales: {
          x: {
            ticks: { color: '#9aa0c7' },
            grid: { color: 'rgba(255,255,255,0.05)' },
          },
          y: {
            ticks: { color: '#9aa0c7' },
            min: 0,
            max: 1,
            grid: { color: 'rgba(255,255,255,0.05)' },
          },
        },
      },
    });
  } else {
    charts.trend.data.labels = labels;
    charts.trend.data.datasets = datasets;
    charts.trend.update();
  }
}

function renderRadarChart() {
  if (!radarCanvas || !window.Chart) return;
  const ctx = radarCanvas.getContext('2d');
  const source = state.sourceDetail && state.selectedSource !== 'all' ? state.sourceDetail : state.summary;
  const components = source?.components || [];
  const labels = components.map((component) => component.key.toUpperCase());
  const values = components.map((component) => Number(component.average?.toFixed(3) || 0));

  if (!charts.radar) {
    charts.radar = new Chart(ctx, {
      type: 'radar',
      data: {
        labels,
        datasets: [
          {
            label: '组件健康度',
            data: values,
            borderColor: '#15d1d1',
            backgroundColor: 'rgba(21, 209, 209, 0.25)',
            borderWidth: 2,
            pointBackgroundColor: '#f6c945',
          },
        ],
      },
      options: {
        scales: {
          r: {
            angleLines: { color: 'rgba(255,255,255,0.08)' },
            grid: { color: 'rgba(255,255,255,0.08)' },
            suggestedMin: 0,
            suggestedMax: 1,
            pointLabels: { color: '#eef1ff', font: { size: 12 } },
            ticks: {
              color: '#9aa0c7',
              backdropColor: 'transparent',
              showLabelBackdrop: false,
              stepSize: 0.2,
            },
          },
        },
        plugins: {
          legend: { labels: { color: '#eef1ff' } },
        },
      },
    });
  } else {
    charts.radar.data.labels = labels;
    charts.radar.data.datasets[0].data = values;
    charts.radar.update();
  }
}

function exportData() {
  const payload = {
    generatedAt: new Date().toISOString(),
    filters: {
      source: state.selectedSource,
      regions: Array.from(state.filters.regions),
      scoreMin: state.filters.scoreMin,
      scoreMax: state.filters.scoreMax,
      search: state.searchTerm,
    },
    summary: state.summary,
    timeseries: state.timeseries,
    results: state.results,
    sourceDetail: state.sourceDetail,
  };
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `edgescout-export-${Date.now()}.json`;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

function setStatus(message) {
  statusMessage.textContent = message;
}

function showError(message) {
  errorMessage.textContent = message;
  errorMessage.classList.remove('hidden');
}

function hideError() {
  errorMessage.textContent = '';
  errorMessage.classList.add('hidden');
}

function formatScore(value) {
  if (value === null || value === undefined || Number.isNaN(value)) {
    return '--';
  }
  return Number(value).toFixed(2);
}

function formatTime(value, options = {}) {
  if (!value) return '--';
  const date = typeof value === 'string' ? new Date(value) : new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '--';
  }
  return date.toLocaleString('zh-CN', { hour12: false, ...options });
}

// Shared helpers for mock数据
function parseCSV(value) {
  if (!value) return [];
  return value
    .split(',')
    .map((part) => part.trim().toLowerCase())
    .filter(Boolean);
}

function sourceOf(record) {
  if (!record) return 'unknown';
  return (record.source || record.Source || record.measurement?.source || 'unknown').toString().toLowerCase();
}

function regionOf(record) {
  return (
    record.region ||
    record.Region ||
    record.measurement?.cfColo ||
    record.measurement?.CFColo ||
    ''
  )
    .toString()
    .toLowerCase();
}

function filterRecords(records, params) {
  const sources = parseCSV(params?.get?.('source'));
  const regions = parseCSV(params?.get?.('region'));
  const minScore = params?.get?.('score_min') ? Number(params.get('score_min')) : null;
  const maxScore = params?.get?.('score_max') ? Number(params.get('score_max')) : null;
  const result = records.filter((record) => {
    const source = sourceOf(record);
    if (sources.length && !sources.includes(source)) {
      return false;
    }
    const region = regionOf(record);
    if (regions.length && !regions.includes(region)) {
      return false;
    }
    if (minScore !== null && !Number.isNaN(minScore) && record.score < minScore) {
      return false;
    }
    if (maxScore !== null && !Number.isNaN(maxScore) && record.score > maxScore) {
      return false;
    }
    return true;
  });
  return result.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
}

function buildSummary(records) {
  const scoreSummary = summariseScores(records);
  const sources = summariseGroups(records, sourceOf);
  const regions = summariseGroups(records, regionOf);
  const components = summariseComponents(records);
  return {
    total: records.length,
    updated_at: records.length ? records[records.length - 1].timestamp : null,
    score: scoreSummary,
    sources,
    regions,
    components,
    recent: lastN(records, 10),
  };
}

function buildSourceDetail(source, records) {
  return {
    source,
    total: records.length,
    score: summariseScores(records),
    regions: summariseGroups(records, regionOf),
    components: summariseComponents(records),
    recent: lastN(records, 10),
  };
}

function buildTimeseries(records, bucket) {
  if (!records.length) return [];
  const duration = parseBucket(bucket);
  const map = new Map();
  const start = new Date(records[0].timestamp).getTime();
  records.forEach((record) => {
    const ts = new Date(record.timestamp).getTime();
    const index = Math.floor((ts - start) / duration);
    const key = start + index * duration;
    const bucketList = map.get(key) || [];
    bucketList.push(record);
    map.set(key, bucketList);
  });
  return Array.from(map.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([timestamp, bucketRecords]) => {
      const summary = summariseScores(bucketRecords);
      const regionGroups = summariseGroups(bucketRecords, regionOf);
      const regions = {};
      regionGroups.forEach((group) => {
        regions[group.key] = group.avg;
      });
      return {
        timestamp: new Date(Number(timestamp)).toISOString(),
        count: bucketRecords.length,
        average: summary.average || 0,
        regions,
      };
    });
}

function parseBucket(value) {
  if (typeof value === 'string' && value.endsWith('m')) {
    return Number(value.replace('m', '')) * 60 * 1000;
  }
  if (typeof value === 'string' && value.endsWith('h')) {
    return Number(value.replace('h', '')) * 60 * 60 * 1000;
  }
  return 60 * 1000;
}

function summariseScores(records) {
  if (!records.length) {
    return { average: 0, min: 0, max: 0, median: 0, latest: null };
  }
  const scores = records.map((record) => record.score).sort((a, b) => a - b);
  const sum = scores.reduce((acc, value) => acc + value, 0);
  const min = scores[0];
  const max = scores[scores.length - 1];
  const mid = Math.floor(scores.length / 2);
  const median = scores.length % 2 ? scores[mid] : (scores[mid - 1] + scores[mid]) / 2;
  const latest = records.reduce((latestRecord, current) => {
    if (!latestRecord) return current;
    return new Date(current.timestamp) > new Date(latestRecord.timestamp) ? current : latestRecord;
  }, null);
  return {
    average: sum / scores.length,
    min,
    max,
    median,
    latest: latest?.timestamp || null,
  };
}

function summariseGroups(records, selector) {
  const groups = new Map();
  records.forEach((record) => {
    const key = selector(record) || 'unknown';
    const list = groups.get(key) || [];
    list.push(record.score);
    groups.set(key, list);
  });
  return Array.from(groups.entries())
    .map(([key, scores]) => {
      const sorted = scores.slice().sort((a, b) => a - b);
      const sum = sorted.reduce((acc, value) => acc + value, 0);
      return {
        key,
        count: sorted.length,
        avg: sum / sorted.length,
        min: sorted[0],
        max: sorted[sorted.length - 1],
      };
    })
    .sort((a, b) => b.count - a.count);
}

function summariseComponents(records) {
  const totals = new Map();
  records.forEach((record) => {
    const components = record.components || record.Components || {};
    Object.entries(components).forEach(([key, value]) => {
      const entry = totals.get(key) || { sum: 0, count: 0 };
      entry.sum += Number(value) || 0;
      entry.count += 1;
      totals.set(key, entry);
    });
  });
  return Array.from(totals.entries())
    .map(([key, entry]) => ({ key, average: entry.count ? entry.sum / entry.count : 0 }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

function lastN(records, n) {
  if (!records.length) return [];
  const slice = records.slice(-n).reverse();
  return slice;
}

function bootstrap() {
  initNavigation();
  initExperienceCanvas();
  initSourceSwitch();
  initFilters();
  loadData();
}

bootstrap();
