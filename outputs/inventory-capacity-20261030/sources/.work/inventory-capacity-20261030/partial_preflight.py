"""只读执行已终态的config/Go/client门禁；不调用整体main或写正式报告。"""
import hashlib
import json
from pathlib import Path
import runpy

WORK = Path(__file__).resolve().parent
module = runpy.run_path(str(WORK / 'finalize_evidence.py'), run_name='readonly_partial_preflight')
config, fixture, public = module['configuration_evidence']()
go, go_count = module['go_evidence'](WORK / 'go/full-2/report.json', WORK / 'go/full-2/events.jsonl')
client, client_count, protocols = module['client_evidence'](WORK / 'client/full-2/report.json')
print(json.dumps({'scope': 'readonly_config_go_client_only_not_whole_check', 'formal_report_written': False,
    'archive_executed': False, 'config_python': config['python_tests'], 'go': go_count,
    'client': client_count, 'protocol_child_reports': [str(p) for p in protocols],
    'finalizer_sha256': hashlib.sha256((WORK / 'finalize_evidence.py').read_bytes()).hexdigest()}, ensure_ascii=False))
