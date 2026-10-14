import { useSearchParams, Navigate } from "react-router";
import { api } from "../api/client";
import { useAuth } from "../auth/useAuth";
import SignIn from "../components/SignIn";
import { errorMessage } from "../lib/signInErrors";
import { useLoad } from "../lib/useLoad";

export default function Join() {
  const { user } = useAuth();
  const [params] = useSearchParams();
  const invite = params.get("invite");
  const check = useLoad(invite, async () => {
    const { data } = await api.GET("/auth/invites/{code}", {
      params: { path: { code: invite ?? "" } },
    });
    return data?.usable ?? false;
  });

  if (user) return <Navigate to="/" replace />;

  let error = errorMessage(params.get("error"));
  if (error === null && invite === null)
    error = errorMessage("invite_required");
  if (error === null && check.data === false)
    error = errorMessage("invite_invalid");

  return (
    <SignIn
      intro="You've been invited to BBB Watch: what changed on the Big Blue Bulletin, and what to sign up for."
      error={error}
      {...(invite !== null && { invite })}
      disabled={invite === null || check.data !== true}
    />
  );
}
