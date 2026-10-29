from pathlib import Path
import subprocess, json, hashlib, datetime, time, re, sys
ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

files = set()
for base, suffixes in [('client/assets/scripts', {'.ts', '.meta'}), ('client/tests', {'.cjs', '.mjs'}), ('tools/client', {'.cjs', '.mjs', '.py'})]:
    files.update(p.relative_to(ROOT).as_posix() for p in (ROOT / base).rglob('*') if p.is_file() and p.suffix in suffixes)
for base, pattern in [('shared/proto', '*.proto'), ('server/api/generated/minimirv1', '*.pb.go'), ('client/assets/scenes', '*.scene'), ('client/assets/resources/prefabs', '*.prefab')]:
    files.update(p.relative_to(ROOT).as_posix() for p in (ROOT / base).glob(pattern))
files.update(['shared/proto/protocol_manifest.json', 'client/assets/scripts/generated/protobuf/client_pb.js', 'conf/toolchain/go-pb-generation.json', 'conf/toolchain/client-pb-verification.json', 'client/assets/resources/development/gameplay.json', 'client/assets/resources/development/manifest.json', 'client/assets/resources/development/maps/first-map.json', 'config/development/first-loop.json'])
files = sorted(files)
snapshot = lambda: {f: digest(ROOT / f) for f in files}
before = snapshot()
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
(OUT / 'final-before.json').write_text(json.dumps({'utc': started, 'files': before}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
results = []
for name, args in [('models', ['node', 'client/tests/ui/compile_models.mjs']), ('full-ui-app', ['node', '--test', 'client/tests/ui/*.test.cjs', 'client/tests/app/*.test.cjs']), ('strict', ['node', 'tools/client/typecheck.mjs']), ('protocol-check', ['task', 'protocol-check']), ('protocol-lint', ['python', 'tools/config/generate_protocol.py', '--check'])]:
    cmd = ['powershell', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(ROOT / 'tools/dev.ps1'), *args]
    at = time.monotonic()
    result = subprocess.run(cmd, cwd=ROOT, capture_output=True)
    log = OUT / (name + '.txt')
    log.write_bytes(result.stdout + result.stderr)
    record = {'name': name, 'command': cmd, 'exit_code': result.returncode, 'seconds': round(time.monotonic() - at, 3), 'log': log.relative_to(ROOT).as_posix(), 'sha256': digest(log)}
    results.append(record)
    print(json.dumps(record, ensure_ascii=False), flush=True)
    print((result.stdout + result.stderr).decode('utf-8', errors='replace')[-1000:], flush=True)
after = snapshot()
current = snapshot()
text = (OUT / 'full-ui-app.txt').read_text(encoding='utf-8')
counts = {k: int(re.search('ℹ ' + k + r' (\d+)', text).group(1)) for k in ['tests', 'pass', 'fail', 'skipped']}
pb = json.loads((ROOT / 'conf/toolchain/client-pb-verification.json').read_text(encoding='utf-8'))
report = {'scope': 'rule16宠物尸体/训练观察与完整分块取消；仅模型/真实TS组件轻量Cocos替身/PB编解码，非真实WS服务、Creator或容量验收', 'started_utc': started, 'freeze_started_before_commands': True, 'before_is_pre_final_commands_not_pre_implementation': True, 'input_files': len(files), 'fixture_sha256': before['config/development/first-loop.json'], 'public_rule_version': json.loads((ROOT / 'client/assets/resources/development/gameplay.json').read_text(encoding='utf-8'))['rule_version'], 'before': before, 'after': after, 'current': current, 'stable': before == after == current, 'changed_during_commands': [f for f in files if before[f] != after[f]], 'commands': results, 'full_ui_app_actual_counts': counts, 'pb': {k: pb[k] for k in ['message_count', 'int64_field_count', 'int64_boundary_cases']}, 'saved_layouts_unchanged': all(before[f] == after[f] for f in files if f.endswith(('.scene', '.prefab'))), 'limitations': ['17/购符仍关闭', '128尸体仅组件与编码样本，不是GPU/真机/内存容量验收', 'PB到Session用实际codec/连接/Session及假socket，不称真实网络服务', '无每次训练独立消息/经济水位，观察来自完整基线', '初次中途测试Long解析与漏重编失败日志保留，不宣称已运行修前产品反例'], 'history': {p.name: digest(p) for p in OUT.glob('*.txt') if p.name.startswith(('targeted-', 'full-intermediate-', 'strict-intermediate'))}}
(OUT / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print('FINAL_STABLE', report['stable'], counts, flush=True)
sys.exit(0 if report['stable'] and all(r['exit_code'] == 0 for r in results) else 1)
