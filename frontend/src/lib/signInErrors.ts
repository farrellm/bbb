// Messages for the ?error= codes the sign-in callback redirects with.
export const signInErrors: Record<string, string> = {
  expired: "Sign-in took too long. Please try again.",
  cancelled: "Sign-in was cancelled.",
  email_unverified:
    "Google hasn't verified the email on that account, so it can't be used here.",
  failed: "Something went wrong signing you in. Please try again.",
  handoff_expired:
    "That sign-in expired. Open Pingry BBB Watch from your Home Screen and try again.",
  invite_required:
    "You need an invite link to join. Ask the parent who shared Pingry BBB Watch with you.",
  invite_invalid:
    "That invite link has expired or been used up. Ask the parent who shared Pingry BBB Watch with you for a new one.",
};

export function errorMessage(code: string | null): string | null {
  if (code === null) return null;
  return signInErrors[code] ?? signInErrors.failed ?? null;
}
