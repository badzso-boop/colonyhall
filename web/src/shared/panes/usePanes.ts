import { useCallback, useMemo } from "react";
import { useSearchParams } from "react-router";
import { fromQuery, reduce, toQuery, type PaneAction, type PaneState } from "./panes";

const paneKeys = ["thread", "work", "tab", "focus"];

/**
 * The pane state, kept in the URL (rule 9). Opening a pane adds a history
 * entry, so Back closes it; switching tabs or focus replaces the entry.
 * Other query parameters are left alone.
 */
export function usePanes(): [PaneState, (a: PaneAction) => void] {
  const [params, setParams] = useSearchParams();
  const state = useMemo(() => fromQuery(params), [params]);
  const dispatch = useCallback(
    (a: PaneAction) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          paneKeys.forEach((k) => next.delete(k));
          for (const [k, v] of Object.entries(toQuery(reduce(fromQuery(prev), a)))) next.set(k, v);
          return next;
        },
        { replace: a.type === "selectTab" || a.type === "focus" },
      );
    },
    [setParams],
  );
  return [state, dispatch];
}
