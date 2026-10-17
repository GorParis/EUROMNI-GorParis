$ErrorActionPreference = "Stop"

$pilotRoot = Join-Path $env:LOCALAPPDATA "EUROMNI\pilot"
$cmsPath = Join-Path $pilotRoot "app\euromni-cms.exe"
$cmsPidPath = Join-Path $pilotRoot "cms.pid"
$dataPath = Join-Path $pilotRoot "postgres-data"
$pgCtl = Join-Path $pilotRoot "postgres-binaries\bin\pg_ctl.exe"

if (Test-Path -LiteralPath $cmsPidPath -PathType Leaf) {
  $cmsPid = 0
  if (-not [int]::TryParse((Get-Content -LiteralPath $cmsPidPath -Raw).Trim(), [ref]$cmsPid)) {
    throw "Invalid CMS process ID in $cmsPidPath"
  }
  $process = Get-CimInstance Win32_Process -Filter "ProcessId = $cmsPid" -ErrorAction SilentlyContinue
  if ($null -ne $process -and [string]::Equals($process.ExecutablePath, $cmsPath, [StringComparison]::OrdinalIgnoreCase)) {
    Stop-Process -Id $cmsPid
    Write-Output "Stopped EUROMNI CMS process $cmsPid."
  }
  Remove-Item -LiteralPath $cmsPidPath -Force
}

if ((Test-Path -LiteralPath $pgCtl -PathType Leaf) -and (Test-Path -LiteralPath $dataPath -PathType Container)) {
  $status = & $pgCtl status -D $dataPath 2>&1
  $statusCode = $LASTEXITCODE
  if ($statusCode -eq 0) {
    & $pgCtl stop -D $dataPath -m fast -w -t 30
    if ($LASTEXITCODE -ne 0) {
      throw "Could not stop the local PostgreSQL pilot (pg_ctl exit $LASTEXITCODE)"
    }
  } elseif ($statusCode -eq 3) {
    Write-Output "Local PostgreSQL pilot is already stopped."
  } else {
    $status
    throw "Could not inspect the local PostgreSQL pilot (pg_ctl exit $statusCode)"
  }
}
