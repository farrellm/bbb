import { useCallback, useEffect, useRef, useState } from "react";

type State<T> =
  | { key: string; generation: number; status: "ok"; data: T }
  | { key: string; generation: number; status: "error"; error: Error };

export type Load<T> = {
  data: T | undefined;
  error: Error | undefined;
  loading: boolean;
  reload: () => void;
};

// useLoad runs load whenever key changes (null skips it) and on reload().
// While a new key loads, data stays the last result so the screen doesn't
// blank; loading says whether it is current.
export function useLoad<T>(
  key: string | null,
  load: () => Promise<T>,
): Load<T> {
  const loadRef = useRef(load);
  useEffect(() => {
    loadRef.current = load;
  });
  const [state, setState] = useState<State<T>>();
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    if (key === null) return;
    let live = true;
    loadRef.current().then(
      (data) => {
        if (live) setState({ key, generation, status: "ok", data });
      },
      (err: unknown) => {
        if (live)
          setState({
            key,
            generation,
            status: "error",
            error: err instanceof Error ? err : new Error(String(err)),
          });
      },
    );
    return () => {
      live = false;
    };
  }, [key, generation]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return {
    data: state?.status === "ok" ? state.data : undefined,
    error:
      state?.status === "error" && state.key === key ? state.error : undefined,
    loading: key !== null && state?.key !== key,
    reload,
  };
}
