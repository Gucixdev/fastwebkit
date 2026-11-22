import { describe, it, expect } from "vitest";
import { signal, effect, computed } from "./reactive";

describe("signal", () => {
  it("reads back the current value", () => {
    const [get] = signal(1);
    expect(get()).toBe(1);
  });

  it("write() updates what read() returns", () => {
    const [get, set] = signal(1);
    set(2);
    expect(get()).toBe(2);
  });

  it("does not notify effects when written with the same value", () => {
    const [get, set] = signal(1);
    let runs = 0;
    effect(() => {
      get();
      runs++;
    });
    expect(runs).toBe(1);
    set(1);
    expect(runs).toBe(1);
  });
});

describe("effect", () => {
  it("runs immediately on creation", () => {
    let runs = 0;
    effect(() => runs++);
    expect(runs).toBe(1);
  });

  it("re-runs when a signal it read changes", () => {
    const [get, set] = signal(1);
    let seen: number[] = [];
    effect(() => seen.push(get()));
    set(2);
    set(3);
    expect(seen).toEqual([1, 2, 3]);
  });

  it("only re-runs for signals read on its most recent run (dynamic dependencies)", () => {
    const [cond, setCond] = signal(true);
    const [a, setA] = signal("a");
    const [b, setB] = signal("b");
    let runs = 0;

    effect(() => {
      runs++;
      return cond() ? a() : b();
    });
    expect(runs).toBe(1);

    setCond(false); // now reading b(), not a()
    expect(runs).toBe(2);

    setA("a2"); // a is no longer read - should NOT trigger a re-run
    expect(runs).toBe(2);

    setB("b2"); // b is now read - should trigger
    expect(runs).toBe(3);
  });

  it("the returned unsubscribe stops further re-runs", () => {
    const [get, set] = signal(1);
    let runs = 0;
    const unsubscribe = effect(() => {
      get();
      runs++;
    });
    unsubscribe();
    set(2);
    expect(runs).toBe(1);
  });
});

describe("computed", () => {
  it("derives from a signal and updates when it changes", () => {
    const [get, set] = signal(2);
    const doubled = computed(() => get() * 2);
    expect(doubled()).toBe(4);
    set(5);
    expect(doubled()).toBe(10);
  });

  it("composes from multiple signals", () => {
    const [a, setA] = signal(1);
    const [b, setB] = signal(2);
    const sum = computed(() => a() + b());
    expect(sum()).toBe(3);
    setA(10);
    expect(sum()).toBe(12);
    setB(20);
    expect(sum()).toBe(30);
  });
});
