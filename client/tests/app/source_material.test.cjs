const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const file=path.resolve(__dirname,'../../assets/scripts/app/SourceMapView.ts');
// 真实3.8.8 Material资产的overridePipelineStates为警告空操作；初始状态只通过copy的overrides生效。
class Material{
 constructor(){this.passes=[];this.illegalRuntimeOverrideCalls=0;this.destroyCalls=0;}
 copy(base,options){this.parentEffect=base.effect;this.passes=[{blendState:options?.states?.blendState??base.passes[0].blendState}];}
 overridePipelineStates(){this.illegalRuntimeOverrideCalls++;}
 destroy(){this.destroyCalls++;}
}
const cc={Material,gfx:{BlendFactor:{SRC_ALPHA:2,ONE:1,ONE_MINUS_SRC_ALPHA:4}}},moduleObject={exports:{}};
new Function('require','module','exports',ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}).outputText)(id=>id==='cc'?cc:require('../../../.work/ui-tests/app/SourceMapModel.js'),moduleObject,moduleObject.exports);
const {SourceMapView}=moduleObject.exports;
test('来源火把在Material.copy初始化时形成真实加色管线，不调用仅对实例生效的运行覆盖',()=>{
 const source={effect:{name:'builtin-sprite'},passes:[{blendState:{targets:[{blend:true,blendSrc:2,blendDst:4}]}}]},view=Object.create(SourceMapView.prototype),sprite={getSharedMaterial:()=>source};
 const additive=view.getAdditiveMaterial(sprite),target=additive.passes[0].blendState.targets[0];
 assert.equal(target.blendSrc,2);assert.equal(target.blendDst,1);assert.equal(target.blendSrcAlpha,1);assert.equal(target.blendDstAlpha,4);
 assert.equal(additive.parentEffect,source.effect);assert.equal(additive.illegalRuntimeOverrideCalls,0);assert.equal(source.passes[0].blendState.targets[0].blendDst,4);
 assert.equal(view.getAdditiveMaterial(sprite),additive);view.destroyMaterial();view.destroyMaterial();assert.equal(additive.destroyCalls,1);
});
