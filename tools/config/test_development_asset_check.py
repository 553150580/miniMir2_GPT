"""验证历史素材复核只容许已登记的转换器和本机来源路径有差异。"""

from copy import deepcopy
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch


sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'resources'))
import build_development_assets as assets


class DevelopmentAssetCheckTests(unittest.TestCase):
    def test_historical_manifest_preserves_provenance_and_rejects_content_drift(self):
        archived = {
            'converter_sha256': assets.ARCHIVED_CONVERTER_SHA256,
            'wil_decoder_sha256': '原解码器摘要',
            'sources': [{'package': 'Tiles', 'wil_path': '原机/Tiles.wil',
                         'wix_path': '原机/Tiles.wix', 'wil_sha256': '原包摘要'}],
            'frames': {'Tiles:0': {'png_sha256': '原帧摘要'}},
        }
        regenerated = deepcopy(archived)
        regenerated['converter_sha256'] = '新版转换器摘要'
        regenerated['sources'][0].update(wil_path='本机/Tiles.wil', wix_path='本机/Tiles.wix')
        with tempfile.TemporaryDirectory() as directory, patch.object(assets, 'OUTPUT', Path(directory)):
            saved = Path(directory) / 'manifest.json'
            saved.write_text(assets.canonical(archived), encoding='utf-8')
            before = saved.read_bytes()
            checker = assets.Importer(check=True)
            checker.write_json('manifest.json', regenerated)
            drifted_source = deepcopy(regenerated)
            drifted_source['sources'][0]['wil_sha256'] = '不同原包摘要'
            drifted_frame = deepcopy(regenerated)
            drifted_frame['frames']['Tiles:0']['png_sha256'] = '不同原帧摘要'
            for drifted in (drifted_source, drifted_frame):
                with self.assertRaises(ValueError):
                    checker.write_json('manifest.json', drifted)
            self.assertEqual(saved.read_bytes(), before)
            archived['converter_sha256'] = '未登记的历史摘要'
            saved.write_text(assets.canonical(archived), encoding='utf-8')
            with self.assertRaises(ValueError):
                checker.write_json('manifest.json', regenerated)

    def test_historical_map_checks_geometry_and_source_digest_after_relocation(self):
        name = 'maps/first-map.json'
        archived = {'source_path': '原机/Map/0.map', 'source_sha256': '原图摘要', 'width': 17}
        regenerated = dict(archived, source_path='本机/Map/0.map')
        self.assertTrue(assets.historical_json_matches(name, archived, regenerated))
        self.assertFalse(assets.historical_json_matches(name, archived, dict(regenerated, width=13)))
        self.assertFalse(assets.historical_json_matches(name, archived, dict(regenerated, source_sha256='不同来源')))
