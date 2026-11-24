param(
  [string]$Version = "2.1.1",
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\tbible",
  [string]$BaseUri = "",
  [switch]$Source,
  [switch]$AddToPath
)

# tbible v2 installer for Windows.
#
# By default this downloads a prebuilt release binary and verifies it against
# the published SHA-256 manifest. Use -Source to build from a local checkout.

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = "petealeon/tbible"
$RepoRoot = Split-Path -Parent $PSScriptRoot
$Tmp = Join-Path ([IO.Path]::GetTempPath()) ("tbible-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Tmp -Force | Out-Null

try {
  if ($Source) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
      throw "Go 1.27+ is required for -Source. See https://go.dev/dl/"
    }
    Write-Host "== building tbible $Version from source =="
    Push-Location $RepoRoot
    try {
      $ldflags = "-s -w -X github.com/petealeon/tbible/v2/internal/version.Version=$Version"
      & go build -trimpath -ldflags $ldflags -o (Join-Path $Tmp "tbible.exe") ./cmd/tbible
      if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    }
    finally {
      Pop-Location
    }
  }
  else {
    $asset = "tbible-$Version-windows-amd64.zip"
    $manifest = "tbible-$Version-checksums.txt"
    $base = if ($BaseUri) { $BaseUri.TrimEnd("/") } else { "https://github.com/$Repo/releases/download/v$Version" }

    Write-Host "== downloading tbible $Version for windows/amd64 =="
    $assetPath = Join-Path $Tmp $asset
    $manifestPath = Join-Path $Tmp $manifest
    Invoke-WebRequest -Uri "$base/$asset" -OutFile $assetPath -UseBasicParsing
    Invoke-WebRequest -Uri "$base/$manifest" -OutFile $manifestPath -UseBasicParsing

    $expected = $null
    foreach ($line in (Get-Content $manifestPath)) {
      $parts = $line -split "\s+", 2
      if ($parts.Count -eq 2 -and $parts[1].TrimStart("*") -eq $asset) {
        $expected = $parts[0].ToLowerInvariant()
        break
      }
    }
    if (-not $expected) { throw "no checksum found for $asset" }

    $actual = (Get-FileHash -Path $assetPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "checksum mismatch for $asset" }

    $unpacked = Join-Path $Tmp "unpacked"
    Expand-Archive -Path $assetPath -DestinationPath $unpacked -Force
    $exe = Get-ChildItem -Path $unpacked -Filter "tbible.exe" -Recurse | Select-Object -First 1
    if (-not $exe) { throw "archive did not contain tbible.exe" }
    Copy-Item -Path $exe.FullName -Destination (Join-Path $Tmp "tbible.exe") -Force
  }

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Copy-Item -Path (Join-Path $Tmp "tbible.exe") -Destination (Join-Path $InstallDir "tbible.exe") -Force
  Write-Host "installed: $(Join-Path $InstallDir 'tbible.exe')"

  if ($AddToPath) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) { $userPath = "" }
    if ($userPath -notlike "*$InstallDir*") {
      $separator = if ($userPath.EndsWith(";") -or $userPath -eq "") { "" } else { ";" }
      [Environment]::SetEnvironmentVariable("Path", "$userPath$separator$InstallDir", "User")
      Write-Host "added to user PATH: $InstallDir (restart your terminal)"
    }
  }
  else {
    Write-Host "Add $InstallDir to your PATH to run tbible from anywhere."
  }

  & (Join-Path $InstallDir "tbible.exe") --version
}
finally {
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
