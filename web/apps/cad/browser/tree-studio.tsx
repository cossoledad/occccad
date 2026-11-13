import 'antd/dist/reset.css';
import '../src/styles.css';
import '../src/design/design-system.css';
import '../src/features/workbench/workbench.css';
import {AppProviders} from '../src/app/providers';
import {installVisualTokens} from '../src/design/visual-tokens';
installVisualTokens(document.documentElement);
import {Profiler,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {SpecificationTree,type SpecificationTreeNode} from '../src/features/workbench/specification-tree';
import {CadIcon} from '../src/cad/overlay/cad-icons';

// A declared UI microbenchmark, not a business model or assembly-capacity test.
const nodes:SpecificationTreeNode[]=[{key:'root',kind:'PART',title:'曲面设计示例',icon:<CadIcon name="part"/>,children:Array.from({length:5000},(_,i)=>({key:`feature-${i}`,kind:'FEATURE',entityId:`feature-${i}`,title:`曲面特征 ${String(i+1).padStart(4,'0')}`,icon:<CadIcon name="surface"/>,capabilities:['EDIT','DELETE']}))}];
const samples:number[]=[];
function Harness(){
 const [highlighted,setHighlighted]=useState<string>(),[selected,setSelected]=useState<string[]>([]);
 Object.assign(window,{treeHarness:{samples,highlight:(i:number)=>setHighlighted(`feature-${i}`)}});
 return <AppProviders><main className="workbench-content light-scene" style={{position:'fixed',inset:0,display:'block',background:'linear-gradient(#dce1e5,#f0f2f4)'}}>
  <aside className="workbench-structure" style={{position:'absolute',inset:'0 auto 0 0',width:330}}>
   <Profiler id="structure-tree" onRender={(_id,_phase,duration)=>samples.push(duration)}>
    <SpecificationTree nodes={nodes} selectedKeys={selected} selectionToken={selected.join(',')} highlightedKey={highlighted} onSelect={value=>setSelected(value.map(n=>n.key))}/>
   </Profiler>
  </aside>
 </main></AppProviders>;
}
createRoot(document.getElementById('root')!).render(<Harness/>);
