import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import {CadNumberInput} from '../../src/cad/overlay/cad-number-input';

export function mountNumberFixture(maximum?:number) {
 function Fixture() {
  const [value,setValue]=useState<number|null>(12.3456789);
  return <div><CadNumberInput value={value} max={maximum} aria-label="精确值" onChange={setValue}/>
   <output data-testid="canonical-value">{String(value)}</output><button>移出焦点</button></div>;
 }
 const host=document.createElement('div');document.body.appendChild(host);
 createRoot(host).render(<Fixture/>);
}
