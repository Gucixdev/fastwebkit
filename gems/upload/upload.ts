// Drag-and-drop (+ click-to-browse) file upload: validates client-side before ever sending a
// byte, then posts via XMLHttpRequest (not fetch - fetch still has no upload-progress event,
// XHR does) so a caller can wire the ProgressBar gem to real bytes-sent, not a guess.
export interface ValidateOptions {
  maxSizeBytes?: number;
  accept?: string[]; // MIME types or "image/*"-style prefixes
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let value = bytes;
  let unitIndex = -1;
  do {
    value /= 1024;
    unitIndex++;
  } while (value >= 1024 && unitIndex < units.length - 1);
  return `${value.toFixed(1)} ${units[unitIndex]}`;
}

function acceptMatches(mime: string, pattern: string): boolean {
  if (pattern.endsWith("/*")) return mime.startsWith(pattern.slice(0, -1));
  return mime === pattern;
}

// Pure: returns an error message, or null if the file passes every configured check.
export function validateFile(file: { name: string; size: number; type: string }, options: ValidateOptions = {}): string | null {
  if (options.maxSizeBytes !== undefined && file.size > options.maxSizeBytes) {
    return `${file.name} is ${formatBytes(file.size)}, which is over the ${formatBytes(options.maxSizeBytes)} limit`;
  }
  if (options.accept && options.accept.length > 0 && !options.accept.some((p) => acceptMatches(file.type, p))) {
    return `${file.name} has type "${file.type}", which isn't accepted (${options.accept.join(", ")})`;
  }
  return null;
}

export interface UploadOptions extends ValidateOptions {
  url: string;
  fieldName?: string;
  onProgress?: (percent: number) => void;
  onError?: (message: string) => void;
  onSuccess?: (response: string) => void;
}

export class UploadZone {
  readonly el: HTMLElement;
  private readonly input: HTMLInputElement;

  constructor(container: HTMLElement, private readonly options: UploadOptions) {
    this.el = document.createElement("div");
    this.el.className = "fwa-upload-zone";
    this.el.textContent = "Drop a file here, or click to choose one";

    this.input = document.createElement("input");
    this.input.type = "file";
    this.input.style.display = "none";
    this.el.append(this.input);

    this.el.addEventListener("click", () => this.input.click());
    this.input.addEventListener("change", () => {
      const file = this.input.files?.[0];
      if (file) this.handle(file);
    });

    this.el.addEventListener("dragover", (e) => {
      e.preventDefault();
      this.el.classList.add("fwa-upload-zone-over");
    });
    this.el.addEventListener("dragleave", () => this.el.classList.remove("fwa-upload-zone-over"));
    this.el.addEventListener("drop", (e) => {
      e.preventDefault();
      this.el.classList.remove("fwa-upload-zone-over");
      const file = e.dataTransfer?.files?.[0];
      if (file) this.handle(file);
    });

    container.append(this.el);
  }

  handle(file: File): void {
    const error = validateFile(file, this.options);
    if (error) {
      this.options.onError?.(error);
      return;
    }

    const form = new FormData();
    form.append(this.options.fieldName ?? "file", file);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", this.options.url);
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) this.options.onProgress?.(Math.round((e.loaded / e.total) * 100));
    });
    xhr.addEventListener("load", () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        this.options.onSuccess?.(xhr.responseText);
      } else {
        this.options.onError?.(`upload failed: ${xhr.status} ${xhr.statusText}`);
      }
    });
    xhr.addEventListener("error", () => this.options.onError?.("upload failed: network error"));
    xhr.send(form);
  }
}
