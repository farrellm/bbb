import { describe, expect, it } from "vitest";
import { decodeKey, sameKey } from "./pushKeys";

describe("decodeKey", () => {
  it("reads unpadded base64url", () => {
    expect([...decodeKey("_-8")]).toEqual([0xff, 0xef]);
    expect([...decodeKey("AQID")]).toEqual([1, 2, 3]);
  });
});

describe("sameKey", () => {
  it("compares the subscribed key's bytes", () => {
    const key = new Uint8Array([1, 2, 3]).buffer;
    expect(sameKey(key, "AQID")).toBe(true);
    expect(sameKey(key, "AQIE")).toBe(false);
    expect(sameKey(null, "AQID")).toBe(false);
  });
});
