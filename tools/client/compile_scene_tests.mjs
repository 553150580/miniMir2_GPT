import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../../', import.meta.url));
const creatorApp = process.env.MINIMIR_CREATOR_APP ?? '/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const typescriptRoot = path.join(creatorApp, 'Contents/Resources/app.asar.unpacked/node_modules/typescript');
const version = JSON.parse(readFileSync(path.join(typescriptRoot, 'package.json'), 'utf8')).version;
if (!version.startsWith('5.8.')) throw new Error(`需要 Creator 自带 TypeScript 5.8，实际为 ${version}`);
const source = path.join(root, 'client/assets/scripts/core/scene');
const output = path.join(root, '.work/scene-tests');
mkdirSync(output, { recursive: true });
// 只编译服务脚本到临时目录，不写客户端 JS、场景、预制体或引擎声明。
const result = spawnSync(process.execPath, [path.join(typescriptRoot, 'lib/tsc.js'),
    '--strict', '--target', 'ES2020', '--lib', 'ES2020', '--module', 'commonjs',
    '--noEmitOnError', '--rootDir', source, '--outDir', output,
    ...['CreatorSceneBackend.ts', 'SceneLoader.ts', 'SceneManager.ts'].map(file => path.join(source, file)),
], { cwd: root, stdio: 'inherit' });
if (result.error) throw result.error;
if (result.status !== 0) process.exit(result.status ?? 1);
const report = {
    scope: '纯 TypeScript 服务与模拟 Director 测试',
    typescript: version,
    compiler: path.join(typescriptRoot, 'lib/tsc.js'),
    node: process.version,
    output,
    creatorBuildVerified: false,
    realDeviceVerified: false,
};
writeFileSync(path.join(output, 'compile-status.json'), `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(`场景服务编译通过：Creator TypeScript ${version}；未执行 Creator 构建或真机验收。`);
