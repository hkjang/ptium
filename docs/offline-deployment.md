# Offline Docker deployment

The release asset `ptium-<version>.tar.gz` is a gzip-compressed Docker image
archive. It contains `ptium:<version>`, the literal `ptium-<version>:latest`
alias, and `postgres:16-alpine` for a fully
offline baseline installation on Linux/AMD64. The checksum file and
`docker-compose.offline.yml` are published beside it.

## Prepare on an internet-connected machine

Download these release assets and copy them to approved removable media:

- `ptium-<version>.tar.gz`
- `ptium-<version>.tar.gz.sha256`
- `docker-compose.ptium-<version>.yml`
- `ptium-<version>.env.example`
- `load-ptium-<version>.ps1`

Verify the SHA-256 checksum before crossing the network boundary.

## Import on a Windows Docker host

```powershell
.\load-ptium-0.1.0.ps1 -Archive .\ptium-0.1.0.tar.gz
```

The Windows loader automatically reads the adjacent
`ptium-0.1.0.tar.gz.sha256` file and stops before import if it does not match.
Use `-SkipChecksum` only when verification has already been enforced by the
network-transfer process.

On a Linux Docker host, no helper is required:

```bash
sha256sum -c ptium-0.1.0.tar.gz.sha256
gzip -dc ptium-0.1.0.tar.gz | docker load
docker image inspect ptium-0.1.0:latest ptium:0.1.0 postgres:16-alpine >/dev/null
```

## Configure and start

Copy `ptium-<version>.env.example` to `.env`, replace every placeholder and then
run with the versioned Compose file:

```bash
docker compose --env-file .env -f docker-compose.ptium-0.1.0.yml up -d
docker compose --env-file .env -f docker-compose.ptium-0.1.0.yml ps
curl --fail http://localhost:8080/readyz
```

Ptium is available at `http://<host>:8080`. PostgreSQL is intentionally not
published to the host. Its named volume survives container replacement.

The example starts safely with both OIDC and development authentication
disabled. Before users can sign in, configure the internal OIDC issuer/client
and at least one bootstrap administrator. Development authentication is only
for an isolated evaluation host and must not be exposed to an untrusted
network.

For Keycloak, set the reachable realm issuer (for example
`https://sso.internal/realms/company`) and the public SPA client ID. Keycloak must
allow the exact Ptium origin and redirect URI. Discovery and JWKS refresh happen
automatically; the Keycloak container itself is not part of the bundle because
most offline environments already operate a central identity service.

## Upgrade

1. Back up the `ptium-postgres` volume/database.
2. Import the newer archive with `gzip -dc ... | docker load`.
3. Set `PTIUM_VERSION` in `.env` to the new version.
4. Run `docker compose --env-file .env -f docker-compose.ptium-<version>.yml up -d`.

The Go service applies forward-only, idempotent database migrations during start.
