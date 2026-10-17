$ErrorActionPreference = "Stop"

$pilotRoot = Join-Path $env:LOCALAPPDATA "EUROMNI\pilot"
$appPath = Join-Path $pilotRoot "app"
$dataPath = Join-Path $pilotRoot "postgres-data"
$postgresBin = Join-Path $pilotRoot "postgres-binaries\bin"
$pgCtl = Join-Path $postgresBin "pg_ctl.exe"
$cmsPath = Join-Path $appPath "euromni-cms.exe"
$migratePath = Join-Path $appPath "euromni-db-migrate.exe"
$databaseCredentialPath = Join-Path $pilotRoot "postgres-credential.xml"
$ownerCredentialPath = Join-Path $pilotRoot "owner-credential.xml"
$databasePort = 55432
$cmsAddress = "127.0.0.1:18081"
$databaseUrl = $null

foreach ($requiredPath in @($pgCtl, $cmsPath, $migratePath, $databaseCredentialPath, $ownerCredentialPath)) {
  if (-not (Test-Path -LiteralPath $requiredPath -PathType Leaf)) {
    throw "Local pilot file is missing: $requiredPath"
  }
}

function Test-LocalPort([int]$Port) {
  $client = New-Object System.Net.Sockets.TcpClient
  $connect = $null
  try {
    $connect = $client.BeginConnect("127.0.0.1", $Port, $null, $null)
    if (-not $connect.AsyncWaitHandle.WaitOne(500)) {
      return $false
    }
    $client.EndConnect($connect)
    return $true
  } catch {
    return $false
  } finally {
    if ($null -ne $connect) {
      $connect.AsyncWaitHandle.Close()
    }
    $client.Close()
  }
}

function Test-CmsHealth {
  try {
    $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:18081/health" -TimeoutSec 2
    return $response.StatusCode -eq 200
  } catch {
    return $false
  }
}

function Get-PilotCmsProcess {
  Get-CimInstance Win32_Process -Filter "Name = 'euromni-cms.exe'" -ErrorAction SilentlyContinue |
    Where-Object { [string]::Equals($_.ExecutablePath, $cmsPath, [StringComparison]::OrdinalIgnoreCase) } |
    Select-Object -First 1
}

if (-not (Test-LocalPort $databasePort)) {
  $postgresLog = Join-Path $pilotRoot "postgres.log"
  $pgCtlOut = Join-Path $pilotRoot "pgctl-start.out"
  $pgCtlErr = Join-Path $pilotRoot "pgctl-start.err"
  $pgCtlArguments = 'start -D "{0}" -l "{1}" -o "-h 127.0.0.1 -p {2}" -w -t 60' -f $dataPath, $postgresLog, $databasePort
  Start-Process -FilePath $pgCtl -ArgumentList $pgCtlArguments `
    -RedirectStandardOutput $pgCtlOut -RedirectStandardError $pgCtlErr | Out-Null

  $databaseReady = $false
  for ($attempt = 0; $attempt -lt 120; $attempt++) {
    if (Test-LocalPort $databasePort) {
      $databaseReady = $true
      break
    }
    Start-Sleep -Milliseconds 500
  }
  if (-not $databaseReady) {
    Get-Content $pgCtlOut, $pgCtlErr -ErrorAction SilentlyContinue
    Get-Content $postgresLog -Tail 40 -ErrorAction SilentlyContinue
    throw "Local PostgreSQL did not start on 127.0.0.1:$databasePort"
  }
}

if (Test-CmsHealth) {
  $existingCms = Get-PilotCmsProcess
  if ($null -eq $existingCms) {
    throw "Port 18081 responds to an unexpected process; refusing to treat it as the EUROMNI pilot"
  }
  Set-Content -LiteralPath (Join-Path $pilotRoot "cms.pid") -Value $existingCms.ProcessId -Encoding ascii
  Write-Output "EUROMNI pilot is ready at http://127.0.0.1:18081/admin/"
  return
}
if (Test-LocalPort 18081) {
  throw "Port 18081 is already in use by another process"
}

$databaseCredential = Import-Clixml -LiteralPath $databaseCredentialPath
$databasePassword = $databaseCredential.GetNetworkCredential().Password
$databaseUrl = "postgres://euromni_pilot:$databasePassword@127.0.0.1:$databasePort/euromni_pilot?sslmode=disable&connect_timeout=5"
$env:DATABASE_URL = $databaseUrl
try {
  & $migratePath
  if ($LASTEXITCODE -ne 0) {
    throw "Schema migration failed with exit code $LASTEXITCODE"
  }

  $env:EUROMNI_ARTIFACT_DIR = Join-Path $pilotRoot "artifacts"
  $env:LISTEN_ADDR = $cmsAddress
  $cmsOut = Join-Path $pilotRoot "cms.out.log"
  $cmsErr = Join-Path $pilotRoot "cms.err.log"
  $cms = Start-Process -FilePath $cmsPath -WorkingDirectory $appPath -PassThru `
    -RedirectStandardOutput $cmsOut -RedirectStandardError $cmsErr
  $cmsReady = $false
  for ($attempt = 0; $attempt -lt 40; $attempt++) {
    if ($cms.HasExited) {
      Get-Content $cmsErr -Tail 40 -ErrorAction SilentlyContinue
      throw "EUROMNI CMS exited with code $($cms.ExitCode)"
    }
    if (Test-CmsHealth) {
      $cmsReady = $true
      break
    }
    Start-Sleep -Milliseconds 500
  }
  if (-not $cmsReady) {
    Get-Content $cmsErr -Tail 40 -ErrorAction SilentlyContinue
    Stop-Process -Id $cms.Id
    throw "EUROMNI CMS did not become healthy at http://127.0.0.1:18081"
  }

  Set-Content -LiteralPath (Join-Path $pilotRoot "cms.pid") -Value $cms.Id -Encoding ascii
  Write-Output "EUROMNI pilot is ready at http://127.0.0.1:18081/admin/"
} finally {
  Remove-Item Env:DATABASE_URL, Env:EUROMNI_ARTIFACT_DIR, Env:LISTEN_ADDR -ErrorAction SilentlyContinue
}
