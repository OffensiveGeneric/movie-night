<#
Movie Night installer and updater for Windows.

    irm https://raw.githubusercontent.com/OffensiveGeneric/movie-night/main/scripts/install.ps1 | iex

Downloads the newest release, checks it against the release's SHA256SUMS,
installs it to %LOCALAPPDATA%\MovieNight with a "Movie Night" desktop
shortcut, then:
  - on a computer that isn't set up yet, starts "movienight setup";
  - otherwise updates Syncthing, Syncplay and VLC (skipped while you're
    watching) and restarts Movie Night if it was running.

To join someone without being asked anything, set their code (and your
name) first:

    $env:MOVIENIGHT_JOIN = 'MN1-...'; $env:MOVIENIGHT_NAME = 'Dad'
    irm https://raw.githubusercontent.com/OffensiveGeneric/movie-night/main/scripts/install.ps1 | iex

Run it again any time to update. Works in Windows PowerShell 5.1 and 7.
#>
param(
    [switch]$SkipDeps  # don't update Syncthing/Syncplay/VLC
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$repo = 'OffensiveGeneric/movie-night'
$installed = Join-Path $env:LOCALAPPDATA 'MovieNight\movienight.exe'
$headers = @{ 'User-Agent' = 'movienight-installer' }

Write-Host 'Looking for the newest Movie Night...'
# Assign first: Windows PowerShell 5.1 passes a JSON array down the pipeline
# as one object, so piping Invoke-RestMethod straight into Where-Object
# would filter the whole list at once.
$releases = Invoke-RestMethod "https://api.github.com/repos/$repo/releases?per_page=10" -Headers $headers
$release = $releases | Where-Object { -not $_.draft } | Select-Object -First 1
if (-not $release) { throw "No releases found for $repo." }
$exeAsset = $release.assets | Where-Object { $_.name -eq 'movienight.exe' }
$sumAsset = $release.assets | Where-Object { $_.name -eq 'SHA256SUMS' }
if (-not $exeAsset -or -not $sumAsset) { throw "Release $($release.tag_name) is missing movienight.exe or SHA256SUMS." }

# Download into a fresh, empty folder and check it before running anything.
$work = Join-Path ([IO.Path]::GetTempPath()) ('movienight-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
try {
    $exe = Join-Path $work 'movienight.exe'
    $sumFile = Join-Path $work 'SHA256SUMS'
    Invoke-WebRequest $exeAsset.browser_download_url -OutFile $exe -UseBasicParsing -Headers $headers
    Invoke-WebRequest $sumAsset.browser_download_url -OutFile $sumFile -UseBasicParsing -Headers $headers
    $expected = Get-Content $sumFile | Where-Object { $_ -match '\s\*?movienight\.exe\s*$' } |
        ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1
    $actual = (Get-FileHash $exe -Algorithm SHA256).Hash
    if (-not $expected -or $actual -ne $expected) {
        throw "Checksum mismatch for movienight.exe (expected $expected, got $actual). Nothing was installed."
    }
    Write-Host "Got $($release.tag_name) (checksum OK)."
    Unblock-File $exe

    # A running copy can't be replaced; stop it and start it again afterwards.
    $wasRunning = @(Get-Process movienight -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and $_.Path -eq $installed }).Count -gt 0
    if ($wasRunning) {
        Write-Host 'Closing Movie Night to update it...'
        Get-Process movienight | Where-Object { $_.Path -eq $installed } | Stop-Process -Force
        Start-Sleep -Seconds 1
    }

    & $exe install
    if ($LASTEXITCODE -ne 0) { throw 'Installing Movie Night failed.' }
}
finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}

$config = Join-Path $env:APPDATA 'movienight\config.json'
if ($env:MOVIENIGHT_JOIN) {
    # Pairing code given up front: join without any questions.
    $joinArgs = @('setup', 'join', $env:MOVIENIGHT_JOIN)
    if ($env:MOVIENIGHT_NAME) { $joinArgs += @('-name', $env:MOVIENIGHT_NAME) }
    Write-Host ''
    Write-Host 'Pairing with the code you were sent...'
    '' | & $installed @joinArgs
    $setupFailed = $LASTEXITCODE -ne 0
}
elseif (-not (Test-Path $config)) {
    Write-Host ''
    Write-Host "This computer isn't set up yet. Starting setup..."
    & $installed setup
    $setupFailed = $LASTEXITCODE -ne 0
}
elseif (-not $SkipDeps) {
    & $installed update
}

if ($wasRunning) {
    Start-Process $installed
}
Write-Host ''
if ($setupFailed) {
    Write-Warning 'Movie Night is installed, but setup did not finish; see the messages above.'
    exit 1
}
Write-Host 'Done. Start Movie Night from the "Movie Night" shortcut on your desktop.'
