import { describe, it, expect, vi } from "vitest";
import { ChatStore, copyText, ChatView, actionButton } from "./chat";

describe("ChatStore", () => {
  it("getThread walks from a leaf back to the root, in display order", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");
    const followUp = store.add(reply.id, "user", "how are you");

    expect(store.getThread(followUp.id).map((m) => m.text)).toEqual(["hi", "hello", "how are you"]);
  });

  it("fork() creates a sibling at the same parent, leaving the original untouched", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");

    const forked = store.fork(reply.id, "hello there!");

    expect(forked.parentId).toBe(root.id);
    expect(forked.id).not.toBe(reply.id);
    expect(store.get(reply.id)!.text).toBe("hello"); // original unchanged
    expect(store.childrenOf(root.id)).toHaveLength(2);
  });

  it("forking does not affect the other branch's thread", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");
    const forked = store.fork(reply.id, "hello there!");

    expect(store.getThread(reply.id).map((m) => m.text)).toEqual(["hi", "hello"]);
    expect(store.getThread(forked.id).map((m) => m.text)).toEqual(["hi", "hello there!"]);
  });

  it("regenerate() forks an assistant message", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");

    const regenerated = store.regenerate(reply.id, "hey!");
    expect(regenerated.role).toBe("assistant");
    expect(regenerated.parentId).toBe(root.id);
  });

  it("regenerate() refuses to regenerate a user message", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    expect(() => store.regenerate(root.id, "x")).toThrow();
  });

  it("siblingIndex reports position and count among branches at the same parent", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");
    const forked = store.fork(reply.id, "hello there!");

    expect(store.siblingIndex(reply.id)).toEqual({ index: 0, count: 2 });
    expect(store.siblingIndex(forked.id)).toEqual({ index: 1, count: 2 });
  });

  it("siblingIndex is {0,1} for a message with no siblings", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    expect(store.siblingIndex(root.id)).toEqual({ index: 0, count: 1 });
  });
});

describe("copyText", () => {
  it("returns the message's raw text", () => {
    expect(copyText({ id: "m1", parentId: null, role: "user", text: "hello", createdAt: 0 })).toBe("hello");
  });
});

describe("ChatView", () => {
  it("renders one bubble per message in the thread, in order", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");

    const view = new ChatView(document.createElement("div"), store);
    view.render(reply.id);

    const bubbles = view.el.querySelectorAll(".fwa-chat-message");
    expect(bubbles).toHaveLength(2);
    expect(bubbles[0]!.classList.contains("fwa-chat-user")).toBe(true);
    expect(bubbles[1]!.classList.contains("fwa-chat-assistant")).toBe(true);
  });

  it("renders no actions container when renderActions is not given", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");

    const view = new ChatView(document.createElement("div"), store);
    view.render(root.id);

    expect(view.el.querySelector(".fwa-chat-actions")).toBeNull();
  });

  it("renders no actions container when renderActions returns an empty array", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");

    const view = new ChatView(document.createElement("div"), store, { renderActions: () => [] });
    view.render(root.id);

    expect(view.el.querySelector(".fwa-chat-actions")).toBeNull();
  });

  it("renders whatever elements renderActions returns, per message - core gem has no opinion on which", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");

    const view = new ChatView(document.createElement("div"), store, {
      renderActions: (message) => [actionButton(`act-${message.role}`, "fwa-chat-action", () => {})],
    });
    view.render(reply.id);

    const bubbles = view.el.querySelectorAll(".fwa-chat-message");
    expect(bubbles[0]!.querySelector(".fwa-chat-actions button")!.textContent).toBe("act-user");
    expect(bubbles[1]!.querySelector(".fwa-chat-actions button")!.textContent).toBe("act-assistant");
  });

  it("renderActions gets the store too, so an app can wire fork/regenerate itself", () => {
    const store = new ChatStore();
    const root = store.add(null, "user", "hi");
    const reply = store.add(root.id, "assistant", "hello");

    const view = new ChatView(document.createElement("div"), store, {
      renderActions: (message, s) => [actionButton("Regenerate", "x", () => s.regenerate(message.id, "redo"))],
    });
    view.render(reply.id);

    const bubbles = view.el.querySelectorAll(".fwa-chat-message");
    bubbles[1]!.querySelector<HTMLButtonElement>("button")!.click();

    expect(store.childrenOf(root.id)).toHaveLength(2);
  });
});

describe("actionButton", () => {
  it("creates a labeled, classed button wired to the given click handler", () => {
    const onClick = vi.fn();
    const btn = actionButton("Copy", "fwa-chat-action", onClick);

    expect(btn.tagName).toBe("BUTTON");
    expect(btn.textContent).toBe("Copy");
    expect(btn.className).toBe("fwa-chat-action");

    btn.click();
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
