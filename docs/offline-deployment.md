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
.\load-ptium-1.10.3.ps1 -Archive .\ptium-1.10.3.tar.gz
```

```bash
./load-ptium-1.10.3.sh ptium-1.10.3.tar.gz
```

Both loaders verify the adjacent `.sha256` file and stop before import if it
does not match. Pass `-SkipChecksum` only when verification has already been
enforced by the network-transfer process. Without a helper:

```bash
sha256sum -c ptium-1.10.3.tar.gz.sha256
gzip -dc ptium-1.10.3.tar.gz | docker load
docker image inspect ptium-1.10.3:latest ptium:1.10.3 >/dev/null
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

## Where uploaded images are kept

Everything Ptium stores is small except the pictures people upload for their
slides. By default those go into the database too, which keeps the deployment to
one thing to back up. A deployment with many or large images can put them on a
volume instead:

| | `ASSET_STORAGE=database` (default) | `ASSET_STORAGE=filesystem` |
| --- | --- | --- |
| Where the bytes live | The `assets` table | One file per image under `ASSET_DIR` |
| To back up | The database | The database **and** the volume |
| To mount | Nothing | A volume, writable by uid/gid 65532 |
| Replicas | Any number | Any number with a ReadWriteMany volume; one with ReadWriteOnce |

```dotenv
ASSET_STORAGE=filesystem
ASSET_DIR=/var/lib/ptium/assets
```

On Kubernetes, uncomment the `ptium-assets` PersistentVolumeClaim at the bottom
of the manifest along with the matching volume and volumeMount, and set
`ASSET_STORAGE` in the ConfigMap. On Compose the bundled file already declares
the `ptium-assets` volume, so switching is one variable in `.env`.

Ptium checks the directory at startup and refuses to start — naming the
directory and the reason — if it is missing, read-only, or owned by another
user. That is deliberate: a broken volume should stop a rollout, not the first
person who uploads a logo.

**Switching is safe in one direction.** Images uploaded while the bytes were in
the database keep working after the switch, and each one is moved onto the
volume the first time it is read, so the database empties itself as the pictures
get used. Going back to `database` afterwards means keeping that volume mounted,
because the rows no longer carry the bytes. Deleting the volume deletes the
images: Ptium then answers `410` with "this image's file is missing from the
image storage volume" instead of pretending the image is there.

## Start with Compose

Copy `ptium-<version>.env.example` to `.env`, set `DATABASE_URL` and replace
every remaining placeholder:

```bash
docker compose --env-file .env -f docker-compose.ptium-1.10.3.yml up -d
docker compose --env-file .env -f docker-compose.ptium-1.10.3.yml ps
curl --fail http://localhost:8080/readyz
```

Ptium is then available at `http://<host>:8080`.

## Start on Kubernetes

```bash
kubectl create secret generic ptium \
  --from-literal=DATABASE_URL='postgres://ptium:...@postgres:5432/ptium?sslmode=require' \
  --from-literal=KEY_ENCRYPTION_SECRET="$(openssl rand -base64 32)"
kubectl apply -f ptium-1.10.3.kubernetes.yaml
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
disabled. Development authentication is only for an isolated evaluation host and
must not be exposed to an untrusted network.

### First sign-in

An air-gapped installation usually has to be usable before anyone wires up the
identity service, so name the first administrator in the environment:

```dotenv
BOOTSTRAP_ADMIN=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=at-least-twelve-characters
BOOTSTRAP_ADMIN_NAME=Ptium Administrator
```

On Kubernetes put both values in the same secret as `DATABASE_URL`; the manifest
already reads them from there.

The account is created on the first start with a bcrypt hash, and the login page
then offers a username and password form. The password is recorded only once —
changing it in the product survives a restart, and the environment variable is
not read again. If it is lost, start once with
`BOOTSTRAP_ADMIN_PASSWORD_RESET=true` and remove that variable afterwards. Once
OIDC is in place, delete `BOOTSTRAP_ADMIN_PASSWORD` from the environment so the
password is no longer held in the deployment.

The session lives in an HttpOnly cookie, so it survives closing the tab, and it is
renewed once past half of `SESSION_LIFETIME` (default 12h) — an active person is
not signed out mid-task, while an idle session still lapses. Every session issued
before a password change stops working.

### OIDC

For Keycloak, set the reachable realm issuer (for example
`https://sso.internal/realms/company`) and the SPA client ID. Keycloak must allow
the exact Ptium origin and redirect URI. Discovery and JWKS refresh happen
automatically; the Keycloak container is not part of the bundle because most
offline environments already operate a central identity service.

If the realm client is confidential, add `OIDC_CLIENT_SECRET`. Ptium then
exchanges the authorization code itself instead of handing the secret to the
browser. Leave it unset for a public client.

## Upgrade

1. Back up the Ptium database, and the image volume if `ASSET_STORAGE=filesystem`.
2. Import the newer archive with `gzip -dc … | docker load`.
3. Set `PTIUM_VERSION` in `.env` (or the image tag in the manifest) to the new
   version.
4. Re-run `docker compose … up -d`, or `kubectl rollout restart deploy/ptium`.

Migrations are applied during start and are safe to run from several replicas at
once. Built-in templates are regenerated from code on every boot, so a new
release picks up design changes without any extra step.
