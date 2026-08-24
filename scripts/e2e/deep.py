"""The parts a click-through misses: the presenter's second window, the MCP
endpoint, a template someone actually uploads, and whether the workspace can be
used without a mouse."""
import atexit
import os, sys, json, time, urllib.request, urllib.error
from playwright.sync_api import sync_playwright

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/")
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
OUT = sys.argv[1] if len(sys.argv) > 1 else "."
RUN = str(int(time.time()))[-6:]
failures = []


# A sweep that stops early has still found what it found. When a page never
# appears and playwright raises, the operator used to see a traceback and
# nothing else — the thirteen routes that had already rendered nothing went
# with it. The tally is printed however the run ends.
def stopped_early(kind, error, trace):
    failures.append(f"the sweep stopped early: {kind.__name__}: {error}".replace("\n", " ")[:300])
    sys.__excepthook__(kind, error, trace)


def report():
    print()
    print(f"{len(failures)} failures")
    for failure in failures:
        print(" \u2717 " + failure)


sys.excepthook = stopped_early
atexit.register(report)

def api(path, method="GET", body=None, raw=False, headers=None):
    data = json.dumps(body).encode() if body is not None else None
    head = {"X-Ptium-Dev-Secret": SECRET}
    if data:
        head["Content-Type"] = "application/json"
    if headers and "Authorization" in headers:
        # An API key stands on its own; development auth refuses a bearer token
        # that is not one of its own, which is the point of it.
        head.pop("X-Ptium-Dev-Secret")
    head.update(headers or {})
    request = urllib.request.Request(BASE + path, data=data, headers=head, method=method)
    try:
        with urllib.request.urlopen(request) as response:
            payload = response.read()
            return response.status, (payload if raw else (json.loads(payload) if payload else None))
    except urllib.error.HTTPError as error:
        return error.code, error.read()

print("── MCP ──")
status, key = api("/api/v1/api-keys", "POST", {"name": f"mcp-{RUN}", "scopes": ["mcp:use", "presentations:read", "presentations:write"]})
token = (key or {}).get("data", {}).get("secret")
if not token:
    failures.append(f"could not mint an MCP key: {status} {str(key)[:200]}")
else:
    auth = {"Authorization": "Bearer " + token}
    status, body = api("/mcp", "POST", {"jsonrpc": "2.0", "id": 1, "method": "initialize",
                                        "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                                                   "clientInfo": {"name": "e2e", "version": "1"}}}, headers=auth)
    if status != 200:
        failures.append(f"MCP initialize → {status}: {str(body)[:200]}")
    status, body = api("/mcp", "POST", {"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, headers=auth)
    tools = ((body or {}).get("result") or {}).get("tools") if isinstance(body, dict) else None
    print("   tools:", [t["name"] for t in tools or []][:8])
    if not tools:
        failures.append(f"MCP tools/list returned nothing: {status} {str(body)[:200]}")
    status, body = api("/mcp", "POST", {"jsonrpc": "2.0", "id": 3, "method": "tools/call",
                                        "params": {"name": "ptium.list_presentations", "arguments": {"limit": 2}}}, headers=auth)
    if status != 200 or not isinstance(body, dict) or "result" not in body:
        failures.append(f"MCP list_presentations → {status}: {str(body)[:200]}")
    # A tool that does not exist must be an error, not a crash.
    status, body = api("/mcp", "POST", {"jsonrpc": "2.0", "id": 4, "method": "tools/call",
                                        "params": {"name": "ptium.nope", "arguments": {}}}, headers=auth)
    if status >= 500:
        failures.append(f"an unknown MCP tool gave {status}")

print("── a deck exported, then uploaded back as a template ──")
status, decks = api("/api/v1/presentations?limit=10")
existing = [d for d in decks["data"] if (d.get("slideCount") or 0) > 2]
if existing:
    deck = existing[0]["id"]
else:
    # A fresh database has nothing to export, so the sweep writes its own deck.
    status, made = api("/api/v1/presentations", "POST", {
        "title": f"내보내기용 덱 {RUN}", "prompt": "왕복 점검", "requestedSlideCount": 4, "language": "ko"})
    deck = made["data"]["id"]
    api(f"/api/v1/presentations/{deck}/source", "PUT", {"source":
        "# 왕복 점검\n@cover\n> 내보내고 다시 읽습니다\n\n# 실적 요약\n- 매출 1,240억\n  - 신규 채널이 절반\n- 이익률 9.8%\n!notes 원인을 채널별로 나눕니다.\n\n# 채널별 매출\n::table 분기 비교\n- 채널 | 3분기 | 4분기\n- 직영 | 420억 | 480억\n::\n\n# 다음 단계\n@closing\n> 결정과 실행을 나눠 요청합니다\n"})
status, package = api(f"/api/v1/presentations/{deck}/export?format=pptx", raw=True)
print(f"   exported {len(package):,} bytes")
boundary = "----ptium-deep"
parts = [f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"round-trip-{RUN}.pptx\"\r\nContent-Type: application/vnd.openxmlformats-officedocument.presentationml.presentation\r\n\r\n".encode()
         + package + b"\r\n",
         f"--{boundary}\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\n왕복 템플릿 {RUN}\r\n".encode(),
         f"--{boundary}\r\nContent-Disposition: form-data; name=\"scope\"\r\n\r\nprivate\r\n".encode(),
         f"--{boundary}--\r\n".encode()]
request = urllib.request.Request(BASE + "/api/v1/templates", data=b"".join(parts), method="POST",
                                 headers={"X-Ptium-Dev-Secret": SECRET,
                                          "Content-Type": f"multipart/form-data; boundary={boundary}"})
try:
    with urllib.request.urlopen(request) as response:
        template = json.loads(response.read())["data"]
    print("   uploaded template:", template["name"], "layouts:", template["layoutCount"])
    if template["layoutCount"] < 1:
        failures.append("an uploaded template reports no layouts")
    status, body = api(f"/api/v1/templates/{template['id']}/preview.svg?width=320", raw=True)
    if status != 200:
        failures.append(f"an uploaded template has no preview: {status}")
    # And a deck can be generated into it.
    status, draft = api("/api/v1/presentations", "POST", {
        "title": f"업로드 템플릿 생성 {RUN}", "prompt": "사내 표준 템플릿으로 만드는 3장짜리 현황 보고",
        "requestedSlideCount": 3, "language": "ko", "templateId": template["id"]})
    draft_id = draft["data"]["id"]
    api(f"/api/v1/presentations/{draft_id}/generate", "POST", {})
    for _ in range(60):
        status, state = api(f"/api/v1/presentations/{draft_id}")
        if state["data"]["status"] in ("completed", "failed"):
            break
        time.sleep(1)
    print("   generated into it:", state["data"]["status"], len(state["data"].get("slides") or []), "slides")
    if state["data"]["status"] == "failed":
        failures.append(f"generating into an uploaded template failed: {state['data'].get('errorMessage')}")
    elif state["data"]["status"] != "completed":
        # A deployment with a real provider takes minutes, not seconds. That is a
        # valid deployment, so the sweep says what it saw and moves on.
        print("   (still with the model; the rest of this section is skipped)")
    else:
        status, inspected = api(f"/api/v1/presentations/{draft_id}/inspect")
        print("   defects:", inspected["data"]["defects"], "advisories:", inspected["data"]["advisories"])
        if inspected["data"]["defects"]:
            failures.append(f"a deck generated into an uploaded template has defects: {inspected['data']['findings']}")
except urllib.error.HTTPError as error:
    failures.append(f"uploading an exported deck as a template failed: {error.code} {error.read()[:300]!r}")

print("── a deck someone already has, read back in ──")
# The deck exported above is a real PowerPoint file; importing it is the round
# trip a customer makes with last quarter's report.
boundary = "----ptium-import"
parts = [f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"old-deck-{RUN}.pptx\"\r\nContent-Type: application/vnd.openxmlformats-officedocument.presentationml.presentation\r\n\r\n".encode()
         + package + b"\r\n",
         f"--{boundary}--\r\n".encode()]
request = urllib.request.Request(BASE + "/api/v1/presentations/import", data=b"".join(parts), method="POST",
                                 headers={"X-Ptium-Dev-Secret": SECRET,
                                          "Content-Type": f"multipart/form-data; boundary={boundary}"})
try:
    with urllib.request.urlopen(request) as response:
        imported = json.loads(response.read())["data"]
    print("   imported:", imported["presentation"]["title"], "|", imported["slides"], "slides")
    for warning in imported.get("warnings") or []:
        print("   warning:", warning)
    if imported["slides"] < 3:
        failures.append(f"the import read only {imported['slides']} slides out of a deck that has more")
    titles = [slide["title"] for slide in imported["presentation"]["slides"]]
    if not any(title.strip() for title in titles):
        failures.append("every imported slide came back without a title")
    status, inspected = api(f"/api/v1/presentations/{imported['presentation']['id']}/inspect")
    print("   imported deck defects:", inspected["data"]["defects"], "advisories:", inspected["data"]["advisories"])
    if inspected["data"]["defects"]:
        failures.append(f"the imported deck has defects: {inspected['data']['findings']}")
except urllib.error.HTTPError as error:
    failures.append(f"importing an exported deck failed: {error.code} {error.read()[:300]!r}")

with sync_playwright() as play:
    browser = play.chromium.launch()
    context = browser.new_context(viewport={"width": 1400, "height": 900},
                                  extra_http_headers={"X-Ptium-Dev-Secret": SECRET})
    page = context.new_page()

    print("── the presenter's second window ──")
    page.goto(f"{BASE}/presentations/{deck}/editor", wait_until="networkidle")
    page.wait_for_selector(".freeform-page", timeout=25000)
    page.wait_for_timeout(2000)
    page.get_by_role("button", name="발표", exact=True).first.click()
    page.wait_for_timeout(2500)
    if page.locator(".presentation-mode").count() == 0:
        failures.append("the presentation did not start")
    else:
        with context.expect_page() as opened:
            page.keyboard.press("p")
        presenter = opened.value
        presenter.wait_for_load_state("networkidle")
        presenter.wait_for_timeout(2500)
        text = presenter.locator("body").inner_text()
        if "다음" not in text and "노트" not in text:
            failures.append("the presenter window shows neither the next slide nor the notes")
        # Moving in one window moves the other.
        page.keyboard.press("ArrowRight")
        page.wait_for_timeout(1200)
        moved = presenter.locator("body").inner_text()
        if moved == text:
            failures.append("the presenter window did not follow the audience screen")
        presenter.screenshot(path=f"{OUT}/deep-presenter.png")
        page.keyboard.press("Escape")
        page.wait_for_timeout(800)

    print("── keyboard only ──")
    page.goto(f"{BASE}/dashboard", wait_until="networkidle")
    page.wait_for_timeout(1500)
    reachable = []
    for _ in range(14):
        page.keyboard.press("Tab")
        reachable.append(page.evaluate("() => { const a = document.activeElement; return a ? (a.getAttribute('aria-label') || a.innerText || a.tagName).slice(0,26) : '' }"))
    print("   tab order:", [r.replace("\n", " ") for r in reachable[:8]])
    if len([r for r in reachable if r.strip()]) < 6:
        failures.append("tabbing through the dashboard reaches almost nothing")

    print("── names on controls ──")
    unnamed = page.evaluate("""() => {
      const out = []
      document.querySelectorAll('button, a[href]').forEach(el => {
        const name = (el.innerText || '').trim() || el.getAttribute('aria-label') || el.getAttribute('title')
        if (!name) out.push(el.className ? String(el.className).slice(0, 40) : el.tagName)
      })
      return out
    }""")
    if unnamed:
        failures.append(f"controls with no accessible name on the dashboard: {unnamed[:6]}")
    for path in ["/templates", "/images", "/guide"]:
        page.goto(BASE + path, wait_until="networkidle")
        page.wait_for_timeout(1500)
        unnamed = page.evaluate("""() => {
          const out = []
          document.querySelectorAll('button, a[href]').forEach(el => {
            const name = (el.innerText || '').trim() || el.getAttribute('aria-label') || el.getAttribute('title')
            if (!name) out.push(el.className ? String(el.className).slice(0, 40) : el.tagName)
          })
          return out
        }""")
        if unnamed:
            failures.append(f"controls with no accessible name on {path}: {unnamed[:6]}")
        images = page.evaluate("""() => Array.from(document.images).filter(i => !i.alt).length""")
        if images:
            failures.append(f"{images} images without alt text on {path}")

    browser.close()

sys.exit(1 if failures else 0)
