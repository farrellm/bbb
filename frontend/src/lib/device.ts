// deviceName names a subscribed device from its browser's user agent, as
// a parent would: "iPhone", "Chrome on Mac".
export function deviceName(ua: string): string {
  if (/iPhone|iPod/.test(ua)) return "iPhone";
  if (/iPad/.test(ua)) return "iPad";
  if (/Android/.test(ua)) return "Android phone";
  const os = /Mac OS X|Macintosh/.test(ua)
    ? "Mac"
    : /Windows/.test(ua)
      ? "Windows PC"
      : /CrOS/.test(ua)
        ? "Chromebook"
        : /Linux/.test(ua)
          ? "Linux"
          : "";
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /Chrome\//.test(ua)
        ? "Chrome"
        : /Safari\//.test(ua)
          ? "Safari"
          : "";
  if (os === "") return browser === "" ? "Unknown device" : browser;
  return browser === "" ? os : `${browser} on ${os}`;
}

// thisDevice is how the signed-in device is named in messages: "this
// iPhone", or "this device" when that would read oddly.
export function thisDevice(ua: string): string {
  const name = deviceName(ua);
  return name === "iPhone" || name === "iPad" ? `this ${name}` : "this device";
}
