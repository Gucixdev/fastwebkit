import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { Spring } from "./spring";

// Spring drives itself via requestAnimationFrame, so these tests take control of both rAF and
// performance.now() to advance the simulation in fixed, deterministic steps instead of racing
// real frames.
describe("Spring", () => {
  let now = 0;
  let rafCallbacks: Array<(t: number) => void> = [];

  beforeEach(() => {
    now = 0;
    rafCallbacks = [];
    vi.stubGlobal("performance", { now: () => now });
    vi.stubGlobal("requestAnimationFrame", (cb: (t: number) => void) => {
      rafCallbacks.push(cb);
      return rafCallbacks.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
  });

  afterEach(() => vi.unstubAllGlobals());

  function tick(dtMs: number): void {
    now += dtMs;
    const due = rafCallbacks;
    rafCallbacks = [];
    for (const cb of due) cb(now);
  }

  it("starts at the initial value and does not move until set() is called", () => {
    const spring = new Spring(5);
    expect(spring.value).toBe(5);
    expect(rafCallbacks).toHaveLength(0);
  });

  it("moves toward the target over successive frames", () => {
    const spring = new Spring(0, { stiffness: 170, damping: 26 });
    spring.set(100);
    tick(16);
    const afterOneFrame = spring.value;
    expect(afterOneFrame).toBeGreaterThan(0);
    expect(afterOneFrame).toBeLessThan(100);

    tick(16);
    expect(spring.value).toBeGreaterThan(afterOneFrame);
  });

  it("settles exactly at the target and stops scheduling frames", () => {
    const spring = new Spring(0, { stiffness: 170, damping: 26, precision: 0.01 });
    spring.set(10);
    for (let i = 0; i < 200 && rafCallbacks.length > 0; i++) tick(16);
    expect(spring.value).toBe(10);
    expect(rafCallbacks).toHaveLength(0);
  });

  it("notifies subscribers with every intermediate value", () => {
    const spring = new Spring(0);
    const seen: number[] = [];
    spring.subscribe((x) => seen.push(x));
    spring.set(10);
    tick(16);
    tick(16);
    expect(seen.length).toBeGreaterThanOrEqual(2);
  });

  it("a mid-flight re-target decelerates from the current value, not from 0", () => {
    const spring = new Spring(0, { stiffness: 170, damping: 26 });
    spring.set(100);
    tick(16);
    tick(16);
    const midFlightValue = spring.value;
    expect(midFlightValue).toBeGreaterThan(0);

    spring.set(50);
    tick(16);
    // it should continue from where it was, not snap back toward 0 first
    expect(spring.value).toBeGreaterThan(0);
  });

  it("unsubscribe stops further notifications", () => {
    const spring = new Spring(0);
    const seen: number[] = [];
    const unsubscribe = spring.subscribe((x) => seen.push(x));
    spring.set(10);
    tick(16);
    unsubscribe();
    const countBeforeMoreTicks = seen.length;
    tick(16);
    tick(16);
    expect(seen.length).toBe(countBeforeMoreTicks);
  });
});
