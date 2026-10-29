"""用同一Creator工程实构候选目录，保留正在提供预览的旧包原字节。"""
from pathlib import Path
import argparse, hashlib, json, os, subprocess, sys, time

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def snapshot(folder, suffixes=None):
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(folder.rglob('*'))
            if p.is_file() and (suffixes is None or p.suffix in suffixes)}


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--attempt', required=True)
    args = parser.parse_args()
    assert args.attempt.isalnum()
    out = WORK / 'creator' / ('full-' + args.attempt)
    assert out.is_dir() and not (out / 'candidate').exists()
    candidate = out / 'candidate' / 'web-mobile'
    original_config = ROOT / 'conf/creator-build.json'
    config = json.loads(original_config.read_text(encoding='utf-8'))
    assert config['platform'] == 'web-mobile' and config['outputName'] == 'web-mobile'
    config['buildPath'] = str(candidate.parent)
    controlled_config = out / 'creator-build-candidate.json'
    write(controlled_config, config)
    before = snapshot(ROOT / 'client/assets', {'.scene', '.prefab'})
    source_before = snapshot(ROOT / 'client/assets/scripts', {'.ts', '.js'})
    main_before = snapshot(ROOT / 'client/build/web-mobile')
    assert main_before and (ROOT / 'client/build/web-mobile/index.html').is_file()
    write(out / 'served-build-before.json', main_before)
    app = Path(os.environ['MINIMIR_CREATOR_EXECUTABLE'])
    assert app.is_file()
    log = ROOT / '.work/creator-cli.log'
    command = [str(app), '--no-sandbox', '--disable-gpu', '--project', str(ROOT / 'client'),
               '--build', 'configPath=' + str(controlled_config)]
    started = time.monotonic()
    with log.open('wb') as output:
        result = subprocess.run(command, cwd=ROOT, stdout=output, stderr=subprocess.STDOUT)
    after = snapshot(ROOT / 'client/assets', {'.scene', '.prefab'})
    source_after = snapshot(ROOT / 'client/assets/scripts', {'.ts', '.js'})
    main_after = snapshot(ROOT / 'client/build/web-mobile')
    write(out / 'served-build-after.json', main_after)
    artifact = snapshot(candidate)
    write(out / 'candidate-files.json', artifact)
    preserved = before == after
    source_stable = source_before == source_after
    old_preview_preserved = main_before == main_after
    success = result.returncode == 36 and (candidate / 'index.html').is_file() and preserved and source_stable and old_preview_preserved
    report = dict(creator='3.8.8', platform='web-mobile', exit_code=result.returncode, success=success,
                  scene_prefab_layout_preserved=preserved, source_stable_during_build=source_stable,
                  served_build_preserved=old_preview_preserved, served_build_files=len(main_before),
                  asset_hashes=after, script_hashes=source_after, build_root=str(candidate),
                  build_config=str(controlled_config), original_build_config_sha256=sha(original_config),
                  build_config_sha256=sha(controlled_config), command=command,
                  artifact_manifest=str(out / 'candidate-files.json'), artifact_manifest_sha256=sha(out / 'candidate-files.json'),
                  artifact_file_count=len(artifact), seconds=round(time.monotonic() - started, 2), log=str(log),
                  physical_mobile_verified=False,
                  scope='真实Creator同源构建；仅构建配置输出路径更改，不等于主预览切换或GUI验收')
    write(ROOT / 'docs/客户端构建检查.json', report)
    print(json.dumps({k: v for k, v in report.items() if k not in {'asset_hashes', 'script_hashes', 'command'}}, ensure_ascii=False))
    raise SystemExit(0 if success else 1)


if __name__ == '__main__':
    main()
