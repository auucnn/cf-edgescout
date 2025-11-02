let trendChart;
let radarChart;

export function renderTrend(canvas, points) {
  if (!window.Chart || !canvas) {
    return;
  }
  const labels = points.map((point) => new Date(point.timestamp).toLocaleTimeString());
  const data = points.map((point) => point.average);

  if (!trendChart) {
    trendChart = new window.Chart(canvas, {
      type: 'line',
      data: {
        labels,
        datasets: [
          {
            label: '平均得分',
            data,
            borderColor: '#38bdf8',
            backgroundColor: 'rgba(56, 189, 248, 0.25)',
            borderWidth: 2,
            tension: 0.3,
            fill: true,
            pointRadius: 0,
          },
        ],
      },
      options: buildLineOptions(),
    });
    return;
  }

  trendChart.data.labels = labels;
  trendChart.data.datasets[0].data = data;
  trendChart.update();
}

export function renderRadar(canvas, components) {
  if (!window.Chart || !canvas) {
    return;
  }
  const labels = components.map((component) => component.key);
  const data = components.map((component) => component.average);

  if (!radarChart) {
    radarChart = new window.Chart(canvas, {
      type: 'radar',
      data: {
        labels,
        datasets: [
          {
            label: '组件健康',
            data,
            backgroundColor: 'rgba(34, 211, 238, 0.25)',
            borderColor: '#22d3ee',
            borderWidth: 2,
            pointBackgroundColor: '#22d3ee',
          },
        ],
      },
      options: buildRadarOptions(),
    });
    return;
  }

  radarChart.data.labels = labels;
  radarChart.data.datasets[0].data = data;
  radarChart.update();
}

function buildLineOptions() {
  return {
    maintainAspectRatio: false,
    scales: {
      x: {
        ticks: { color: '#94a3b8' },
        grid: { color: 'rgba(148, 163, 184, 0.15)' },
      },
      y: {
        ticks: {
          color: '#94a3b8',
          callback: (value) => value.toFixed(2),
        },
        suggestedMin: 0,
        suggestedMax: 1,
        grid: { color: 'rgba(148, 163, 184, 0.15)' },
      },
    },
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (context) => `平均得分: ${context.parsed.y.toFixed(2)}`,
        },
      },
    },
  };
}

function buildRadarOptions() {
  return {
    maintainAspectRatio: false,
    scales: {
      r: {
        beginAtZero: true,
        suggestedMax: 1,
        angleLines: { color: 'rgba(148, 163, 184, 0.2)' },
        grid: { color: 'rgba(148, 163, 184, 0.15)' },
        pointLabels: { color: '#94a3b8', font: { size: 12 } },
        ticks: {
          backdropColor: 'transparent',
          color: '#94a3b8',
          stepSize: 0.2,
        },
      },
    },
    plugins: {
      legend: { display: false },
    },
  };
}
