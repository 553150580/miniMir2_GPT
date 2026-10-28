const fs=require('node:fs');
const path=require('node:path');
const {createRequire}=require('node:module');
const root=path.resolve(__dirname,'../../../..');
const ts=require(path.join(root,'tools/client/creator_paths.cjs')).typescript;
const Long=createRequire(path.join(root,'client/assets/scripts/app/GameSession.ts'))('long');
class ProtocolResponseError extends Error{constructor(messageId,code){super('明确协议拒绝');this.messageId=messageId;this.code=code;}}
function load(file,requireModule){const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,experimentalDecorators:true,esModuleInterop:true}});const module={exports:{}};new Function('require','module','exports',code.outputText)(requireModule,module,module.exports);return module.exports;}
function harness(){
    let operation=0;
    class Connection{constructor(){this.generation=1;this.binding=Long.UONE;this.connected=true;this.sent=[];}subscribe(listener){this.listener=listener;}onStatus(listener){this.status=message=>{this.generation++;this.connected=false;listener(message);};}deliver(id,value,extra={}){this.listener({id,value,error:0,binding:this.binding,...extra});}send(id,value){this.sent.push({id,value});}}
    const result=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?{GameConnection:Connection,newOperationId:()=>new Uint8Array(16).fill(++operation)}
        :id==='./BaselineAssembler'?{BaselineAssembler:class{reset(){}accept(value){return value;}}}:id==='./GameErrors'?{protocolErrorMessage:()=> '协议错误'}
        :id==='./TeleportModel'?require(path.join(root,'.work/ui-tests/app/TeleportModel.js')):id==='./RoleBindingModel'?require(path.join(root,'.work/ui-tests/app/RoleBindingModel.js')):id==='./MapTravelModel'?require(path.join(root,'.work/ui-tests/app/MapTravelModel.js')):id==='./SkillModel'?require(path.join(root,'.work/ui-tests/app/SkillModel.js')):{});
    result.session.role={job:0,name:'测试角色',key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(9)}};
    return result;
}
function entry(scene='9007199254740993',map='1',sceneKind=1,command='1'){return {sceneEpoch:Long.fromString(scene,true),baselineId:Long.fromString(scene,true),roomId:Long.fromString(map,true),sceneKind,resourceVersion:Long.fromInt(13,true),playerHandle:7,nextEconomicCommandSeq:Long.fromString(command,true)};}
function ready(h,value){h.session.connection.deliver(20002,value);h.session.connection.deliver(6,value);return value;}
function context(h){const i=h.currentEntryIdentity();return {...i,life:'0',rule:h.session.entry.resourceVersion};}
module.exports={root,Long,load,harness,entry,ready,context,ProtocolResponseError};
