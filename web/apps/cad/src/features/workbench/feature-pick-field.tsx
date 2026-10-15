import { Button } from "antd";
export function FeaturePickField({ label, value, active, disabled, onActivate, onClear, compact=false, selected }: {
    label: string;
    value: string;
    active: boolean;
    disabled?: boolean;
    onActivate: () => void;
    onClear?: () => void;
    compact?: boolean;
    selected?: boolean;
}) {
    if(compact)return <div className="feature-reference-row">
      <span className="feature-reference-detail"><strong>{label}</strong><span title={value}>{value}</span></span>
      <Button size="small" disabled={disabled} aria-label={`选择${label}`} aria-pressed={active} type={active?'primary':'default'} onClick={onActivate}>{active?'选择中':(selected??!!onClear)?'更换':'选择'}</Button>
      {onClear&&<Button size="small" aria-label={`清空${label}`} onClick={onClear}>清空</Button>}
    </div>;
    return <div className="feature-pick-field">
  <Button block disabled={disabled} aria-pressed={active} type={active ? "primary" : "default"} onClick={onActivate}>{label}：{value}</Button>
  {onClear && <Button size="small" onClick={onClear} aria-label={`清空${label}`}>清空</Button>}
 </div>;
}
