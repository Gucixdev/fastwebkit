import { describe, it, expect } from "vitest";
import { computeBarLayout, computeLineLayout, drawBarChart, drawLineChart } from "./charts";

describe("computeBarLayout", () => {
  it("returns one rect per datum, tallest value touching the top padding line", () => {
    const rects = computeBarLayout(
      [
        { label: "a", value: 10 },
        { label: "b", value: 5 },
      ],
      { width: 100, height: 50, padding: 0 },
    );
    expect(rects).toHaveLength(2);
    expect(rects[0]!.height).toBe(50);
    expect(rects[1]!.height).toBe(25);
    expect(rects[0]!.y).toBe(0);
    expect(rects[1]!.y).toBe(25);
  });

  it("divides width evenly across bars", () => {
    const rects = computeBarLayout(
      [
        { label: "a", value: 1 },
        { label: "b", value: 1 },
        { label: "c", value: 1 },
      ],
      { width: 90, height: 10, padding: 0 },
    );
    expect(rects.map((r) => r.x)).toEqual([0, 30, 60]);
    expect(rects.every((r) => r.width === 30)).toBe(true);
  });

  it("does not divide by zero when every value is 0", () => {
    const rects = computeBarLayout([{ label: "a", value: 0 }], { width: 10, height: 10 });
    expect(rects[0]!.height).toBe(0);
  });

  it("returns an empty array for no data", () => {
    expect(computeBarLayout([], { width: 10, height: 10 })).toEqual([]);
  });
});

describe("computeLineLayout", () => {
  it("spaces points evenly on x and scales y between min and max", () => {
    const points = computeLineLayout([0, 5, 10], { width: 100, height: 100, padding: 0 });
    expect(points).toHaveLength(3);
    expect(points[0]!.x).toBe(0);
    expect(points[1]!.x).toBe(50);
    expect(points[2]!.x).toBe(100);
    expect(points[0]!.y).toBe(100); // min value -> bottom
    expect(points[2]!.y).toBe(0); // max value -> top
  });

  it("renders a flat series at mid-height instead of dividing by zero", () => {
    const points = computeLineLayout([5, 5, 5], { width: 100, height: 100, padding: 0 });
    expect(points.every((p) => p.y === 50)).toBe(true);
  });

  it("handles a single value by centering it", () => {
    const points = computeLineLayout([5], { width: 100, height: 100, padding: 10 });
    expect(points).toEqual([{ x: 10, y: 50 }]);
  });

  it("returns an empty array for no data", () => {
    expect(computeLineLayout([], { width: 10, height: 10 })).toEqual([]);
  });
});

describe("draw functions", () => {
  // jsdom has no real canvas 2D backend (getContext("2d") returns null), so these only assert
  // the gem degrades silently instead of throwing - the geometry itself is covered above, and
  // actual pixel output is verified by hand against the live demo.
  it("drawBarChart does not throw when canvas has no 2D context", () => {
    const canvas = document.createElement("canvas");
    expect(() => drawBarChart(canvas, [{ label: "a", value: 1 }])).not.toThrow();
  });

  it("drawLineChart does not throw when canvas has no 2D context", () => {
    const canvas = document.createElement("canvas");
    expect(() => drawLineChart(canvas, [1, 2, 3])).not.toThrow();
  });
});
