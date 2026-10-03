import { InputNumber, type InputNumberProps } from "antd";
import { formatDisplayNumber } from "../../utils/display-number";

/** Keep the underlying numeric value and the user's in-progress text intact. */
export function CadNumberInput<T extends number|string=number>(props:InputNumberProps<T>) {
  return <InputNumber<T> {...props} formatter={props.formatter??((value,info)=>info.userTyping?info.input:value===undefined||value===""?"":formatDisplayNumber(Number(value)))}/>;
}
