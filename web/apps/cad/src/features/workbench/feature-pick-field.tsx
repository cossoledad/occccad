import { Button } from "antd";
export function FeaturePickField({ label, value, active, disabled, onActivate, onClear }: {
    label: string;
    value: string;
    active: boolean;
    disabled?: boolean;
    onActivate: () => void;
    onClear?: () => void;
}) {
    return <div className="feature-pick-field">
  <Button block disabled={disabled} aria-pressed={active} type={active ? "primary" : "default"} onClick={onActivate}>{label}：{value}</Button>
  {onClear && <Button size="small" onClick={onClear} aria-label={`清空${label}`}>清空</Button>}
 </div>;
}
