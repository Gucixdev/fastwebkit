// Toast notifications: the SaaS staple for "saved", "upload failed", etc. One stack mounted
// once per page; notify() pushes onto it and schedules its own auto-dismiss. The scheduling/
// id/dedupe logic is plain data manipulation on an array (pure, tested); the DOM container is
// a thin renderer driven by that array.
export type NotificationType = "info" | "success" | "error";

export interface Notification {
  id: number;
  message: string;
  type: NotificationType;
}

let nextId = 1;

export class NotificationCenter {
  readonly el: HTMLElement;
  private items: Notification[] = [];
  private readonly timers = new Map<number, ReturnType<typeof setTimeout>>();

  constructor(container: HTMLElement) {
    this.el = document.createElement("div");
    this.el.className = "fwa-notifications";
    container.append(this.el);
  }

  notify(message: string, type: NotificationType = "info", durationMs = 4000): number {
    const id = nextId++;
    this.items.push({ id, message, type });
    this.render();
    if (durationMs > 0) {
      this.timers.set(
        id,
        setTimeout(() => this.dismiss(id), durationMs),
      );
    }
    return id;
  }

  dismiss(id: number): void {
    this.items = this.items.filter((n) => n.id !== id);
    const timer = this.timers.get(id);
    if (timer) {
      clearTimeout(timer);
      this.timers.delete(id);
    }
    this.render();
  }

  get all(): readonly Notification[] {
    return this.items;
  }

  private render(): void {
    this.el.innerHTML = "";
    for (const n of this.items) {
      const toast = document.createElement("div");
      toast.className = `fwa-toast fwa-toast-${n.type}`;
      toast.textContent = n.message;
      toast.addEventListener("click", () => this.dismiss(n.id));
      this.el.append(toast);
    }
  }
}
