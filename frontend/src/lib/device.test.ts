import { describe, expect, it } from "vitest";
import { deviceName, thisDevice } from "./device";

const iphone =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1";
const macChrome =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36";
const winEdge =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0";
const linuxFirefox =
  "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0";

describe("deviceName", () => {
  it("names phones by kind and computers by browser and OS", () => {
    expect(deviceName(iphone)).toBe("iPhone");
    expect(deviceName(macChrome)).toBe("Chrome on Mac");
    expect(deviceName(winEdge)).toBe("Edge on Windows PC");
    expect(deviceName(linuxFirefox)).toBe("Firefox on Linux");
    expect(deviceName("")).toBe("Unknown device");
  });
});

describe("thisDevice", () => {
  it("says iPhone where it can", () => {
    expect(thisDevice(iphone)).toBe("this iPhone");
    expect(thisDevice(macChrome)).toBe("this device");
  });
});
