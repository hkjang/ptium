#!/usr/bin/env bash
# Verifies and loads an offline Ptium bundle on a host with no registry access.
set -euo pipefail

archive="${1:-}"
if [[ -z "$archive" || ! -f "$archive" ]]; then
    echo "Usage: load-offline.sh <ptium-VERSION.tar.gz>" >&2
    exit 1
fi

checksum_file="$archive.sha256"
if [[ -f "$checksum_file" ]]; then
    expected="$(cut -d' ' -f1 < "$checksum_file")"
    actual="$(sha256sum "$archive" | cut -d' ' -f1)"
    if [[ "$expected" != "$actual" ]]; then
        echo "Checksum mismatch for $archive." >&2
        echo "  expected $expected" >&2
        echo "  actual   $actual" >&2
        exit 1
    fi
    echo "Checksum verified: $actual"
else
    echo "Warning: $checksum_file is missing; loading without verification." >&2
fi

docker load --input "$archive"
docker images --format '{{.Repository}}:{{.Tag}}\t{{.Size}}' | grep '^ptium' || true

cat <<'NEXT'

Next steps:
  1. Copy the environment sample to .env and set DATABASE_URL for your PostgreSQL cluster.
  2. docker compose -f docker-compose.ptium-<version>.yml --env-file .env up -d
     or, on Kubernetes, create the ptium secret and apply ptium-<version>.kubernetes.yaml.
NEXT
