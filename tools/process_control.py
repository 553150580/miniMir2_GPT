"""开发进程身份与优雅停止；Windows 用创建时钟和映像路径防 PID 复用，不强杀。"""
import ctypes
from ctypes import wintypes
import os
from pathlib import Path
import subprocess
import sys
import time


def windows_identity(pid):
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    kernel.GetProcessTimes.argtypes = [wintypes.HANDLE] + [ctypes.POINTER(wintypes.FILETIME)] * 4
    kernel.QueryFullProcessImageNameW.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
    kernel.GetExitCodeProcess.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
    handle = kernel.OpenProcess(0x1000, False, int(pid))
    if not handle:
        return None
    try:
        code = wintypes.DWORD()
        if not kernel.GetExitCodeProcess(handle, ctypes.byref(code)) or code.value != 259:
            return None
        created, exited, system, user = [wintypes.FILETIME() for _ in range(4)]
        if not kernel.GetProcessTimes(handle, ctypes.byref(created), ctypes.byref(exited), ctypes.byref(system), ctypes.byref(user)):
            return None
        buffer = ctypes.create_unicode_buffer(32768)
        size = wintypes.DWORD(len(buffer))
        if not kernel.QueryFullProcessImageNameW(handle, 0, buffer, ctypes.byref(size)):
            return None
        return dict(pid=int(pid), executable=buffer.value, start_ticks=(created.dwHighDateTime << 32) | created.dwLowDateTime)
    finally:
        kernel.CloseHandle(handle)


def windows_owned(meta, executable):
    if not meta or not meta.get('start_ticks'):
        return False
    current = windows_identity(meta['pid'])
    return bool(current and current['start_ticks'] == meta['start_ticks']
                and os.path.normcase(current['executable']) == os.path.normcase(str(Path(executable).resolve())))


def background_options():
    if os.name != 'nt':
        return dict(start_new_session=True)
    startup = subprocess.STARTUPINFO()
    startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
    startup.wShowWindow = subprocess.SW_HIDE
    return dict(creationflags=subprocess.CREATE_NEW_CONSOLE, startupinfo=startup)


def windows_stop(meta, executable):
    if not windows_owned(meta, executable):
        raise RuntimeError('停止前进程身份已变化，未发送控制事件')
    subprocess.run([sys.executable, str(Path(__file__).resolve()), str(meta['pid']), str(meta['start_ticks']), str(executable)], check=True)


def signal_main():
    pid, ticks = map(int, sys.argv[1:3])
    if not windows_owned(dict(pid=pid, start_ticks=ticks), sys.argv[3]):
        raise RuntimeError('进程身份不符，拒绝附着控制台')
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    kernel.FreeConsole()
    if not kernel.AttachConsole(pid):
        raise ctypes.WinError(ctypes.get_last_error())
    handler_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.DWORD)
    handler = handler_type(lambda event: True)
    kernel.SetConsoleCtrlHandler.argtypes = [handler_type, wintypes.BOOL]
    try:
        kernel.SetConsoleCtrlHandler(handler, True)
        if not kernel.GenerateConsoleCtrlEvent(1, pid):
            # CREATE_NEW_CONSOLE 的根不一定有同 PID 控制组；只对已核对的专属控制台广播。
            members = (wintypes.DWORD * 128)()
            count = kernel.GetConsoleProcessList(members, len(members))
            if count == 0 or count > len(members):
                raise RuntimeError('无法完整核对本任务独立控制台成员')
            for member in members[:count]:
                if member in (pid, os.getpid()):
                    continue
                info = windows_identity(member)
                if not info or info['start_ticks'] < ticks or os.path.normcase(info['executable']) != os.path.normcase(str(Path(sys.argv[3]).resolve())):
                    raise RuntimeError('独立控制台存在未登记进程，拒绝广播')
            if not kernel.GenerateConsoleCtrlEvent(1, 0):
                raise ctypes.WinError(ctypes.get_last_error())
        time.sleep(0.2)
    finally:
        kernel.FreeConsole()


if __name__ == '__main__':
    signal_main()
