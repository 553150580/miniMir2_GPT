# 本文件保存已终态通过的会话59259实际命令；归档后未重跑Mongo用例。
$safeZoneOldE2E = $env:MINIMIR_MONGO_E2E
$env:MINIMIR_MONGO_E2E = '1'
try {
    .\tools\dev.ps1 go test -race -count=1 ./internal/integration -run '^TestMapTravelMongoWS$' -v
    $safeZoneTestExit = $LASTEXITCODE
} finally {
    $env:MINIMIR_MONGO_E2E = $safeZoneOldE2E
}
exit $safeZoneTestExit
