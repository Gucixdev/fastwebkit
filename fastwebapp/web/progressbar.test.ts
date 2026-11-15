import { describe, it, expect } from "vitest";
import { clampPercent, ProgressBar } from "./progressbar";

describe("clampPercent", () => {
  it("passes through in-range values", () => {
    expect(clampPercent(37)).toBe(37);
  });
  it("clamps below 0 and above 100", () => {
    expect(clampPercent(-5)).toBe(0);
    expect(clampPercent(150)).toBe(100);
  });
  it("treats NaN as 0", () => {
    expect(clampPercent(NaN)).toBe(0);
  });
});

describe("ProgressBar", () => {
  it("mounts into the container and starts at 0%", () => {
    const container = document.createElement("div");
    const bar = new ProgressBar(container);
    expect(container.contains(bar.el)).toBe(true);
    const fill = bar.el.querySelector<HTMLElement>(".fwa-progressbar-fill")!;
    expect(fill.style.width).toBe("0%");
  });

  it("set() updates the fill width and clamps out-of-range input", () => {
    const bar = new ProgressBar(document.createElement("div"));
    bar.set(42);
    expect(bar.el.querySelector<HTMLElement>(".fwa-progressbar-fill")!.style.width).toBe("42%");
    bar.set(200);
    expect(bar.el.querySelector<HTMLElement>(".fwa-progressbar-fill")!.style.width).toBe("100%");
  });

  it("set() returns to determinate mode after setIndeterminate()", () => {
    const bar = new ProgressBar(document.createElement("div"));
    bar.setIndeterminate();
    expect(bar.currentMode).toBe("indeterminate");
    expect(bar.el.classList.contains("fwa-progressbar-indeterminate")).toBe(true);

    bar.set(10);
    expect(bar.currentMode).toBe("determinate");
    expect(bar.el.classList.contains("fwa-progressbar-indeterminate")).toBe(false);
  });
});
