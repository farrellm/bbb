import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { api, type User } from "../api/client";
import { AuthContext } from "./context";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // /me also renews the session cookie, so every launch extends it.
    api
      .GET("/me")
      .then(({ data }) => setUser(data ?? null))
      .catch((err: unknown) => {
        console.warn("me", err);
        setUser(null);
      })
      .finally(() => setLoading(false));
  }, []);

  const signOut = useCallback(async () => {
    await api.POST("/auth/logout");
    setUser(null);
  }, []);

  const value = useMemo(
    () => ({ user, loading, signedIn: setUser, signOut }),
    [user, loading, signOut],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
