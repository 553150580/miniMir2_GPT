import creatorPaths from '../../../tools/client/creator_paths.cjs';
import { mkdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const creator = process.env.MINIMIR_CREATOR_APP ?? '/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const typescript = creatorPaths.typescriptRoot;
const version = JSON.parse(readFileSync(path.join(typescript, 'package.json'), 'utf8')).version;
if (!version.startsWith('5.8.')) throw new Error('UI纯模型测试使用 Creator 自带 TypeScript 5.8');
const source = path.join(root, 'client/assets/scripts');
const output = path.join(root, '.work/ui-tests');
mkdirSync(output, { recursive: true });
const result = spawnSync(process.execPath, [path.join(typescript, 'lib/tsc.js'), '--strict', '--noEmitOnError',
    '--target', 'ES2020', '--lib', 'ES2020', '--module', 'commonjs', '--rootDir', source, '--outDir', output,
    ...['ui/PanelStackState.ts', 'ui/VirtualListWindow.ts', 'input/NavigationIntent.ts', 'app/GridProjection.ts', 'app/BeamGeometry.ts', 'app/EntityHeadAnchor.ts', 'app/CombatStatusModel.ts', 'app/StatusPlayback.ts', 'app/CastingSupplyModel.ts', 'app/HudModel.ts', 'app/SystemMessageLog.ts', 'app/PotionModel.ts', 'app/SkillModel.ts', 'app/CastPlayback.ts', 'app/SkillAssetsModel.ts'].map(file => path.join(source, file)),
], { cwd: root, stdio: 'inherit' });
if (result.error) throw result.error;
if (result.status !== 0) process.exit(result.status ?? 1);
console.log(`UI/输入纯模型编译通过（TypeScript ${version}）；Creator组件仍使用真实工程类型另行验证。`);
