import { summariseSummary, summariseSource, summariseTimeseries } from './summary.js';

export class MockAdapter {
  constructor(path) {
    this.path = path;
    this.cache = null;
  }

  async query(path, params) {
    const records = await this.loadRecords();
    const filtered = filterRecords(records, params);

    if (path.endsWith('/summary')) {
      return summariseSummary(filtered);
    }
    if (path.endsWith('/timeseries')) {
      const bucket = parseBucket(params.get('bucket'));
      return summariseTimeseries(filtered, bucket);
    }
    if (path.startsWith('/results/') && path !== '/results/') {
      const source = decodeURIComponent(path.replace('/results/', ''));
      const scoped = filterBySource(filtered, source);
      return summariseSource(source, scoped);
    }

    const limit = toNumber(params.get('limit'), 50);
    const offset = toNumber(params.get('offset'), 0);
    const slice = limit > 0 ? filtered.slice(offset, offset + limit) : filtered.slice(offset);
    return {
      total: filtered.length,
      offset,
      limit,
      results: slice.map(serializeRecord),
    };
  }

  async loadRecords() {
    if (this.cache) {
      return this.cache;
    }
    const response = await fetch(this.path);
    const data = await response.json();
    const records = Array.isArray(data.records) ? data.records.map(normalizeRecord) : [];
    records.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
    this.cache = records;
    return records;
  }
}

function filterRecords(records, params) {
  const sources = normalizeSet(params.get('source'));
  const regions = normalizeSet(params.get('region'));
  const scoreMin = params.get('score_min');
  const scoreMax = params.get('score_max');

  return records
    .filter((record) => {
      if (sources.size > 0 && !sources.has(normalize(sourceOf(record)))) {
        return false;
      }
      if (regions.size > 0 && !regions.has(normalize(regionOf(record)))) {
        return false;
      }
      if (scoreMin && record.score < Number(scoreMin)) {
        return false;
      }
      if (scoreMax && record.score > Number(scoreMax)) {
        return false;
      }
      return true;
    })
    .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
}

function filterBySource(records, source) {
  const target = normalize(source);
  return records.filter((record) => normalize(sourceOf(record)) === target);
}

function normalizeRecord(record) {
  const measurement = normalizeMeasurement(record.measurement || {});
  return {
    ...record,
    measurement,
    components: record.components || {},
    timestamp: record.timestamp ? new Date(record.timestamp).toISOString() : new Date().toISOString(),
  };
}

function serializeRecord(record) {
  return {
    ...record,
    measurement: record.measurement,
    components: record.components,
    timestamp: record.timestamp,
  };
}

function normalizeMeasurement(measurement) {
  const cfColo = measurement.cfColo || measurement.CFColo || measurement.cf_colo || measurement.colocation || '';
  const rttMs =
    measurement.rttMs ?? measurement.rtt_ms ?? measurement.RTTMs ?? measurement.RTT_ms ?? measurement.rtt ?? null;
  return {
    ...measurement,
    ip: measurement.ip || measurement.IP || measurement.address || '',
    domain: measurement.domain || measurement.Domain || '',
    cfColo,
    cf_colo: cfColo,
    colo: measurement.colo || measurement.Colo || cfColo,
    alpn: measurement.alpn || measurement.ALPN || '',
    rttMs,
    rtt_ms: rttMs,
  };
}

function normalize(value) {
  return (value || '').toString().trim().toLowerCase();
}

function normalizeSet(value) {
  if (!value) {
    return new Set();
  }
  return new Set(
    value
      .split(',')
      .map((item) => normalize(item))
      .filter(Boolean),
  );
}

function sourceOf(record) {
  return record.source || record.measurement?.domain || record.measurement?.alpn || 'unknown';
}

function regionOf(record) {
  return record.region || record.measurement?.cfColo || record.measurement?.cf_colo || '';
}

function parseBucket(raw) {
  if (!raw) {
    return 60_000;
  }
  const match = raw.match(/^(\d+)(ms|s|m|h)$/i);
  if (!match) {
    return 60_000;
  }
  const value = Number(match[1]);
  const unit = match[2].toLowerCase();
  const multipliers = { ms: 1, s: 1000, m: 60_000, h: 3_600_000 };
  return value * (multipliers[unit] || 60_000);
}

function toNumber(value, fallback) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}
