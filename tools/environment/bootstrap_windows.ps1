# Windows 用户目录引导；只下载已锁定且摘要匹配的归档，不修改系统工具或全局 PATH。
[CmdletBinding()]
param([string[]]$Tools = @('uv', 'go', 'node', 'protoc', 'gh', 'dotnet', 'mongod'), [switch]$InitializePython, [switch]$BrowserTools)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$lock = Get-Content -LiteralPath (Join-Path $projectRoot 'conf/toolchain/windows-amd64.lock.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$prefix = [Environment]::ExpandEnvironmentVariables($lock.prefix)
$downloads = Join-Path $prefix 'downloads'
New-Item -ItemType Directory -Path $downloads -Force | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
foreach ($name in $Tools) {
    $spec = $lock.tools.$name
    if (-not $spec) { throw "Unknown locked tool: $name" }
    $archive = Join-Path $downloads ([IO.Path]::GetFileName(([uri]$spec.url).AbsolutePath))
    $algorithm = if ($spec.sha512) { 'SHA512' } else { 'SHA256' }
    $expected = if ($spec.sha512) { $spec.sha512 } else { $spec.sha256 }
    if (-not (Test-Path -LiteralPath $archive)) {
        Write-Output "Downloading $name $($spec.version)"
        Invoke-WebRequest -UseBasicParsing -Uri $spec.url -OutFile ($archive + '.partial') -TimeoutSec 180
        if ((Get-FileHash -LiteralPath ($archive + '.partial') -Algorithm $algorithm).Hash.ToLowerInvariant() -ne $expected) { throw "Archive checksum mismatch: $name" }
        Move-Item -LiteralPath ($archive + '.partial') -Destination $archive
    }
    if ((Get-FileHash -LiteralPath $archive -Algorithm $algorithm).Hash.ToLowerInvariant() -ne $expected) { throw "Cached archive checksum mismatch: $name" }
    $target = Join-Path $prefix $spec.directory
    $executable = Join-Path $prefix $spec.executable
    if (-not (Test-Path -LiteralPath $executable)) {
        if (Test-Path -LiteralPath $target) { throw "Incomplete installation requires review: $target" }
        if ($spec.format -eq 'nsis') {
            $extractor = Join-Path $env:ProgramFiles '7-Zip/7z.exe'
            if (-not (Test-Path -LiteralPath $extractor)) { throw 'QEMU portable extraction requires an existing 7-Zip installation; no installer is executed.' }
            & $extractor x $archive ('-o' + $target) -y | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'QEMU portable extraction failed' }
        } else { [IO.Compression.ZipFile]::ExtractToDirectory($archive, $target) }
    }
    if (-not (Test-Path -LiteralPath $executable)) { throw "Locked executable missing: $executable" }
    Write-Output "Verified $name $($spec.version): $executable"
}
if ($InitializePython) {
    $uv = Join-Path $prefix $lock.tools.uv.executable
    $env:UV_PYTHON_INSTALL_DIR = Join-Path $prefix 'python/managed'
    & $uv python install $lock.python_version --install-dir $env:UV_PYTHON_INSTALL_DIR --no-bin --no-registry
    if ($LASTEXITCODE -ne 0) { throw 'Locked Python installation failed' }
    $managedPython = & $uv python find --managed-python --no-project $lock.python_version
    if ($LASTEXITCODE -ne 0) { throw 'Locked managed Python not found' }
    $venv = Join-Path $prefix 'python/tools-venv'
    if (-not (Test-Path -LiteralPath (Join-Path $venv 'Scripts/python.exe'))) {
        & $uv venv $venv --python $managedPython --no-project
        if ($LASTEXITCODE -ne 0) { throw 'Isolated Python environment creation failed' }
    }
    & $uv pip install --python (Join-Path $venv 'Scripts/python.exe') --require-hashes --only-binary :all: -r (Join-Path $projectRoot 'conf/toolchain/python-tools.windows.requirements.lock')
    if ($LASTEXITCODE -ne 0) { throw 'Hash-locked Python package installation failed' }
    & (Join-Path $venv 'Scripts/python.exe') -I -c 'import platform,PIL; print(platform.python_version(), PIL.__version__)'
    if ($LASTEXITCODE -ne 0) { throw 'Locked Python runtime verification failed' }
    $initialize = @((Join-Path $projectRoot 'tools/dev'), 'python3', 'tools/environment/setup_windows.py')
    if ($BrowserTools) { $initialize += '--browser' }
    & (Join-Path $venv 'Scripts/python.exe') @initialize
    if ($LASTEXITCODE -ne 0) { throw 'Locked project generator initialization failed' }
}
