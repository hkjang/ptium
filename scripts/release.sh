#!/usr/bin/env bash
# The release ritual, in one place, so it cannot be run half-way.
#
# What it does: checks the version is stamped in every file that carries it,
# builds the offline bundle (which runs the first-run and upgrade checks and
# stops the release if either fails), then pushes, tags and publishes with the
# seven files a target site downloads.
#
# It exists because doing this by hand went wrong: build-offline.sh exits 1 when
# a check fails, but its output was being read through `| tail`, and a
# pipeline's status is the last command's. A release went out on a build that
# had failed. Nothing here is piped, and every step's status stops the rest.
#
#     scripts/release.sh --dry-run     # everything up to the first push
#     scripts/release.sh               # and then push, tag, publish
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

dry_run=false
version=""
for argument in "$@"; do
    case "$argument" in
        --dry-run) dry_run=true ;;
        -*) echo "Unknown option: $argument" >&2; exit 2 ;;
        *) version="$argument" ;;
    esac
done
[ -n "$version" ] || version="$(tr -d '[:space:]' < VERSION)"
tag="v$version"
notes="docs/release-notes-$tag.md"

fail() { echo "✗ $*" >&2; exit 1; }

# The version is written in five places. A release whose manifest, schema or
# install note names another version is a release that lies about itself in the
# one place an operator reads it.
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || fail "'$version' is not a release version."
grep -q "service.version: $version" api/openapi.yaml || fail "api/openapi.yaml does not say service.version: $version"
grep -q "image: ptium:$version" deploy/kubernetes.yaml || fail "deploy/kubernetes.yaml does not run ptium:$version"
grep -q "ptium-$version.tar.gz" docs/offline-deployment.md || fail "docs/offline-deployment.md still installs another version"
[ -f "$notes" ] || fail "$notes is missing: a release says what changed."

# Nothing uncommitted, nothing already tagged, and the branch this project
# releases from.
[ -z "$(git status --porcelain)" ] || fail "the worktree has uncommitted changes."
branch="$(git rev-parse --abbrev-ref HEAD)"
[ "$branch" = "main" ] || fail "releases are cut from main; this is $branch."
! git rev-parse -q --verify "refs/tags/$tag" > /dev/null || fail "$tag already exists."

echo "── building $tag ──"
# Not piped, on purpose: see the note at the top of this file.
bash scripts/build-offline.sh "$version"

assets=(
    "dist/ptium-$version.tar.gz"
    "dist/ptium-$version.tar.gz.sha256"
    "dist/docker-compose.ptium-$version.yml"
    "dist/ptium-$version.env.example"
    "dist/load-ptium-$version.ps1"
    "dist/load-ptium-$version.sh"
    "dist/ptium-$version.kubernetes.yaml"
)
for asset in "${assets[@]}"; do
    [ -s "$asset" ] || fail "$asset was not built."
done

if [ "$dry_run" = true ]; then
    echo "── dry run: everything is ready for $tag, nothing was pushed ──"
    printf '   %s\n' "${assets[@]}"
    exit 0
fi

gh auth status --hostname github.com > /dev/null || fail "the GitHub CLI is not signed in."

echo "── publishing $tag ──"
git push origin main
git tag -a "$tag" -m "Ptium $tag"
git push origin "$tag"
gh release create "$tag" --repo hkjang/ptium --title "Ptium $tag" --notes-file "$notes" "${assets[@]}"
echo "── $tag published ──"
