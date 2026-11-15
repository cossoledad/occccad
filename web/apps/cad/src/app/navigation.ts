import { useCallback, useEffect, useRef } from "react";
import { createPath, resolvePath, useLocation, useNavigate, type NavigateFunction, type NavigateOptions, type To } from "react-router-dom";

/** One browser history, shared by links, tabs and native Back/Forward.
 * Repeating an absolute destination is not another visit. No mouse-button
 * translation: browsers may navigate without dispatching a page mouse event. */
export function useAppNavigation(): NavigateFunction {
  const location = useLocation(), navigate = useNavigate();
  const current = useRef({ location, navigate });
  current.current = { location, navigate };
  return useCallback(((to, options) => {
    const { location, navigate } = current.current;
    if (typeof to === "number") return navigate(to);
    if (createPath(resolvePath(to, location.pathname)) === createPath(location)) return;
    return navigate(to, options);
  }) as NavigateFunction, []);
}

/** Application restoration metadata replaces this visit through the router;
 * never overwrite its history.state (idx/key/usr) with raw replaceState. */
export function useReplaceRouteQuery() {
  const location = useLocation(), navigate = useAppNavigation();
  const current = useRef(location); current.current = location;
  return useCallback((update: (params: URLSearchParams) => void) => {
    const location = current.current, params = new URLSearchParams(location.search);
    update(params);
    const search = params.toString();
    void navigate({ pathname: location.pathname, search: search ? `?${search}` : "", hash: location.hash },
      { replace: true, state: location.state });
  }, [navigate]);
}

/** Discard deferred UI work immediately on a native history traversal. A POP
 * can return to a key that was visited before React committed the intervening
 * navigation; comparing location keys alone cannot reject that stale draft. */
export function useHistoryTraversal(discard: () => void) {
  const latest = useRef(discard); latest.current = discard;
  useEffect(() => {
    const onPop = () => latest.current();
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
}

/** An asynchronous document operation may update its cache after departure,
 * but its completion cannot redirect a different browser history visit. */
export function useNavigationIntent() {
  const location=useLocation(),navigate=useAppNavigation(),current=useRef(location),alive=useRef(true);
  current.current=location;
  useEffect(()=>{alive.current=true;return()=>{alive.current=false}},[]);
  return useCallback(()=>{
    const key=current.current.key;
    const isCurrent=()=>alive.current&&current.current.key===key&&(window.history.state?.key??"default")===key;
    return {isCurrent,navigate:(to:To,options?:NavigateOptions)=>{if(isCurrent())return navigate(to,options)}};
  },[navigate]);
}
