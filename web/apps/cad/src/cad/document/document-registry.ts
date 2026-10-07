import type {DocumentView,DocumentStructureNode,InstancePath} from '../../types';
import type {WorkbenchCatalog} from '../command/workbench-catalog';

/** A domain supplies its model contract and edit defaults to common navigation. */
export interface DocumentAdapter {
 readonly id:string;
 validate(view:DocumentView):void;
 workingBody(view:DocumentView):string|undefined;
 structure(view:DocumentView):DocumentStructureNode|undefined;
}
export class DocumentRegistry {
 private readonly adapters=new Map<string,DocumentAdapter>();
 constructor(adapters:readonly DocumentAdapter[]=[]){this.registerMany(adapters);}
 registerMany(adapters:readonly DocumentAdapter[]):void {
  const ids=new Set<string>();
  for(const adapter of adapters){if(!adapter.id||ids.has(adapter.id)||this.adapters.has(adapter.id))throw new Error(`Duplicate document adapter: ${adapter.id}`);if(typeof adapter.validate!=="function"||typeof adapter.workingBody!=="function"||typeof adapter.structure!=="function")throw new Error(`Incomplete document adapter: ${adapter.id}`);ids.add(adapter.id);}
  for(const adapter of adapters)this.adapters.set(adapter.id,adapter);
 }
 get(id:string):DocumentAdapter {const adapter=this.adapters.get(id);if(!adapter)throw new Error(`Missing document adapter: ${id}`);return adapter;}
 validate(view:DocumentView):DocumentView {this.get(view.document.type).validate(view);return view;}
 validateCatalog(catalog:WorkbenchCatalog):void {for(const declaration of catalog.documents){const adapter=this.get(declaration.adapter);if(adapter.id!==declaration.id)throw new Error('Document adapter identity mismatch');}}
 target(view:DocumentView,instancePath?:InstancePath){this.validate(view);return {documentId:view.document.id,documentType:view.document.type,instancePath};}
}
const partAdapter:DocumentAdapter={id:'PART',validate(view){if(!view.part||view.product)throw new Error('Invalid Part document projection');},workingBody:view=>view.part?.activeBodyId||undefined,structure:view=>view.structureTree};
const productAdapter:DocumentAdapter={id:'PRODUCT',validate(view){if(!view.product||view.part)throw new Error('Invalid Product document projection');},workingBody:()=>undefined,structure:view=>view.structureTree};
export const documentRegistry=new DocumentRegistry([partAdapter,productAdapter]);
