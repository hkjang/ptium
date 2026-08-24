#!/usr/bin/env bash
# Builds the air-gapped release bundle: the Ptium image alone, plus the compose
# file, environment sample and Kubernetes manifest a target site needs.
# PostgreSQL is deliberately not bundled — the deployment supplies its own DSN.
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
platform="${PLATFORM:-linux/amd64}"
version="${1:-$(tr -d '[:space:]' < "$repository_root/VERSION")}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
    echo "Version '$version' is not a valid release version." >&2
    exit 1
fi

revision="$(git -C "$repository_root" rev-parse HEAD)"
if [[ ! "$revision" =~ ^[0-9a-f]{40}$ ]]; then
    echo "A committed Git revision is required before building the offline release." >&2
    exit 1
fi

image="ptium:$version"
alias_image="ptium-$version:latest"
dist="$repository_root/dist"
archive="$dist/ptium-$version.tar.gz"

mkdir -p "$dist"
cd "$repository_root"

docker buildx build --platform "$platform" --load \
    --build-arg "VERSION=$version" --build-arg "REVISION=$revision" \
    --tag "$image" --tag "$alias_image" .

# Stream straight into gzip so the uncompressed tar never touches the disk.
docker save "$image" "$alias_image" | gzip -9 > "$archive"

digest="$(sha256sum "$archive" | cut -d' ' -f1)"
printf '%s  %s' "$digest" "$(basename "$archive")" > "$archive.sha256"

cp "$repository_root/docker-compose.offline.yml" "$dist/docker-compose.ptium-$version.yml"
cp "$repository_root/.env.offline.example" "$dist/ptium-$version.env.example"
cp "$repository_root/deploy/kubernetes.yaml" "$dist/ptium-$version.kubernetes.yaml"
cp "$repository_root/scripts/load-offline.ps1" "$dist/load-ptium-$version.ps1"
cp "$repository_root/scripts/load-offline.sh" "$dist/load-ptium-$version.sh"

docker image inspect "$image" "$alias_image" > /dev/null

# What the archive holds is what a target site will run, so it is worth running
# once here: an empty database, the bootstrap administrator, and a deck.
# Every check below has to be able to stop the release. Piping this script's
# output through anything (tail, tee) hides its exit code, so it says so itself.
if command -v python3 > /dev/null; then
    # Exit 2 is the check saying it could not run — docker would not start a
    # container, the host would not give up a port. The release still stops,
    # because an unrun check has proved nothing, but it is not told it failed
    # something it was never asked.
    python3 "$repository_root/scripts/e2e/firstrun.py" --image "$image" || {
        status=$?
        if [ "$status" -eq 2 ]; then
            echo "The first-run check could not be run on this host; nothing was proved about the image." >&2
        else
            echo "The image that was just built does not come up on an empty database." >&2
        fi
        exit 1
    }
    # And on a database an older release wrote, which is what an upgrade runs.
    # Skips itself when no older image is on this host.
    python3 "$repository_root/scripts/e2e/upgrade.py" --to "$image" || {
        status=$?
        if [ "$status" -eq 2 ]; then
            echo "The upgrade check could not be run on this host; nothing was proved about the image." >&2
        else
            echo "The image that was just built does not open a database an older release wrote." >&2
        fi
        exit 1
    }
fi
printf 'Created %s\nSHA256  %s\n' "$archive" "$digest"
