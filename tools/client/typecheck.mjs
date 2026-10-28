// 使用同版本Creator自带的真实引擎声明，和纯模型测试、真实构建分别记录。
import {spawnSync} from 'node:child_process';
import path from 'node:path';
import creator from './creator_paths.cjs';
const result=spawnSync(process.execPath,[path.join(creator.typescriptRoot,'lib/tsc.js'),'--noEmit','-p','client/tsconfig.json'],{stdio:'inherit'});
if(result.error)throw result.error;
console.log(JSON.stringify({strict_typecheck:result.status===0,typescript:creator.typescript.version,creator_build_verified:false}));
process.exit(result.status??1);
