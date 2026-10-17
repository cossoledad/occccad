import type {CadCommand} from "../../cad/command/command-registry";
export const APP_OPTIONS_COMMAND="app.options";
export const globalOptionsCommand=(open?:()=>void):CadCommand=>({id:APP_OPTIONS_COMMAND,execute:()=>open?.(),isEnabled:()=>Boolean(open)});
