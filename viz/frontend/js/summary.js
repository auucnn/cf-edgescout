export function summariseSummary(records) {
  const sorted = [...records].sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
  const total = sorted.length;
  if (total === 0) {
    return {
      total: 0,
      updated_at: null,
      score: defaultScoreSummary(),
      sources: [],
      regions: [],
      components: [],
      recent: [],
    };
  }

  return {
    total,
    updated_at: sorted[total - 1].timestamp,
    score: summariseScores(sorted),
    sources: summariseGroups(sorted, sourceOf),
    regions: summariseGroups(sorted, regionOf),
    components: summariseComponents(sorted),
    recent: lastN(sorted, 10).reverse().map(serializeRecord),
  };
}

export function summariseSource(source, records) {
  const sorted = [...records].sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
  return {
    source,
    total: sorted.length,
    score: summariseScores(sorted),
    regions: summariseGroups(sorted, regionOf),
    components: summariseComponents(sorted),
    recent: lastN(sorted, 10).reverse().map(serializeRecord),
  };
}

export function summariseTimeseries(records, bucketMs = 60_000) {
  if (!records.length) {
    return [];
  }
  const sorted = [...records].sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
  const start = new Date(sorted[0].timestamp);
  const buckets = new Map();

  for (const record of sorted) {
    const ts = new Date(record.timestamp);
    const key = start.getTime() + Math.floor((ts.getTime() - start.getTime()) / bucketMs) * bucketMs;
    if (!buckets.has(key)) {
      buckets.set(key, []);
    }
    buckets.get(key).push(record);
  }

  return Array.from(buckets.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([key, group]) => ({
      timestamp: new Date(key).toISOString(),
      count: group.length,
      average: average(group.map((r) => r.score)),
      regions: averageBy(group, regionOf),
    }));
}

function summariseScores(records) {
  if (records.length === 0) {
    return defaultScoreSummary();
  }
  const scores = records.map((record) => record.score).sort((a, b) => a - b);
  const averageScore = average(scores);
  const min = scores[0];
  const max = scores[scores.length - 1];
  const median = scores.length % 2 === 1
    ? scores[(scores.length - 1) / 2]
    : (scores[scores.length / 2 - 1] + scores[scores.length / 2]) / 2;
  const latest = records.reduce((latest, record) => {
    const ts = new Date(record.timestamp);
    return ts > latest ? ts : latest;
  }, new Date(records[0].timestamp));

  return {
    average: round(averageScore),
    min: round(min),
    max: round(max),
    median: round(median),
    latest: latest.toISOString(),
  };
}

function summariseGroups(records, selector) {
  const groups = new Map();
  for (const record of records) {
    const key = (selector(record) || 'unknown').toLowerCase();
    if (!groups.has(key)) {
      groups.set(key, []);
    }
    groups.get(key).push(record.score);
  }

  return Array.from(groups.entries())
    .map(([key, values]) => ({
      key,
      count: values.length,
      average: round(average(values)),
      min: round(Math.min(...values)),
      max: round(Math.max(...values)),
    }))
    .sort((a, b) => (a.count === b.count ? a.key.localeCompare(b.key) : b.count - a.count));
}

function summariseComponents(records) {
  const components = new Map();
  for (const record of records) {
    if (!record.components) {
      continue;
    }
    Object.entries(record.components).forEach(([key, value]) => {
      const scores = components.get(key) || [];
      scores.push(Number(value));
      components.set(key, scores);
    });
  }

  return Array.from(components.entries())
    .map(([key, values]) => ({
      key,
      average: round(average(values)),
    }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

function lastN(records, n) {
  if (records.length <= n) {
    return [...records];
  }
  return records.slice(records.length - n);
}

function average(values) {
  if (!values.length) {
    return 0;
  }
  const sum = values.reduce((total, value) => total + Number(value), 0);
  return sum / values.length;
}

function averageBy(records, selector) {
  const groups = new Map();
  for (const record of records) {
    const key = (selector(record) || 'unknown').toLowerCase();
    const entry = groups.get(key) || { sum: 0, count: 0 };
    entry.sum += Number(record.score);
    entry.count += 1;
    groups.set(key, entry);
  }
  const result = {};
  for (const [key, entry] of groups) {
    if (entry.count === 0) {
      continue;
    }
    result[key] = round(entry.sum / entry.count);
  }
  return result;
}

function serializeRecord(record) {
  return {
    ...record,
    timestamp: record.timestamp,
  };
}

function sourceOf(record) {
  return record.source || record.measurement?.domain || record.measurement?.alpn || 'unknown';
}

function regionOf(record) {
  return record.region || record.measurement?.cf_colo || record.measurement?.cfColo || '';
}

function defaultScoreSummary() {
  return {
    average: 0,
    min: 0,
    max: 0,
    median: 0,
    latest: null,
  };
}

function round(value) {
  return Math.round(Number(value) * 100) / 100;
}
