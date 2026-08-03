[CmdletBinding()]
param(
    [string]$Version,
    [string]$Repository = "hkjang/ptium"
)

$ErrorActionPreference = "Stop"
$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = (Get-Content (Join-Path $RepositoryRoot "VERSION") -Raw).Trim()
}
$Tag = "v$Version"
$Notes = Join-Path $RepositoryRoot "docs/release-notes-v$Version.md"
$Assets = @(
    (Join-Path $RepositoryRoot "dist/ptium-$Version.tar.gz"),
    (Join-Path $RepositoryRoot "dist/ptium-$Version.tar.gz.sha256"),
    (Join-Path $RepositoryRoot "dist/docker-compose.ptium-$Version.yml"),
    (Join-Path $RepositoryRoot "dist/ptium-$Version.env.example"),
    (Join-Path $RepositoryRoot "dist/load-ptium-$Version.ps1")
)

foreach ($Path in @($Notes) + $Assets) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Required release file is missing: $Path"
    }
}

Push-Location $RepositoryRoot
try {
    if ((git status --porcelain).Length -ne 0) {
        throw "The worktree must be clean before publishing a release."
    }
    gh auth status --hostname github.com
    if ($LASTEXITCODE -ne 0) { throw "GitHub CLI is not authenticated." }

    git push origin main
    if ($LASTEXITCODE -ne 0) { throw "Pushing main failed." }

    git rev-parse --verify --quiet "refs/tags/$Tag" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        git tag -a $Tag -m "Ptium $Tag"
        if ($LASTEXITCODE -ne 0) { throw "Creating tag $Tag failed." }
    }
    git push origin $Tag
    if ($LASTEXITCODE -ne 0) { throw "Pushing tag $Tag failed." }

    gh release view $Tag --repo $Repository | Out-Null
    if ($LASTEXITCODE -eq 0) {
        gh release upload $Tag @Assets --repo $Repository --clobber
    } else {
        gh release create $Tag @Assets --repo $Repository --title "Ptium $Tag" --notes-file $Notes --verify-tag
    }
    if ($LASTEXITCODE -ne 0) { throw "Publishing GitHub Release $Tag failed." }
    gh release view $Tag --repo $Repository --json url,tagName,name
} finally {
    Pop-Location
}

