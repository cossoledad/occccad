import source from "../../../../../../services/internal/workbenchconfig/icons.json";
import { compileIconCatalog, type IconSource } from "./icon-catalog";

/** Shared immutable artwork; viewport cursors need no command catalog/runtime. */
export const builtinIcons = compileIconCatalog(source as unknown as IconSource);
