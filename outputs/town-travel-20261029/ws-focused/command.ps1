$townPreviousE2E = $env:MINIMIR_MONGO_E2E
$env:MINIMIR_MONGO_E2E = '1'
$townTestExit = 1
try {
  .\tools\dev.ps1 go test -race -count=1 ./internal/integration -run '^TestMapTravelMongoWS$' -v
  $townTestExit = $LASTEXITCODE
} finally {
  if ($null -eq $townPreviousE2E) { Remove-Item Env:MINIMIR_MONGO_E2E -ErrorAction SilentlyContinue } else { $env:MINIMIR_MONGO_E2E = $townPreviousE2E }
}
exit $townTestExit
