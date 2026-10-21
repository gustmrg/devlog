#Requires -Version 5.1
$ErrorActionPreference = 'Stop'

$Repository = 'gustmrg/devlog'
$Binary = 'devlog.exe'
$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\devlog'
$ApiRoot = "https://api.github.com/repos/$Repository"

if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
    Write-Error "unsupported architecture: $env:PROCESSOR_ARCHITECTURE (Windows builds are amd64 only)"
    exit 1
}
$Arch = 'amd64'

$TmpDir = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir | Out-Null
try {
    $Release = Invoke-RestMethod -Uri "$ApiRoot/releases/latest" -Headers @{ 'User-Agent' = 'devlog-installer' }
    $Version = $Release.tag_name -replace '^v', ''
    if (-not $Version) { throw 'could not determine the latest release version' }

    $Archive = "devlog_${Version}_windows_${Arch}.zip"
    Write-Host "Downloading DevLog v$Version for windows/$Arch..."

    function Download-Asset($Name, $Destination) {
        $Asset = $Release.assets | Where-Object { $_.name -eq $Name } | Select-Object -First 1
        if (-not $Asset) { throw "release asset was not found: $Name" }
        Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $Destination
    }

    Download-Asset $Archive (Join-Path $TmpDir $Archive)
    $ChecksumsFile = Join-Path $TmpDir 'checksums.txt'
    Download-Asset 'checksums.txt' $ChecksumsFile

    $Expected = Get-Content $ChecksumsFile |
        Where-Object { $_ -match "\s$([regex]::Escape($Archive))$" } |
        ForEach-Object { ($_ -split '\s+')[0] } |
        Select-Object -First 1
    if (-not $Expected) { throw 'archive checksum was not found' }

    $Actual = (Get-FileHash -Algorithm SHA256 (Join-Path $TmpDir $Archive)).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected.ToLowerInvariant()) { throw 'checksum verification failed' }

    Expand-Archive -Path (Join-Path $TmpDir $Archive) -DestinationPath $TmpDir -Force
    $Extracted = Join-Path $TmpDir $Binary
    if (-not (Test-Path $Extracted)) { throw "release archive did not contain $Binary" }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item $Extracted (Join-Path $InstallDir $Binary) -Force

    $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($UserPath -split ';') -notcontains $InstallDir) {
        [Environment]::SetEnvironmentVariable('Path', "$UserPath;$InstallDir", 'User')
        Write-Host "Added $InstallDir to your user PATH (restart your terminal to pick it up)."
    }

    Write-Host "Installed DevLog v$Version to $(Join-Path $InstallDir $Binary)"
    if (-not (Test-Path (Join-Path $HOME '.devlog\config.json'))) {
        Write-Host "Next step: run 'devlog setup' to configure DevLog on this device."
    }
}
finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
