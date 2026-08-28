"""The flows that change something: generating, editing text, applying source,
saving a slide, uploading a template, restoring from the bin, admin settings."""
import atexit
import os, re, sys, json, time, urllib.request
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

    # ── a captured screenshot, pasted ──
    # The gesture everybody arrives with: capture a screen, come back to the
    # browser, Ctrl+V. Until v1.53.0 the editor only answered it while the
    # canvas already held focus, so the ordinary sequence — nothing clicked
    # yet — pasted nothing and said nothing about why, and the same keys were
    # dead in 코드 and 템플릿 미리보기.
    print("── pasting a captured image ──")
    # A 4x4 PNG, small enough to keep in the sweep and real enough to upload.
    PASTED_PNG = ("iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAYAAACp8Z5+AAAAFklEQVR4nGP8z8Dwn4GKgIma"
                  "hg0PAwEAqXsD/9E4KFQAAAAASUVORK5CYII=")
    PASTE = """
    ([encoded, alsoText]) => {
      const bytes = Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0))
      const carried = new DataTransfer()
      carried.items.add(new File([bytes], 'image.png', {type: 'image/png'}))
      if (alsoText) carried.setData('text/plain', '붙여넣는 글')
      const event = new ClipboardEvent('paste', {clipboardData: carried, bubbles: true, cancelable: true})
      ;(document.activeElement || document.body).dispatchEvent(event)
    }
    """

    def pasted_images():
        # A slide's floating objects live inside its content blob, which is
        # where the editor reads them from too.
        found = 0
        for slide in api(f"/presentations/{deck_id}")["slides"]:
            content = slide.get("content") or {}
            if isinstance(content, str):
                content = json.loads(content)
            found += len([e for e in (content.get("elements") or []) if e.get("kind") == "image"])
        return found

    def paste_lands(what, also_text=False, expect=True):
        before = pasted_images()
        page.evaluate(PASTE, [PASTED_PNG, also_text])
        page.wait_for_timeout(4000)
        landed = pasted_images() > before
        if landed != expect:
            failures.append(f"{what}: the slide went from {before} images to {pasted_images()}, "
                            f"expected {'one more' if expect else 'no change'}")

    page.get_by_role("button", name="편집", exact=True).first.click()
    page.wait_for_timeout(1500)
    page.evaluate("document.activeElement && document.activeElement.blur()")
    paste_lands("a screenshot pasted with nothing clicked")
    check("pasted image")

    page.get_by_role("button", name="코드", exact=True).first.click()
    page.wait_for_timeout(2500)
    page.locator(".source-editor textarea").first.click()
    paste_lands("a screenshot pasted while the source editor holds the caret")
    # A paste that is also text is the text box's own business.
    page.get_by_role("button", name="코드", exact=True).first.click()
    page.wait_for_timeout(2000)
    page.locator(".source-editor textarea").first.click()
    paste_lands("a copied paragraph pasted into the source editor", also_text=True, expect=False)
    page.get_by_role("button", name="편집", exact=True).first.click()
    page.wait_for_timeout(1500)

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

    # ── a slide kept out of the talk ──
    # Everything that draws a slide asks the server for it by its number in the
    # deck, and until v1.54.0 the show counted its own places instead. With one
    # slide marked 발표에서 건너뛰기 the projector drew the skipped slide,
    # mislabelled every slide after it, and never reached the last slide of the
    # deck — and the speaker's own window, handed the whole deck, drew a
    # different list again.
    print("── presenting past a skipped slide ──")
    page.goto(f"{BASE}/presentations/{deck_id}/editor", wait_until="networkidle")
    page.wait_for_selector(".canvas-area", timeout=20000)
    page.wait_for_timeout(1500)
    rail = page.locator(".slide-thumbnail-row")
    deck_slides = rail.count()
    if deck_slides < 3:
        failures.append(f"the deck has {deck_slides} slides, too few to present past a skip")
    else:
        rail.nth(1).click()
        page.wait_for_timeout(1200)
        page.get_by_role("button", name="발표에서 건너뛰기").click()
        page.wait_for_timeout(3000)
        expected = [n for n in range(1, deck_slides + 1) if n != 2]

        drawn = []
        def note(request):
            if "preview" in request.url and "width=1600" in request.url:
                found = re.search(r"[?&]slide=(\d+)", request.url)
                if found:
                    number = int(found.group(1))
                    if number not in drawn:
                        drawn.append(number)
        page.on("request", note)
        rail.nth(0).click()
        page.wait_for_timeout(900)
        page.get_by_role("button", name="발표", exact=False).first.click()
        page.wait_for_timeout(3000)
        for _ in range(deck_slides):
            page.keyboard.press("ArrowRight")
            page.wait_for_timeout(2200)
        page.remove_listener("request", note)
        if 2 in drawn:
            failures.append("presenting drew the slide marked 발표에서 건너뛰기")
        if drawn[:len(expected)] != expected:
            failures.append(f"presenting drew deck slides {drawn[:len(expected)]}, expected {expected}")
        if deck_slides not in drawn:
            failures.append(f"presenting never reached slide {deck_slides}, the last in the deck")

        # The grid a speaker opens mid-talk to find a slide to jump to. It asked
        # for the current slide's neighbours, so it drew three pictures and left
        # the rest of the deck as empty grey boxes with a number on them.
        page.keyboard.press("g")
        page.wait_for_timeout(9000)
        boxes = page.locator(".present-overview-grid button").count()
        pictures = page.locator(".present-overview-grid button img").count()
        if boxes < 2:
            failures.append("the slide overview opened with nothing in it")
        elif pictures < boxes:
            failures.append(f"the slide overview drew {pictures} of {boxes} slides; the rest are empty boxes")
        page.keyboard.press("Escape")
        page.wait_for_timeout(1000)
        page.keyboard.press("Escape")
        page.wait_for_timeout(1200)
        # Put the slide back in the show so nothing after this reads a short deck.
        rail.nth(1).click()
        page.wait_for_timeout(1000)
        page.get_by_role("button", name="발표에서 건너뛰기").click()
        page.wait_for_timeout(2500)
    check("presenting past a skip")

    # ── the presenter's timer ──
    # The button says 타이머 일시정지 and until v1.55.2 did neither thing it
    # says: it read the clock from the moment the talk started rather than the
    # moment it was held, so it showed 00:00:00 for the whole pause — and the
    # pause was counted anyway. Held at nine seconds for a six-second question,
    # it came back at nineteen.
    print("── the presenter's timer ──")

    def clock_seconds(text):
        hours, minutes, secs = (int(part) for part in text.strip().split(":"))
        return hours * 3600 + minutes * 60 + secs

    page.goto(f"{BASE}/presentations/{deck_id}/editor", wait_until="networkidle")
    page.wait_for_selector(".canvas-area", timeout=20000)
    page.wait_for_timeout(1500)
    presenter = context.new_page()
    presenter.goto(f"{BASE}/presentations/{deck_id}/presenter", wait_until="networkidle")
    presenter.wait_for_timeout(1500)
    page.bring_to_front()
    page.get_by_role("button", name="발표", exact=False).first.click()
    page.wait_for_timeout(1500)
    presenter.bring_to_front()
    clock = presenter.locator(".presenter-clock button").first
    presenter.wait_for_timeout(8000)
    running = clock_seconds(clock.inner_text())
    if running < 5:
        failures.append(f"the presenter clock read {running}s after eight seconds of presenting")
    clock.click()                       # 일시정지
    presenter.wait_for_timeout(1200)
    held = clock_seconds(clock.inner_text())
    if held < running - 1:
        failures.append(f"pausing the timer at {running}s showed {held}s instead of holding it")
    presenter.wait_for_timeout(6000)
    if clock_seconds(clock.inner_text()) != held:
        failures.append("a paused presenter timer kept moving")
    clock.click()                       # 재개
    presenter.wait_for_timeout(2500)
    resumed = clock_seconds(clock.inner_text())
    if resumed > held + 4:
        failures.append(f"resuming a timer held at {held}s came back at {resumed}s: the pause was counted anyway")
    presenter.get_by_role("button", name="초기화").click()
    presenter.wait_for_timeout(1500)
    if clock_seconds(presenter.locator(".presenter-clock button").first.inner_text()) > 3:
        failures.append("resetting the presenter timer did not clear it")
    presenter.close()
    page.bring_to_front()
    page.keyboard.press("Escape")
    page.wait_for_timeout(1000)
    check("presenter timer")

    # ── the same deck in two windows ──
    # One window's save wins and the other is refused, which is right. What the
    # refusal then tells the loser is the part that was wrong: it asked them to
    # refresh, and refreshing throws away what they had just typed. Measured in
    # two windows — the edit is on the screen after the refusal and gone after
    # the reload.
    print("── the same deck in two windows ──")
    run = int(time.time()) % 10000
    second = context.new_page()
    for window in (page, second):
        window.goto(f"{BASE}/presentations/{deck_id}/editor", wait_until="domcontentloaded")
        window.wait_for_selector(".canvas-area", timeout=25000)
        window.wait_for_timeout(2000)

    def retitle(window, index, text):
        window.bring_to_front()
        window.locator(".slide-thumbnail-row").nth(index).click()
        window.wait_for_timeout(1200)
        field = window.locator("aside input, .inspector-body input").first
        field.click()
        field.fill(text)
        window.wait_for_timeout(500)
        window.keyboard.press("Control+s")
        window.wait_for_timeout(3500)
        toast = window.locator(".toast, .toast-stack").first
        return toast.inner_text().replace("\n", " ") if toast.count() else ""

    retitle(page, 0, f"먼저 고친 장 {run}")
    refused = retitle(second, 1, f"나중에 고친 장 {run}")
    titles = [slide["title"] for slide in api(f"/presentations/{deck_id}")["slides"]]
    if f"먼저 고친 장 {run}" not in titles:
        failures.append("the first window's save did not survive the second window's")
    if f"나중에 고친 장 {run}" in titles:
        failures.append("a stale window overwrote the deck instead of being refused")
    if "먼저 바뀌" not in refused:
        failures.append(f"a stale save was answered with {refused[:80]!r}")
    # Asking somebody to refresh has to say what refreshing costs.
    if "새로고침" in refused and not ("사라" in refused and "복사" in refused):
        failures.append(f"the conflict asks for a refresh without saying it loses the edit: {refused[:110]!r}")
    second.close()
    page.bring_to_front()
    check("two windows")

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
