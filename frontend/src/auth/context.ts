import { createContext } from "react";
import type { User } from "../api/client";

export type AuthValue = {
  user: User | null;
  loading: boolean;
  // signedIn records a user signed in some other way (the iOS handoff).
  signedIn: (u: User) => void;
  signOut: () => Promise<void>;
};

export const AuthContext = createContext<AuthValue | null>(null);
