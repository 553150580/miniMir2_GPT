const assert=require('node:assert/strict');
// rule19仅移动Settings原Help，为追加保存策略区留位置；历史HUD哈希只还原这两项显式授权变化。
module.exports=function restoreHistoricalSettingsHelp(scene){
 if(scene.length<=436)return scene;
 assert.equal(scene[435]._name,'Help');assert.equal(scene[435]._parent.__id__,425);
 assert.deepEqual(scene[435]._lpos,{__type__:'cc.Vec3',x:0,y:-385,z:0});assert.deepEqual(scene[436]._contentSize,{__type__:'cc.Size',width:220,height:36});
 scene[435]._lpos.y=0;scene[436]._contentSize.width=330;scene[436]._contentSize.height=60;return scene;
};

// 只在完整保存场景的副本上投影本轮唯一授权View改动；旧HUD整段哈希继续保护其它字段。
module.exports.normalizeInventoryViewport=function normalizeInventoryViewport(scene){
 assert(scene.length>=1191);const view=scene[270],viewSize=scene[271],content=scene[273],contentSize=scene[274];
 assert.equal(view._name,'View');assert.equal(view._parent.__id__,267);
 assert.deepEqual(view._lpos,{__type__:'cc.Vec3',x:0,y:28,z:0});assert.deepEqual(view._components,[{__id__:271},{__id__:272},{__id__:1190}]);
 assert.equal(viewSize.node.__id__,270);assert.deepEqual(viewSize._contentSize,{__type__:'cc.Size',width:600,height:414});
 assert.equal(content._name,'Content');assert.equal(content._parent.__id__,270);assert.deepEqual(content._lpos,{__type__:'cc.Vec3',x:0,y:207,z:0});
 assert.equal(contentSize.node.__id__,273);assert.deepEqual(contentSize._contentSize,{__type__:'cc.Size',width:600,height:414});
 const widgets=scene.filter(row=>row.__type__==='cc.Widget'&&row.node?.__id__===270);assert.equal(widgets.length,1);assert.equal(widgets[0],scene[1190]);
 assert.deepEqual(scene[1190],{__type__:'cc.Widget',_name:'',_objFlags:0,node:{__id__:270},_enabled:true,__prefab:null,_id:'',_alignFlags:45,_target:null,
  _left:0,_right:0,_top:0,_bottom:56,_horizontalCenter:0,_verticalCenter:0,_isAbsLeft:true,_isAbsRight:true,_isAbsTop:true,_isAbsBottom:true,_isAbsHorizontalCenter:true,_isAbsVerticalCenter:true,_alignMode:2});
 view._lpos.y=0;view._components=[{__id__:271},{__id__:272}];viewSize._contentSize.height=470;content._lpos.y=235;contentSize._contentSize.height=470;
 const frozen={270:'cdc5674d0c9453c88b1cd837269ab19d0d07457d1761cdfec217cce8a624bd07',271:'bdc3868a51d393fe7ca970f38480921676179adc9d1bbcddf6a08b40370b4f1e',273:'ece137d35dfad9a451de96dd328cc7e5ebd320eba59564660dd41c1e4bb28ddb',274:'9321af6c76c1affc2a8bf92cee1006079a5a590b31f55da1a6f1a51edae6e061'};
 for(const [index,hash]of Object.entries(frozen))assert.equal(require('node:crypto').createHash('sha256').update(JSON.stringify(scene[index])).digest('hex'),hash,'View四个旧对象含未授权字段变化');
 return scene;
};

// 后续确认窗仅启用正文换行；历史前缀先验证当前合同，再还原这一明确差异。
module.exports.normalizeConfirmTextWrap=function normalizeConfirmTextWrap(scene){
 assert.equal(scene[481]._name,'Message');assert.equal(scene[481]._parent.__id__,474);
 assert.equal(scene[474]._name,'Confirm');assert.equal(scene[483].__type__,'cc.Label');
 assert.equal(scene[483].node.__id__,481);assert.equal(scene[483]._enableWrapText,true);
 assert.equal(scene[483]._fontSize,24);assert.equal(scene[483]._lineHeight,30);assert.equal(scene[483]._overflow,1);
 scene[483]._enableWrapText=false;return scene;
};
