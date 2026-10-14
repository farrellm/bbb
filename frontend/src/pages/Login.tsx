import { Navigate, useSearchParams } from "react-router";
import { useAuth } from "../auth/useAuth";
import SignIn from "../components/SignIn";
import { errorMessage } from "../lib/signInErrors";

export default function Login() {
  const { user } = useAuth();
  const [params] = useSearchParams();
  const next = params.get("next") ?? undefined;
  if (user) return <Navigate to={next ?? "/"} replace />;
  return (
    <SignIn
      intro="What changed on the Big Blue Bulletin, and what to sign up for."
      error={errorMessage(params.get("error"))}
      {...(next !== undefined && { next })}
    />
  );
}
