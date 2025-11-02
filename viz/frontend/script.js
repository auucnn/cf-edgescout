const navToggle = document.querySelector('.nav-toggle');
const navLinks = document.querySelector('.nav-links');
const switches = document.querySelectorAll('.switch');
const frontendPanel = document.getElementById('frontend-panel');
const backendPanel = document.getElementById('backend-panel');
const experienceCanvas = document.getElementById('experience-canvas');
const latencyCanvas = document.getElementById('latency-chart');
const waveform = document.getElementById('waveform');
const logStream = document.getElementById('log-stream');

navToggle.addEventListener('click', () => {
  navLinks.classList.toggle('open');
});

navLinks.addEventListener('click', (event) => {
  if (event.target.tagName === 'A') {
    navLinks.classList.remove('open');
  }
});

switches.forEach((switchButton) => {
  switchButton.addEventListener('click', () => {
    switches.forEach((btn) => btn.classList.remove('active'));
    switchButton.classList.add('active');
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

  const peaks = role === 'frontend'
    ? [40, 120, 80, 180, 140, 220, 160]
    : [160, 80, 200, 120, 220, 100, 240];

  ctx.beginPath();
  ctx.moveTo(0, experienceCanvas.height);
  const step = experienceCanvas.width / (peaks.length - 1);

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

paintExperience('frontend');

const latencyCtx = latencyCanvas.getContext('2d');

const latencyState = {
  regions: ['北美', '欧洲', '亚太', '拉美', '中东'],
  values: Array(5).fill().map(() => Array(40).fill(120)),
};

function drawLatency() {
  latencyCtx.clearRect(0, 0, latencyCanvas.width, latencyCanvas.height);
  latencyCtx.lineWidth = 2;
  latencyCtx.lineJoin = 'round';
  latencyCtx.lineCap = 'round';

  const colors = [
    'rgba(109, 121, 255, 0.8)',
    'rgba(21, 209, 209, 0.75)',
    'rgba(246, 201, 69, 0.75)',
    'rgba(255, 111, 145, 0.75)',
    'rgba(136, 132, 255, 0.75)'
  ];

  latencyState.values.forEach((series, idx) => {
    latencyCtx.beginPath();
    series.forEach((value, valueIdx) => {
      const x = (valueIdx / (series.length - 1)) * latencyCanvas.width;
      const y = latencyCanvas.height - (value / 300) * latencyCanvas.height;
      if (valueIdx === 0) {
        latencyCtx.moveTo(x, y);
      } else {
        latencyCtx.lineTo(x, y);
      }
    });
    latencyCtx.strokeStyle = colors[idx % colors.length];
    latencyCtx.stroke();
  });

  latencyCtx.fillStyle = 'rgba(255,255,255,0.3)';
  latencyCtx.font = '12px "JetBrains Mono", monospace';
  latencyCtx.textAlign = 'right';
  latencyCtx.textBaseline = 'top';
  latencyState.regions.forEach((region, index) => {
    latencyCtx.fillText(`${region} ${latencyState.values[index].slice(-1)[0]}ms`, latencyCanvas.width - 10, 10 + index * 18);
  });
}

function updateLatency() {
  latencyState.values = latencyState.values.map((series) => {
    const next = series.slice(1);
    const variation = Math.random() * 40 - 20;
    const newValue = Math.max(60, Math.min(260, series[series.length - 1] + variation));
    next.push(Math.round(newValue));
    return next;
  });

  drawLatency();
}

drawLatency();
setInterval(updateLatency, 1400);

function renderWaveform() {
  waveform.innerHTML = '';
  const fragments = 20;
  for (let i = 0; i < fragments; i += 1) {
    const bar = document.createElement('span');
    bar.style.height = `${Math.random() * 120 + 20}px`;
    waveform.appendChild(bar);
  }
}

setInterval(renderWaveform, 800);
renderWaveform();

const logEvents = [
  'Scheduler ▶︎ 重新平衡北美节点权重',
  'Sampler ✓ 收到新一轮探针数据 (99 节点)',
  'Scorer ⚡ 即时评分更新，边缘节点 #47 -> 0.92',
  'Store ⬇︎ 压缩批次完成 1.8GB → 210MB',
  'Viz ⟳ 推送增量渲染指令，流式刷新成功',
  'Geo ⇅ 切换亚太分片路由策略',
  'Exporter ☁️ 推送延迟指标到公共仪表',
  'Fetcher ⏱️ 采集窗口缩短至 35s，应对流量激增'
];

function pushLogEntry() {
  const entry = document.createElement('li');
  const message = logEvents[Math.floor(Math.random() * logEvents.length)];
  const timestamp = new Date().toLocaleTimeString('zh-CN', { hour12: false });
  entry.textContent = `[${timestamp}] ${message}`;
  logStream.prepend(entry);
  while (logStream.children.length > 8) {
    logStream.lastChild.remove();
  }
}

setInterval(pushLogEntry, 2000);
pushLogEntry();

let rafId;
let frame = 0;

function animateOrbs() {
  frame += 1;
  document.documentElement.style.setProperty('--orb-angle', frame);
  rafId = requestAnimationFrame(animateOrbs);
}

animateOrbs();

window.addEventListener('beforeunload', () => cancelAnimationFrame(rafId));
