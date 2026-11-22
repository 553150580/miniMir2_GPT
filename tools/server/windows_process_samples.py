"""按进程句柄读取 Windows CPU、常驻内存和私有提交。"""

import ctypes
from ctypes import wintypes
import os
import time


class ProcessMemory(ctypes.Structure):
    _fields_ = [('cb', wintypes.DWORD), ('page_faults', wintypes.DWORD)] + [
        (field, ctypes.c_size_t) for field in (
            'peak_working_set', 'working_set', 'peak_paged_pool', 'paged_pool',
            'peak_nonpaged_pool', 'nonpaged_pool', 'pagefile_usage', 'peak_pagefile_usage', 'private_usage',
        )
    ]


class ProcessSamples:
    def __init__(self, processes):
        self.kernel = ctypes.WinDLL('kernel32', use_last_error=True)
        self.kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
        self.kernel.OpenProcess.restype = wintypes.HANDLE
        self.kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        self.kernel.GetProcessTimes.argtypes = [wintypes.HANDLE] + [ctypes.POINTER(wintypes.FILETIME)] * 4
        self.kernel.K32GetProcessMemoryInfo.argtypes = [wintypes.HANDLE, ctypes.POINTER(ProcessMemory), wintypes.DWORD]
        self.slots = {}
        self.latest = {}
        try:
            for name, pid in processes.items():
                handle = self.kernel.OpenProcess(0x1000, False, pid)
                if not handle:
                    raise ctypes.WinError(ctypes.get_last_error())
                self.slots[name] = (pid, handle)
        except OSError:
            self.close()
            raise

    def read(self):
        result = {}
        for name, (pid, handle) in self.slots.items():
            created, exited, system, user = [wintypes.FILETIME() for _ in range(4)]
            if not self.kernel.GetProcessTimes(handle, *(ctypes.byref(stamp) for stamp in [created, exited, system, user])):
                result[name] = {'pid': pid, 'read_error': ctypes.get_last_error()}
                continue
            seconds = sum((stamp.dwHighDateTime << 32) | stamp.dwLowDateTime for stamp in [system, user]) / 1e7
            now = time.monotonic()
            previous = self.latest.get(name)
            self.latest[name] = (now, seconds)
            row = dict(pid=pid, cpu_seconds=seconds)
            if previous:
                cores = (seconds - previous[1]) / (now - previous[0])
                row.update(cpu_cores=cores, cpu_host_percent=100 * cores / (os.cpu_count() or 1))
            memory = ProcessMemory()
            memory.cb = ctypes.sizeof(memory)
            if self.kernel.K32GetProcessMemoryInfo(handle, ctypes.byref(memory), memory.cb):
                row.update(working_set_bytes=memory.working_set, private_bytes=memory.private_usage)
            else:
                row['memory_read_error'] = ctypes.get_last_error()
            result[name] = row
        return result

    def close(self):
        for _, handle in self.slots.values():
            self.kernel.CloseHandle(handle)
        self.slots.clear()
