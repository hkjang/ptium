"""An end-to-end sweep of the REST surface, run against a real server.

Every check states what it expects. A failure prints the request, the status and
the body, so the next step is reading code rather than reproducing.
"""
import json, os, sys, urllib.request, urllib.error, zipfile, io, zlib, struct, time

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/") + "/api/v1"
RUN = str(int(time.time()))[-6:]  # each run works on its own names
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
failures, checks = [], 0

def call(method, path, body=None, raw=False, files=None, expect=None, note=""):
    global checks
    url = path if path.startswith("http") else BASE + path
    headers = {"X-Ptium-Dev-Secret": SECRET}
    data = None
    if files:
        boundary = "----ptium-e2e"
        parts = []
        for name, (filename, content, ctype) in files.items():
            parts.append(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"".encode())
            if filename:
                parts[-1] += f"; filename=\"{filename}\"\r\nContent-Type: {ctype}\r\n\r\n".encode()
            else:
                parts[-1] += b"\r\n\r\n"
            parts[-1] += content if isinstance(content, bytes) else content.encode()
            parts[-1] += b"\r\n"
        parts.append(f"--{boundary}--\r\n".encode())
        data = b"".join(parts)
        headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"
    elif body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request) as response:
            status, payload = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, payload = error.code, error.read()
    checks += 1
    if expect is not None and status not in (expect if isinstance(expect, (list, tuple)) else [expect]):
        failures.append(f"{method} {path} → {status}, want {expect}. {note}\n    {payload[:400]!r}")
    if raw:
        return status, payload
    try:
        return status, json.loads(payload) if payload else None
    except Exception:
        return status, payload

def data_of(result):
    status, body = result
    return (body or {}).get("data") if isinstance(body, dict) else None

def png(rgb=(200, 120, 60), size=8):
    def chunk(kind, payload):
        body = kind + payload
        return struct.pack(">I", len(payload)) + body + struct.pack(">I", zlib.crc32(body) & 0xffffffff)
    rows = b"".join(b"\x00" + bytes(rgb) * size for _ in range(size))
    # Uploading identical bytes is a reuse, by design. Two runs a while apart
    # used to produce the same picture — the colour came from three digits of the
    # clock — and the later run then worked on the earlier run's image, under the
    # earlier run's name. The run's own text goes in a comment so every run
    # uploads a picture no other run has.
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0))
            + chunk(b"tEXt", b"ptium-run\x00" + RUN.encode())
            + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b""))

print("── identity and settings ──")
me = data_of(call("GET", "/me", expect=200))
print("   signed in as", (me or {}).get("email"), "admin:", (me or {}).get("isAdmin"))
call("GET", "/profile", expect=200)
call("PATCH", "/profile", {"displayName": "E2E Tester", "company": "Ptium", "jobTitle": "QA"}, expect=200)
call("GET", "/settings", expect=200)
call("GET", "/admin/settings", expect=[200, 403])

print("── templates ──")
templates = data_of(call("GET", "/templates?limit=100", expect=200)) or []
print(f"   {len(templates)} templates")
builtin = [t for t in templates if t["kind"] == "builtin"]
if len(builtin) < 50:
    failures.append(f"built-in template count is {len(builtin)}, expected 50")
first = templates[0]
call("GET", f"/templates/{first['id']}", expect=200)
call("GET", f"/templates/{first['id']}/preview.svg?width=320", raw=True, expect=200)
call("PUT", f"/templates/{first['id']}/favorite", {"favorite": True}, expect=200)
again = data_of(call("GET", "/templates?limit=100", expect=200)) or []
if not any(t["id"] == first["id"] and t.get("favorite") for t in again):
    failures.append("a starred template did not come back starred")
call("PUT", f"/templates/{first['id']}/favorite", {"favorite": False}, expect=200)
call("GET", "/templates/00000000-0000-0000-0000-000000000000", expect=404)
call("GET", "/templates/not-a-uuid", expect=400)

print("── presentations ──")
deck = data_of(call("POST", "/presentations", {
    "title": f"E2E 점검 {RUN}", "prompt": "전수 점검용 덱", "requestedSlideCount": 5, "language": "ko",
    "templateId": first["id"], "audience": "임원", "tone": "professional"}, expect=201))
deck_id = deck["id"]
call("GET", f"/presentations/{deck_id}", expect=200)
call("GET", "/presentations?limit=5", expect=200)
source = """# E2E 점검
@cover
> 전수 점검

# 지표
::kpi 핵심
- 처리량 | 1,200건
- 오류율 | 0.2%
::
- 첫 번째 요점
- 두 번째 요점

# 표
::table 분기 실적
- 항목 | 1분기 | 2분기
- 매출 | 120억 | 140억
::

# 마무리
@closing
> 감사합니다
"""
applied = data_of(call("PUT", f"/presentations/{deck_id}/source", {"source": source}, expect=200))
if not applied or not applied.get("applied"):
    failures.append("applying deck source did not report applied")
print("   applied:", (applied or {}).get("applied"), "findings:", len((applied or {}).get("findings") or []))
dry = data_of(call("PUT", f"/presentations/{deck_id}/source", {"source": source, "dryRun": True}, expect=200))
if dry and dry.get("applied"):
    failures.append("a dry run reported itself as applied")
call("GET", f"/presentations/{deck_id}/source", expect=200)
call("POST", f"/presentations/{deck_id}/source/preview.svg?slide=2&width=400", {"source": source}, raw=True, expect=200)
call("PUT", f"/presentations/{deck_id}/source", {"source": "# 한 장\n@cover\n", "version": 1}, expect=409, note="a stale version must conflict")
call("PUT", f"/presentations/{deck_id}/source", {"source": ""}, expect=422, note="empty source produces no slides")
call("PUT", f"/presentations/{deck_id}/source", {"source": "# x\n" * 40000}, expect=422, note="oversized source is refused")

print("── inspect, preview, export ──")
inspected = data_of(call("GET", f"/presentations/{deck_id}/inspect", expect=200))
print("   slides:", (inspected or {}).get("slides"), "defects:", (inspected or {}).get("defects"),
      "advisories:", (inspected or {}).get("advisories"))
if (inspected or {}).get("defects"):
    failures.append(f"a plain four-slide deck has defects: {inspected}")
call("GET", f"/presentations/{deck_id}/preview.svg?slide=1&width=400", raw=True, expect=200)
status, pptx = call("GET", f"/presentations/{deck_id}/export?format=pptx", raw=True, expect=200)
try:
    package = zipfile.ZipFile(io.BytesIO(pptx))
    slides = [n for n in package.namelist() if n.startswith("ppt/slides/slide")]
    print(f"   pptx {len(pptx):,} bytes, {len(slides)} slides")
    if len(slides) != 4:
        failures.append(f"exported {len(slides)} slides, expected 4")
    bad = package.testzip()
    if bad:
        failures.append(f"the exported package is corrupt at {bad}")
except Exception as error:
    failures.append(f"the exported pptx could not be read: {error}")
# PDF export is not offered yet, and the product says so in the export dialog
# and the guide. What matters is that it is refused clearly rather than half done.
status, body = call("GET", f"/presentations/{deck_id}/export?format=pdf", expect=422,
                    note="until PDF exists, it must be refused with a reason")
if not (body or {}).get("error", {}).get("code") == "unsupported_export_format":
    failures.append(f"the pdf refusal does not say why: {body}")
call("GET", f"/presentations/{deck_id}/export?format=keynote", expect=422)

print("── canvas ──")
regions = data_of(call("GET", f"/presentations/{deck_id}/slides/2/regions", expect=200))
print("   regions on slide 2:", [r["slot"] for r in (regions or {}).get("regions", [])])
if not (regions or {}).get("regions"):
    failures.append("slide 2 reports no editable regions")
call("GET", f"/presentations/{deck_id}/slides/99/regions", expect=404)

print("── documents become decks ──")
sheet = "분기,매출\n1분기,1180\n2분기,1240\n3분기,1390\n"
imported = data_of(call("POST", "/presentations/import",
                        files={"file": (f"매출-{RUN}.csv", sheet, "text/csv")}, expect=201)) or {}
if (imported.get("slides") or 0) < 2:
    failures.append(f"a spreadsheet imported as {imported.get('slides')!r} slides")
document_id = ((imported.get("presentation") or {}).get("id")) or ""
if document_id:
    written = (data_of(call("GET", f"/presentations/{document_id}/source", expect=200)) or {}).get("source", "")
    for wanted in ["::columns", "- 1분기 | 1180", f"!source 매출-{RUN}.csv"]:
        if wanted not in written:
            failures.append(f"the imported spreadsheet lost {wanted!r}")
    print(f"   spreadsheet -> {imported.get('slides')} slides, cited")
call("POST", "/presentations/import", files={"file": ("보고서.pdf", b"%PDF-1.7", "application/pdf")}, expect=422,
     note="a file nothing here can read must say so")

print("── the library comes first ──")
registered = data_of(call("POST", "/snippets", {
    "name": f"회사 소개 {RUN}",
    "source": f"# 회사 소개 {RUN}\n@content\n> 2003년 설립\n- 임직원 1,240명\n- 매출 8,200억\n",
    "tags": ["표준"],
}, expect=201)) or {}
queued = data_of(call("POST", "/presentations/generate", {
    "prompt": f"회사 소개 {RUN}와 2026년 사업 계획을 임원에게 보고", "requestedSlideCount": 6, "language": "ko",
}, expect=202)) or {}
library_deck = queued.get("id", "")
for _ in range(40):
    state = data_of(call("GET", f"/presentations/{library_deck}")) or {}
    if state.get("status") not in ("queued", "generating"):
        break
    time.sleep(1)
written = (data_of(call("GET", f"/presentations/{library_deck}/source", expect=200)) or {}).get("source", "")
if "임직원 1,240명" not in written:
    failures.append("generation wrote its own version of a slide the library already had")
used = [s for s in (data_of(call("GET", "/snippets?limit=100")) or []) if s["id"] == registered.get("id")]
if not used or used[0].get("useCount", 0) < 1:
    failures.append(f"the registered slide was not counted as used: {used!r}")
else:
    print(f"   the deck took the registered slide (used {used[0]['useCount']}x)")

print("── commanding the deck ──")
plan = data_of(call("POST", f"/presentations/{deck_id}/command", {"text": "2번과 3번 합쳐줘", "dryRun": True}, expect=200)) or {}
if not (plan.get("plan") or []):
    failures.append(f"a command was understood as nothing: {plan!r}")
elif plan.get("applied"):
    failures.append("a dry run changed the deck")
else:
    print(f"   plan: {plan['plan'][0].get('reason')} ({plan.get('slides')} -> {plan.get('slidesAfter')})")
call("POST", f"/presentations/{deck_id}/command", {"text": "이 덱 어때?"}, expect=422,
     note="a sentence that names no edit must say so rather than guessing")

print("── quality score ──")
measured = data_of(call("GET", f"/presentations/{deck_id}/inspect", expect=200)) or {}
score = measured.get("score") or {}
if not (0 <= int(score.get("total", -1)) <= 100):
    failures.append(f"the deck's quality score is {score.get('total')!r}")
if len(score.get("slides") or []) != measured.get("slides"):
    failures.append(f"the score covers {len(score.get('slides') or [])} of {measured.get('slides')} slides")
if {entry.get("key") for entry in (score.get("dimensions") or [])} != {
        "readability", "structure", "visual", "accessibility", "evidence"}:
    failures.append(f"the score's dimensions are {score.get('dimensions')!r}")
print(f"   score {score.get('total')} weakest slide {score.get('weakest')}")

print("── images ──")
asset = data_of(call("POST", "/assets", files={"file": (f"logo-{RUN}.png", png(rgb=(int(RUN[-3:]) % 255, 120, 60)), "image/png")}, expect=201))
asset_id = asset["id"]
same = call("POST", "/assets", files={"file": (f"logo-copy-{RUN}.png", png(rgb=(int(RUN[-3:]) % 255, 120, 60)), "image/png")}, expect=200)
if not (data_of(same) or {}).get("reused"):
    failures.append("uploading identical bytes did not report a reuse")
call("POST", "/assets", files={"file": ("note.txt", b"not an image", "text/plain")}, expect=422)
call("PATCH", f"/assets/{asset_id}", {"tags": ["로고", "E2E"]}, expect=200)
call("PUT", f"/assets/{asset_id}/favorite", {"favorite": True}, expect=200)
listed = data_of(call("GET", "/assets?favorite=true&sort=used", expect=200)) or []
if not any(a["id"] == asset_id for a in listed):
    failures.append("a starred image is missing from the starred list")
call("GET", "/assets/tags", expect=200)
call("GET", f"/assets/{asset_id}", raw=True, expect=200)
call("PATCH", f"/assets/{asset_id}", {"name": ""}, expect=422)

print("── images inside a deck ──")
with_image = source + f"\n# 그림\n@picture\n::image logo-{RUN}.png\n"
call("PUT", f"/presentations/{deck_id}/source", {"source": with_image}, expect=200)
used = [a for a in (data_of(call("GET", "/assets", expect=200)) or []) if a["id"] == asset_id]
if not used or used[0]["deckCount"] != 1:
    failures.append(f"the image should be counted in one deck: {used}")
status, pptx = call("GET", f"/presentations/{deck_id}/export?format=pptx", raw=True, expect=200)
media = [n for n in zipfile.ZipFile(io.BytesIO(pptx)).namelist() if n.startswith("ppt/media/")]
if not media:
    failures.append("the exported deck carries no image although one is placed")

print("── saved slides ──")
snippet = data_of(call("POST", "/snippets", {"presentationId": deck_id, "slide": 2, "tags": ["표준"]}, expect=201))
snippet_id = snippet["id"]
print("   saved:", snippet["name"])
call("GET", "/snippets?sort=lastUsed", expect=200)
call("GET", "/snippets/tags", expect=200)
rendered = data_of(call("POST", f"/snippets/{snippet_id}/render", {"presentationId": deck_id}, expect=200))
if not (rendered or {}).get("slide"):
    failures.append("rendering a saved slide returned nothing")
call("GET", f"/snippets/{snippet_id}/preview.svg?presentationId={deck_id}&width=320", raw=True, expect=200)
call("PUT", f"/snippets/{snippet_id}/favorite", {"favorite": True}, expect=200)
call("PATCH", f"/snippets/{snippet_id}", {"name": f"회사 표준 지표 {RUN}"}, expect=200)
second = data_of(call("POST", "/snippets", {"source": "# 다른 조각\n- 한 줄\n", "name": f"다른 조각 {RUN}"}, expect=201))
call("PATCH", f"/snippets/{second['id']}", {"name": f"회사 표준 지표 {RUN}"}, expect=409, note="a taken name must conflict")
call("DELETE", f"/snippets/{second['id']}", expect=204)
call("GET", f"/snippets/{second['id']}", expect=404)

print("── grids ──")
call("GET", "/grids", expect=200)
call("PUT", f"/grids/e2e-raci-{RUN}", {"name": f"e2e-raci-{RUN}", "title": "E2E RACI",
     "columns": [{"label": "역할"}, {"label": "책임"}],
     "values": {"A": {"role": "accent1", "label": "승인"}, "R": {"role": "positive", "label": "실행"}},
     "order": ["R", "A"], "legend": True}, expect=[200, 201])
call("DELETE", f"/grids/e2e-raci-{RUN}", expect=[204, 200])

print("── revisions, duplicate, trash ──")
revisions = data_of(call("GET", f"/presentations/{deck_id}/revisions", expect=200)) or []
print("   revisions:", len(revisions))
if revisions:
    call("POST", f"/presentations/{deck_id}/revisions/{revisions[-1]['id']}/restore", {}, expect=200)
copy = data_of(call("POST", f"/presentations/{deck_id}/duplicate", {}, expect=201))
call("DELETE", f"/presentations/{copy['id']}", expect=204)
trashed = data_of(call("GET", "/presentations?deleted=true", expect=200)) or []
if not any(p["id"] == copy["id"] for p in trashed):
    failures.append("a deleted deck is not in the recycle bin")
call("POST", f"/presentations/{copy['id']}/restore", {}, expect=200)
call("DELETE", f"/presentations/{copy['id']}", expect=204)
call("DELETE", f"/presentations/{copy['id']}?permanent=true", expect=[204, 200, 404])

print("── generation without an AI provider (the air-gapped path) ──")
draft = data_of(call("POST", "/presentations", {
    "title": "오프라인 생성", "prompt": "2026년 하반기 클라우드 전환 로드맵과 투자 타당성을 6장으로 정리해줘",
    "requestedSlideCount": 6, "language": "ko", "templateId": first["id"]}, expect=201))
call("POST", f"/presentations/{draft['id']}/generate", {}, expect=[200, 202])
# Without a provider the deck is written locally in a second; with one it is a
# round trip to a model, which on a self-hosted one is minutes. Both are valid
# deployments, so the sweep waits and then says which it saw.
for _ in range(240):
    state = data_of(call("GET", f"/presentations/{draft['id']}", expect=200))
    if state["status"] in ("completed", "failed"):
        break
    time.sleep(1)
print("   status:", state["status"], "slides:", len(state.get("slides") or []))
if state["status"] not in ("completed", "failed") :
    print("   (still with the model after four minutes; the rest of this section is skipped)")
elif state["status"] != "completed":
    failures.append(f"generation ended as {state['status']}: {state.get('errorMessage')}")
elif len(state.get("slides") or []) != 6:
    failures.append(f"asked for 6 slides, generated {len(state.get('slides') or [])}")
if state["status"] == "completed":
    generated = data_of(call("GET", f"/presentations/{draft['id']}/inspect", expect=200))
    print("   generated deck defects:", (generated or {}).get("defects"), "advisories:", (generated or {}).get("advisories"))
    if (generated or {}).get("defects"):
        failures.append(f"the generated deck has defects: {generated.get('findings')}")

print("── api keys and mcp ──")
key = data_of(call("POST", "/api-keys", {"name": "e2e", "scopes": ["presentations:read"]}, expect=201))
call("GET", "/api-keys", expect=200)
if key and key.get("token"):
    token = key["token"]
    request = urllib.request.Request(BASE + "/presentations", headers={"Authorization": "Bearer " + token})
    try:
        with urllib.request.urlopen(request) as response:
            checks += 1
            if response.status != 200:
                failures.append(f"an api key could not read presentations: {response.status}")
    except urllib.error.HTTPError as error:
        failures.append(f"an api key could not read presentations: {error.code} {error.read()[:200]!r}")
    # A read-only key must not write.
    request = urllib.request.Request(BASE + "/presentations", data=b"{}", method="POST",
                                     headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    try:
        urllib.request.urlopen(request)
        failures.append("a read-only api key was allowed to create a deck")
    except urllib.error.HTTPError as error:
        checks += 1
        if error.code != 403:
            failures.append(f"a read-only key writing gave {error.code}, expected 403")
    call("DELETE", f"/api-keys/{key['id']}", expect=[204, 200])

print("── authentication and errors ──")
request = urllib.request.Request(BASE + "/presentations")
try:
    urllib.request.urlopen(request)
    failures.append("an unauthenticated request was allowed")
except urllib.error.HTTPError as error:
    checks += 1
    if error.code != 401:
        failures.append(f"unauthenticated gave {error.code}, expected 401")
call("GET", "/presentations/00000000-0000-0000-0000-000000000000", expect=404)
call("POST", "/presentations", {"title": ""}, expect=[201, 422])
call("GET", "/nope", expect=404)

print()
print(f"{checks} checks, {len(failures)} failures")
for failure in failures:
    print(" ✗ " + failure)
sys.exit(1 if failures else 0)
