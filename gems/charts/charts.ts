// Bar and line charts on a plain <canvas>, no charting library - the geometry (scale a value
// into a pixel rect) is simple enough to own, matching every other gem here. Split into pure
// layout functions (scale values into rectangles/points - fully unit-testable) and a thin draw
// step that just paints what the layout computed, so the geometry is tested even though actual
// canvas painting can't be asserted on in a headless test environment.
export interface ChartRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface ChartPoint {
  x: number;
  y: number;
}

export interface BarDatum {
  label: string;
  value: number;
}

export interface ChartLayoutOptions {
  width: number;
  height: number;
  padding?: number;
}

// Pure: one rect per datum, scaled so the tallest bar touches the top padding line and every
// bar sits on the baseline. Guards a flat all-zero dataset (would otherwise divide by zero).
export function computeBarLayout(data: BarDatum[], options: ChartLayoutOptions): ChartRect[] {
  const padding = options.padding ?? 8;
  const plotWidth = Math.max(0, options.width - padding * 2);
  const plotHeight = Math.max(0, options.height - padding * 2);
  if (data.length === 0) return [];

  const maxValue = Math.max(...data.map((d) => d.value), 0);
  const barWidth = plotWidth / data.length;

  return data.map((d, i) => {
    const h = maxValue > 0 ? (d.value / maxValue) * plotHeight : 0;
    return {
      x: padding + i * barWidth,
      y: padding + (plotHeight - h),
      width: barWidth,
      height: h,
    };
  });
}

// Pure: one point per value, evenly spaced on x, scaled on y the same way computeBarLayout
// scales height - shared min/max so a flat series renders as a flat mid-height line, not an
// error.
export function computeLineLayout(values: number[], options: ChartLayoutOptions): ChartPoint[] {
  const padding = options.padding ?? 8;
  const plotWidth = Math.max(0, options.width - padding * 2);
  const plotHeight = Math.max(0, options.height - padding * 2);
  if (values.length === 0) return [];
  if (values.length === 1) {
    return [{ x: padding, y: padding + plotHeight / 2 }];
  }

  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min;
  const step = plotWidth / (values.length - 1);

  return values.map((v, i) => {
    const t = range > 0 ? (v - min) / range : 0.5;
    return {
      x: padding + i * step,
      y: padding + (1 - t) * plotHeight,
    };
  });
}

export function drawBarChart(canvas: HTMLCanvasElement, data: BarDatum[], color = "#7fd1a8"): void {
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const rects = computeBarLayout(data, { width: canvas.width, height: canvas.height });
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = color;
  const gap = 2;
  for (const r of rects) {
    ctx.fillRect(r.x + gap / 2, r.y, Math.max(0, r.width - gap), r.height);
  }
}

export function drawLineChart(canvas: HTMLCanvasElement, values: number[], color = "#7fd1a8"): void {
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const points = computeLineLayout(values, { width: canvas.width, height: canvas.height });
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  if (points.length === 0) return;
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  ctx.beginPath();
  ctx.moveTo(points[0]!.x, points[0]!.y);
  for (const p of points.slice(1)) ctx.lineTo(p.x, p.y);
  ctx.stroke();
}
