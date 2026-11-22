// The framework's one reactive primitive: a signal plus effects that re-run when a signal
// they read changes. Every gem (layout, spring, render3d) is built on this, not on each other.
type Effect0 = () => void;
type Effect = { run: () => void; unsubs: Set<() => void> };

let activeEffect: Effect | null = null;

export function signal<T>(initial: T): [() => T, (next: T) => void] {
  let value = initial;
  const subscribers = new Set<Effect>();

  const read = (): T => {
    if (activeEffect) {
      const current = activeEffect;
      subscribers.add(current);
      current.unsubs.add(() => subscribers.delete(current));
    }
    return value;
  };

  const write = (next: T): void => {
    if (next === value) return;
    value = next;
    for (const e of [...subscribers]) e.run();
  };

  return [read, write];
}

// Re-running tears down last run's subscriptions first, so an effect only ever listens to the
// signals it read on its most recent run (branches that stop being read stop being watched).
export function effect(fn: Effect0): () => void {
  const e: Effect = { unsubs: new Set(), run: () => {} };
  e.run = () => {
    for (const unsub of e.unsubs) unsub();
    e.unsubs.clear();
    const previous = activeEffect;
    activeEffect = e;
    try {
      fn();
    } finally {
      activeEffect = previous;
    }
  };
  e.run();
  return () => {
    for (const unsub of e.unsubs) unsub();
    e.unsubs.clear();
  };
}

export function computed<T>(fn: () => T): () => T {
  const [get, set] = signal<T>(fn());
  effect(() => set(fn()));
  return get;
}
