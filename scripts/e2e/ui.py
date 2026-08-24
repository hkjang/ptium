"""A walk through every screen, watching for anything the browser complains
about and for pages that render nothing."""
import atexit
import os, sys, json, urllib.request
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

def api(path):
    request = urllib.request.Request(BASE + "/api/v1" + path, headers={"X-Ptium-Dev-Secret": SECRET})
    with urllib.request.urlopen(request) as response:
        return json.loads(response.read())["data"]

decks = api("/presentations?limit=20")
ready = [d for d in decks if (d.get("slideCount") or 0) > 0] or decks
deck = ready[0]["id"]
print("using deck", ready[0]["title"])

with sync_playwright() as play:
    browser = play.chromium.launch()
    context = browser.new_context(viewport={"width": 1500, "height": 1000},
                                  extra_http_headers={"X-Ptium-Dev-Secret": SECRET})
    page = context.new_page()
    problems = []
    page.on("console", lambda m: problems.append(("console", m.text)) if m.type == "error" else None)
    page.on("pageerror", lambda e: problems.append(("pageerror", str(e))))
    page.on("requestfailed", lambda r: problems.append(("request", f"{r.url[:90]} {r.failure}")))
    page.on("response", lambda r: problems.append(("http", f"{r.status} {r.url[:90]}"))
            if r.status >= 400 and "/api/" in r.url else None)

    # Two things a person notices before they read a word: text too small to
    # read, and text drawn on top of other text. Both are measurable, so neither
    # has to wait for somebody to look at the right screen — the workspace was
    # set at 7-10px until v1.8.0, and three collisions had been sitting in the
    # editor's own header for as long.
    READABILITY = """
    () => {
      const small = [], collided = []
      const own = (el) => [...el.childNodes].filter((n) => n.nodeType === 3)
        .map((n) => n.textContent.trim()).join('').replace(/\\s+/g, '')
      // A slide draws the deck's own typography, at whatever size the deck says.
      const slide = '.slide-thumbnail, .generating-slide, .freeform-page, .present-slide, .slide-canvas, .preview-slide'
      const elements = [...document.querySelectorAll('body *')]
      for (const el of elements) {
        if (own(el).length < 2 || !el.offsetParent || el.closest(slide)) continue
        const style = getComputedStyle(el)
        if (style.visibility === 'hidden' || style.opacity === '0') continue
        const size = parseFloat(style.fontSize)
        if (size && size < 11) small.push([el.className || el.tagName, Math.round(size * 10) / 10, own(el).slice(0, 30)])
      }
      // Block boxes only: a wrapped inline element's rectangle spans the whole
      // line and would read as an overlap with everything else on it.
      const boxes = []
      for (const el of elements) {
        if (own(el).length < 2 || !el.offsetParent || el.closest(slide)) continue
        const style = getComputedStyle(el)
        if (style.position !== 'static' || style.display.startsWith('inline')) continue
        const rect = el.getBoundingClientRect()
        if (rect.width > 2 && rect.height > 2) boxes.push([el, rect])
      }
      for (let i = 0; i < boxes.length; i++) {
        for (let j = i + 1; j < boxes.length; j++) {
          const [a, ra] = boxes[i], [b, rb] = boxes[j]
          if (a.contains(b) || b.contains(a) || a.parentElement !== b.parentElement) continue
          const x = Math.min(ra.right, rb.right) - Math.max(ra.left, rb.left)
          const y = Math.min(ra.bottom, rb.bottom) - Math.max(ra.top, rb.top)
          if (x > 4 && y > 4) collided.push([(a.className || a.tagName) + ' over ' + (b.className || b.tagName),
                                             Math.round(x) + 'x' + Math.round(y)])
        }
      }
      return { small: small.slice(0, 6), collided: collided.slice(0, 6) }
    }
    """

    def readable(path):
        found = page.evaluate(READABILITY)
        for row in found["small"]:
            failures.append(f"{path}: interface text at {row[1]}px — {row[0]} {row[2]!r}")
        for row in found["collided"]:
            failures.append(f"{path}: two elements drawn over each other — {row[0]} ({row[1]})")

    def visit(path, wait=1600, expect_text=None):
        problems.clear()
        page.goto(BASE + path, wait_until="networkidle")
        page.wait_for_timeout(wait)
        body = page.locator("body").inner_text()
        if len(body.strip()) < 40:
            failures.append(f"{path} rendered almost nothing")
        if expect_text and expect_text not in body:
            failures.append(f"{path} does not show {expect_text!r}")
        for kind, message in problems:
            failures.append(f"{path}: {kind} {message}")
        readable(path)
        return body

    print("── every route ──")
    visit("/dashboard", expect_text="워크스페이스")
    visit("/presentations", expect_text="프레젠테이션")
    visit("/templates", expect_text="템플릿")
    visit("/images", expect_text="내 이미지")
    visit("/guide", expect_text="사용 가이드")
    visit("/profile", expect_text="개인")
    visit("/api-keys", expect_text="API")
    visit("/docs", expect_text="API")
    visit("/admin", expect_text="관리")
    visit("/admin/settings", expect_text="설정")
    visit("/admin/users", expect_text="사용자")
    visit("/admin/errors", expect_text="오류")
    visit("/nonexistent-page", expect_text="404")
    visit(f"/presentations/{deck}/editor", wait=3000)


    # A deck of fifty slides has a rail taller than any window, and a slide
    # zoomed to 200% is wider than the canvas. Both live inside a grid whose
    # items default to min-height:auto, so both used to grow to their content
    # instead of scrolling — and everything past the first screenful was drawn
    # where nobody could reach it.
    SCROLLS = """
    () => {
      const box = (selector) => {
        const el = document.querySelector(selector)
        if (!el) return null
        return { height: el.clientHeight, content: el.scrollHeight,
                 scrolls: el.scrollHeight > el.clientHeight + 2 }
      }
      return { rail: box('.slide-list'), canvas: box('.freeform-scroll'),
               inspector: box('.inspector-panel'), viewport: window.innerHeight }
    }
    """

    def scrolls(path):
        found = page.evaluate(SCROLLS)
        for name, region in found.items():
            if name == "viewport" or not region:
                continue
            # Nothing may be taller than the window it lives in: that is the
            # shape of a region that grew instead of scrolling.
            if region["height"] > found["viewport"]:
                failures.append(f"{path}: the {name} is {region['height']}px tall in a "
                                f"{found['viewport']}px window — it grew instead of scrolling")
        return found

    print("── the editor's panels and modes ──")
    page.goto(f"{BASE}/presentations/{deck}/editor", wait_until="networkidle")
    page.wait_for_selector(".freeform-page", timeout=25000)
    page.wait_for_timeout(2000)
    regions = scrolls(f"/presentations/{deck}/editor")
    if regions["rail"] and not regions["rail"]["scrolls"] and regions["rail"]["content"] > regions["viewport"]:
        failures.append("the slide rail holds more than the window and does not scroll")
    for name in ["내용", "디자인", "이미지", "슬라이드", "노트", "격자"]:
        problems.clear()
        page.get_by_role("button", name=name, exact=True).first.click()
        page.wait_for_timeout(1800)
        for kind, message in problems:
            failures.append(f"editor panel {name}: {kind} {message}")
    for mode in ["템플릿 미리보기", "코드", "편집"]:
        problems.clear()
        page.get_by_role("button", name=mode, exact=True).first.click()
        page.wait_for_timeout(2200)
        for kind, message in problems:
            failures.append(f"editor mode {mode}: {kind} {message}")
    page.screenshot(path=f"{OUT}/ui-editor.png")

    print("── editing on the canvas ──")
    problems.clear()
    page.get_by_role("button", name="편집", exact=True).first.click()
    page.wait_for_timeout(1500)
    before = page.locator(".freeform-element").count()
    page.get_by_role("button", name="텍스트", exact=True).first.click()
    page.wait_for_timeout(900)
    after = page.locator(".freeform-element").count()
    if after <= before:
        failures.append(f"adding a text box did not add an object ({before} → {after})")
    page.keyboard.press("Delete")
    page.wait_for_timeout(600)
    # Undo/redo through the toolbar
    page.keyboard.press("Control+z")
    page.wait_for_timeout(600)
    for kind, message in problems:
        failures.append(f"canvas edit: {kind} {message}")

    print("── presenting ──")
    problems.clear()
    page.keyboard.press("F5")
    page.wait_for_timeout(2500)
    if page.locator(".presentation-mode").count() == 0:
        failures.append("F5 did not start the presentation")
    else:
        page.keyboard.press("ArrowRight"); page.wait_for_timeout(700)
        page.keyboard.press("g"); page.wait_for_timeout(700)
        if page.locator(".present-overview").count() == 0:
            failures.append("G did not open the slide overview")
        page.keyboard.press("Escape"); page.wait_for_timeout(500)
        page.keyboard.press("?"); page.wait_for_timeout(700)
        if page.locator(".modal").count() == 0:
            failures.append("? did not open the shortcut sheet in presentation mode")
        page.keyboard.press("Escape"); page.wait_for_timeout(400)
        page.keyboard.press("Escape"); page.wait_for_timeout(800)
    page.screenshot(path=f"{OUT}/ui-present.png")
    for kind, message in problems:
        failures.append(f"presenting: {kind} {message}")

    print("── creating ──")
    problems.clear()
    page.goto(f"{BASE}/create", wait_until="networkidle")
    page.wait_for_timeout(2000)
    page.fill("textarea", "사내 데이터 플랫폼 고도화 계획을 실무진에게 공유하는 5장짜리 자료")
    page.click("text=디자인 고르기")
    page.wait_for_timeout(2500)
    page.screenshot(path=f"{OUT}/ui-create.png")
    if page.locator(".template-tile").count() == 0:
        failures.append("the design step shows no templates")
    page.click("text=모든 디자인 보기")
    page.wait_for_timeout(1800)
    if page.locator(".template-browser .template-tile").count() == 0:
        failures.append("the full library modal shows no designs")
    page.keyboard.press("Escape")
    page.wait_for_timeout(600)
    for kind, message in problems:
        failures.append(f"create: {kind} {message}")

    print("── search and the command palette ──")
    problems.clear()
    page.goto(f"{BASE}/dashboard", wait_until="networkidle")
    page.wait_for_timeout(1200)
    page.keyboard.press("Control+k")
    page.wait_for_timeout(600)
    if page.locator(".command-palette").count() == 0:
        failures.append("Ctrl+K did not open the search palette")
    page.keyboard.press("Escape")
    for kind, message in problems:
        failures.append(f"palette: {kind} {message}")

    browser.close()

sys.exit(1 if failures else 0)
