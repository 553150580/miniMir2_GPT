$ErrorActionPreference = 'Stop'
# 仅终止已经逐项列出的本轮测试Chrome；让Node自行保留内存证据并执行finally。
$qaNode = Get-CimInstance Win32_Process -Filter 'ProcessId = 19820'
if (-not $qaNode -or $qaNode.Name -ne 'node.exe' -or
    $qaNode.CommandLine -notlike '*verify_level_growth_browser.cjs --report-dir .work/level-growth-20261029/browser/run-02' -or
    ([DateTimeOffset]$qaNode.CreationDate).ToUnixTimeMilliseconds() -ne 1793285726831) {
    throw 'QA Node身份或创建时间变化，拒绝停止'
}
$qaChrome = Get-CimInstance Win32_Process -Filter 'ProcessId = 83088'
if (-not $qaChrome -or $qaChrome.Name -ne 'chrome.exe' -or $qaChrome.ParentProcessId -ne $qaNode.ProcessId -or
    ([DateTimeOffset]$qaChrome.CreationDate).ToUnixTimeMilliseconds() -ne 1793285727071 -or
    $qaChrome.CommandLine -notlike '*playwright_chromiumdev_profile-Q53Nwp*') {
    throw 'QA Chrome身份或创建时间变化，拒绝停止'
}
$qaAll = @(Get-CimInstance Win32_Process)
$qaIds = [System.Collections.Generic.HashSet[uint32]]::new()
[void]$qaIds.Add([uint32]$qaChrome.ProcessId)
do {
    $qaAdded = $false
    foreach ($qaProcess in $qaAll) {
        if ($qaIds.Contains([uint32]$qaProcess.ParentProcessId) -and $qaIds.Add([uint32]$qaProcess.ProcessId)) { $qaAdded = $true }
    }
} while ($qaAdded)
$qaSnapshot = @($qaAll | Where-Object { $qaIds.Contains([uint32]$_.ProcessId) } |
    Select-Object ProcessId, ParentProcessId, Name, CreationDate, CommandLine)
$qaPath = Join-Path $PSScriptRoot 'stop-owned-processes-before.json'
if (Test-Path -LiteralPath $qaPath) { throw '已有停止证据，拒绝覆盖' }
[System.IO.File]::WriteAllText($qaPath, ($qaSnapshot | ConvertTo-Json -Depth 4), [System.Text.UTF8Encoding]::new($false))
Stop-Process -Id $qaChrome.ProcessId -ErrorAction Stop
Write-Output '已精确停止run-02 Chrome根83088；Node/Python及主服务保持运行，让finally写完整报告。'
