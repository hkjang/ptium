"""Several people at once, against a server built with the race detector.

One Generator, one Store, one settings cache and one preview renderer serve every
request this server takes. Anything a request writes onto one of those is a
request writing under another request's feet — which is how a fifty-slide deck
came to have its output budget reset mid-answer by somebody rewriting a slide.
That one was found by pointing the race detector at a test; this points it at
the whole server, doing everything at once.

    cd server && go build -race -o /tmp/ptium-race ./cmd/ptium
    DATABASE_URL=... /tmp/ptium-race > /tmp/ptium-race.log 2>&1 &
    python3 scripts/e2e/race.py --log /tmp/ptium-race.log --seconds 90

Without --log it still drives the load and checks the answers; the race detector
writes to the server's own output, so finding what it said needs that file.
"""
import argparse, concurrent.futures as futures, hashlib, json, os, random, sys, threading, time
import urllib.error, urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--log", help="the server's output, where the race detector writes")
parser.add_argument("--seconds", type=int, default=60)
options = parser.parse_args()

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/") + "/api/v1"
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
RUN = str(int(time.time()))[-6:]
failures, counts = [], {}


def call(method, path, body=None, raw=False):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"X-Ptium-Dev-Secret": SECRET}
    if data:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            payload, status = response.read(), response.status
    except urllib.error.HTTPError as error:
        return error.code, error.read()
    except Exception as error:  # a connection the server dropped is worth reporting
        return 0, str(error).encode()
    if raw or not payload:
        return status, payload
    parsed = json.loads(payload)
    return status, (parsed.get("data", parsed) if isinstance(parsed, dict) else parsed)


def note(name, status, allowed=(200, 201, 202, 204)):
    counts[name] = counts.get(name, 0) + 1
    if status not in allowed:
        failures.append(f"{name} answered {status}")


def listed(path):
    """What the server sent, or a clear word about why it sent nothing.

    A connection this server never accepted comes back from call() as the error
    text, and iterating that yields the bytes of the message: the sweep died on
    "'int' object has no attribute 'get'" while the real news was that nothing
    was listening. A harness that misreports its own failure costs the reader
    the time it was written to save."""
    status, payload = call("GET", path)
    if isinstance(payload, list):
        return payload
    said = payload.decode("utf-8", "replace") if isinstance(payload, (bytes, bytearray)) else str(payload)
    print(f"the server did not answer {path} ({status}): {said[:160]}")
    print(f"is it running? PTIUM_URL={BASE.rsplit('/api/', 1)[0]}")
    sys.exit(1)


decks = [d for d in listed("/presentations?limit=40") if (d.get("slideCount") or 0) >= 3][:8]
templates = listed("/templates?limit=20")
assets = listed("/assets?limit=20")
if not decks or not templates:
    print("this database has no decks or templates to work with")
    sys.exit(1)
print(f"decks {len(decks)} · templates {len(templates)} · assets {len(assets)} · {options.seconds}s", flush=True)
stop = time.time() + options.seconds


def readers():
    while time.time() < stop:
        deck = random.choice(decks)
        note("read", call("GET", f"/presentations/{deck['id']}")[0])
        note("inspect", call("GET", f"/presentations/{deck['id']}/inspect")[0])
        note("preview", call("GET", f"/presentations/{deck['id']}/preview.svg?slide=1&width=600", raw=True)[0])


def writers():
    while time.time() < stop:
        status, made = call("POST", "/presentations", {"title": f"동시 {RUN} {random.randint(0, 9999)}",
                                                       "prompt": "동시 요청 점검"})
        note("create", status, (201,))
        if status != 201:
            continue
        deck = made["id"]
        note("apply source", call("PUT", f"/presentations/{deck}/source",
                                  {"source": "# 한 장\n@cover\n> 리드\n\n# 두 장\n@content\n"
                                             "- 첫 줄과 [문서](https://example.com/a)\n- **둘째 줄**\n"
                                             "!notes 노트에도 [링크](https://example.com/n)\n"})[0])
        note("export", call("GET", f"/presentations/{deck}/export?format=pptx", raw=True)[0])
        # Paper is drawn from the same drawing every screen uses, embeds a font
        # and holds a document's worth of state per request. Whatever that costs,
        # it has to cost it under load too.
        note("print", call("GET", f"/presentations/{deck}/export?format=pdf", raw=True)[0])
        note("handout", call("GET", f"/presentations/{deck}/export?format=pdf&notes=true", raw=True)[0])
        note("delete", call("DELETE", f"/presentations/{deck}")[0], (204,))


def admins():
    while time.time() < stop:
        note("settings", call("GET", "/admin/settings")[0])
        note("templates", call("GET", "/templates?kind=builtin&limit=10")[0])
        note("overview", call("GET", "/admin/overview")[0])


def images():
    # The same picture, read by many at once. With ASSET_STORAGE=filesystem and
    # bytes still in the database, every one of these readers wants to move it
    # onto the volume; they must agree on what it holds.
    while time.time() < stop and assets:
        asset = random.choice(assets)
        status, body = call("GET", f"/assets/{asset['id']}", raw=True)
        note("image", status, (200, 410))
        if status == 200:
            digests.setdefault(asset["id"], set()).add(hashlib.sha256(body).hexdigest())


def generators():
    # One deck at a time, and never more than the worker can finish: queueing
    # faster than it drains leaves thousands of decks behind, which is not load
    # testing, it is filling somebody's database. Every id is recorded the
    # moment it exists so the cleanup below can find it even if this thread is
    # interrupted.
    while time.time() < stop:
        status, queued = call("POST", "/presentations/generate", {
            "title": f"동시 생성 {RUN} {random.randint(0, 9999)}", "prompt": "동시 생성 점검입니다.",
            "language": "ko", "slideCount": random.choice([5, 9, 20]),
            "templateId": random.choice(templates)["id"]})
        note("generate", status, (202,))
        if status != 202:
            continue
        deck = queued["id"]
        with lock:
            made.append(deck)
        state = {}
        for _ in range(180):
            status, body = call("GET", f"/presentations/{deck}")
            state = body if isinstance(body, dict) else {}
            if state.get("status") in ("completed", "ready", "failed"):
                break
            time.sleep(1)
        if state.get("status") == "failed":
            failures.append(f"a generation under load failed: {state.get('errorMessage')}")


digests = {}
made, lock = [], threading.Lock()
with futures.ThreadPoolExecutor(max_workers=14) as pool:
    jobs = [pool.submit(readers) for _ in range(4)]
    jobs += [pool.submit(writers) for _ in range(3)]
    jobs += [pool.submit(admins) for _ in range(2)]
    jobs += [pool.submit(images) for _ in range(3)]
    jobs += [pool.submit(generators)]
    for job in jobs:
        job.result()

# Whatever this queued, this removes — finished or not.
for deck in made:
    note("clean up", call("DELETE", f"/presentations/{deck}")[0], (204, 404))

for asset, seen in digests.items():
    if len(seen) > 1:
        failures.append(f"one image answered with {len(seen)} different bodies: {asset}")

print("requests:", json.dumps(dict(sorted(counts.items())), ensure_ascii=False))
if options.log:
    text = open(options.log, encoding="utf-8", errors="replace").read()
    races = text.count("WARNING: DATA RACE")
    print(f"race detector: {races} report(s) in {options.log}")
    if races:
        start = text.index("WARNING: DATA RACE")
        print(text[start:start + 1200])
        failures.append(f"the race detector reported {races} race(s)")
elif "--log" not in sys.argv:
    print("no --log given: the answers were checked, the race detector's own output was not")

print()
print(f"{sum(counts.values())} requests, {len(failures)} failures")
for failure in failures[:10]:
    print(" ✗", failure)
sys.exit(1 if failures else 0)
