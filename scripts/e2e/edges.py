"""The edges: sizes at and past the limits, text nobody expects, and two people
editing at once. What matters is that the server answers rather than falls over,
and that the answer says what went wrong."""
import json, os, sys, time, zipfile, io, urllib.request, urllib.error, threading, zlib, struct

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/") + "/api/v1"
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
RUN = str(int(time.time()))[-6:]
failures, checks = [], 0

def call(method, path, body=None, raw=False, files=None, expect=None, note=""):
    global checks
    headers = {"X-Ptium-Dev-Secret": SECRET}
    data = None
    if files:
        boundary = "----ptium-edges"
        parts = []
        for name, (filename, content, ctype) in files.items():
            head = f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\""
            head += f"; filename=\"{filename}\"\r\nContent-Type: {ctype}\r\n\r\n" if filename else "\r\n\r\n"
            parts.append(head.encode() + (content if isinstance(content, bytes) else content.encode()) + b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode())
        data = b"".join(parts)
        headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"
    elif body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request) as response:
            status, payload = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, payload = error.code, error.read()
    except Exception as error:
        checks += 1
        failures.append(f"{method} {path} raised {error}. {note}")
        return 0, None
    checks += 1
    if expect is not None and status not in (expect if isinstance(expect, (list, tuple)) else [expect]):
        failures.append(f"{method} {path} → {status}, want {expect}. {note}\n    {payload[:300]!r}")
    if raw:
        return status, payload
    try:
        return status, json.loads(payload) if payload else None
    except Exception:
        return status, payload

def data_of(result):
    status, body = result
    return (body or {}).get("data") if isinstance(body, dict) else None

def deck(title, **extra):
    payload = {"title": f"{title} {RUN}", "prompt": "edge", "requestedSlideCount": 3, "language": "ko"}
    payload.update(extra)
    return data_of(call("POST", "/presentations", payload, expect=201))["id"]

print("── deck size limits ──")
big = deck("50장")
source50 = "".join(f"# 슬라이드 {i}\n- 내용 {i}\n\n" for i in range(1, 51))
call("PUT", f"/presentations/{big}/source", {"source": source50}, expect=200, note="fifty slides is the documented maximum")
source51 = source50 + "# 하나 더\n- 넘침\n"
call("PUT", f"/presentations/{big}/source", {"source": source51}, expect=422, note="fifty-one must be refused, not truncated")
state = data_of(call("GET", f"/presentations/{big}", expect=200))
if len(state.get("slides") or []) != 50:
    failures.append(f"the deck should still hold 50 slides, has {len(state.get('slides') or [])}")

print("── text nobody expects ──")
odd = deck("이상한 글자")
strange = """# 이모지 🎉와 <script>alert(1)</script> & "따옴표"
@cover
> RTL: مرحبا · 제로폭:​ · 탭:\t끝

# 아주 긴 제목이 상자를 넘칠 때 어떻게 되는지 확인하기 위한 대단히 길고 장황하며 끝나지 않을 것 같은 제목
- """ + "매우 긴 항목 " * 60 + """
- 두 번째

# 잘못 쓴 구문
::kpi 닫지 않은 컴포넌트
- 값 | 1
# 다음 슬라이드가 컴포넌트를 닫습니다
- 정상 항목
::table
- a | b | c | d | e | f | g | h
"""
applied = data_of(call("PUT", f"/presentations/{odd}/source", {"source": strange}, expect=200,
                       note="odd text must compile rather than fail"))
print("   warnings:", (applied or {}).get("warnings"))
status, svg = call("GET", f"/presentations/{odd}/preview.svg?slide=1&width=600", raw=True, expect=200)
if b"<script>alert(1)</script>" in svg:
    failures.append("the preview did not escape a script tag")
status, pptx = call("GET", f"/presentations/{odd}/export?format=pptx", raw=True, expect=200)
try:
    package = zipfile.ZipFile(io.BytesIO(pptx))
    xml = package.read([n for n in package.namelist() if n.startswith("ppt/slides/slide1")][0]).decode()
    if "<script>" in xml:
        failures.append("the exported xml did not escape a script tag")
    if "🎉" not in xml:
        failures.append("an emoji was dropped from the export")
except Exception as error:
    failures.append(f"the export of odd text is unreadable: {error}")
inspected = data_of(call("GET", f"/presentations/{odd}/inspect", expect=200))
print("   odd deck defects:", (inspected or {}).get("defects"), "advisories:", (inspected or {}).get("advisories"))

print("── canvas object limits ──")
canvas = deck("개체")
call("PUT", f"/presentations/{canvas}/source", {"source": "# 한 장\n- 내용\n"}, expect=200)
state = data_of(call("GET", f"/presentations/{canvas}", expect=200))
slide = state["slides"][0]
def elements(count):
    return [{"id": f"e{i}", "kind": "text", "text": f"상자 {i}", "x": 1, "y": 1, "width": 10, "height": 5}
            for i in range(count)]
def patch(count, expect):
    body = {"slides": [{"id": slide["id"], "title": slide["title"], "layout": slide.get("layout", "content"),
                        "content": {"type": "template", "fields": {"title": [{"text": slide["title"]}]},
                                     "elements": elements(count)}}]}
    return call("PATCH", f"/presentations/{canvas}", body, expect=expect)
patch(200, 200)
patch(201, [422, 400])

print("── images at the edges ──")
call("POST", "/assets", files={"file": ("empty.png", b"", "image/png")}, expect=[400, 422])
huge = b"\x89PNG\r\n\x1a\n" + b"0" * (17 << 20)
call("POST", "/assets", files={"file": ("huge.png", huge, "image/png")}, expect=[413, 422])
# An SVG is a document: it is accepted and served inert, never executed.
call("POST", "/assets", files={"file": (f"evil-{RUN}.svg",
     f'<svg xmlns="http://www.w3.org/2000/svg"><title>{RUN}</title><script>alert(1)</script></svg>'.encode(),
     "image/svg+xml")}, expect=201)

print("── templates at the edges ──")
call("POST", "/templates", files={"file": ("notes.txt", b"not a package", "text/plain"),
                                  "scope": (None, "private", "")}, expect=[400, 422])
call("POST", "/templates", files={"file": ("empty.pptx", b"PK\x03\x04broken", "application/vnd.openxmlformats-officedocument.presentationml.presentation"),
                                  "scope": (None, "private", "")}, expect=[400, 422])

print("── two editors at once ──")
race = deck("동시 편집")
call("PUT", f"/presentations/{race}/source", {"source": "# 처음\n- 하나\n"}, expect=200)
current = data_of(call("GET", f"/presentations/{race}", expect=200))
version = current.get("version")
results = []
def edit(text):
    results.append(call("PUT", f"/presentations/{race}/source",
                        {"source": f"# {text}\n- {text}\n", "version": version}))
threads = [threading.Thread(target=edit, args=(f"편집 {i}",)) for i in range(2)]
for thread in threads: thread.start()
for thread in threads: thread.join()
codes = sorted(status for status, _ in results)
if codes != [200, 409]:
    failures.append(f"two simultaneous edits gave {codes}, expected one 200 and one 409")
print("   concurrent edits:", codes)

print("── saved slides at the edges ──")
call("POST", "/snippets", {"name": f"큰 조각 {RUN}", "source": "# 큰\n" + "- 줄\n" * 6000}, expect=422)
call("POST", "/snippets", {"name": f"빈 조각 {RUN}", "source": "   "}, expect=422)
call("POST", "/snippets", {"presentationId": big, "slide": 999}, expect=404)
call("POST", "/snippets", {"presentationId": "00000000-0000-0000-0000-000000000000", "slide": 1}, expect=404)

print("── pagination ──")
status, body = call("GET", "/presentations?limit=1&offset=0", expect=200)
if isinstance(body, dict) and body.get("meta"):
    print("   meta:", body["meta"])
call("GET", "/presentations?limit=1000", expect=200, note="an absurd limit should be clamped, not refused")
call("GET", "/presentations?limit=-1", expect=200)
call("GET", "/assets?sort=nonsense", expect=200, note="an unknown sort falls back rather than failing")

print()
print(f"{checks} checks, {len(failures)} failures")
for failure in failures:
    print(" ✗ " + failure)
sys.exit(1 if failures else 0)
