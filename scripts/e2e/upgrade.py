"""A database an old release wrote, opened by the one being shipped.

firstrun.py covers the first day of a deployment. This covers every day after
it: the operator already has decks, and the upgrade note in
docs/offline-deployment.md tells them to load the new image and restart. What
that runs is the migrations, against data written by a schema that is
fifty-odd migrations behind.

    python3 scripts/e2e/upgrade.py                       # oldest image present → VERSION
    python3 scripts/e2e/upgrade.py --from ptium:0.9.0

Skips itself when the older image is not on this host: a fresh clone has only
the image it just built, and a check that cannot run should say so rather than
fail.
"""
import argparse, http.cookiejar, json, os, random, socket, subprocess, sys, time, urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--from", dest="old", help="the image to write the database with")
parser.add_argument("--to", dest="new", help="the image to upgrade to (default: ptium:<VERSION>)")
parser.add_argument("--postgres", default="postgres:16-alpine")
options = parser.parse_args()

root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
version = open(os.path.join(root, "VERSION")).read().strip()
new_image = options.new or f"ptium:{version}"
failures = []


def docker(*arguments, check=True):
    result = subprocess.run(["docker", *arguments], capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"docker {' '.join(arguments)}\n{result.stderr.strip()}")
        sys.exit(2)
    return result.stdout.strip()


def oldest_image():
    tags = [line for line in docker("images", "ptium", "--format", "{{.Tag}}").splitlines()
            if line and line[0].isdigit() and line != version]
    if not tags:
        return ""
    tags.sort(key=lambda tag: [int(part) if part.isdigit() else 0 for part in tag.split(".")])
    return f"ptium:{tags[0]}"


owner_moved = False
old_image = options.old or oldest_image()
if not old_image:
    print("no older ptium image on this host: nothing to upgrade from")
    sys.exit(0)


def free_port():
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


def run_container(*arguments):
    """Starts a container, retrying once on a port the host cannot publish.

    Docker on a virtualised host occasionally answers "ports are not available"
    for a port nothing is using. A check that fails for that reason is reporting
    on the host, not on the image, and a release stopped by it is stopped for
    nothing."""
    name = arguments[arguments.index("--name") + 1] if "--name" in arguments else None

    def reserve():
        # A container a killed run left behind holds the name, and docker then
        # refuses to start this one. That is the check's own mess, not a verdict
        # on the image. The retry below needs this as much as the first attempt
        # does: a run that fails on the port has already created the container,
        # so retrying without clearing the name fails on the name instead — and
        # the port error that started it is never printed.
        if name:
            subprocess.run(["docker", "rm", "-f", name], capture_output=True, text=True)

    reserve()
    result = subprocess.run(["docker", *arguments], capture_output=True, text=True)
    if result.returncode == 0:
        return published_port(arguments)
    if "ports are not available" not in result.stderr:
        print(f"docker {' '.join(arguments)}\n{result.stderr.strip()}")
        # Exit 2 says the check could not be run. Exit 1 says the image failed
        # it, and saying that when docker would not start a container accuses
        # the release of something it did not do.
        sys.exit(2)
    # The port is the thing that failed, so the retry does not ask for it again.
    # Docker's forwarder answers 500 for a port nothing is using and keeps
    # answering it for that port; another port works straight away.
    attempts = list(arguments)
    for attempt in range(3):
        print(f"docker {' '.join(attempts)}\n{result.stderr.strip()}\n   retrying on another port")
        time.sleep(2)
        attempts = with_another_port(attempts)
        reserve()
        result = subprocess.run(["docker", *attempts], capture_output=True, text=True)
        if result.returncode == 0:
            return published_port(attempts)
        if "ports are not available" not in result.stderr:
            break
    print(f"docker {' '.join(attempts)}\n{result.stderr.strip()}")
    sys.exit(2)


def with_another_port(arguments):
    """The same command, asking for a host port that is free now."""
    out = list(arguments)
    for index, value in enumerate(out):
        if value == "-p" and index + 1 < len(out) and ":" in out[index + 1]:
            _, inside = out[index + 1].split(":", 1)
            out[index + 1] = f"{free_port()}:{inside}"
    return out


def published_port(arguments):
    """The host port a start ended up using, so the caller can talk to it."""
    for index, value in enumerate(arguments):
        if value == "-p" and index + 1 < len(arguments) and ":" in arguments[index + 1]:
            return int(arguments[index + 1].split(":", 1)[0])
    return 0


# The name carries this process, so two checks running at once cannot collide.
# A four-digit draw repeats, and a container a killed run left behind takes the
# name with it: docker then refuses to start, which reads exactly like the image
# being broken.
run = f"ptium-upgrade-{os.getpid()}-{random.randint(1000, 9999)}"
network, database, before, after = f"{run}-net", f"{run}-db", f"{run}-old", f"{run}-new"
old_port, new_port = free_port(), free_port()
secret = "devsecret-devsecret-devsecret-devsecret"
password = "upgrade-password-1234"


def wait_for(url, seconds=90):
    deadline = time.time() + seconds
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=5) as response:
                if response.status < 500:
                    return True
        except Exception:
            time.sleep(1)
    return False

def blame_the_host(container):
    """Whether a server that cannot be reached is running perfectly well.

    A port the host will not forward — held by something else, or refused by
    Docker's own forwarder — looks exactly like an image that will not start.
    It is not, and saying it is accuses the release of something it did not do.
    """
    running = docker("inspect", "-f", "{{.State.Running}}", container, check=False).strip()
    logs = docker("logs", container, check=False)
    return running == "true" and "listening" in logs


def old_call(path, body=None, method=None):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(f"http://127.0.0.1:{old_port}/api/v1{path}", data=data,
                                     method=method or ("POST" if data else "GET"),
                                     headers={"X-Ptium-Dev-Secret": secret, "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=180) as response:
        payload = response.read()
        return json.loads(payload)["data"] if payload else None


jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))


def new_call(path, body=None, method=None, raw=False):
    base = f"http://127.0.0.1:{new_port}"
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(base + path, data=data, method=method or ("POST" if data else "GET"),
                                     headers={"Content-Type": "application/json",
                                              "Origin": base, "Sec-Fetch-Site": "same-origin"})
    try:
        with opener.open(request, timeout=180) as response:
            payload = response.read()
            return payload if raw else (json.loads(payload)["data"] if payload else None)
    except Exception as error:
        failures.append(f"{path}: {error}")
        return None


print(f"── {old_image} wrote it, {new_image} opens it ──")
docker("network", "create", network)
try:
    docker("run", "-d", "--name", database, "--network", network,
           "-e", "POSTGRES_PASSWORD=up", "-e", "POSTGRES_DB=ptium", options.postgres)
    for _ in range(60):
        if subprocess.run(["docker", "exec", database, "pg_isready", "-U", "postgres"],
                          capture_output=True).returncode == 0:
            break
        time.sleep(1)
    dsn = f"postgres://postgres:up@{database}:5432/ptium?sslmode=disable"
    old_port = run_container("run", "-d", "--name", before, "--network", network, "-p", f"{old_port}:8080",
           "-e", f"DATABASE_URL={dsn}", "-e", "DEV_AUTH_ENABLED=true", "-e", f"DEV_AUTH_SECRET={secret}",
           "-e", "DEV_AUTH_ALLOW_REMOTE=true", "-e", "DEV_AUTH_EMAIL=up@ptium.local", old_image)
    if not wait_for(f"http://127.0.0.1:{old_port}/readyz"):
        print(docker("logs", before, check=False))
        if blame_the_host(before):
            print("   the container is up and listening; this host will not forward its port")
            sys.exit(2)
        failures.append(f"{old_image} never became ready")
    else:
        deck = old_call("/presentations", {"title": "예전 버전에서 만든 덱", "prompt": "업그레이드 점검",
                                           "requestedSlideCount": 5, "language": "ko"})
        old_call(f"/presentations/{deck['id']}/generate", {}, method="POST")
        state = {}
        for _ in range(90):
            time.sleep(2)
            state = old_call(f"/presentations/{deck['id']}")
            if state.get("status") in ("completed", "failed"):
                break
        made = len(state.get("slides") or [])
        print(f"   {old_image} made a deck: {state.get('status')}, {made} slide(s)")
        if made < 1:
            failures.append(f"{old_image} could not make a deck to upgrade")

        docker("rm", "-f", before)
        new_port = run_container("run", "-d", "--name", after, "--network", network, "-p", f"{new_port}:8080",
               "-e", f"DATABASE_URL={dsn}", "-e", "BOOTSTRAP_ADMIN=admin@example.com",
               "-e", f"BOOTSTRAP_ADMIN_PASSWORD={password}", new_image)
        if not wait_for(f"http://127.0.0.1:{new_port}/readyz"):
            print(docker("logs", after, check=False))
            failures.append(f"{new_image} did not come up on a database {old_image} wrote")
        else:
            migrated = docker("logs", after, check=False)
            if "database migrations ready" not in migrated:
                failures.append("the migrations did not report themselves ready")
            # The deck belongs to the identity the old release signed in as, and
            # this one signs in as the administrator. Moving it is the test's
            # doing, not the product's: what is being checked is whether a deck
            # written two hundred releases ago still opens, draws and exports.
            docker("exec", database, "psql", "-U", "postgres", "-d", "ptium", "-c",
                   f"update presentations set owner_id=(select id from users where email='admin@example.com') "
                   f"where id='{deck['id']}'")
            owner_moved = True
            new_call("/api/v1/auth/login", {"username": "admin@example.com", "password": password})
            carried = new_call(f"/api/v1/presentations/{deck['id']}") or {}
            slides = len(carried.get("slides") or [])
            print(f"   after the upgrade: {carried.get('title')} · {slides} slide(s) · {carried.get('status')}")
            if slides != made:
                failures.append(f"the deck had {made} slide(s) and now has {slides}")
            measured = new_call(f"/api/v1/presentations/{deck['id']}/inspect") or {}
            print(f"   measured: {measured.get('defects')} defect(s), {measured.get('advisories')} advisory(ies)")
            for kind, query in (("pptx", "format=pptx"), ("pdf", "format=pdf"), ("handout", "format=pdf&notes=true")):
                data = new_call(f"/api/v1/presentations/{deck['id']}/export?{query}", raw=True) or b""
                marker = b"PK\x03\x04" if kind == "pptx" else b"%PDF-"
                if not data.startswith(marker):
                    failures.append(f"a deck from {old_image} cannot be exported as {kind}")
            source = (new_call(f"/api/v1/presentations/{deck['id']}/source") or {}).get("source") or ""
            if len(source.splitlines()) < 5:
                failures.append("the deck's source did not read back")
            drawn = new_call(f"/api/v1/presentations/{deck['id']}/preview.svg?slide=1&width=600", raw=True) or b""
            if b"<svg" not in drawn:
                failures.append("a deck from an old release does not draw")
            print(f"   exported and drawn · source {len(source.splitlines())} lines")

            # And back again. An operator whose upgrade goes wrong redeploys the
            # tag they had; what makes that possible is that no migration takes
            # anything away, and the day one does, this is where it shows.
            docker("rm", "-f", after)
            old_port = run_container("run", "-d", "--name", before, "--network", network, "-p", f"{old_port}:8080",
                   "-e", f"DATABASE_URL={dsn}", "-e", "DEV_AUTH_ENABLED=true", "-e", f"DEV_AUTH_SECRET={secret}",
                   "-e", "DEV_AUTH_ALLOW_REMOTE=true", "-e", "DEV_AUTH_EMAIL=up@ptium.local", old_image)
            if not wait_for(f"http://127.0.0.1:{old_port}/readyz"):
                print(docker("logs", before, check=False))
                failures.append(f"{old_image} does not come up again on a database {new_image} has written")
            else:
                if owner_moved:
                    # Give it back, so the old release is asked as the identity
                    # that made it.
                    docker("exec", database, "psql", "-U", "postgres", "-d", "ptium", "-c",
                           f"update presentations set owner_id=(select id from users where email='up@ptium.local') "
                           f"where id='{deck['id']}'")
                try:
                    back = old_call(f"/presentations/{deck['id']}")
                    kept = len(back.get("slides") or [])
                    print(f"   rolled back to {old_image}: the deck still opens with {kept} slide(s)")
                    if kept != made:
                        failures.append(f"after a rollback the deck has {kept} slide(s), not {made}")
                except Exception as error:
                    failures.append(f"after a rollback the deck cannot be read: {error}")
finally:
    docker("rm", "-f", before, after, database, check=False)
    docker("network", "rm", network, check=False)

print()
for failure in failures:
    print(" ✗", failure)
print(f"{len(failures)} failure(s)")
sys.exit(1 if failures else 0)
