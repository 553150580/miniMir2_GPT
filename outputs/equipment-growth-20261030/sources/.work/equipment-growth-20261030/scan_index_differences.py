"""只读批量比较本阶段暂存文件及受测输入的实际Git原blob，提前列齐差异。"""
from pathlib import Path
import hashlib, json, runpy

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
module = runpy.run_path(str(WORK / 'verify_index.py'), run_name='read_only_scan_helpers')
indexed = module['index']()
names = {name.decode('utf-8') for name in module['git']('diff', '--cached', '--name-only', '-z').split(b'\0') if name}
for relative, key in [('go/full-3/report.json', 'source_sha256'), ('client/full-1/report.json', 'before')]:
    names.update(json.loads((WORK / relative).read_text(encoding='utf-8'))[key])
reader = module['Blobs']()
differences = []
try:
    for name in sorted(names):
        path = ROOT / name
        assert path.resolve().is_relative_to(ROOT.resolve()) and name in indexed and path.is_file()
        current = path.read_bytes()
        current_sha = hashlib.sha256(current).hexdigest()
        index_sha = reader.content(indexed[name][1])
        if current_sha != index_sha:
            raw = reader.content(indexed[name][1], collect=True)
            differences.append({'path': name, 'working_sha256': current_sha, 'index_sha256': index_sha,
                                'working_bytes': len(current), 'index_bytes': len(raw),
                                'only_crlf_normalization': current.replace(b'\r\n', b'\n') == raw})
finally:
    reader.close()
report = {'read_only': True, 'files_checked': len(names), 'differences': differences,
          'scope': '当前阶段全部暂存路径与Go/client受测输入；不代替完整index门禁或原资源核验'}
(WORK / 'index-differences.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps(report, ensure_ascii=False))
