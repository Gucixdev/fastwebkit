import { describe, it, expect, vi } from "vitest";
import { matches, filterByQuery, debounce, SearchBar } from "./searchbar";

describe("matches", () => {
  it("matches everything on an empty query", () => {
    expect(matches("Raspberry Pi 5 Case", "")).toBe(true);
    expect(matches("Raspberry Pi 5 Case", "   ")).toBe(true);
  });

  it("is case-insensitive", () => {
    expect(matches("Raspberry Pi 5 Case", "RASPBERRY")).toBe(true);
  });

  it("requires every term to appear, in any order", () => {
    expect(matches("Raspberry Pi 5 Aluminum Case", "pi case")).toBe(true);
    expect(matches("Raspberry Pi 5 Aluminum Case", "case pi")).toBe(true);
  });

  it("rejects when any term is missing", () => {
    expect(matches("Raspberry Pi 5 Case", "pi phone")).toBe(false);
  });
});

describe("filterByQuery", () => {
  it("filters a list of objects by a projected text field", () => {
    const items = [{ title: "Pi Case" }, { title: "USB Cable" }];
    expect(filterByQuery(items, "pi", (i) => i.title)).toEqual([{ title: "Pi Case" }]);
  });
});

describe("debounce", () => {
  it("only calls the function once after the delay, with the last arguments", () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    debounced("a");
    debounced("b");
    debounced("c");
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(100);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenCalledWith("c");
    vi.useRealTimers();
  });
});

describe("SearchBar", () => {
  it("mounts an <input type=search> and fires onQuery after the debounce", () => {
    vi.useFakeTimers();
    const container = document.createElement("div");
    const bar = new SearchBar(container, { debounceMs: 50 });
    expect(container.querySelector("input[type=search]")).toBe(bar.input);

    const received: string[] = [];
    bar.onQuery((q) => received.push(q));

    bar.input.value = "pi";
    bar.input.dispatchEvent(new Event("input"));
    expect(received).toEqual([]);

    vi.advanceTimersByTime(50);
    expect(received).toEqual(["pi"]);
    vi.useRealTimers();
  });

  it("clear() resets the value and notifies listeners immediately", () => {
    const container = document.createElement("div");
    const bar = new SearchBar(container);
    bar.input.value = "pi";
    const received: string[] = [];
    bar.onQuery((q) => received.push(q));

    bar.clear();
    expect(bar.value).toBe("");
    expect(received).toEqual([""]);
  });
});
