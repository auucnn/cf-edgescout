import { MockAdapter } from './mock.js';

const DEFAULT_TIMEOUT = 8000;

export class ApiClient {
  constructor(base = window.EDGESCOUT_API_BASE) {
    this.base = normalizeBase(base);
    this.offline = false;
    this.mock = new MockAdapter('mock-data.json');
  }

  async get(path, params = new URLSearchParams()) {
    if (!this.offline) {
      try {
        const response = await this.fetchWithTimeout(this.composeURL(path, params));
        if (!response.ok) {
          throw new Error(`请求失败: ${response.status}`);
        }
        return response.json();
      } catch (error) {
        console.warn('API 请求失败，切换至 mock 数据。', error);
        this.offline = true;
      }
    }
    return this.mock.query(path, params);
  }

  composeURL(path, params) {
    const base = this.base || window.location.origin;
    const url = new URL(path, base.endsWith('/') ? base : `${base}/`);
    params.forEach((value, key) => {
      if (!url.searchParams.has(key)) {
        url.searchParams.set(key, value);
      }
    });
    return url.toString();
  }

  async fetchWithTimeout(url) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), DEFAULT_TIMEOUT);
    try {
      return await fetch(url, {
        headers: { Accept: 'application/json' },
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timer);
    }
  }

  isOffline() {
    return this.offline;
  }
}

function normalizeBase(base) {
  if (!base) {
    return '';
  }
  try {
    const url = new URL(base, window.location.origin);
    return url.toString().replace(/\/$/, '');
  } catch (error) {
    console.warn('无效的 API 基址，降级为相对路径。', error);
    return '';
  }
}
