// Chat message store as a tree, not a flat list - how ChatGPT/Claude/DeepSeek's "edit message"
// and "fork" actually work under the hood: editing or forking a message doesn't delete history,
// it starts a sibling branch from the same parent, and which branch you're viewing is just which
// leaf is "active" right now. getThread() walks a leaf back to the root to produce the one line
// of conversation shown on screen.
//
// This gem is deliberately just that - the tree and a bare bubble renderer. Whether a given app
// even has per-message actions, and which ones (copy, fork, regenerate, delete, react, ...), is
// an app-level decision: a chatbot wants fork/regenerate, a plain two-person messenger wants
// neither and maybe wants "delete for everyone" instead, something else might want neither plus
// reactions. ChatView takes an optional renderActions hook instead of hardcoding any of them.
export type Role = "user" | "assistant";

export interface ChatMessage {
  id: string;
  parentId: string | null;
  role: Role;
  text: string;
  createdAt: number;
}

let nextId = 1;
function makeId(): string {
  return `m${nextId++}`;
}

export class ChatStore {
  private messages = new Map<string, ChatMessage>();

  add(parentId: string | null, role: Role, text: string): ChatMessage {
    const message: ChatMessage = { id: makeId(), parentId, role, text, createdAt: Date.now() };
    this.messages.set(message.id, message);
    return message;
  }

  get(id: string): ChatMessage | undefined {
    return this.messages.get(id);
  }

  // Every message sharing `parentId`, in creation order - the sibling list a "< 1/3 >" branch
  // switcher (an app's own UI, if it wants one) would page through.
  childrenOf(parentId: string | null): ChatMessage[] {
    return [...this.messages.values()].filter((m) => m.parentId === parentId);
  }

  // Walks parentId back to a root, returning root-to-leaf in display order.
  getThread(leafId: string): ChatMessage[] {
    const thread: ChatMessage[] = [];
    let current = this.messages.get(leafId);
    while (current) {
      thread.unshift(current);
      current = current.parentId === null ? undefined : this.messages.get(current.parentId);
    }
    return thread;
  }

  // "Fork" = start a new sibling branch at the same point as an existing message, instead of
  // appending after it - the caller supplies what goes in the new branch (typically the same
  // role with edited text, to redo a turn without losing the original). A tree operation any
  // app can use (or never call) - not tied to any particular UI button.
  fork(messageId: string, newText: string): ChatMessage {
    const original = this.messages.get(messageId);
    if (!original) throw new Error(`fork: no such message ${messageId}`);
    return this.add(original.parentId, original.role, newText);
  }

  // Regenerate is the same tree operation as fork, applied to an assistant reply - named for
  // what it means in a chatbot UI rather than given separate logic. A plain messenger has no
  // use for this and simply never calls it.
  regenerate(assistantMessageId: string, newText: string): ChatMessage {
    const original = this.messages.get(assistantMessageId);
    if (!original) throw new Error(`regenerate: no such message ${assistantMessageId}`);
    if (original.role !== "assistant") throw new Error("regenerate: only assistant messages can be regenerated");
    return this.fork(assistantMessageId, newText);
  }

  siblingIndex(messageId: string): { index: number; count: number } {
    const message = this.messages.get(messageId);
    if (!message) return { index: 0, count: 0 };
    const siblings = this.childrenOf(message.parentId);
    return { index: siblings.findIndex((m) => m.id === messageId), count: siblings.length };
  }
}

// copyText/copyToClipboard are plain utilities an app can wire into its own action, not
// something ChatView calls on its own.
export function copyText(message: ChatMessage): string {
  return message.text;
}

// Browser-only clipboard write - not asserted in unit tests (no real clipboard in a headless
// DOM environment). Swallows a denied-permission error rather than throwing, since a failed
// copy shouldn't break the rest of a chat UI.
export async function copyToClipboard(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // ignored - see comment above
  }
}

export interface ChatViewOptions {
  // Called once per message to build whatever action buttons/labels this app wants under that
  // bubble (Copy+Fork+Regenerate for a chatbot, nothing for a read-only log, a Delete button
  // for a messenger, ...). Returning an empty array (or omitting the option) renders no actions.
  renderActions?: (message: ChatMessage, store: ChatStore) => HTMLElement[];
}

export class ChatView {
  readonly el: HTMLElement;

  constructor(
    container: HTMLElement,
    private readonly store: ChatStore,
    private readonly options: ChatViewOptions = {},
  ) {
    this.el = document.createElement("div");
    this.el.className = "fwa-chat";
    container.append(this.el);
  }

  render(leafId: string): void {
    this.el.innerHTML = "";
    for (const message of this.store.getThread(leafId)) {
      this.el.append(this.renderMessage(message));
    }
  }

  private renderMessage(message: ChatMessage): HTMLElement {
    const bubble = document.createElement("div");
    bubble.className = `fwa-chat-message fwa-chat-${message.role}`;
    bubble.dataset.messageId = message.id;

    const text = document.createElement("div");
    text.className = "fwa-chat-text";
    text.textContent = message.text;
    bubble.append(text);

    const actionEls = this.options.renderActions?.(message, this.store) ?? [];
    if (actionEls.length > 0) {
      const actions = document.createElement("div");
      actions.className = "fwa-chat-actions";
      for (const el of actionEls) actions.append(el);
      bubble.append(actions);
    }

    return bubble;
  }
}

// actionButton is a small helper for building the kind of button renderActions typically
// returns - exported so an app doesn't have to re-write "create a <button>, set its label and
// class, wire a click handler" itself.
export function actionButton(label: string, className: string, onClick: () => void): HTMLButtonElement {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = className;
  btn.textContent = label;
  btn.addEventListener("click", onClick);
  return btn;
}
