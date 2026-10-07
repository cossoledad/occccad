import {builtinCatalog, projectToolbars} from "../cad/command/workbench-catalog";
import type {ToolbarCatalog} from "../types";
export const mockToolbarCatalog:ToolbarCatalog={...builtinCatalog,toolbars:projectToolbars(builtinCatalog)};
