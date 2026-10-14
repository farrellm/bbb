declare global {
  interface Navigator {
    // Safari's own flag for a home-screen web app.
    standalone?: boolean;
  }
}

// isStandalone is true when running as an installed home-screen app,
// which on iOS has its own cookie store, separate from Safari's.
export function isStandalone(): boolean {
  return (
    navigator.standalone === true ||
    window.matchMedia("(display-mode: standalone)").matches
  );
}
