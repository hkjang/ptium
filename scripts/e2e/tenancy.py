"""Whose is whose: what one account can reach of another's, what a link gives
away, what a key may do, and which doors want an administrator.

The share link that drew a slide marked !skip to anyone holding it would have
been caught here, so this is where that check lives now.
"""
import argparse
import atexit
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/") + "/api/v1"
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
RUN = uuid.uuid4().hex[:6]
failures = []
checks = 0


# A sweep that stops early has still found what it found.
def stopped_early(kind, error, trace):
    failures.append(f"the sweep stopped early: {kind.__name__}: {error}".replace("\n", " ")[:300])
    sys.__excepthook__(kind, error, trace)


def report():
    print()
    print(f"{checks} checks, {len(failures)} failures")
    for failure in failures:
        print(" ✗ " + failure)


sys.excepthook = stopped_early
atexit.register(report)


def account(name, roles=""):
    """Headers for a development identity. Dev auth is an administrator by
    default, so a check about what a person without that role can reach has to
    ask for the plain role by name."""
    token = f"dev:{name}-{RUN}@ptium.local" + (f":{roles}" if roles else "")
    return {"X-Ptium-Dev-Secret": SECRET, "Authorization": f"Bearer {token}"}


def call(auth, method, path, body=None, raw=False):
    global checks
    data = json.dumps(body).encode() if body is not None else None
    headers = dict(auth)
    if data:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            payload = response.read()
            if raw:
                return response.status, payload
            return response.status, (json.loads(payload)["data"] if payload else None)
    except urllib.error.HTTPError as error:
        return error.code, error.read()[:200]


def refused(auth, method, path, body=None, note=""):
    """A door that must not open. 404 is a fine answer and a better one: it does
    not say whether the thing exists."""
    global checks
    status, payload = call(auth, method, path, body, raw=True)
    checks += 1
    if status not in (401, 403, 404):
        failures.append(f"{method} {path} answered {status} to {note or 'another account'}: {payload[:120]!r}")
    return status


parser = argparse.ArgumentParser()
parser.parse_args()

alice, bob = account("alice"), account("bob")
plain = account("plain", "user")

print("── one account's work, seen from another ──")
status, deck = call(alice, "POST", "/presentations",
                    {"title": f"앨리스의 덱 {RUN}", "prompt": "인수 검토", "language": "ko"})
checks += 1
if status != 201:
    failures.append(f"a deck could not be made to check with: {status} {deck!r}")
else:
    deck_id = deck["id"]
    call(alice, "PUT", f"/presentations/{deck_id}/source",
         {"source": "# 앨리스만 볼 것\n- 인수 대상 후보 3곳\n!notes 이사회 전까지 비공개\n"})
    for method, path, body in [
        ("GET", f"/presentations/{deck_id}", None),
        ("GET", f"/presentations/{deck_id}/source", None),
        ("GET", f"/presentations/{deck_id}/inspect", None),
        ("GET", f"/presentations/{deck_id}/preview.svg?slide=1&width=400", None),
        ("GET", f"/presentations/{deck_id}/export?format=pptx", None),
        ("GET", f"/presentations/{deck_id}/export?format=pdf", None),
        ("GET", f"/presentations/{deck_id}/revisions", None),
        ("PUT", f"/presentations/{deck_id}", {"title": "밥이 바꿈"}),
        ("PUT", f"/presentations/{deck_id}/source", {"source": "# 밥\n- 덮어씀\n"}),
        ("POST", f"/presentations/{deck_id}/generate", {}),
        ("DELETE", f"/presentations/{deck_id}", None),
    ]:
        refused(bob, method, path, body)
    listed = call(bob, "GET", "/presentations?limit=100")[1]
    items = listed if isinstance(listed, list) else (listed or {}).get("items") or []
    checks += 1
    if any(item.get("id") == deck_id for item in items):
        failures.append("a deck of another account's is in this one's listing")
    print(f"   another account is refused on {11} doors and sees nothing in its listing")

print("── images, saved slides and keys ──")
png = bytes.fromhex("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489"
                    "0000000a49444154789c63000100000500010d0a2db40000000049454e44ae426082")
boundary = uuid.uuid4().hex
form = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"비밀-{RUN}.png\"\r\n"
        f"Content-Type: image/png\r\n\r\n").encode() + png + f"\r\n--{boundary}--\r\n".encode()
request = urllib.request.Request(BASE + "/assets", data=form, method="POST",
                                 headers={**alice, "Content-Type": f"multipart/form-data; boundary={boundary}"})
try:
    with urllib.request.urlopen(request, timeout=120) as response:
        asset = json.loads(response.read())["data"]
except urllib.error.HTTPError as error:
    asset = None
    failures.append(f"an image could not be uploaded to check with: {error.code}")
checks += 1
if asset:
    for method, path in [("GET", f"/assets/{asset['id']}"), ("GET", f"/assets/{asset['id']}/content"),
                         ("DELETE", f"/assets/{asset['id']}")]:
        refused(bob, method, path)

status, snippet = call(alice, "POST", "/snippets", {"name": f"비밀 슬라이드 {RUN}",
                                                    "source": "# 비공개\n- 인수 후보\n"})
checks += 1
if status != 201:
    failures.append(f"a snippet could not be saved to check with: {status} {snippet!r}")
else:
    refused(bob, "GET", f"/snippets/{snippet['id']}")
    refused(bob, "DELETE", f"/snippets/{snippet['id']}")
    listed = call(bob, "GET", "/snippets?limit=100")[1]
    items = listed if isinstance(listed, list) else (listed or {}).get("snippets") or (listed or {}).get("items") or []
    checks += 1
    if any(item.get("id") == snippet["id"] for item in items):
        failures.append("a saved slide of another account's is in this one's listing")

status, created = call(alice, "POST", "/api-keys", {"name": f"앨리스 키 {RUN}"})
key = (created or {}).get("apiKey") if isinstance(created, dict) else None
checks += 1
if not key:
    failures.append(f"an api key could not be made to check with: {status}")
else:
    refused(bob, "DELETE", f"/api-keys/{key['id']}")
    listed = call(bob, "GET", "/api-keys")[1]
    items = listed if isinstance(listed, list) else (listed or {}).get("items") or []
    checks += 1
    if any(item.get("id") == key["id"] for item in items):
        failures.append("an api key of another account's is in this one's listing")
    still = call(alice, "GET", "/api-keys")[1]
    mine = still if isinstance(still, list) else (still or {}).get("items") or []
    checks += 1
    if not any(item.get("id") == key["id"] for item in mine):
        failures.append("an api key was revoked by an account that does not own it")
print("   images, saved slides and keys are the owner's alone")

print("── what a key may do is what its scopes say ──")
status, created = call(alice, "POST", "/api-keys", {"name": f"읽기 전용 {RUN}", "scopes": ["presentations:read"]})
secret = (created or {}).get("secret") if isinstance(created, dict) else None
checks += 1
if not secret:
    failures.append(f"a read-only key could not be made: {status}")
else:
    keyed = {"Authorization": f"Bearer {secret}"}
    status, own = call(alice, "POST", "/presentations", {"title": f"키 점검 {RUN}", "prompt": "점검", "language": "ko"})
    checks += 1
    if call(keyed, "GET", "/presentations?limit=1", raw=True)[0] != 200:
        failures.append("a key with presentations:read cannot read presentations")
    for method, path, body in [
        ("POST", "/presentations", {"title": "키가 만든 덱", "prompt": "x", "language": "ko"}),
        ("PUT", f"/presentations/{own['id']}", {"title": "키가 고침"}),
        ("DELETE", f"/presentations/{own['id']}", None),
        ("GET", "/admin/settings", None),
        ("GET", "/api-keys", None),
    ]:
        refused(keyed, method, path, body, note="a key that was not given that scope")
    call(alice, "DELETE", f"/presentations/{own['id']}")
    print("   a read-only key reads, and is refused everything else")

print("── the administrator's doors ──")
for method, path, body in [
    ("GET", "/admin/settings", None),
    ("PUT", "/admin/settings", {"values": {"ai.provider": "fallback"}}),
    ("GET", "/admin/users", None),
    ("GET", "/admin/errors", None),
]:
    refused(plain, method, path, body, note="an account without the administrator role")
print("   an account without the role is refused")

print("── a link is the deck as it is shown ──")
status, shared = call(alice, "POST", "/presentations",
                      {"title": f"공유 점검 {RUN}", "prompt": "점검", "language": "ko"})
checks += 1
if status != 201:
    failures.append(f"a deck could not be made to share: {status}")
else:
    call(alice, "PUT", f"/presentations/{shared['id']}/source",
         {"source": "# 보이는 장\n- 공개해도 되는 요점\n!notes 이사회 전까지 비공개\n\n"
                    "# 숨긴 장\n- 아직 공유하지 않을 내용\n!skip\n\n# 세 번째 장\n- 공개 요점\n"})
    made = call(alice, "POST", f"/presentations/{shared['id']}/shares", {})[1] or {}
    token = (made.get("url") or "").rstrip("/").split("/")[-1]
    checks += 1
    if not token:
        failures.append("a share was created without a link to open")
    else:
        anonymous = {}
        status, seen = call(anonymous, "GET", f"/shared/{token}")
        checks += 1
        if status != 200:
            failures.append(f"a share link does not open: {status}")
        payload = json.dumps(seen, ensure_ascii=False)
        checks += 1
        if (seen or {}).get("slideCount") != 2:
            failures.append(f"a link with one skipped slide shows {(seen or {}).get('slideCount')} slides, want 2")
        for private in ["이사회 전까지 비공개", "아직 공유하지 않을 내용", "!skip", "!notes"]:
            checks += 1
            if private in payload:
                failures.append(f"a share link carries {private!r}")
        for position in (1, 2, 3):
            status, drawn = call(anonymous, "GET", f"/shared/{token}/preview.svg?slide={position}&width=400", raw=True)
            checks += 1
            if status == 200 and "아직 공유하지 않을 내용" in drawn.decode(errors="replace"):
                failures.append(f"a share link draws the skipped slide at position {position}")
        for path in [f"/shared/{token}/export?format=pptx", f"/shared/{token}/source", f"/shared/{token}/inspect"]:
            refused(anonymous, "GET", path, note="whoever holds a link")
        shares = call(alice, "GET", f"/presentations/{shared['id']}/shares")[1]
        rows = shares if isinstance(shares, list) else (shares or {}).get("items") or []
        if rows:
            call(alice, "DELETE", f"/presentations/{shared['id']}/shares/{rows[0]['id']}")
            checks += 1
            if call(anonymous, "GET", f"/shared/{token}", raw=True)[0] != 410:
                failures.append("a link that was taken back still opens")
        print("   a link shows the show, and nothing else")
    call(alice, "DELETE", f"/presentations/{shared['id']}")

print("── what someone types cannot become markup ──")
# Both the workspace and the shared page render these drawings as HTML, so
# anything a person types into a deck that survives as markup is script running
# in whoever opens it. Every drawing the product hands to a browser is asked.
nasty = '</text><script>alert(1)</script><text x="0" y="0"'
injected_source = (f"# {nasty}\n> {nasty}\n- {nasty}\n::kpi\n- {nasty} :: {nasty}\n::\n"
                   f"!notes {nasty}\n!source {nasty}\n"
                   f'- 열기: [여기](https://example.com/plan) 그리고 [저기](https://example.com/x"onmouseover="alert(1))\n')
status, hostile = call(alice, "POST", "/presentations",
                       {"title": f"주입 {nasty}", "prompt": "점검", "language": "ko"})
checks += 1
if status != 201:
    failures.append(f"a deck could not be made to check drawing with: {status}")
else:
    call(alice, "PUT", f"/presentations/{hostile['id']}/source", {"source": injected_source})
    status, saved = call(alice, "POST", "/snippets", {"name": f"주입 {RUN}", "source": f"# {nasty}\n- {nasty}\n"})
    snippet_id = saved.get("id") if isinstance(saved, dict) else ""
    templates = call(alice, "GET", "/templates?limit=1")[1]
    rows = templates if isinstance(templates, list) else (templates or {}).get("items") or []
    made = call(alice, "POST", f"/presentations/{hostile['id']}/shares", {})[1] or {}
    link = (made.get("url") or "").rstrip("/").split("/")[-1]
    drawings = [
        ("the deck's own preview", "GET", f"/presentations/{hostile['id']}/preview.svg?slide=1&width=800", None),
        ("a preview of unsaved source", "POST", f"/presentations/{hostile['id']}/source/preview.svg?slide=1&width=800",
         {"source": injected_source}),
    ]
    if snippet_id:
        drawings.append(("a saved slide's preview", "GET", f"/snippets/{snippet_id}/preview.svg?width=400", None))
    if rows:
        drawings.append(("a template's preview", "GET", f"/templates/{rows[0]['id']}/preview.svg?width=400", None))
    if link:
        drawings.append(("what a link draws", "GET", f"/shared/{link}/preview.svg?slide=1&width=800", None))
    for name, method, path, body in drawings:
        status, drawn = call(alice, method, path, body, raw=True)
        drawing = drawn.decode(errors="replace") if status == 200 and isinstance(drawn, bytes) else ""
        checks += 1
        if status != 200:
            failures.append(f"{name} did not draw: {status}")
            continue
        for probe in ["<script", "onerror=", "onload=", "<foreignObject", "<iframe"]:
            checks += 1
            if probe in drawing:
                failures.append(f"{name} carries {probe!r} — what was typed became markup")
        # Every address that is drawn as a link is one that can sit inside an
        # attribute and is a scheme a reader may follow. "href=\"…\" target=" is
        # the anchor's own attributes and not a break-out, so the addresses are
        # read rather than the shape around them.
        for address in re.findall(r'href="([^"]*)"', drawing):
            checks += 1
            if any(mark in address for mark in ('"', "<", ">", "\\")):
                failures.append(f"{name} drew an address that cannot sit in an attribute: {address!r}")
            checks += 1
            if not address.startswith(("https://", "http://", "mailto:", "#")):
                failures.append(f"{name} drew an address a reader should not follow: {address!r}")
    status, drawn = call(alice, "GET", f"/presentations/{hostile['id']}/preview.svg?slide=1&width=800", raw=True)
    drawing = drawn.decode(errors="replace") if status == 200 else ""
    checks += 1
    if 'rel="noreferrer noopener"' not in drawing:
        failures.append("a link in a drawing opens without rel=\"noreferrer noopener\"")
    print(f"   {len(drawings)} drawings carry the words and none of the markup")
    if snippet_id:
        call(alice, "DELETE", f"/snippets/{snippet_id}")
    call(alice, "DELETE", f"/presentations/{hostile['id']}")

sys.exit(1 if failures else 0)
