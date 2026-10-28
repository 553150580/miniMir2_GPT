import creatorPaths from '../client/creator_paths.cjs';
// 仅检查独立TypeScript声明/常量；优先使用已安装Creator的TypeScript，不生成产物。
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const creatorApp=process.env.COCOS_CREATOR_PATH||'/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const creatorTsc=path.join(creatorPaths.typescriptRoot,'lib/tsc.js');
let hasCreatorTsc=true;
try{await fs.access(creatorTsc);}catch{hasCreatorTsc=false;}
const executable=hasCreatorTsc?process.execPath:'tsc';
const prefix=hasCreatorTsc?[creatorTsc]:[];
const version=spawnSync(executable,[...prefix,'--version'],{encoding:'utf8'});
if(version.error||version.status!==0){
  console.error(JSON.stringify({verification_status:'not_run',reason:'未找到可执行的tsc；Creator实际构建仍需其配套编译链。'}));
  process.exit(2);
}
const pointer=JSON.parse(await fs.readFile(path.join(root,'config/releases/preview_current.json'),'utf8'));
const files=[path.join(root,'client/assets/scripts/generated/protocol_ids.ts'),path.join(root,'client/assets/scripts/generated/config_types.ts')];
const clientDts=path.join(root,'client/assets/scripts/generated/protobuf/client_pb.d.ts');
try{await fs.access(clientDts);files.push(clientDts);}catch{}
const runtimeModules=path.join(root,'conf/toolchain/node-tools.lock.json');
let extra=[];
try{await fs.access(path.join(root,'client/node_modules'));extra=['--moduleResolution','node','--baseUrl',path.join(root,'client/node_modules')];}
catch{try{const lock=JSON.parse(await fs.readFile(runtimeModules,'utf8'));extra=['--moduleResolution','node','--baseUrl',path.join(lock.installation_root,'node_modules')];}catch{}}
const result=spawnSync(executable,[...prefix,'--noEmit','--strict','--target',hasCreatorTsc?'ES2020':'ES2018',...extra,...files],{encoding:'utf8'});
if(result.error||result.status!==0){
  console.error(JSON.stringify({typescript:version.stdout.trim(),diagnostics:(result.stdout+result.stderr).slice(0,6000)}));
  process.exitCode=2;
}else console.log(JSON.stringify({typescript:version.stdout.trim(),compiler:hasCreatorTsc?'creator_3.8.8':'path_tsc',files:files.map(x=>path.relative(root,x)),typecheck:'passed',creator_build:'not_run'}));
