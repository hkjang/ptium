# Offline deployment

The release asset `ptium-<version>.tar.gz` is a gzip-compressed Docker image
archive holding `ptium:<version>` and the literal `ptium-<version>:latest`
alias, built for Linux/AMD64.

That image is the whole runtime: one static binary that serves the workspace,
the REST API and the MCP endpoint on port 8080. There is no reverse proxy, no
shell entrypoint and **no bundled database** — a deployment points
`DATABASE_URL` at the PostgreSQL cluster it already operates, whether that is a
pod, a managed service or an existing VM.

## Prepare on an internet-connected machine

Download these release assets and copy them to approved removable media:

- `ptium-<version>.tar.gz`
- `ptium-<version>.tar.gz.sha256`
- `docker-compose.ptium-<version>.yml`
- `ptium-<version>.env.example`
- `ptium-<version>.kubernetes.yaml`
- `load-ptium-<version>.ps1` / `load-ptium-<version>.sh`

Verify the SHA-256 checksum before crossing the network boundary.

To reproduce the same bundle from source on a build host:

```bash
./scripts/build-offline.sh          # Linux/macOS
```

```powershell
.\scripts\build-offline.ps1         # Windows
```

## Import on the target host

```powershell
.\load-ptium-0.2.0.ps1 -Archive .\ptium-0.2.0.tar.gz
```

```bash
./load-ptium-0.2.0.sh ptium-0.2.0.tar.gz
```

Both loaders verify the adjacent `.sha256` file and stop before import if it
does not match. Pass `-SkipChecksum` only when verification has already been
enforced by the network-transfer process. Without a helper:

```bash
sha256sum -c ptium-0.2.0.tar.gz.sha256
gzip -dc ptium-0.2.0.tar.gz | docker load
docker image inspect ptium-0.2.0:latest ptium:0.2.0 >/dev/null
```

## Provide the database

Ptium needs one PostgreSQL 14+ database and the privileges to create its own
tables; it applies forward-only, idempotent migrations on start. Create the
database and role on the existing cluster, then record the DSN:

```sql
CREATE ROLE ptium LOGIN PASSWORD 'a-long-random-password';
CREATE DATABASE ptium OWNER ptium;
```

```dotenv
DATABASE_URL=postgres://ptium:a-long-random-password@postgres.internal:5432/ptium?sslmode=require
```

Nothing else is required: templates, generated decks, settings, credentials and
incidents all live in that database, so the container itself is disposable.

## Start with Compose

Copy `ptium-<version>.env.example` to `.env`, set `DATABASE_URL` and replace
every remaining placeholder:

```bash
docker compose --env-file .env -f docker-compose.ptium-0.2.0.yml up -d
docker compose --env-file .env -f docker-compose.ptium-0.2.0.yml ps
curl --fail http://localhost:8080/readyz
```

Ptium is then available at `http://<host>:8080`.

## Start on Kubernetes

```bash
kubectl create secret generic ptium \
  --from-literal=DATABASE_URL='postgres://ptium:...@postgres:5432/ptium?sslmode=require' \
  --from-literal=KEY_ENCRYPTION_SECRET="$(openssl rand -base64 32)"
kubectl apply -f ptium-0.2.0.kubernetes.yaml
```

The manifest runs two replicas as a non-root user with a read-only root
filesystem. Every replica applies the same migrations and runs a generation
worker; work is claimed with `SELECT … FOR UPDATE SKIP LOCKED`, so scaling out
never generates a deck twice.

If an ingress controller fronts the service, raise its request-body limit to at
least the administrator-configured `generation.max_template_mb` (default 32
MiB) — template uploads carry a whole PowerPoint package. The bundled manifest
sets `proxy-body-size: 64m` for the nginx ingress controller.

## Authentication

The examples start safely with both OIDC and development authentication
disabled. Before users can sign in, configure the internal OIDC issuer/client
and at least one bootstrap administrator. Development authentication is only for
an isolated evaluation host and must not be exposed to an untrusted network.

For Keycloak, set the reachable realm issuer (for example
`https://sso.internal/realms/company`) and the public SPA client ID. Keycloak
must allow the exact Ptium origin and redirect URI. Discovery and JWKS refresh
happen automatically; the Keycloak container is not part of the bundle because
most offline environments already operate a central identity service.

## Upgrade

1. Back up the Ptium database.
2. Import the newer archive with `gzip -dc … | docker load`.
3. Set `PTIUM_VERSION` in `.env` (or the image tag in the manifest) to the new
   version.
4. Re-run `docker compose … up -d`, or `kubectl rollout restart deploy/ptium`.

Migrations are applied during start and are safe to run from several replicas at
once. Built-in templates are regenerated from code on every boot, so a new
release picks up design changes without any extra step.
