const state = {
  filters: {
    source: 'official',
    region: '',
    scoreMin: '',
    scoreMax: '',
    search: '',
  },
  summary: null,
  timeseries: [],
  results: [],
  total: 0,
  usingMock: false,
  loading: false,
  error: '',
  sourceDetail: null,
};

const listeners = new Set();

export function getState() {
  return { ...state, filters: { ...state.filters } };
}

export function subscribe(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function update(partial) {
  if (partial.filters) {
    Object.assign(state.filters, partial.filters);
  }
  Object.assign(state, { ...partial, filters: state.filters });
  listeners.forEach((listener) => listener(getState()));
}

export function setError(message) {
  update({ error: message });
}

export function setLoading(loading) {
  update({ loading });
}

export function setUsingMock(usingMock) {
  update({ usingMock });
}
