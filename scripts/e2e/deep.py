"""The parts a click-through misses: the presenter's second window, the MCP
endpoint, a template someone actually uploads, and whether the workspace can be
used without a mouse."""
import os, sys, json, time, urllib.request, urllib.error
from playwright.sync_api import sync_playwright

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/")
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
OUT = sys.argv[1] if len(sys.argv) > 1 else "."
RUN = str(int(time.time()))[-6:]
failures = []

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
deck = [d for d in decks["data"] if (d.get("slideCount") or 0) > 2][0]["id"]
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
    if state["data"]["status"] != "completed":
        failures.append(f"generating into an uploaded template ended as {state['data']['status']}: {state['data'].get('errorMessage')}")
    status, inspected = api(f"/api/v1/presentations/{draft_id}/inspect")
    print("   defects:", inspected["data"]["defects"], "advisories:", inspected["data"]["advisories"])
    if inspected["data"]["defects"]:
        failures.append(f"a deck generated into an uploaded template has defects: {inspected['data']['findings']}")
except urllib.error.HTTPError as error:
    failures.append(f"uploading an exported deck as a template failed: {error.code} {error.read()[:300]!r}")

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

print()
print(f"{len(failures)} failures")
for failure in failures:
    print(" ✗ " + failure)
sys.exit(1 if failures else 0)
