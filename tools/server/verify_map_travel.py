"""使用现成的四服隔离试验台比较真实 Creator 包的地图往返。"""

import argparse
import json
import os
from pathlib import Path
import tempfile

from verify_server_directory import ROOT, browser_build, verify


def run(web_root, expect_styles):
    build = browser_build(web_root)
    source = ROOT / 'server/internal/integration/server_directory_test.go'
    text = source.read_text(encoding='utf-8')
    read_child = 'must(t, json.Unmarshal(raw, &node))'
    if text.count(read_child) != 1:
        raise RuntimeError('未定位四服试验台的子进程配置入口')
    # 两个版本使用相同的隔离生命值，防止加载时间差把半途死亡计入切图耗时。
    text = text.replace(read_child, read_child + '\n\tfor i := range node.Config.Fixture.PlayerGrowth {\n'
                        '\t\tnode.Config.Fixture.PlayerGrowth[i].HPBase = 500\n\t}')
    with tempfile.TemporaryDirectory(prefix='map-travel-benchmark-', dir=ROOT / '.work') as folder:
        temporary = Path(folder)
        replaced = temporary / source.name
        replaced.write_text(text, encoding='utf-8', newline='\n')
        overlay = temporary / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(source): str(replaced)}}), encoding='utf-8')
        os.environ['GOFLAGS'] = f'-overlay={overlay.as_posix()}'
        os.environ['MINIMIR_BENCHMARK_MAP_TRAVEL'] = '1'
        os.environ['MINIMIR_EXPECT_REGIONAL_MAP_STYLES'] = str(int(expect_styles))
        return verify(build)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--web-root', type=Path, required=True)
    parser.add_argument('--expect-styles', action='store_true')
    args = parser.parse_args()
    if os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1' or os.name != 'nt':
        parser.error('请使用 tools/dev.ps1 的 Windows 隔离试验台')
    return run(args.web_root, args.expect_styles)


if __name__ == '__main__':
    raise SystemExit(main())
