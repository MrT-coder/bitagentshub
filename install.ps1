# Installs BitAgentsHub from its GitHub Releases.
#
#   irm https://raw.githubusercontent.com/MrT-coder/bitagentshub/master/install.ps1 | iex
#
# Environment:
#   BAH_VERSION      tag to install, e.g. v0.2.0 (default: latest release)
#   BAH_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\bitagentshub\bin)
#   GITHUB_TOKEN     optional, avoids GitHub API rate limits

# Runs in its own scope so `irm | iex` leaves the caller's session as it was.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'MrT-coder/bitagentshub'
    $version = $env:BAH_VERSION
    $installDir = $env:BAH_INSTALL_DIR
    if (-not $installDir) { $installDir = Join-Path $env:LOCALAPPDATA 'bitagentshub\bin' }

    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { throw "bah: unsupported architecture $arch" }
    }

    $headers = @{ 'User-Agent' = 'bah-installer'; 'Accept' = 'application/vnd.github+json' }
    if ($env:GITHUB_TOKEN) { $headers['Authorization'] = "Bearer $env:GITHUB_TOKEN" }

    $releaseUrl = if ($version) { "https://api.github.com/repos/$repo/releases/tags/$version" } else { "https://api.github.com/repos/$repo/releases/latest" }
    try {
        $release = Invoke-RestMethod -Uri $releaseUrl -Headers $headers
    } catch {
        throw "bah: cannot read release $releaseUrl (rate limited? set GITHUB_TOKEN): $($_.Exception.Message)"
    }

    $archive = $release.assets | Where-Object { $_.name -like "bah_*_windows_$arch.zip" } | Select-Object -First 1
    $sums = $release.assets | Where-Object { $_.name -eq 'checksums.txt' } | Select-Object -First 1
    if (-not $archive -or -not $sums) { throw "bah: release $($release.tag_name) has no windows_$arch archive" }

    # Asset API URLs work for public and private repositories; the download host
    # redirects to storage and the Authorization header is not forwarded there.
    function Get-Asset($asset, $path) {
        $h = $headers.Clone()
        $h['Accept'] = 'application/octet-stream'
        Invoke-WebRequest -Uri $asset.url -Headers $h -OutFile $path -UseBasicParsing
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("bah-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $zip = Join-Path $tmp $archive.name
        $sumsFile = Join-Path $tmp 'checksums.txt'
        Write-Host "Downloading BitAgentsHub $($release.tag_name) (windows/$arch)..."
        Get-Asset $archive $zip
        Get-Asset $sums $sumsFile

        $line = Get-Content $sumsFile | Where-Object { $_ -match "\s$([regex]::Escape($archive.name))$" } | Select-Object -First 1
        if (-not $line) { throw "bah: $($archive.name) is missing from checksums.txt" }
        $expected = ($line -split '\s+')[0].ToLower()
        $actual = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLower()
        if ($expected -ne $actual) { throw "bah: checksum mismatch for $($archive.name)" }

        Expand-Archive -Path $zip -DestinationPath (Join-Path $tmp 'x') -Force
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        $target = Join-Path $installDir 'bah.exe'
        # A running exe cannot be overwritten but can be renamed; leftovers from
        # earlier upgrades go once nothing runs them anymore.
        Get-ChildItem $installDir -Filter 'bah.exe.*old' | Remove-Item -Force -ErrorAction SilentlyContinue
        if (Test-Path $target) {
            $old = "$target.old"
            if (Test-Path $old) { $old = "$target.$([Guid]::NewGuid().ToString('N')).old" }
            Move-Item $target $old
        }
        Copy-Item (Join-Path $tmp 'x\bah.exe') $target
    } finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not (($userPath -split ';') -contains $installDir)) {
        $newPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "Added $installDir to your user PATH (open a new terminal)."
    }
    if (-not (($env:Path -split ';') -contains $installDir)) { $env:Path = "$env:Path;$installDir" }

    Write-Host "Installed $(& $target -version) to $target"
}
