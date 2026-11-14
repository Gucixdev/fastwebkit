import { describe, it, expect } from "vitest";
import { isValidSize, clampPixelRatio } from "./scene";

// Scene3D itself constructs a real THREE.WebGLRenderer, which needs a real WebGL context -
// jsdom's canvas.getContext() returns null for "webgl"/"webgl2" (no GPU in this environment,
// and no headless-GL package is installed here). Rather than mock enough of the WebGL API to
// let three.js's constructor limp through (which would test the mock, not the gem), this file
// tests the pure helpers extracted out of Scene3D's resize logic, and the real rendering is
// verified the honest way: visually, via the live demo in a real browser.
describe("isValidSize", () => {
  it("is true for positive dimensions", () => {
    expect(isValidSize(800, 600)).toBe(true);
  });
  it("is false when either dimension is zero or negative", () => {
    expect(isValidSize(0, 600)).toBe(false);
    expect(isValidSize(800, 0)).toBe(false);
    expect(isValidSize(-1, 600)).toBe(false);
  });
});

describe("clampPixelRatio", () => {
  it("passes through ratios at or below the cap", () => {
    expect(clampPixelRatio(1)).toBe(1);
    expect(clampPixelRatio(2)).toBe(2);
  });
  it("caps a high-DPI ratio at the default of 2", () => {
    expect(clampPixelRatio(3)).toBe(2);
    expect(clampPixelRatio(4)).toBe(2);
  });
  it("respects a custom cap", () => {
    expect(clampPixelRatio(4, 3)).toBe(3);
  });
});
