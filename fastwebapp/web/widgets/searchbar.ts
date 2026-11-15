// A search input that debounces before firing, plus a pure matcher any list view (datatable,
// item grid, ...) can filter with. Debounce logic and matching are pure functions - the DOM
// bit is a thin wrapper that wires an <input> to them.
export interface SearchBarOptions {
  placeholder?: string;
  debounceMs?: number;
}

// Pure: true if every whitespace-separated term in `query` appears somewhere in `haystack`
// (case-insensitive). Multi-term so "pi case" matches "Raspberry Pi 5 ... Case" without the
// words needing to be adjacent.
export function matches(haystack: string, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (q === "") return true;
  const h = haystack.toLowerCase();
  return q.split(/\s+/).every((term) => h.includes(term));
}

export function filterByQuery<T>(items: T[], query: string, toText: (item: T) => string): T[] {
  return items.filter((item) => matches(toText(item), query));
}

// debounce() is generic (not DOM-specific) so it's independently testable with fake timers.
export function debounce<Args extends unknown[]>(fn: (...args: Args) => void, ms: number): (...args: Args) => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return (...args: Args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

export class SearchBar {
  readonly input: HTMLInputElement;
  private readonly listeners = new Set<(query: string) => void>();

  constructor(container: HTMLElement, options: SearchBarOptions = {}) {
    this.input = document.createElement("input");
    this.input.type = "search";
    this.input.className = "fwa-searchbar";
    this.input.placeholder = options.placeholder ?? "Search…";
    container.append(this.input);

    const fire = debounce((query: string) => {
      for (const fn of this.listeners) fn(query);
    }, options.debounceMs ?? 200);

    this.input.addEventListener("input", () => fire(this.input.value));
  }

  onQuery(fn: (query: string) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  get value(): string {
    return this.input.value;
  }

  clear(): void {
    this.input.value = "";
    for (const fn of this.listeners) fn("");
  }
}
