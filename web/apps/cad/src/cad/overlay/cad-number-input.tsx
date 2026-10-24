import { InputNumber, type InputNumberProps } from "antd";
import {useRef} from "react";
import { formatDisplayNumber } from "../../utils/display-number";

// A new formatter identity makes Antd reformat even the active input draft.
const displayFormatter = (value:number|string|undefined,info:{userTyping:boolean;input:string}) =>
  info.userTyping?info.input:value===undefined||value===""?"":formatDisplayNumber(Number(value));

/** Keep the underlying numeric value and the user's in-progress text intact. */
export function CadNumberInput<T extends number|string=number>(props:InputNumberProps<T>) {
  const preserved = useRef<T|undefined>(undefined);
  const preserveDisplay = (target:EventTarget) => {
    // Antd reparses the displayed text on blur/Enter. An untouched rounded
    // display is not a new numeric intent; ordinary edits still use its parser.
    const input = target as HTMLInputElement;
    if (!props.formatter && !props.parser && props.precision===undefined && props.value!==undefined && props.value!==null &&
        (props.min===undefined || Number(props.value)>=Number(props.min)) &&
        (props.max===undefined || Number(props.value)<=Number(props.max)) &&
        input.value===formatDisplayNumber(Number(props.value)) && Number(input.value)!==Number(props.value)) {
      preserved.current=props.value;
    }
  };
  return <InputNumber<T> {...props} formatter={props.formatter??displayFormatter}
    onBlurCapture={event=>{preserveDisplay(event.target);props.onBlurCapture?.(event);}}
    onInputCapture={event=>{preserved.current=undefined;props.onInputCapture?.(event);}}
    onFocusCapture={event=>{preserved.current=undefined;props.onFocusCapture?.(event);}}
    onKeyDownCapture={event=>{preserved.current=undefined;if(event.key==='Enter'&&(event.target as HTMLElement).tagName==='INPUT'){
      preserveDisplay(event.target);
      const text=(event.target as HTMLInputElement).value;
      const parsed=props.parser?props.parser(text):text.trim();
      if(parsed===''||!Number.isFinite(Number(parsed))||(props.min!==undefined&&Number(parsed)<Number(props.min))||(props.max!==undefined&&Number(parsed)>Number(props.max))){event.preventDefault();event.stopPropagation();}
    }props.onKeyDownCapture?.(event);}}
    onChange={value=>{const original=preserved.current;preserved.current=undefined;
      if(original!==undefined && value!==null && Number(value)===Number(formatDisplayNumber(Number(original))))return;
      props.onChange?.(value);}}/>;
}
