// 真实PB/HTTP/WS恢复探针；凭据、票据及原操作正文只存于本进程内存。
'use strict';
const assert=require('node:assert/strict');
const crypto=require('node:crypto');
const fs=require('node:fs');
const path=require('node:path');
const readline=require('node:readline');
const root=path.resolve(__dirname,'../..');
const pb=require(path.join(root,'client/assets/scripts/generated/protobuf/client_pb.js')).minimir.v1;
const Long=require(path.join(root,'client/node_modules/long'));
const manifest=JSON.parse(fs.readFileSync(path.join(root,'shared/proto/protocol_manifest.json'),'utf8'));
const ids=Object.fromEntries([...manifest.messages,...manifest.error_codes].map(x=>[x.name,x.value]));
const responseCodecs=Object.fromEntries(manifest.messages.map(x=>[x.value,pb[x.proto_message]]));
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const unsigned=value=>Long.fromString(String(value),true);
const id=()=>crypto.randomBytes(16);
function variable(value){let n=BigInt(value),out=[];do{let b=Number(n&127n);n>>=7n;out.push(b|(n?128:0));}while(n);return Buffer.from(out);}
function encode(msg,request,binding,value){return Buffer.concat([Buffer.from([1,0,msg>>8,msg&255]),variable(request),variable(binding),Buffer.from(responseCodecs[msg].encode(responseCodecs[msg].create(value)).finish())]);}
function decode(bytes){const raw=Buffer.from(bytes);assert(raw.length>=6&&raw[0]===1&&!(raw[1]&~1),'应用帧头不合法');let offset=4;function next(){let n=0n,s=0n;for(let i=0;i<10;i++){assert(offset<raw.length,'应用帧截断');const b=raw[offset++];n|=BigInt(b&127)<<s;if(!(b&128))return n;s+=7n;}throw Error('应用帧整数溢出');}
 const message=raw.readUInt16BE(2),request=Number(next()),binding=next(),error=raw[1]&1?Number(next()):0;
 return{message,request,binding,error,value:error?null:responseCodecs[message]?.decode(raw.subarray(offset))};}
async function until(predicate,label,timeout=15000){const end=Date.now()+timeout;do{if(predicate())return;await sleep(20);}while(Date.now()<end);throw Error('等待超时：'+label);}
class Session{
 constructor(url){this.url=url;this.request=0;this.binding=0n;this.frames=[];this.chunks=new Map();this.entities=[];this.closed=false;}
 async connect(){this.ws=new WebSocket(this.url);this.ws.binaryType='arraybuffer';this.ws.addEventListener('message',event=>{try{this.receive(decode(event.data));}catch(error){this.failure=error.message;}});this.ws.addEventListener('close',()=>{this.closed=true;});this.ws.addEventListener('error',()=>{this.failure='WS连接错误';});await until(()=>this.ws.readyState===WebSocket.OPEN||this.failure,'WS打开');assert(!this.failure,this.failure);}
 receive(frame){frame.ordinal=(this.ordinal||0)+1;this.ordinal=frame.ordinal;this.frames.push(frame);if(this.frames.length>2048)this.frames.shift();if(frame.error)return;
  this.binding=frame.binding;
  if(frame.message===ids.MsgS2CEnterLine)this.entry=frame.value;
  if(frame.message===ids.MsgS2CSceneBaseline){const chunk=frame.value,key=[chunk.scene_epoch,chunk.baseline_id,chunk.state_sequence].map(String).join(':');if(this.chunkKey!==key){this.chunkKey=key;this.chunks.clear();}this.chunks.set(chunk.chunk_index,chunk);if(this.chunks.size===chunk.chunk_count){this.entities=Array.from(this.chunks.values()).sort((a,b)=>a.chunk_index-b.chunk_index).flatMap(x=>x.entities);this.baseline=chunk;this.baselineOrdinal=frame.ordinal;}}
  if(frame.message===ids.MsgS2CInventory){this.inventory=frame.value;this.inventoryOrdinal=frame.ordinal;}
  if(frame.message===ids.MsgS2CWallet){this.wallet=frame.value;this.walletOrdinal=frame.ordinal;}
  if(frame.message===ids.MsgS2CHostingState){this.hosting=frame.value;this.hostingOrdinal=frame.ordinal;}
  if(frame.message===ids.MsgS2CSyncReady)this.ready=frame.value;
 }
 send(message,value){assert(!this.closed,'WS已关闭');const request=++this.request;this.ws.send(encode(message,request,this.binding,value));return request;}
 async result(message,request){await until(()=>this.failure||this.frames.some(x=>x.message===message&&x.request===request),'协议回执 '+message);assert(!this.failure,this.failure);return this.frames.find(x=>x.message===message&&x.request===request);}
 async enter(ticket){await this.connect();const request=this.send(ids.MsgC2SEnterLine,{account_ticket:ticket}),frame=await this.result(ids.MsgS2CEnterLine,request);assert.equal(frame.error,0,'入场错误码');
  await until(()=>this.baseline&&String(this.baseline.scene_epoch)===String(this.entry.scene_epoch)&&String(this.baseline.baseline_id)===String(this.entry.baseline_id),'ACK前完整基线');assert(this.entities.length>0,'首基线没有实体');assert(!this.inventory&&!this.wallet,'ACK前提前观察资产');
  this.initialBody={...this.body(),hp_max:String(this.player().hp_max),mp_max:String(this.player().mp_max)};
  this.send(ids.MsgC2SEnterWorldAck,{scene_epoch:this.entry.scene_epoch,baseline_id:unsigned(BigInt(String(this.entry.baseline_id))+1n)});
  const premature=this.send(ids.MsgC2SMoveIntent,{scene_epoch:this.entry.scene_epoch,input_sequence:1,direction:2});assert.equal((await this.result(ids.MsgC2SMoveIntent,premature)).error,ids.CodeLineRecovering,'错ACK没有保持入场屏障');assert(!this.inventory&&!this.wallet&&!this.hosting&&!this.ready,'错ACK提前激活资产或身体');
  const ack=this.send(ids.MsgC2SEnterWorldAck,{scene_epoch:this.entry.scene_epoch,baseline_id:this.entry.baseline_id});assert.equal((await this.result(ids.MsgS2CSyncReady,ack)).error,0);await until(()=>this.inventory&&this.wallet&&this.hosting,'ACK后资产');
  this.send(ids.MsgC2SEnterWorldAck,{scene_epoch:this.entry.scene_epoch,baseline_id:this.entry.baseline_id});this.send(ids.MsgC2SEnterWorldAck,{scene_epoch:this.entry.scene_epoch,baseline_id:this.entry.baseline_id});
  const barrier=this.send(ids.MsgC2SMoveIntent,{scene_epoch:this.entry.scene_epoch,input_sequence:1,direction:8});assert.equal((await this.result(ids.MsgC2SMoveIntent,barrier)).error,ids.CodeInvalidArgument,'重复ACK后actor屏障异常');assert.equal(this.frames.filter(x=>x.message===ids.MsgS2CSyncReady).length,1,'重复ACK重新激活');assert.equal(this.frames.filter(x=>x.message===ids.MsgS2CHostingReport).length,1,'重复ACK重播战报');this.handshakeChecked=true;
 }
 async reject(ticket,expectedCode){await this.connect();const request=this.send(ids.MsgC2SEnterLine,{account_ticket:ticket});await until(()=>this.closed||this.frames.some(x=>x.message===ids.MsgS2CEnterLine&&x.request===request&&x.error),'无效票拒绝或ACK前关闭');assert(!this.entry&&!this.ready&&!this.inventory&&!this.wallet,'无效票曾成功入场或激活');assert(!this.frames.some(x=>(x.message===ids.MsgS2CEnterLine||x.message===ids.MsgS2CSyncReady)&&!x.error),'无效票收到成功Entry或Ready');const error=this.frames.find(x=>x.message===ids.MsgS2CEnterLine&&x.request===request&&x.error);if(error)assert.equal(error.error,expectedCode,'无效票错误码不符');this.close();return{rejected:true,observation:error?'error_frame':'closed_before_success',success_entry_or_ready_seen:false};}
 async pause(){const request=this.send(ids.MsgC2SAutoBattle,{scene_epoch:this.entry.scene_epoch,enabled:false,monster_count:1});const response=await this.result(ids.MsgS2CHostingState,request);assert.equal(response.error,0);assert.equal(response.value.auto_battle_enabled,false);await until(()=>this.baselineOrdinal>response.ordinal,'关闭自动后完整身体观察');}
 async heartbeat(){const request=this.send(ids.MsgC2SHeartbeat,{client_monotonic_ms:unsigned(1)});assert.equal((await this.result(ids.MsgS2CHeartbeat,request)).error,0);}
 operation(){return{op_id:id(),command_seq:this.ready.next_economic_command_seq,expected_revision:this.inventory.revision,expected_rule_version:unsigned(state.ruleVersion)};}
 async economic(message,value){const request=this.send(message,value);const response=await this.result(ids.MsgS2COperationResult,request);assert.equal(response.error,0);assert.equal(response.value.error_code,0,'资产操作业务拒绝');assert.equal(response.value.pending,false,'资产尚未明确提交');const next=BigInt(String(response.value.command_seq))+1n;if(next>BigInt(String(this.ready.next_economic_command_seq)))this.ready.next_economic_command_seq=unsigned(next);await until(()=>this.inventoryOrdinal>response.ordinal&&this.walletOrdinal>response.ordinal&&this.hostingOrdinal>response.ordinal&&BigInt(String(this.inventory.revision))>=BigInt(String(response.value.new_revision))&&BigInt(String(this.wallet.revision))>=BigInt(String(response.value.new_revision))&&BigInt(String(this.hosting.state_revision))>=BigInt(String(response.value.new_revision)),'资产结果后背包钱包托管完整观察');return response.value;}
 player(){const result=this.entities.find(x=>x.entity_handle===this.entry.player_handle);assert(result,'没有当前玩家实体');return result;}
 body(){const p=this.player();return{life:String(p.life),hp:String(p.hp),mp:String(p.mp),position:{x:p.position.x,y:p.position.y},attack_sequence:p.attack_sequence,cast_sequence:p.cast_sequence};}
 permanent(){const inventory=pb.InventoryResult.create({...this.inventory,revision:unsigned(0)}),wallet=pb.WalletState.create({...this.wallet,revision:unsigned(0)});return{inventory_pb:Buffer.from(pb.InventoryResult.encode(inventory).finish()).toString('hex'),wallet_pb:Buffer.from(pb.WalletState.encode(wallet).finish()).toString('hex'),hosting_seconds:this.hosting.remain_hosting_sec};}
 close(){this.ws?.close();}
}
let state={sessions:[]};
async function rpc(message,value,token=''){const raw=encode(message,1,0,value);let response;try{response=await fetch(state.account+'/rpc',{method:'POST',headers:{'Content-Type':'application/octet-stream',...(token?{Authorization:'Bearer '+token}:{})},body:raw,signal:AbortSignal.timeout(5000)});}catch{throw Error('account不可连接');}assert(response.ok,'account HTTP状态异常');const frame=decode(await response.arrayBuffer());assert.equal(frame.error,0,'account业务拒绝 '+frame.error);return frame.value;}
async function ticket(){return(await rpc(ids.MsgC2SEntryTicket,{role:state.role.key},state.token)).ticket;}
async function join(value){const session=new Session(state.game);state.sessions.push(session);await session.enter(value);return session;}
async function reject(value,expectedCode){const session=new Session(state.game);state.sessions.push(session);return session.reject(value,expectedCode);}
function signature(value){return crypto.createHash('sha256').update(JSON.stringify(value)).digest('hex');}
const actions={
 async initialize(command){state.account=command.account;state.game=command.game;state.ruleVersion=command.rule_version;
  assert(/^http:\/\/127\.0\.0\.1:\d+$/.test(state.account)&&/^ws:\/\/127\.0\.0\.1:\d+\/ws$/.test(state.game),'探针只允许回环地址');
  const login=await rpc(ids.MsgC2SLocalRegister,{account:'rec_'+crypto.randomBytes(6).toString('hex'),password:crypto.randomBytes(8).toString('hex')});state.token=login.account_token;
  state.role=await rpc(ids.MsgC2SCreateRole,{origin_server_id:unsigned(1),slot:1,job:2,name:'恢复'+crypto.randomBytes(5).toString('hex'),op_id:id()},state.token);
  state.session=await join(await ticket());await state.session.pause();state.claim={benefit_kind:0,operation:state.session.operation()};state.claimResult=await state.session.economic(ids.MsgC2SClaimHostingTime,state.claim);assert.equal(state.session.hosting.remain_hosting_sec,21600,'首次免费领取不是6h');
  const fixture=JSON.parse(fs.readFileSync(path.join(root,'config/development/first-loop.json'),'utf8'));state.spawn={...fixture.map.player_spawn};const origin={...state.session.player().position};
  const target=[{x:origin.x+1,y:origin.y},{x:origin.x-1,y:origin.y}].find(p=>p.x>=0&&p.x<fixture.map.width&&(p.x!==state.spawn.x||p.y!==state.spawn.y)&&!fixture.map.blocked.some(b=>b.x===p.x&&b.y===p.y)&&!state.session.entities.some(e=>e.entity_handle!==state.session.entry.player_handle&&e.position.x===p.x&&e.position.y===p.y));assert(target,'没有正常可走邻格');
  state.session.send(ids.MsgC2SMoveIntent,{scene_epoch:state.session.entry.scene_epoch,input_sequence:1,destination:target});await until(()=>state.session.player().position.x===target.x&&state.session.player().position.y===target.y,'正常移动离开出生点');
  state.preissued=await ticket();state.originalBody=state.session.body();return{registered_normally:true,created_normally:true,complete_baseline_before_ack:true,wrong_ack_blocked_without_assets:true,duplicate_ack_single_activation:true,free_six_hours_committed:true,body:state.originalBody};
 },
 async account_down(){await state.session.heartbeat();let unavailable=false;try{await ticket();}catch(error){unavailable=error.message==='account不可连接';}assert(unavailable,'停account仍取得新票');
  const original=state.session;state.session=await join(state.preissued);await state.session.pause();assert.deepEqual(state.session.body(),state.originalBody,'健康入场改变身体');
  const item=state.session.inventory.items.find(x=>!x.locked);assert(item,'没有可正常锁定的原赠物');const value={item_instance_id:item.instance_id,item_version:item.version,locked:true,operation:state.session.operation()};await state.session.economic(ids.MsgC2SSetItemLock,value);assert(state.session.inventory.items.find(x=>Buffer.from(x.instance_id).equals(Buffer.from(item.instance_id))).locked,'停account后资产未锁定');
  state.persisted=state.session.permanent();original.close();const replay=await reject(state.preissued,ids.CodeUnauthenticated);return{existing_ws_heartbeat:true,new_ticket_unavailable:true,preissued_ticket_entered_with_account_down:true,consumed_ticket_replay:replay,asset_committed_with_account_down:true,permanent_sha256:signature(state.persisted)};
 },
 async account_up(){const before=state.session.body(),old=state.session;state.session=await join(await ticket());await state.session.pause();assert.deepEqual(state.session.body(),before,'健康新票身体改变');assert.deepEqual(state.session.permanent(),state.persisted,'健康新票永久资产改变');old.close();state.crashTicket=await ticket();state.beforeCrash=state.session.body();return{new_ticket_after_restart:true,healthy_body_preserved:true,permanent_preserved:true,skill_cooldown_verified:false};},
 async game_restarted(){await until(()=>state.session.closed,'故障旧WS断开');const rejected=await reject(state.crashTicket,ids.CodeRouteChanged);state.session=await join(await ticket());const initial=state.session.initialBody;await state.session.pause();assert.deepEqual(state.session.permanent(),state.persisted,'冷重建永久资产改变');assert.equal(initial.hp,initial.hp_max,'冷重建首基线没有初始HP');assert.equal(initial.mp,initial.mp_max,'冷重建首基线没有初始MP');assert.deepEqual(initial.position,state.spawn,'冷重建首基线没有回到配置出生点');assert.notDeepEqual(state.beforeCrash.position,initial.position,'没有形成临时位置丢失对照');
  const next=String(state.session.ready.next_economic_command_seq),revision=String(state.session.inventory.revision),replay=await state.session.economic(ids.MsgC2SClaimHostingTime,state.claim);assert.equal(String(state.session.ready.next_economic_command_seq),next,'旧op回放改变本地经济游标');assert.equal(String(replay.new_revision),revision,'重查原op推进角色版本');assert.equal(BigInt(String(replay.command_seq))+1n,BigInt(next),'重查原op推进经济序号');assert.deepEqual(state.session.permanent(),state.persisted,'重查原领取重复发放');return{old_ws_closed:true,old_epoch_ticket:rejected,cold_initial_body:true,permanent_preserved:true,original_op_replayed_without_reward:true,permanent_sha256:signature(state.persisted)};
 },
 async finish(){for(const session of state.sessions)session.close();await sleep(100);return{closed:true};}
};
if(require.main===module){const lines=readline.createInterface({input:process.stdin,crlfDelay:Infinity});let queue=Promise.resolve();lines.on('line',line=>{queue=queue.then(async()=>{let command;try{command=JSON.parse(line);assert(actions[command.action],'未知探针动作');const result=await actions[command.action](command);process.stdout.write(JSON.stringify({action:command.action,passed:true,result})+'\n');}catch(error){process.stdout.write(JSON.stringify({action:command?.action,passed:false,error:error.message})+'\n');}});});lines.on('close',()=>{queue.finally(()=>{for(const session of state.sessions)session.close();setTimeout(()=>process.exit(0),100).unref();});});}
