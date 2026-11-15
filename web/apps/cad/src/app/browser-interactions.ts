const editable = "input,textarea,select,[contenteditable]:not([contenteditable='false']),[role='textbox']";
const copyable = ".ant-modal-body,.ant-drawer-body,[data-text-selectable],.inspector-object-data,.measurement-results dd,.measurement-coordinate output,.automation-debug-observations";

/** Installed once by AppProviders, including React portals. Native editing and
 * copyable output keep their context menu; application menus still bubble. */
export function installBrowserInteractions(root: Document): () => void {
  const element = (target: EventTarget | null) => target instanceof Element ? target : target instanceof Node ? target.parentElement : null;
  const allowText = (target: EventTarget | null) => Boolean(element(target)?.closest(`${editable},${copyable}`));
  const contextMenu = (event: MouseEvent) => {
    if (!allowText(event.target)) event.preventDefault();
  };
  const selectStart = (event: Event) => {
    if (!allowText(event.target)) event.preventDefault();
  };
  root.addEventListener("contextmenu", contextMenu, true);
  root.addEventListener("selectstart", selectStart, true);
  return () => {
    root.removeEventListener("contextmenu", contextMenu, true);
    root.removeEventListener("selectstart", selectStart, true);
  };
}
