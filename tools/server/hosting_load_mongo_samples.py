"""采样本轮独立 MongoDB 的进程内存、缓存和事务计数。"""

import datetime as dt
import json
import re
import time
from urllib.parse import parse_qs, urlsplit

from pymongo import MongoClient
from pymongo.errors import PyMongoError


def _select(document, *fields):
    return {field: document[field] for field in fields if field in document}


def _status_view(status):
    wired_tiger = status.get('wiredTiger', {})
    allocator = status.get('tcmalloc', {})
    return {
        'pid': status['pid'],
        'version': status['version'],
        'uptime_seconds': status['uptime'],
        'memory_mib': _select(status.get('mem', {}), 'resident', 'virtual'),
        'wired_tiger_cache': _select(
            wired_tiger.get('cache', {}),
            'maximum bytes configured',
            'bytes currently in the cache',
            'bytes belonging to page images in the cache',
            'bytes not belonging to page images in the cache',
            'bytes allocated for updates',
            'tracked dirty bytes in the cache',
            'pages currently held in the cache',
            'pages read into cache',
            'pages written from cache',
            'pages evicted by application threads',
        ),
        'wired_tiger_session': _select(wired_tiger.get('session', {}), 'open session count'),
        'allocator': {
            'generic': _select(allocator.get('generic', {}),
                               'current_allocated_bytes', 'heap_size', 'peak_memory_usage'),
            'tcmalloc': _select(allocator.get('tcmalloc', {}),
                                'pageheap_free_bytes', 'pageheap_unmapped_bytes',
                                'central_cache_free_bytes', 'thread_cache_free_bytes',
                                'transfer_cache_free_bytes', 'total_free_bytes'),
        },
        'connection_counts': _select(status.get('connections', {}),
                                     'current', 'active', 'totalCreated', 'rejected'),
        'logical_sessions': _select(status.get('logicalSessionRecordCache', {}),
                                    'activeSessionsCount', 'sessionCatalogSize'),
        'transactions': _select(
            status.get('transactions', {}),
            'currentActive', 'currentInactive', 'currentOpen',
            'totalStarted', 'totalCommitted', 'totalAborted',
        ),
    }


def isolated_mongo_uri(endpoint):
    uri = urlsplit(endpoint['uri'])
    expected_set = 'minimirLoad_' + endpoint['run_id']
    expected_port = endpoint['port']
    if (endpoint.get('endpoint_verified') is not True or
            endpoint.get('ownership_verified') is not True or
            not re.fullmatch(r'[a-f0-9]{32}', endpoint['run_id']) or
            uri.scheme != 'mongodb' or uri.netloc != f'127.0.0.1:{expected_port}' or
            parse_qs(uri.query) != {'replicaSet': [expected_set], 'directConnection': ['true']} or
            endpoint.get('replica_set') != expected_set or
            endpoint.get('database') != 'minimir_dev_game_test_load_' + endpoint['run_id']):
        raise RuntimeError('拒绝连接未经核验的本轮隔离 MongoDB')
    return endpoint['uri']


class MongoLoadSampler:
    def __init__(self, endpoint, output):
        uri = isolated_mongo_uri(endpoint)
        self._pid = endpoint['pid']
        self._version = endpoint['version']
        self._output = output
        self._client = MongoClient(uri, appname='hosting-load-sampler',
                                   timeoutMS=2000, connectTimeoutMS=2000,
                                   serverSelectionTimeoutMS=2000, maxPoolSize=1, retryReads=False)

    def record(self, phase):
        row = {'phase': phase, 'sampled_at': dt.datetime.now(dt.timezone.utc).isoformat()}
        started = time.monotonic()
        try:
            status = self._client.admin.command({'serverStatus': 1, 'tcmalloc': 1})
            if status.get('pid') != self._pid or status.get('version') != self._version:
                raise RuntimeError('MongoDB 采样时进程身份发生变化')
            row.update(_status_view(status))
        except PyMongoError as error:
            # 探针超时单独留证，由压测业务监控判定写确认结果。
            row.update(error=type(error).__name__, error_code=getattr(error, 'code', None))
        row['sampled_seconds'] = time.monotonic() - started
        self._output.write(json.dumps(row) + '\n')
        self._output.flush()

    def close(self):
        self._client.close()
