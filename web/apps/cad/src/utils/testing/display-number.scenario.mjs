import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {formatDisplayNumber}=await server.ssrLoadModule('/src/utils/display-number.ts');
 for(const [value,expected] of [[12.3456,'12.35'],[-12.3456,'-12.35'],[100,'100'],[0.1,'0.1'],[-0,'0'],[-0.001,'0'],[1e-9,'0'],[1000000.125,'1000000.13'],[Infinity,'']])assert.equal(formatDisplayNumber(value),expected);
 assert.equal(formatDisplayNumber(12.34567,8),'12.35');
 const {formatDimensionLabelText}=await server.ssrLoadModule('/src/cad/sketch/dimension-value-format.ts');
 for(const [text,expected] of [['R (12.345)','R (12.35)'],['ΔX -12.345','ΔX -12.35'],['1.234e-3°','0°'],['12.345 mm','12.35 mm'],['width * 1.234','width * 1.234'],['Line1.234','Line1.234']])assert.equal(formatDimensionLabelText(text),expected);
 const {sketchDimensionText}=await server.ssrLoadModule('/src/cad/sketch/sketch-constraint-layout.ts');
 assert.equal(sketchDimensionText({kind:'LENGTH',value:1.23456,unit:'mm',references:[]}), '1.23','artifact labels use exact model value rather than previously rounded server text');
 const {CadNumberInput}=await server.ssrLoadModule('/src/cad/overlay/cad-number-input.tsx');
 const input=CadNumberInput({value:12.3456789});assert.equal(input.props.value,12.3456789,'display formatting preserves full controlled value');
 assert.equal(input.props.formatter(12.3456789,{userTyping:false,input:''}),'12.35');
 assert.equal(input.props.formatter(undefined,{userTyping:true,input:'-.'}),'-.' ,'partial numeric input is not reformatted');
 console.log('Shared two-decimal presentation preserves model values and input drafts');
}finally{await server.close();}
