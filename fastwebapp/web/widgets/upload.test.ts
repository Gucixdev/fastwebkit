import { describe, it, expect, vi, beforeEach } from "vitest";
import { formatBytes, validateFile, UploadZone } from "./upload";

describe("formatBytes", () => {
  it("renders small sizes in bytes", () => {
    expect(formatBytes(500)).toBe("500 B");
  });
  it("renders KB/MB/GB with one decimal", () => {
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(1024 * 1024 * 2)).toBe("2.0 MB");
  });
});

describe("validateFile", () => {
  it("passes a file with no constraints", () => {
    expect(validateFile({ name: "a.png", size: 100, type: "image/png" })).toBeNull();
  });

  it("rejects a file over the size limit", () => {
    const msg = validateFile({ name: "big.png", size: 2000, type: "image/png" }, { maxSizeBytes: 1000 });
    expect(msg).toContain("big.png");
    expect(msg).toContain("over");
  });

  it("accepts an exact MIME match", () => {
    expect(validateFile({ name: "a.png", size: 1, type: "image/png" }, { accept: ["image/png"] })).toBeNull();
  });

  it("accepts a wildcard MIME prefix", () => {
    expect(validateFile({ name: "a.png", size: 1, type: "image/png" }, { accept: ["image/*"] })).toBeNull();
  });

  it("rejects a MIME type that isn't in the accept list", () => {
    const msg = validateFile({ name: "a.pdf", size: 1, type: "application/pdf" }, { accept: ["image/*"] });
    expect(msg).toContain("application/pdf");
  });
});

// A minimal stand-in for XMLHttpRequest so upload wiring is testable without a real network -
// no server exists in a unit test, so this fakes just enough of the API UploadZone calls.
class FakeXHR {
  static instances: FakeXHR[] = [];
  method = "";
  url = "";
  status = 0;
  statusText = "";
  responseText = "";
  sentBody: unknown;
  private listeners = new Map<string, (e: unknown) => void>();
  upload = {
    listeners: new Map<string, (e: unknown) => void>(),
    addEventListener(type: string, fn: (e: unknown) => void) {
      this.listeners.set(type, fn);
    },
  };

  constructor() {
    FakeXHR.instances.push(this);
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  addEventListener(type: string, fn: (e: unknown) => void) {
    this.listeners.set(type, fn);
  }
  send(body: unknown) {
    this.sentBody = body;
  }
  fireLoad() {
    this.listeners.get("load")?.({});
  }
  fireError() {
    this.listeners.get("error")?.({});
  }
  fireProgress(loaded: number, total: number) {
    this.upload.listeners.get("progress")?.({ lengthComputable: true, loaded, total });
  }
}

describe("UploadZone", () => {
  beforeEach(() => {
    FakeXHR.instances = [];
    vi.stubGlobal("XMLHttpRequest", FakeXHR);
  });

  it("rejects an invalid file without ever creating an XHR", () => {
    const onError = vi.fn();
    const zone = new UploadZone(document.createElement("div"), { url: "/upload", maxSizeBytes: 10, onError });
    zone.handle(new File([new Uint8Array(100)], "big.bin", { type: "application/octet-stream" }));

    expect(onError).toHaveBeenCalledOnce();
    expect(FakeXHR.instances).toHaveLength(0);
  });

  it("POSTs a valid file to the configured URL", () => {
    const zone = new UploadZone(document.createElement("div"), { url: "/upload" });
    zone.handle(new File(["hello"], "a.txt", { type: "text/plain" }));

    expect(FakeXHR.instances).toHaveLength(1);
    expect(FakeXHR.instances[0]!.method).toBe("POST");
    expect(FakeXHR.instances[0]!.url).toBe("/upload");
  });

  it("reports upload progress as a percentage", () => {
    const onProgress = vi.fn();
    const zone = new UploadZone(document.createElement("div"), { url: "/upload", onProgress });
    zone.handle(new File(["hello"], "a.txt", { type: "text/plain" }));

    FakeXHR.instances[0]!.fireProgress(50, 100);
    expect(onProgress).toHaveBeenCalledWith(50);
  });

  it("calls onSuccess with the response body on a 2xx load", () => {
    const onSuccess = vi.fn();
    const zone = new UploadZone(document.createElement("div"), { url: "/upload", onSuccess });
    zone.handle(new File(["hello"], "a.txt", { type: "text/plain" }));

    const xhr = FakeXHR.instances[0]!;
    xhr.status = 200;
    xhr.responseText = '{"ok":true}';
    xhr.fireLoad();

    expect(onSuccess).toHaveBeenCalledWith('{"ok":true}');
  });

  it("calls onError on a non-2xx load", () => {
    const onError = vi.fn();
    const zone = new UploadZone(document.createElement("div"), { url: "/upload", onError });
    zone.handle(new File(["hello"], "a.txt", { type: "text/plain" }));

    const xhr = FakeXHR.instances[0]!;
    xhr.status = 500;
    xhr.statusText = "Internal Server Error";
    xhr.fireLoad();

    expect(onError).toHaveBeenCalledOnce();
  });

  it("calls onError on a network error", () => {
    const onError = vi.fn();
    const zone = new UploadZone(document.createElement("div"), { url: "/upload", onError });
    zone.handle(new File(["hello"], "a.txt", { type: "text/plain" }));

    FakeXHR.instances[0]!.fireError();
    expect(onError).toHaveBeenCalledOnce();
  });

  it("dragover prevents default and marks the zone as active", () => {
    const zone = new UploadZone(document.createElement("div"), { url: "/upload" });
    const event = new Event("dragover", { cancelable: true });
    zone.el.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    expect(zone.el.classList.contains("fwa-upload-zone-over")).toBe(true);
  });
});
