import { describe, it, expect } from "vitest";
import { crc32, zipStore, unzipStore } from "./export_zip";

describe("crc32", () => {
  it("matches the well-known CRC32 of the ASCII string 'hello'", () => {
    // 0x3610a686 is the textbook CRC32("hello") - any correct implementation must land here.
    expect(crc32(new TextEncoder().encode("hello"))).toBe(0x3610a686);
  });

  it("is 0 for an empty buffer... actually CRC32('') is 0", () => {
    expect(crc32(new Uint8Array())).toBe(0);
  });
});

describe("zipStore / unzipStore round-trip", () => {
  it("round-trips a single small text file", () => {
    const entries = [{ name: "hello.txt", data: new TextEncoder().encode("hello world") }];
    const zipped = zipStore(entries);
    const back = unzipStore(zipped);

    expect(back).toHaveLength(1);
    expect(back[0]!.name).toBe("hello.txt");
    expect(new TextDecoder().decode(back[0]!.data)).toBe("hello world");
  });

  it("round-trips multiple files with nested paths, preserving order and bytes", () => {
    const entries = [
      { name: "[Content_Types].xml", data: new TextEncoder().encode("<a/>") },
      { name: "xl/worksheets/sheet1.xml", data: new TextEncoder().encode("<b/>") },
    ];
    const back = unzipStore(zipStore(entries));

    expect(back.map((e) => e.name)).toEqual(["[Content_Types].xml", "xl/worksheets/sheet1.xml"]);
    expect(new TextDecoder().decode(back[1]!.data)).toBe("<b/>");
  });

  it("round-trips binary (non-UTF8-safe) bytes exactly", () => {
    const data = new Uint8Array([0, 1, 2, 255, 254, 253, 128]);
    const back = unzipStore(zipStore([{ name: "bin", data }]));
    expect(back[0]!.data).toEqual(data);
  });

  it("starts the output with the local file header signature", () => {
    const zipped = zipStore([{ name: "x", data: new Uint8Array([1]) }]);
    const view = new DataView(zipped.buffer, zipped.byteOffset, 4);
    expect(view.getUint32(0, true)).toBe(0x04034b50);
  });

  it("produces an empty-but-valid archive for zero entries", () => {
    expect(unzipStore(zipStore([]))).toEqual([]);
  });
});
