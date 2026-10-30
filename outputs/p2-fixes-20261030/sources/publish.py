"""限定验收通过后同卷切换本地预览，逐摘要核对并保留原包。"""
import datetime
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import threading

ROOT = next(p for p in Path(__file__).resolve().parents if (p / 'tools/dev').is_file())
WORK = ROOT / '.work/p2-fixes-20261030'
OUT = WORK / 'publish/run-2'
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree(path):
    return {p.relative_to(path).as_posix(): sha(p) for p in sorted(path.rglob('*')) if p.is_file()}


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def safe(path):
    assert path.resolve().is_relative_to(ROOT) and path.resolve() != ROOT
    for parent in [path, *path.parents]:
        assert not parent.is_symlink() and not getattr(parent, 'is_junction', lambda: False)()
        if parent == ROOT:
            break


def command(arguments, label):
    result = subprocess.run(DEV + arguments, cwd=ROOT, capture_output=True, timeout=120)
    (OUT / (label + '.stdout.txt')).write_bytes(result.stdout)
    (OUT / (label + '.stderr.txt')).write_bytes(result.stderr)
    assert result.returncode == 0, label
    try:
        return json.loads(result.stdout) if result.stdout else None
    except json.JSONDecodeError:
        return {'output': result.stdout.decode('utf-8', errors='replace').strip()}


def main():
    creator = load(WORK / 'creator/full-2/report.json')
    assert creator['passed'] and load(WORK / 'server/full-2/report.json')['passed']
    gui_dir = WORK / 'browser/run-20261030T145210Z-f70cb8b2'
    assert load(gui_dir / 'report.json')['passed'] and load(gui_dir / 'browser/report.json')['passed']
    source = Path(creator['build_root'])
    target = ROOT / 'client/build/web-mobile'
    backup = OUT / 'previous-web-mobile'
    assert source == WORK / 'creator/full-2/candidate/web-mobile' and target == ROOT / 'client/build/web-mobile'
    for path in [source, target, backup]:
        safe(path)
    assert not OUT.exists()
    OUT.mkdir()
    manifest = load(WORK / 'creator/full-2/candidate-files.json')
    prefix = source.relative_to(ROOT).as_posix()
    expected = {PurePosixPath(p).relative_to(prefix).as_posix(): digest for p, digest in manifest.items()}
    assert tree(source) == expected
    before = tree(target)
    assert before == load(gui_dir / 'report.json')['served_main_before']
    save(OUT / 'source-before.json', expected)
    save(OUT / 'old-preview-before.json', before)
    command(['python3', 'tools/client/dev.py', 'stop'], 'static-stop')
    moved_old = moved_new = False
    try:
        for path in [source, target, backup]:
            safe(path)
        os.rename(target, backup)
        moved_old = True
        os.rename(source, target)
        moved_new = True
        assert tree(target) == expected and tree(backup) == before
        command(['python3', 'tools/client/dev.py', 'start'], 'static-start')
        command(['python3', 'tools/server/dev.py', 'start'], 'backend-start')
    except BaseException:
        if moved_new:
            safe(source)
            safe(target)
            os.rename(target, source)
        if moved_old:
            safe(backup)
            safe(target)
            os.rename(backup, target)
        command(['python3', 'tools/client/dev.py', 'start'], 'static-rollback-start')
        raise
    report = {'passed': True, 'recorded_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'creator_report': str(WORK / 'creator/full-2/report.json'), 'original_build_root': str(source),
              'current_build_root': str(target), 'original_preview_backup': str(backup), 'artifact_count': len(expected),
              'full_sha256_equal_after_same_volume_rename': True, 'old_preview_preserved': True,
              'local_development_only': True, 'production_ready': False,
              'static': command(['python3', 'tools/client/dev.py', 'status'], 'static-status'),
              'backend': command(['python3', 'tools/server/dev.py', 'status'], 'backend-status')}
    save(OUT / 'report.json', report)
    print(json.dumps({'passed': True, 'artifact_count': len(expected), 'backend_running': report['backend']['running']}))


if __name__ == '__main__':
    stopped = threading.Event()
    def heartbeat():
        while not stopped.wait(20):
            print(json.dumps({'status': 'publishing_local_preview'}), flush=True)
    threading.Thread(target=heartbeat, daemon=True).start()
    try:
        main()
    finally:
        stopped.set()
