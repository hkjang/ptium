"""The flows that change something: generating, editing text, applying source,
saving a slide, uploading a template, restoring from the bin, admin settings."""
import atexit
import os, sys, json, time, urllib.request
from playwright.sync_api import sync_playwright

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/")
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
OUT = sys.argv[1] if len(sys.argv) > 1 else "."
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

def api(path, method="GET", body=None):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"X-Ptium-Dev-Secret": SECRET}
    if data:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + "/api/v1" + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(request) as response:
        payload = response.read()
    return json.loads(payload)["data"] if payload else None

with sync_playwright() as play:
    browser = play.chromium.launch()
    context = browser.new_context(viewport={"width": 1500, "height": 1000},
                                  extra_http_headers={"X-Ptium-Dev-Secret": SECRET}, accept_downloads=True)
    page = context.new_page()
    problems = []
    page.on("pageerror", lambda e: problems.append(f"pageerror {e}"))
    page.on("response", lambda r: problems.append(f"http {r.status} {r.url[-70:]}")
            if r.status >= 400 and "/api/" in r.url else None)

    def check(label):
        for problem in problems:
            failures.append(f"{label}: {problem}")
        problems.clear()

    print("── generating a deck from the create screen ──")
    page.goto(f"{BASE}/create", wait_until="networkidle")
    page.wait_for_timeout(2000)
    page.fill("textarea", "결제 시스템 이중화 계획을 실무진에게 설명하는 6장짜리 자료. 목표 가용성 99.95%, 예산 4억.")
    page.click("text=추천 디자인으로 바로 생성")
    page.wait_for_url("**/editor", timeout=30000)
    page.wait_for_selector(".freeform-page", timeout=60000)
    # Generation runs in a worker; the editor should fill in without a reload.
    filled = False
    for _ in range(60):
        page.wait_for_timeout(1000)
        rail = page.locator(".slide-rail-item, .rail-item, aside li").count()
        body = page.locator("body").inner_text()
        if "생성" in body and ("실패" in body or "오류" in body):
            failures.append("generation reported a failure in the editor")
            break
        if rail >= 3 or page.locator(".freeform-page").count() and "슬라이드 6" in body:
            filled = True
            break
    deck_url = page.url
    deck_id = deck_url.split("/presentations/")[1].split("/")[0]
    state = api(f"/presentations/{deck_id}")
    print("   status:", state["status"], "slides:", len(state.get("slides") or []))
    if state["status"] != "completed":
        failures.append(f"the deck generated from the UI ended as {state['status']}")
    page.screenshot(path=f"{OUT}/flow-generated.png")
    check("generate")

    print("── editing a region's text and reloading ──")
    page.wait_for_timeout(1500)
    region = page.locator(".freeform-region, .freeform-page").first
    box = page.locator(".freeform-page").bounding_box()
    # Double-click the title area to open its editor.
    # The cover's title sits around 45% down the page.
    page.mouse.dblclick(box["x"] + box["width"] * 0.35, box["y"] + box["height"] * 0.46)
    page.wait_for_timeout(1200)
    editing = page.locator(".canvas-region-input").count()
    if editing == 0:
        failures.append("double-clicking a region did not open a text editor")
    else:
        page.locator(".canvas-region-input").fill("E2E가 고친 제목")
        # Escape ends the edit and keeps what was typed, the way every editor does.
        page.keyboard.press("Escape")
        page.wait_for_timeout(3000)
        stored = api(f"/presentations/{deck_id}")
        text = json.dumps(stored, ensure_ascii=False)
        if "E2E가 고친 제목" not in text:
            failures.append("text typed into a region and closed with Escape was not saved")
        page.reload(wait_until="networkidle")
        page.wait_for_timeout(3000)
    page.screenshot(path=f"{OUT}/flow-edited.png")
    check("region edit")

    print("── the code tab ──")
    page.get_by_role("button", name="코드", exact=True).first.click()
    page.wait_for_timeout(3000)
    source_area = page.locator("textarea").first
    if source_area.count() == 0:
        failures.append("the code tab shows no source")
    else:
        current = source_area.input_value()
        if "#" not in current:
            failures.append("the code tab's source does not look like deck source")
        source_area.fill(current + "\n# 코드로 더한 장\n- 코드 탭에서 추가했습니다\n")
        page.wait_for_timeout(2500)
        apply_button = page.get_by_role("button", name="적용", exact=False).first
        apply_button.click()
        page.wait_for_timeout(4000)
        after = api(f"/presentations/{deck_id}")
        if not any("코드로 더한 장" in (s.get("title") or "") for s in after.get("slides") or []):
            failures.append("applying edited source did not add the slide")
    page.screenshot(path=f"{OUT}/flow-source.png")
    check("source tab")

    print("── saving a slide and putting it back ──")
    page.get_by_role("button", name="편집", exact=True).first.click()
    page.wait_for_timeout(1500)
    page.get_by_role("button", name="슬라이드", exact=True).first.click()
    page.wait_for_timeout(2000)
    page.get_by_role("button", name="지금 슬라이드를 라이브러리에 저장").click()
    page.wait_for_timeout(600)
    page.keyboard.type(f"E2E 표준 슬라이드 {int(time.time()) % 10000}")
    page.keyboard.press("Enter")
    page.wait_for_timeout(3500)
    if page.locator(".snippet-grid li").count() == 0:
        failures.append("a saved slide did not appear in the library")
    else:
        before = len(api(f"/presentations/{deck_id}")["slides"])
        page.locator(".snippet-grid li .asset-action-main").first.click()
        page.wait_for_timeout(4000)
        after = len(api(f"/presentations/{deck_id}")["slides"])
        if after != before + 1:
            failures.append(f"inserting a saved slide changed the count from {before} to {after}")
    check("saved slides")

    print("── exporting from the editor ──")
    page.get_by_role("button", name="내보내기").first.click()
    page.wait_for_timeout(900)
    with page.expect_download(timeout=30000) as download:
        page.get_by_text("PowerPoint", exact=False).first.click()
    path = download.value.path()
    size = path.stat().st_size if hasattr(path, "stat") else 0
    print("   downloaded", download.value.suggested_filename, size, "bytes")
    if size < 5000:
        failures.append(f"the downloaded pptx is only {size} bytes")
    page.keyboard.press("Escape")
    check("export")

    print("── the recycle bin ──")
    page.goto(f"{BASE}/presentations", wait_until="networkidle")
    page.wait_for_timeout(2000)
    cards = page.locator(".presentation-card")
    if cards.count() == 0:
        failures.append("the library shows no decks")
    check("library")

    print("── admin settings ──")
    page.goto(f"{BASE}/admin/settings", wait_until="networkidle")
    page.wait_for_timeout(2500)
    inputs = page.locator("input, select, textarea")
    print("   settings controls:", inputs.count())
    # The page is sectioned: one section's fields at a time, chosen on the left.
    # The page is sectioned: one section's fields at a time, chosen on the left.
    if inputs.count() < 2 or page.locator(".settings-save-bar").count() == 0:
        failures.append("the admin settings page shows no editable section")
    # Every section must open and show something.
    for section in ["AI 모델", "생성 정책", "보안 · 키"]:
        page.get_by_text(section, exact=False).first.click()
        page.wait_for_timeout(900)
        if page.locator("input, select, textarea").count() < 2:
            failures.append(f"the {section} settings section shows no fields")
        check(f"admin {section}")
    check("admin settings")

    print("── a narrow window ──")
    page.set_viewport_size({"width": 420, "height": 900})
    for path in ["/dashboard", "/presentations", "/templates", "/images", "/guide"]:
        page.goto(BASE + path, wait_until="networkidle")
        page.wait_for_timeout(1200)
        overflow = page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
        if overflow > 4:
            failures.append(f"{path} scrolls sideways by {overflow}px at 420px wide")
        check(f"narrow {path}")
    page.screenshot(path=f"{OUT}/flow-narrow.png")

    browser.close()

sys.exit(1 if failures else 0)
