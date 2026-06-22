"use client";
import { useEffect, useRef } from "react";

// usePoll calls fn on an interval so live views (instance online state, link
// status, file scan progress) reflect changes within seconds (UI §4.5 / SLO ≤5s).
// Polling pauses while the tab is hidden to avoid wasted requests.
export function usePoll(fn: () => void, ms = 5000) {
  const saved = useRef(fn);
  saved.current = fn;
  useEffect(() => {
    const tick = () => {
      if (typeof document === "undefined" || !document.hidden) saved.current();
    };
    const id = setInterval(tick, ms);
    return () => clearInterval(id);
  }, [ms]);
}
