from pathlib import Path
import hashlib, json, subprocess

root = Path(__file__).resolve().parents[4]
out = Path(__file__).resolve().parent
files = ['client/assets/scripts/app/WorldAudio.ts', 'client/tests/support/world_runtime.cjs', 'client/tests/app/world_audio_destroy.test.cjs', 'client/tests/app/world_audio_input.test.cjs']
snapshot = lambda: {p: hashlib.sha256((root/p).read_bytes()).hexdigest() for p in files}
before = snapshot()
command = ['powershell', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(root/'tools/dev.ps1'), 'node', '--test',
           'client/tests/app/world_audio_destroy.test.cjs', 'client/tests/app/world_audio_input.test.cjs', 'client/tests/app/world_asset_lifecycle.test.cjs',
           'client/tests/app/pet_born_runtime.test.cjs', 'client/tests/app/pet_visual_lifecycle.test.cjs']
result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(out/'after.txt').write_bytes(result.stdout)
report = {'scope':'锁定引擎真实销毁方法/传播与生产WorldAudio、WorldAssets、GameWorldView轻量替身；不代表WS/GPU/浏览器真实播放或硬件听感',
          'command':command, 'exit_code':result.returncode, 'before':before, 'after':snapshot(), 'log_sha256':hashlib.sha256(result.stdout).hexdigest()}
report['current'] = snapshot()
report['stable'] = report['before'] == report['after'] == report['current']
(out/'after.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8', newline='\n')
print(result.stdout.decode('utf-8', errors='replace'))
raise SystemExit(result.returncode)
