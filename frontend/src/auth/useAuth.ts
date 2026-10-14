import { useContext } from "react";
import { AuthContext, type AuthValue } from "./context";

export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}
