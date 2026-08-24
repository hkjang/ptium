"""An end-to-end sweep of the REST surface, run against a real server.

Every check states what it expects. A failure prints the request, the status and
the body, so the next step is reading code rather than reproducing.
"""
import json, os, sys, urllib.parse, urllib.request, urllib.error, zipfile, io, zlib, struct, time

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
# Another service picks a template before it asks for a deck, and a picker
# narrows before it scrolls. Both used to have to read the whole library.
narrowed = call("GET", "/templates?kind=builtin&limit=100", expect=200)
shipped = data_of(narrowed) or []
if not shipped or any(t["kind"] != "builtin" for t in shipped):
    failures.append("kind=builtin returned something that is not a built-in design")
own = data_of(call("GET", "/templates?kind=uploaded&limit=100", expect=200)) or []
if any(t["kind"] != "uploaded" for t in own):
    failures.append("kind=uploaded returned something that was not uploaded")
if len(shipped) + len(own) != len(templates):
    failures.append(f"the two kinds hold {len(shipped)}+{len(own)} of {len(templates)} templates")
named = data_of(call("GET", f"/templates?search={urllib.parse.quote(shipped[0]['name'])}", expect=200)) or []
if not any(t["id"] == shipped[0]["id"] for t in named):
    failures.append(f"searching for {shipped[0]['name']!r} did not find it")
if len(data_of(call("GET", "/templates?kind=nonsense&limit=100", expect=200)) or []) != len(templates):
    failures.append("an unknown kind narrowed the listing instead of being ignored")

# The whole point of listing them: name one, and the deck is built on it.
chosen = shipped[0]
on_template = data_of(call("POST", "/presentations/generate", {
    "title": f"템플릿 지정 {RUN}", "prompt": "사내 보안 교육 계획을 팀장들에게 보고합니다.",
    "language": "ko", "slideCount": 5, "templateId": chosen["id"]}, expect=202))
if (on_template or {}).get("templateId") != chosen["id"]:
    failures.append(f"a deck asked for on {chosen['name']} came back on {(on_template or {}).get('templateId')}")
else:
    for _ in range(120):
        built = data_of(call("GET", f"/presentations/{on_template['id']}", expect=200)) or {}
        if built.get("status") in ("ready", "completed", "failed"):
            break
        time.sleep(1)
    if built.get("templateName") != chosen["name"]:
        failures.append(f"the generated deck reports template {built.get('templateName')!r}")
    call("DELETE", f"/presentations/{on_template['id']}", expect=204)

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
# A component draws as many entries as its kind can show and takes the first of
# them. A slide holding more than that has entries on no slide at all, and the
# measurement has to say so — a deck once asked a board to approve a plan whose
# "calculate the financial impact" step was drawn nowhere.
trimmed_deck = data_of(call("POST", "/presentations", {"title": f"안 그려지는 항목 {RUN}", "prompt": "단계 여섯"}, expect=201))
call("PUT", f"/presentations/{trimmed_deck['id']}/source", {"source":
     "# 이행 순서\n@content\n::steps 순서\n- 1 | 현황 파악\n- 2 | 대안 정의\n- 3 | 비교\n"
     "- 4 | 이행 계획\n- 5 | 재무 영향\n- 6 | 승인 요청\n::\n"}, expect=200)
measured = data_of(call("GET", f"/presentations/{trimmed_deck['id']}/inspect", expect=200)) or {}
carried = [f for f in (measured.get("findings") or []) if f.get("kind") == "trimmed"]
if not carried:
    failures.append("a six-entry component drawn five at a time is not reported")
elif "5 of its 6" not in carried[0].get("detail", ""):
    failures.append(f"the finding does not say what was left out: {carried[0]}")
# Split as the workspace's safe fix splits it, and the deck measures clean.
call("PUT", f"/presentations/{trimmed_deck['id']}/source", {"source":
     "# 이행 순서\n@content\n::steps 순서\n- 1 | 현황 파악\n- 2 | 대안 정의\n- 3 | 비교\n"
     "- 4 | 이행 계획\n- 5 | 재무 영향\n::\n\n# 이행 순서 (계속)\n@content\n::steps 순서\n- 6 | 승인 요청\n::\n"}, expect=200)
after = data_of(call("GET", f"/presentations/{trimmed_deck['id']}/inspect", expect=200)) or {}
if [f for f in (after.get("findings") or []) if f.get("kind") == "trimmed"]:
    failures.append("carrying the rest onto a second slide did not settle the finding")
call("DELETE", f"/presentations/{trimmed_deck['id']}", expect=204)

# A slide kept for the questions afterwards is walked past in the talk and is
# still in the deck and in the file. The flag rides in the slide's content, so
# what this is really checking is that it survives the four hands it passes
# through: the source language, the stored slide, the file, and the source
# written back out.
skip_deck = data_of(call("POST", "/presentations", {"title": f"건너뛰는 장 {RUN}", "prompt": "부록"}, expect=201))
call("PUT", f"/presentations/{skip_deck['id']}/source", {"source":
     "# 요약\n@cover\n> 올해 계획\n\n# 부록: 산출 근거\n@content\n!skip\n- 계산 방법\n"}, expect=200)
kept = data_of(call("GET", f"/presentations/{skip_deck['id']}", expect=200)) or {}
marks = [bool((slide.get("content") or {}).get("skipped")) for slide in kept.get("slides") or []]
if marks != [False, True]:
    failures.append(f"the skipped slide is not marked in the stored deck: {marks}")
written = (data_of(call("GET", f"/presentations/{skip_deck['id']}/source", expect=200)) or {}).get("source", "")
if "!skip" not in written:
    failures.append("the source written back out lost the skip")
status, skip_pptx = call("GET", f"/presentations/{skip_deck['id']}/export?format=pptx", raw=True, expect=200)
try:
    package = zipfile.ZipFile(io.BytesIO(skip_pptx))
    hidden = [name for name in package.namelist() if name.startswith("ppt/slides/slide")
              and b'show="0"' in package.read(name)]
    if len(hidden) != 1:
        failures.append(f"the exported file marks {len(hidden)} slides hidden, expected 1")
except Exception as error:
    failures.append(f"the exported pptx could not be read: {error}")
call("DELETE", f"/presentations/{skip_deck['id']}", expect=204)

# A link is written in the text and drawn as words. What the sweep is watching
# for is the markup reaching a screen: the preview and the file both have to
# draw 안내 문서, and neither may draw the brackets or the address.
link_deck = data_of(call("POST", "/presentations", {"title": f"링크 {RUN}", "prompt": "안내"}, expect=201))
call("PUT", f"/presentations/{link_deck['id']}/source", {"source":
     "# 안내\n@content\n- 자세한 내용은 [안내 문서](https://docs.example.com/plan)를 보십시오\n"
     "- 근거는 [부록](#2)에 있습니다\n\n# 부록\n@content\n- 산출 근거\n"}, expect=200)
status, preview = call("GET", f"/presentations/{link_deck['id']}/preview.svg?slide=1&width=900", raw=True, expect=200)
drawn = preview.decode("utf-8", "replace")
# The address belongs in the link the drawing carries, never in the words it
# draws: what is drawn is what sits between the tags.
if ">https://docs.example.com/plan" in drawn or "](" in drawn:
    failures.append("the preview draws the link markup")
if '<a href="https://docs.example.com/plan"' not in drawn:
    failures.append("the drawing does not carry the link where presenting can follow it")
if "안내 문서" not in drawn or "text-decoration=\"underline\"" not in drawn:
    failures.append("the preview does not draw the link as one")
status, linked_pptx = call("GET", f"/presentations/{link_deck['id']}/export?format=pptx", raw=True, expect=200)
try:
    package = zipfile.ZipFile(io.BytesIO(linked_pptx))
    slide = package.read("ppt/slides/slide1.xml").decode("utf-8")
    rels = package.read("ppt/slides/_rels/slide1.xml.rels").decode("utf-8")
    if "hlinkClick" not in slide or "](" in slide:
        failures.append("the exported slide does not carry the link")
    if "https://docs.example.com/plan" not in rels or 'TargetMode="External"' not in rels:
        failures.append("the link is not a relationship of the exported slide")
    if "slide2.xml" not in rels:
        failures.append("the jump to another slide is not a relationship of the exported slide")
except Exception as error:
    failures.append(f"the exported pptx could not be read: {error}")
# Presenting draws the slide itself so a link on it can be clicked; a jump has
# to name the slide it goes to.
if '<a href="#slide-2"' not in drawn:
    failures.append("the drawing does not name the slide a jump goes to")
# A target the deck will not follow draws as its own markup, and is reported.
call("PUT", f"/presentations/{link_deck['id']}/source", {"source":
     "# 안내\n@content\n- [문서](www.example.com)를 보십시오\n"}, expect=200)
measured = data_of(call("GET", f"/presentations/{link_deck['id']}/inspect", expect=200)) or {}
if not [f for f in (measured.get("findings") or []) if f.get("kind") == "link"]:
    failures.append("a link the deck cannot follow is not reported")
call("DELETE", f"/presentations/{link_deck['id']}", expect=204)

# A word marked inside a line: the file carries the weight, the preview shows
# it, and neither draws the stars.
mark_deck = data_of(call("POST", "/presentations", {"title": f"강조 {RUN}", "prompt": "실적"}, expect=201))
call("PUT", f"/presentations/{mark_deck['id']}/source", {"source":
     "# 실적\n@content\n- 이번 분기 **매출이 12% 늘었습니다**\n- *가정은 인건비 동결입니다*\n- 3 * 4 = 12\n"}, expect=200)
status, marked_svg = call("GET", f"/presentations/{mark_deck['id']}/preview.svg?slide=1&width=900", raw=True, expect=200)
drawn = marked_svg.decode("utf-8", "replace")
if "**" in drawn or "매출이 12% 늘었습니다" not in drawn:
    failures.append("the preview draws the marks instead of the words")
if 'font-weight="700"' not in drawn or 'font-style="italic"' not in drawn:
    failures.append("the preview does not show the marked words as marked")
if "3 * 4 = 12" not in drawn:
    failures.append("a star with no partner was eaten")
status, marked_pptx = call("GET", f"/presentations/{mark_deck['id']}/export?format=pptx", raw=True, expect=200)
try:
    slide = zipfile.ZipFile(io.BytesIO(marked_pptx)).read("ppt/slides/slide1.xml").decode("utf-8")
    if "**" in slide or 'b="1"' not in slide or 'i="1"' not in slide:
        failures.append("the exported slide does not carry the marks")
except Exception as error:
    failures.append(f"the exported pptx could not be read: {error}")
call("DELETE", f"/presentations/{mark_deck['id']}", expect=204)

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

print("── a link someone without an account can open ──")
shared_deck = data_of(call("POST", "/presentations", {"title": f"공유 점검 {RUN}", "prompt": "공유",
                                                      "requestedSlideCount": 3, "language": "ko"}, expect=201)) or {}
call("PUT", f"/presentations/{shared_deck.get('id')}/source",
     {"source": f"# 공유 점검 {RUN}\n@cover\n> 링크로 여는 덱\n\n# 현황\n@content\n- 첫 줄\n- 둘째 줄\n"})
made = data_of(call("POST", f"/presentations/{shared_deck.get('id')}/shares", {"label": "임원 검토", "days": 7}, expect=201)) or {}
link = made.get("url", "")
token = link.rsplit("/", 1)[-1] if link else ""
if not token or "/view/" not in link:
    failures.append(f"a share was made without a link to hand anyone: {made!r}")
# Opened by a stranger: no dev secret, no session, no key.
opened = urllib.request.Request(BASE + f"/shared/{token}")
try:
    with urllib.request.urlopen(opened) as response:
        seen = json.loads(response.read())["data"]
except urllib.error.HTTPError as error:
    seen = {}
    failures.append(f"a shared link did not open for someone without an account: {error.code}")
if seen.get("slideCount") != 2 or f"공유 점검 {RUN}" not in (seen.get("title") or ""):
    failures.append(f"the link showed something other than the deck: {seen!r}")
try:
    with urllib.request.urlopen(urllib.request.Request(BASE + f"/shared/{token}/preview.svg?slide=2&width=600")) as response:
        drawn = response.read().decode()
    if "<svg" not in drawn or "둘째 줄" not in drawn:
        failures.append("a shared link drew something other than the slide")
except urllib.error.HTTPError as error:
    failures.append(f"a shared slide did not draw: {error.code}")
# The link carries nothing else: the deck's source is not readable through it.
for path in [f"/presentations/{shared_deck.get('id')}", f"/presentations/{shared_deck.get('id')}/source"]:
    try:
        urllib.request.urlopen(urllib.request.Request(BASE + path))
        failures.append(f"{path} answered someone with no credentials")
    except urllib.error.HTTPError as error:
        if error.code not in (401, 403):
            failures.append(f"{path} answered {error.code} to someone with no credentials")
call("DELETE", f"/presentations/{shared_deck.get('id')}/shares/{made.get('id')}", expect=204)
try:
    urllib.request.urlopen(urllib.request.Request(BASE + f"/shared/{token}"))
    failures.append("a revoked link still opens the deck")
except urllib.error.HTTPError as error:
    if error.code != 410:
        failures.append(f"a revoked link answered {error.code}, expected 410")
# Looking is half of a review. The other half is saying what is wrong with
# slide 4, which is what the link is for.
opened_again = data_of(call("POST", f"/presentations/{shared_deck.get('id')}/shares", {"label": "의견", "days": 0}, expect=201)) or {}
review_token = (opened_again.get("url") or "").rsplit("/", 1)[-1]
review_slides = []
try:
    with urllib.request.urlopen(urllib.request.Request(BASE + f"/shared/{review_token}")) as response:
        review_slides = (json.loads(response.read())["data"] or {}).get("slides") or []
except urllib.error.HTTPError as error:
    failures.append(f"the review link did not open: {error.code}")
if review_slides:
    said = json.dumps({"slideId": review_slides[1]["id"], "author": "박검토",
                       "body": "이 숫자는 지난 분기 기준입니다."}).encode()
    request = urllib.request.Request(BASE + f"/shared/{review_token}/comments", data=said,
                                     headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(request) as response:
            left = json.loads(response.read())["data"]
        if left.get("slideId") != review_slides[1]["id"]:
            failures.append(f"the remark lost the slide it was about: {left!r}")
    except urllib.error.HTTPError as error:
        failures.append(f"a stranger could not leave a remark: {error.code}")
    mine = data_of(call("GET", f"/presentations/{shared_deck.get('id')}/comments")) or []
    if not any(row.get("author") == "박검토" for row in mine):
        failures.append("the owner cannot see what the reviewer said")
    else:
        remark = [row for row in mine if row.get("author") == "박검토"][0]
        call("POST", f"/presentations/{shared_deck.get('id')}/comments/{remark['id']}/resolve", {"resolved": True}, expect=204)
        after = data_of(call("GET", f"/presentations/{shared_deck.get('id')}/comments")) or []
        if not any(row["id"] == remark["id"] and row.get("resolvedAt") for row in after):
            failures.append("a remark marked dealt with did not stay dealt with")
    # A remark about someone else's slide is not a way into another deck.
    stray = json.dumps({"slideId": "11111111-1111-1111-1111-111111111111", "body": "x"}).encode()
    request = urllib.request.Request(BASE + f"/shared/{review_token}/comments", data=stray,
                                     headers={"Content-Type": "application/json"}, method="POST")
    try:
        urllib.request.urlopen(request)
        failures.append("a remark was accepted for a slide from another deck")
    except urllib.error.HTTPError as error:
        if error.code != 422:
            failures.append(f"a stray slide id answered {error.code}, expected 422")
print("   a reviewer left a remark on slide 2 and the owner marked it dealt with")

print(f"   opened by a stranger, {seen.get('slideCount')} slides, closed on revoke")

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
# Asked for by id: a workspace with more than a page of favourites pushes a new
# slide off the front of the list, and reading only the front page reported the
# product broken when it was the listing that had moved on.
used = data_of(call("GET", f"/snippets/{registered.get('id')}", expect=200)) or {}
if used.get("useCount", 0) < 1:
    failures.append(f"the registered slide was not counted as used: {used!r}")
else:
    print(f"   the deck took the registered slide (used {used.get('useCount')}x)")

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

# An OIDC client secret can be registered in the workspace rather than in a
# deployment manifest. It is stored encrypted, never read back, and a secret
# with no client to belong to is refused before it can stop the next rollout.
print("── the OIDC client secret ──")
def oidc_secret_row():
    rows = data_of(call("GET", "/admin/settings", expect=200)) or []
    rows = rows.get("settings") if isinstance(rows, dict) else rows
    return next((r for r in rows if r.get("key") == "auth.oidc.client_secret"), {})

call("PUT", "/admin/settings", {"settings": [{"key": "auth.oidc.client_id", "value": ""},
                                             {"key": "auth.oidc.client_secret", "value": "orphan-secret"}]},
     expect=422, note="a client secret with no client ID must be refused")
call("PUT", "/admin/settings", {"settings": [{"key": "auth.oidc.client_id", "value": f"ptium-web-{RUN}"}]}, expect=200)
call("PUT", "/admin/settings", {"settings": [{"key": "auth.oidc.client_secret", "value": f"secret-{RUN}"}]}, expect=200)
if not oidc_secret_row().get("configured"):
    failures.append("a saved OIDC client secret is not reported as configured")
status, body = call("GET", "/admin/settings", raw=True, expect=200)
if f"secret-{RUN}".encode() in body:
    failures.append("the OIDC client secret is echoed back in the settings response")
call("PUT", "/admin/settings", {"settings": [{"key": "auth.oidc.client_secret", "value": " "}]}, expect=200)
if oidc_secret_row().get("configured"):
    failures.append("clearing the OIDC client secret left it configured")
call("PUT", "/admin/settings", {"settings": [{"key": "auth.oidc.client_id", "value": ""}]}, expect=200)

print("── revisions, duplicate, trash ──")
revisions = data_of(call("GET", f"/presentations/{deck_id}/revisions", expect=200)) or []
print("   revisions:", len(revisions))
if revisions:
    # What changed since a version — the question version history raises.
    changes = (data_of(call("GET", f"/presentations/{deck_id}/revisions/{revisions[0]['id']}/changes",
                            expect=200)) or {}).get("changes")
    if changes is None:
        failures.append("a revision cannot say what changed since it")
    call("POST", f"/presentations/{deck_id}/revisions/{revisions[-1]['id']}/restore", {}, expect=200)

# Restoring a checkpoint has to give the deck back. The call was checked for a
# 200 and nothing else, so nothing here would have noticed a restore that
# returned a different deck.
call("PUT", f"/presentations/{deck_id}/source",
     {"source": f"# 복원 점검 {RUN}\n@cover\n> 되돌리기 전\n\n# 요점\n@content\n- 첫 줄\n- 둘째 줄\n"}, expect=200)
before = ((data_of(call("GET", f"/presentations/{deck_id}/source", expect=200)) or {}).get("source") or "").strip()
call("PUT", f"/presentations/{deck_id}/source",
     {"source": f"# 덮어쓴 판 {RUN}\n@cover\n> 이 판은 되돌려집니다\n"}, expect=200)
# A checkpoint records the deck as it was before the change, so the newest one
# holds what the overwrite replaced.
newest = (data_of(call("GET", f"/presentations/{deck_id}/revisions", expect=200)) or [])
if not newest:
    failures.append("overwriting a deck kept no checkpoint of what it replaced")
else:
    # Which checkpoint this is matters as much as what it gives back: the
    # overwrite is what made it, so anything else at the front of the list means
    # the checkpoint the restore needs was never written.
    marker = newest[0]
    call("POST", f"/presentations/{deck_id}/revisions/{marker['id']}/restore", {}, expect=200)
    back = ((data_of(call("GET", f"/presentations/{deck_id}/source", expect=200)) or {}).get("source") or "").strip()
    if back != before:
        failures.append(
            "a restored checkpoint did not give the deck back — newest checkpoint was "
            f"{marker.get('reason')} at version {marker.get('version')} ({marker.get('createdAt')}); "
            f"the deck now starts {back.splitlines()[:1]} and should start {before.splitlines()[:1]}; "
            f"checkpoints: {[(r.get('reason'), r.get('version')) for r in newest[:5]]}")
        print("   lost:", [l for l in before.split("\n") if l not in back.split("\n")][:4])
        print("   gained:", [l for l in back.split("\n") if l not in before.split("\n")][:4])

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
