$ErrorActionPreference = 'Stop'
$operaBin = if ($env:OPERA_BIN) { $env:OPERA_BIN } else { 'opera.exe' }
foreach ($mode in @('chat', 'run', 'worker', 'headless')) {
  & $operaBin demo $mode --format jsonl
  if ($LASTEXITCODE -ne 0) { throw "Offline $mode failed" }
}
