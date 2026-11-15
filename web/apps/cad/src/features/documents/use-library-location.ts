import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigationType } from "react-router-dom";
import { useAppNavigation, useHistoryTraversal } from "../../app/navigation";
import { readLibraryLocation, writeLibraryLocation, type LibraryLocation } from "./library-location";

export function useLibraryLocation() {
  const location = useLocation(), navigate = useAppNavigation();
  const navigationType = useNavigationType();
  const value = useMemo(() => readLibraryLocation(location.search), [location.search]);
  // Draft belongs to a history visit. Back/Forward restores synchronously and
  // cannot let a previous visit's delayed search overwrite the destination.
  const [draft, setDraft] = useState<{key: string; value: string}>();
  const pending = useRef<number | undefined>(undefined);
  useHistoryTraversal(() => { window.clearTimeout(pending.current); setDraft(undefined); });
  const query = draft?.key === location.key ? draft.value : value.search;
  const update = (patch: Partial<LibraryLocation>, replace = true) => {
    void navigate({ pathname: "/", search: writeLibraryLocation(location.search, patch) }, { replace });
  };
  useEffect(() => {
    if (query.trim() === value.search) return;
    const timer = window.setTimeout(() => update({ search: query.trim(), offset: 0 }), 250);
    pending.current = timer;
    return () => window.clearTimeout(timer);
  }, [query, value.search, location.key]);
  return { ...value, query, setQuery: (value: string) => setDraft({ key: location.key, value }), update, visitKey: location.key, navigationType };
}
