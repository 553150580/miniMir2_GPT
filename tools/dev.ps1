# 与 POSIX tools/dev 共用分发器；引导完成前拒绝回退到系统旧 Python。
$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$lock = Get-Content -LiteralPath (Join-Path $projectRoot 'conf/toolchain/windows-amd64.lock.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$prefix = [Environment]::ExpandEnvironmentVariables($lock.prefix)
$python = Join-Path $prefix 'python/tools-venv/Scripts/python.exe'
if (-not (Test-Path -LiteralPath $python)) { throw 'Run tools/environment/bootstrap_windows.ps1 and initialize the locked Python environment first.' }
# 使用 CRT 参数规则，兼容 Windows PowerShell 5.1 的引号、中文、空参数和尾部反斜杠。
function ConvertTo-CrtArgument([string]$Value) {
    if ($Value.Length -gt 0 -and $Value -notmatch '[\s"]') { return $Value }
    $result = '"'
    $slashes = 0
    foreach ($character in $Value.ToCharArray()) {
        if ($character -eq '\') { $slashes++; continue }
        if ($character -eq '"') { $result += ('\' * ($slashes * 2 + 1)) + '"' }
        else { $result += ('\' * $slashes) + $character }
        $slashes = 0
    }
    return $result + ('\' * ($slashes * 2)) + '"'
}
$start = New-Object System.Diagnostics.ProcessStartInfo
$start.FileName = $python
$start.UseShellExecute = $false
$start.Arguments = ((@((Join-Path $PSScriptRoot 'dev')) + @($args)) | ForEach-Object { ConvertTo-CrtArgument $_ }) -join ' '
$start.EnvironmentVariables['PYTHONUTF8'] = '1'
$start.EnvironmentVariables['PYTHONIOENCODING'] = 'utf-8'
$start.RedirectStandardInput = $MyInvocation.ExpectingInput
$child = [Diagnostics.Process]::Start($start)
if ($start.RedirectStandardInput) {
    foreach ($line in $input) { $child.StandardInput.WriteLine($line) }
    $child.StandardInput.Close()
}
$child.WaitForExit()
exit $child.ExitCode
