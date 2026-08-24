"""The published image, an empty database, and the first hour of a deployment.

Every other check in this repository runs against a binary built from the
working tree, talking to a database that has been alive for days and a
workspace full of decks. What an operator does on the first day is none of
those things: they load the tar.gz, point the image at an empty PostgreSQL,
sign in as the bootstrap administrator, and make a deck.

That path went unchecked through a dozen releases — the package smoke that runs
after every build measures what PowerPoint sees, against the development server,
not the image that was just written to dist/.

    python3 scripts/e2e/firstrun.py                 # the version in VERSION
    python3 scripts/e2e/firstrun.py --image ptium:1.11.7

Needs docker and a postgres image already present locally; an air-gapped host
has both, since that is what the bundle is for.
"""
import argparse, http.cookiejar, json, os, random, socket, subprocess, sys, time, urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--image", help="the image to run (default: ptium:<VERSION>)")
parser.add_argument("--postgres", default="postgres:16-alpine")
parser.add_argument("--keep", action="store_true", help="leave the containers running")
options = parser.parse_args()

root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
version = open(os.path.join(root, "VERSION")).read().strip()
image = options.image or f"ptium:{version}"
run = f"ptium-firstrun-{random.randint(1000, 9999)}"
network, database, server = f"{run}-net", f"{run}-db", run
password = "first-run-password-1234"
failures = []


def docker(*arguments, check=True):
    result = subprocess.run(["docker", *arguments], capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"docker {' '.join(arguments)}\n{result.stderr.strip()}")
        sys.exit(1)
    return result.stdout.strip()


def free_port():
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


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


port = free_port()
base = f"http://127.0.0.1:{port}"
jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))


def call(path, body=None, method=None, raw=False):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(base + path, data=data, method=method or ("POST" if data else "GET"),
                                     headers={"Content-Type": "application/json",
                                              # What a browser sends. A cookie session
                                              # will not write without it, by design.
                                              "Origin": base, "Sec-Fetch-Site": "same-origin"})
    try:
        with opener.open(request, timeout=180) as response:
            payload = response.read()
            return payload if raw else (json.loads(payload)["data"] if payload else None)
    except Exception as error:
        failures.append(f"{path}: {error}")
        return None


print(f"── {image} on an empty database ──")
docker("network", "create", network)
try:
    docker("run", "-d", "--name", database, "--network", network,
           "-e", "POSTGRES_PASSWORD=firstrun", "-e", "POSTGRES_DB=ptium", options.postgres)
    # The image exits when the database is not there yet — a pod restarts, and
    # compose says restart: unless-stopped for the same reason. A check that
    # raced it would be reporting on the wait, not on the image.
    for attempt in range(60):
        ready = subprocess.run(["docker", "exec", database, "pg_isready", "-U", "postgres"],
                               capture_output=True, text=True)
        if ready.returncode == 0:
            break
        time.sleep(1)
    else:
        failures.append("the database never became ready")
    docker("run", "-d", "--name", server, "--network", network, "-p", f"{port}:8080",
           "-e", f"DATABASE_URL=postgres://postgres:firstrun@{database}:5432/ptium?sslmode=disable",
           "-e", "BOOTSTRAP_ADMIN=admin@example.com",
           "-e", f"BOOTSTRAP_ADMIN_PASSWORD={password}",
           "-e", "BOOTSTRAP_ADMIN_NAME=Ptium Administrator", image)
    if not wait_for(base + "/readyz"):
        print(docker("logs", server, check=False))
        failures.append("the image never became ready")
    else:
        call("/api/v1/auth/login", {"username": "admin@example.com", "password": password})
        templates = call("/api/v1/templates?limit=50") or []
        print(f"   templates seeded: {len(templates)}")
        if len(templates) < 10:
            failures.append(f"a new deployment starts with {len(templates)} templates")
        deck = call("/api/v1/presentations", {"title": "첫 배포 점검",
                                              "prompt": "2026년 하반기 계획을 임원에게 보고합니다. 매출 목표와 인력 계획 포함.",
                                              "requestedSlideCount": 6, "language": "ko"})
        if not deck:
            failures.append("a deck could not be created")
        else:
            call(f"/api/v1/presentations/{deck['id']}/generate", {}, method="POST")
            state = {}
            for _ in range(90):
                time.sleep(2)
                state = call(f"/api/v1/presentations/{deck['id']}") or {}
                if state.get("status") in ("completed", "failed"):
                    break
            slides = len(state.get("slides") or [])
            print(f"   generated: {state.get('status')} with {slides} slide(s)")
            if state.get("status") != "completed" or slides < 1:
                failures.append(f"the first deck did not generate: {state.get('status')}")
            measured = call(f"/api/v1/presentations/{deck['id']}/inspect") or {}
            print(f"   measured: defects {measured.get('defects')} advisories {measured.get('advisories')}")
            if measured.get("defects"):
                failures.append(f"the first deck has {measured.get('defects')} defect(s)")
            files = {}
            for kind, query in (("pptx", "format=pptx"), ("pdf", "format=pdf"), ("handout", "format=pdf&notes=true")):
                files[kind] = call(f"/api/v1/presentations/{deck['id']}/export?{query}", raw=True) or b""
            print("   " + "  ".join(f"{kind} {len(data):,} bytes" for kind, data in files.items()))
            if not files["pptx"].startswith(b"PK"):
                failures.append("the image cannot write a pptx")
            for kind in ("pdf", "handout"):
                if not files[kind].startswith(b"%PDF-"):
                    failures.append(f"the image cannot write the {kind}")
            # The font is what makes a Korean PDF possible, and it lives in the
            # image rather than on the host.
            if b"NanumBarunGothic" not in files["pdf"]:
                failures.append("the image printed without the font it ships")
            drawn = call(f"/api/v1/presentations/{deck['id']}/preview.svg?slide=1&width=600", raw=True) or b""
            if b"<svg" not in drawn:
                failures.append("the image does not draw a slide")
            home = call("/", raw=True) or b""
            if b'<div id="root"' not in home:
                failures.append("the image does not serve the workspace")
            print(f"   preview {len(drawn):,} bytes · workspace {len(home):,} bytes")
finally:
    if not options.keep:
        docker("rm", "-f", server, database, check=False)
        docker("network", "rm", network, check=False)

print()
if failures:
    for failure in failures:
        print(" ✗", failure)
print(f"{len(failures)} failure(s)")
sys.exit(1 if failures else 0)
