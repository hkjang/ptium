[CmdletBinding()]
param(
    [string]$Version,
    [ValidateSet("linux/amd64")]
    [string]$Platform = "linux/amd64"
)

$ErrorActionPreference = "Stop"
$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = (Get-Content (Join-Path $RepositoryRoot "VERSION") -Raw).Trim()
}
if ($Version -notmatch '^\d+\.\d+\.\d+([.-][0-9A-Za-z.-]+)?$') {
    throw "Version '$Version' is not a valid release version."
}

$DistDirectory = Join-Path $RepositoryRoot "dist"
New-Item -ItemType Directory -Force -Path $DistDirectory | Out-Null
$Image = "ptium:$Version"
$VersionedAlias = "ptium-${Version}:latest"
$PostgresImage = "postgres:16-alpine"
$TarPath = Join-Path $DistDirectory "ptium-$Version.tar"
$ArchivePath = "$TarPath.gz"
$ChecksumPath = "$ArchivePath.sha256"
$OfflineComposePath = Join-Path $DistDirectory "docker-compose.ptium-$Version.yml"
$OfflineEnvPath = Join-Path $DistDirectory "ptium-$Version.env.example"
$OfflineLoaderPath = Join-Path $DistDirectory "load-ptium-$Version.ps1"
$Revision = (git -C $RepositoryRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $Revision -notmatch '^[0-9a-f]{40}$') {
    throw "A committed Git revision is required before building the offline release."
}

Push-Location $RepositoryRoot
try {
    docker buildx build --platform $Platform --load --build-arg "VERSION=$Version" --build-arg "REVISION=$Revision" --tag $Image --tag $VersionedAlias .
    if ($LASTEXITCODE -ne 0) { throw "Ptium image build failed." }

    docker pull --platform $Platform $PostgresImage
    if ($LASTEXITCODE -ne 0) { throw "PostgreSQL image pull failed." }

    docker save --output $TarPath $Image $VersionedAlias $PostgresImage
    if ($LASTEXITCODE -ne 0) { throw "Docker image export failed." }

    $InputStream = [System.IO.File]::OpenRead($TarPath)
    try {
        $OutputStream = [System.IO.File]::Create($ArchivePath)
        try {
            $GzipStream = [System.IO.Compression.GZipStream]::new(
                $OutputStream,
                [System.IO.Compression.CompressionLevel]::SmallestSize
            )
            try { $InputStream.CopyTo($GzipStream) } finally { $GzipStream.Dispose() }
        } finally { $OutputStream.Dispose() }
    } finally { $InputStream.Dispose() }

    Remove-Item -LiteralPath $TarPath
    $Hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $ArchivePath).Hash.ToLowerInvariant()
    "$Hash  $(Split-Path -Leaf $ArchivePath)" | Set-Content -NoNewline -Encoding ascii $ChecksumPath
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot "docker-compose.offline.yml") -Destination $OfflineComposePath -Force
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot ".env.offline.example") -Destination $OfflineEnvPath -Force
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot "scripts/load-offline.ps1") -Destination $OfflineLoaderPath -Force

    docker image inspect $Image $VersionedAlias | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Built image inspection failed." }
    Write-Host "Created $ArchivePath"
    Write-Host "SHA256 $Hash"
} finally {
    Pop-Location
}
