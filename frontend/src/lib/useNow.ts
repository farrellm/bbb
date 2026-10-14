import { useEffect, useState } from "react";

// useNow is the current time, refreshed every interval so relative times
// ("4 min ago") stay true while the app is open.
export function useNow(intervalMs = 60_000): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

// useOnReturn calls fn when the app comes back to the foreground, which is
// how a home-screen app is "reopened" on iOS.
export function useOnReturn(fn: () => void): void {
  useEffect(() => {
    const onChange = () => {
      if (document.visibilityState === "visible") fn();
    };
    document.addEventListener("visibilitychange", onChange);
    return () => document.removeEventListener("visibilitychange", onChange);
  }, [fn]);
}
