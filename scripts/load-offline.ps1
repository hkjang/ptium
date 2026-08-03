[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Archive,
    [string]$Version,
    [string]$Checksum,
    [switch]$SkipChecksum
)

$ErrorActionPreference = "Stop"
$ArchivePath = (Resolve-Path $Archive).Path
if (-not $SkipChecksum) {
    $ChecksumPath = if ([string]::IsNullOrWhiteSpace($Checksum)) { "$ArchivePath.sha256" } else { (Resolve-Path $Checksum).Path }
    if (-not (Test-Path -LiteralPath $ChecksumPath -PathType Leaf)) {
        throw "Checksum file not found: $ChecksumPath. Copy the matching .sha256 asset or pass -SkipChecksum explicitly."
    }
    $ExpectedHash = ((Get-Content -LiteralPath $ChecksumPath -Raw).Trim() -split '\s+')[0].ToLowerInvariant()
    if ($ExpectedHash -notmatch '^[0-9a-f]{64}$') {
        throw "Checksum file does not start with a valid SHA-256 digest: $ChecksumPath"
    }
    $ActualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $ArchivePath).Hash.ToLowerInvariant()
    if ($ActualHash -ne $ExpectedHash) {
        throw "SHA-256 mismatch for $ArchivePath. Expected $ExpectedHash but found $ActualHash."
    }
    Write-Host "Verified SHA-256 $ActualHash."
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    if ((Split-Path -Leaf $ArchivePath) -match '^ptium-(.+)\.tar\.gz$') {
        $Version = $Matches[1]
    } else {
        throw "Pass -Version when the archive name is not ptium-<version>.tar.gz."
    }
}

$TemporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("ptium-load-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $TemporaryDirectory | Out-Null
$TarPath = Join-Path $TemporaryDirectory "images.tar"
try {
    $InputStream = [System.IO.File]::OpenRead($ArchivePath)
    try {
        $GzipStream = [System.IO.Compression.GZipStream]::new(
            $InputStream,
            [System.IO.Compression.CompressionMode]::Decompress
        )
        try {
            $OutputStream = [System.IO.File]::Create($TarPath)
            try { $GzipStream.CopyTo($OutputStream) } finally { $OutputStream.Dispose() }
        } finally { $GzipStream.Dispose() }
    } finally { $InputStream.Dispose() }

    docker load --input $TarPath
    if ($LASTEXITCODE -ne 0) { throw "Docker image import failed." }
    docker image inspect "ptium:$Version" "ptium-${Version}:latest" "postgres:16-alpine" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Expected offline images are missing after import." }
    Write-Host "Verified ptium:$Version, ptium-${Version}:latest and postgres:16-alpine."
} finally {
    if (Test-Path -LiteralPath $TemporaryDirectory) {
        Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force
    }
}
