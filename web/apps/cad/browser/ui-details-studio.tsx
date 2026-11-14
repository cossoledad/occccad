import 'antd/dist/reset.css';
import '../src/styles.css';
import '../src/design/design-system.css';
import {Select} from 'antd';
import {createRoot} from 'react-dom/client';
import {AppProviders} from '../src/app/providers';
import {installVisualTokens} from '../src/design/visual-tokens';
installVisualTokens(document.documentElement);

// Deliberately put the widest option outside the virtual window and near a viewport edge.
const options=Array.from({length:1000},(_,value)=>({value,label:value===999?'末项：选择用于交换与归档的完整装配结构，保留零件与实例的颜色和透明度':`选项 ${value}`}));
const grouped=[{title:'对象',items:[{id:'one',title:'当前'}]},{title:'范围',items:[{id:'two',title:'保留第二侧及全部不相连的区域'}]}];
createRoot(document.getElementById('root')!).render(<AppProviders>
 <main style={{position:'fixed',inset:0,background:'#f3f5f7',padding:24}}>
  <div style={{position:'absolute',right:24,top:80,display:'grid',gap:32}}>
   <Select aria-label="长列表" defaultValue={0} style={{width:96}} options={options}/>
   <Select aria-label="分组选项" defaultValue="one" style={{width:96}} fieldNames={{label:'title',value:'id',options:'items'}} options={grouped}/>
  </div>
 </main>
</AppProviders>);
