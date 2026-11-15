// Light/dark (+ any named custom theme) via a `data-theme` attribute on <html> and persisted
// in localStorage. Deliberately not CSS-variable-generation itself - this gem only decides and
// remembers *which* theme is active; the app's own CSS defines what each data-theme value looks
// like (same pattern most static-site dark-mode toggles use), which keeps this gem tiny and
// keeps the actual color choices out of a gem's hands.
const STORAGE_KEY = "fastwebapp-theme";

export type Theme = string;

// Pure: the next theme in a cycle, wrapping around. Used by toggle()/cycle controls.
export function nextTheme(current: Theme, available: Theme[]): Theme {
  if (available.length === 0) return current;
  const i = available.indexOf(current);
  return available[(i + 1) % available.length]!;
}

// Wrapped in try/catch: localStorage can throw (private browsing, blocked storage) and a theme
// preference failing to persist should never break the page.
export function readStoredTheme(): Theme | null {
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

export function storeTheme(theme: Theme): void {
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // ignored - see comment above
  }
}

export class ThemeController {
  private current: Theme;
  private readonly available: Theme[];
  private readonly listeners = new Set<(theme: Theme) => void>();

  constructor(available: Theme[], defaultTheme: Theme) {
    this.available = available;
    this.current = readStoredTheme() ?? defaultTheme;
    this.apply();
  }

  get theme(): Theme {
    return this.current;
  }

  set(theme: Theme): void {
    this.current = theme;
    storeTheme(theme);
    this.apply();
    for (const fn of this.listeners) fn(theme);
  }

  cycle(): void {
    this.set(nextTheme(this.current, this.available));
  }

  onChange(fn: (theme: Theme) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private apply(): void {
    document.documentElement.setAttribute("data-theme", this.current);
  }
}
