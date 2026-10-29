"""出生资源的来源派生、动作表边界和来源锁验证。"""
import copy
import json
from pathlib import Path
import struct
import tempfile
import unittest

import pet_birth_assets as birth


ROOT = Path(__file__).resolve().parents[2]


class PetBirthAssetTests(unittest.TestCase):
    def test_source_contract_derives_cast_and_appear_separately(self):
        compiled = birth.compile_contract(ROOT)
        self.assertEqual(compiled['cast'], {'skill_id': 17, 'package': 'Magic', 'raw_start': 1500, 'count': 10, 'duration_ms': 600, 'blend': 'additive'})
        self.assertEqual({key: compiled['birth'][key] for key in ['count', 'direction_stride', 'duration_ms', 'blend']}, {'count': 10, 'direction_stride': 0, 'duration_ms': 100, 'blend': 'additive'})
        self.assertEqual(compiled['birth']['count'] * compiled['birth']['duration_ms'], 1000)
        contract = compiled['contract']['visual_mapping']
        self.assertNotIn('duration_ms', contract['cast'])
        self.assertNotIn('duration_ms', contract['birth'])
        self.assertEqual(set(compiled['audio']), {'summon_skeleton_cast', 'pet_skeleton_born'})

    def test_frame_table_rejects_truncated_unknown_duplicate_and_invalid_boolean(self):
        original = (ROOT / '资料参考/crystal-supplement/assets/Monster/78.Lib').read_bytes()
        frame_seek = struct.unpack_from('<i', original, 8)[0]
        names = [str(i) for i in range(45)]
        cases = {'truncated': original[:-1], 'extra_tail': original + b'x'}
        changed = bytearray(original)
        changed[frame_seek + 4] = 255
        cases['unknown_action'] = changed
        changed = bytearray(original)
        changed[frame_seek + 4 + 35] = changed[frame_seek + 4]
        cases['duplicate_action'] = changed
        changed = bytearray(original)
        changed[frame_seek + 4 + 34] = 2
        cases['invalid_boolean'] = changed
        changed = bytearray(original)
        struct.pack_into('<i', changed, frame_seek + 4 + 1 + 12, 0)
        cases['zero_interval'] = changed
        for name, data in cases.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                birth.parse_frame_set(data, names)

    def test_frame_table_rejects_out_of_range_image_index(self):
        data = bytearray((ROOT / '资料参考/crystal-supplement/assets/Monster/78.Lib').read_bytes())
        struct.pack_into('<i', data, 12, len(data))
        with self.assertRaisesRegex(ValueError, '图片索引越界'):
            birth.parse_frame_set(data, [str(i) for i in range(45)])

    def test_changed_independent_source_copy_fails_closed(self):
        contract = json.loads((ROOT / birth.CONTRACT).read_text(encoding='utf-8'))
        with tempfile.TemporaryDirectory() as name:
            temp = Path(name)
            changed = copy.deepcopy(contract)
            item = changed['copies'][0]
            path = temp / item['local_reference_path']
            path.parent.mkdir(parents=True)
            path.write_bytes(b'changed source')
            (temp / birth.CONTRACT).parent.mkdir(parents=True)
            (temp / birth.CONTRACT).write_text(json.dumps(changed), encoding='utf-8')
            with self.assertRaisesRegex(ValueError, '摘要或独立性'):
                birth.compile_contract(temp)


if __name__ == '__main__':
    unittest.main()
