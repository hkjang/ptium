"""A walk through every screen, watching for anything the browser complains
about and for pages that render nothing."""
import atexit
import os, sys, json, time, urllib.request
from playwright.sync_api import sync_playwright

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/")
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
OUT = sys.argv[1] if len(sys.argv) > 1 else "."
# Each run names what it makes, so a screen can be told this run's row from
# what an earlier one left.
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

def api(path, method="GET", body=None):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"X-Ptium-Dev-Secret": SECRET}
    if data:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + "/api/v1" + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(request) as response:
        payload = response.read()
        return json.loads(payload)["data"] if payload else None

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
    # The file picker and the button that opens it have to agree about what the
    # product reads. A picker that accepts a PDF above a tooltip that does not
    # mention one tells somebody the feature is not there.
    picker = page.locator("input[type=file]").first
    accepts = (picker.get_attribute("accept") or "") if picker.count() else ""
    told = page.locator("button[title*='슬라이드가 됩니다']").first
    tooltip = (told.get_attribute("title") or "") if told.count() else ""
    for named, said in {".pdf": "PDF", ".docx": "워드", ".xlsx": "엑셀",
                        ".csv": "CSV", ".md": "마크다운"}.items():
        if named not in accepts:
            failures.append(f"the import picker does not accept {named}: {accepts!r}")
        if said not in tooltip:
            failures.append(f"the import button does not say it reads {named} ({said}): {tooltip!r}")
    visit("/templates", expect_text="템플릿")
    visit("/images", expect_text="내 이미지")
    visit("/guide", expect_text="사용 가이드")
    visit("/profile", expect_text="개인")
    # The swatch is never empty: with no colour of their own it shows the
    # deployment's, which on a deployment that never opened the branding screen
    # is the colour this product seeds. Nobody picked it, the drawing ignores
    # it, and saving a name must not turn it into their choice.
    stored = api("/profile") or {}
    had_colour = bool((stored.get("preferences") or {}).get("brandColor"))
    # The theme picker offered four names from an older version of this product,
    # none of them a design key today: an admin whose default is slate-classic
    # read "Aurora" off the screen, and forty-six designs could not be reached.
    shipped = {row["paletteKey"] for row in (api("/templates?limit=200") or []) if row.get("kind") == "builtin"}
    design_on_profile = ""
    picker = page.locator(".profile-design-picker select")
    if not picker.count():
        failures.append("the personalisation screen offers no design to choose")
    else:
        offered = picker.first.evaluate("el => Array.from(el.options).map(o => o.value)")
        if set(offered) != shipped:
            failures.append(f"the design picker offers {len(offered)} of this deployment's {len(shipped)} designs")
        design_on_profile = picker.first.evaluate("el => el.options[el.selectedIndex].text")
    swatch = page.locator("#brand input[type=text], #brand input:not([type=color])").first
    shown = swatch.input_value() if swatch.count() else ""
    hint = page.locator("#brand").inner_text()
    seeded = api("/settings").get("branding.seeded_brand_color")
    if shown and seeded and shown.upper() == str(seeded).upper() and "그대로 둡니다" not in hint:
        failures.append(f"the profile screen shows {shown} and does not say the template keeps its accent")
    if not had_colour:
        page.get_by_role("button", name="변경사항 저장").first.click()
        page.wait_for_timeout(1200)
        written = ((api("/profile") or {}).get("preferences") or {}).get("brandColor")
        if written:
            failures.append(f"saving the profile wrote {written}, a colour nobody chose")
    # Tone and language are free text: any string up to eighty characters, and
    # an administrator's default flows into every profile. The screen offered a
    # fixed handful, so a value outside it left the chips showing nothing and
    # the language select displaying its first option over a different value.
    kept = (api("/profile") or {}).get("preferences") or {}
    api("/profile", "PUT", {"displayName": (api("/profile") or {}).get("displayName") or "",
                            "preferences": {**kept, "defaultTone": "concise", "language": "fr"}})
    visit("/profile", expect_text="개인")
    chip = page.locator("#defaults .choice-chips button.active")
    if not chip.count() or chip.first.inner_text().strip() != "concise":
        failures.append(f"a stored tone 'concise' is shown as {chip.first.inner_text().strip() if chip.count() else '(nothing)'!r}")
    language = page.locator("#defaults select").first
    if language.count() and language.input_value() != "fr":
        failures.append(f"a stored language 'fr' is shown as {language.input_value()!r}")
    # The create screen starts from these same defaults, and it is the screen the
    # deck is actually made on: a select showing "전문적" over a tone of
    # "concise" is the deck being written in a tone nobody read on the screen.
    page.goto(f"{BASE}/create", wait_until="networkidle")
    page.wait_for_timeout(1800)
    page.fill("textarea", "저장된 톤과 언어가 생성 화면에 그대로 보이는지 확인하는 브리프")
    page.click("text=디자인 고르기")
    page.wait_for_timeout(2200)
    for select in page.locator("select").all():
        options = select.evaluate("el => Array.from(el.options).map((option) => option.value)")
        if "concise" in options and select.input_value() != "concise":
            failures.append(f"the create screen shows tone {select.input_value()!r} over a stored 'concise'")
        if "fr" in options and select.input_value() != "fr":
            failures.append(f"the create screen shows language {select.input_value()!r} over a stored 'fr'")
    shown = [s.input_value() for s in page.locator("select").all()]
    if "concise" not in shown or "fr" not in shown:
        failures.append(f"the create screen does not carry the stored tone and language: {shown}")
    api("/profile", "PUT", {"displayName": (api("/profile") or {}).get("displayName") or "", "preferences": kept})
    visit("/api-keys", expect_text="API")
    # A key rotated away has no expiry of its own — it has a grace, and when the
    # grace ends it stops working. The column that answers "when does this key
    # stop" read only the expiry and said "만료 없음" over the one key on the
    # list that was about to die.
    rotating = api("/api-keys", "POST", {"name": f"유예 표시 {RUN}", "scopes": ["presentations:read"]})
    rotating_id = (rotating or {}).get("apiKey", {}).get("id")
    api(f"/api-keys/{rotating_id}/rotate", "POST", {"gracePeriod": "24h"})
    visit("/api-keys", expect_text="API")
    said = ""
    for index in range(page.locator(".api-key-table tbody tr").count()):
        cells = page.locator(".api-key-table tbody tr").nth(index).locator("td").all_inner_texts()
        if f"유예 표시 {RUN}" in cells[0] and "유예" in cells[0]:
            said = cells[4].strip()
    if not said:
        failures.append("a key in its rotation grace is not on the key screen")
    elif "만료 없음" in said:
        failures.append("a key that stops working when its grace ends is shown as never expiring")
    elif "유예 종료" not in said:
        failures.append(f"the key screen does not say when the grace ends: {said!r}")
    for key in (api("/api-keys") or []):
        if f"유예 표시 {RUN}" in str(key.get("name")):
            api(f"/api-keys/{key['id']}", "DELETE")
    visit("/docs", expect_text="API")
    visit("/admin", expect_text="관리")
    visit("/admin/settings", expect_text="설정")
    visit("/admin/users", expect_text="사용자")
    visit("/admin/errors", expect_text="오류")
    visit("/admin/audit", expect_text="감사 기록")
    visit("/admin/queue", expect_text="생성 큐")
    # What this deployment has handed out. Only a deck's owner could see their
    # own links, so nobody could answer what is readable outside.
    visit("/admin/shares", expect_text="공유 링크")
    counted = page.locator(".error-stat-grid article strong").first
    if counted.count() and not counted.inner_text().strip():
        failures.append("the share screen does not say how many links there are")
    rows = page.locator(".error-row")
    if rows.count() > 0 and "회수" not in rows.first.inner_text():
        failures.append("an open link cannot be closed from the share screen")
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

    # A slide marked !build hands out its points one at a time, and nothing was
    # watching that: it needs the server to draw the held-back points and the
    # screen to ask for one more each time somebody presses on.
    built = api("/presentations", "POST", {"title": f"하나씩 {RUN}", "prompt": "점검", "language": "ko"})
    api(f"/presentations/{built['id']}/source", "PUT", {"source":
        "# 표지\n@cover\n> 점검\n\n# 하나씩 나오는 장\n@content\n- 첫 번째 요점\n"
        "- 두 번째 요점\n- 세 번째 요점\n!build\n\n# 마지막\n@closing\n> 끝\n"})
    page.goto(f"{BASE}/presentations/{built['id']}/editor", wait_until="networkidle")
    page.wait_for_timeout(2500)
    page.keyboard.press("F5"); page.wait_for_timeout(2000)
    page.keyboard.press("ArrowRight"); page.wait_for_timeout(2000)
    held = []
    for _ in range(3):
        held.append(page.evaluate("""() => {
            const stage = document.querySelector('.presentation-mode')
            const svg = stage && stage.querySelector('svg')
            return svg ? svg.querySelectorAll('[opacity="0"]').length : -1
        }"""))
        page.keyboard.press("ArrowRight"); page.wait_for_timeout(1500)
    if held != [2, 1, 0]:
        failures.append(f"a slide marked !build revealed its points as {held}, want [2, 1, 0]")
    print("   points held back as the slide is walked:", held)
    page.keyboard.press("Escape"); page.wait_for_timeout(600)
    api(f"/presentations/{built['id']}?permanent=true", "DELETE")
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
    # Which design the stored theme means. This screen, the personalisation
    # screen and the server each answered it separately, and a value an older
    # version stored — "aurora", "graphite" — sent this one to whichever design
    # the listing happened to return first.
    design_on_create = page.evaluate("""() => (Array.from(document.querySelectorAll('strong'))
        .map((el) => el.innerText).filter((text) => text.startsWith('Ptium'))[0] || '')""")
    if not design_on_create:
        failures.append("the design step does not name the design it picked")
    elif design_on_profile and design_on_profile != design_on_create:
        failures.append(f"the personalisation screen says the design is {design_on_profile!r}, this one says {design_on_create!r}")
    page.click("text=모든 디자인 보기")
    page.wait_for_timeout(1800)
    if page.locator(".template-browser .template-tile").count() == 0:
        failures.append("the full library modal shows no designs")
    page.keyboard.press("Escape")
    page.wait_for_timeout(600)
    for kind, message in problems:
        failures.append(f"create: {kind} {message}")

    # A template decides what a deck can be, and the screen that shows a design
    # now says so before forty decks go through it.
    print("── what a template will do to a deck ──")
    problems.clear()
    page.goto(f"{BASE}/templates", wait_until="networkidle")
    page.wait_for_timeout(1500)
    page.get_by_role("button", name="레이아웃 보기").first.click()
    page.wait_for_timeout(2500)
    page.screenshot(path=f"{OUT}/ui-template-health.png")
    if page.locator(".template-health").count() == 0:
        failures.append("a template's detail says nothing about what it will do to a deck")
    else:
        # Two rows: the parts of a deck this design has a layout for, and the
        # components it draws rather than writes out.
        chips = page.locator(".template-health-components span")
        if chips.count() != 9:
            failures.append(f"the template report shows {chips.count()} chip(s), expected 4 roles and 5 components")
        elif page.locator(".template-health-components .drawn").count() != 9:
            failures.append(f"a shipped design is reported as missing "
                            f"{9 - page.locator('.template-health-components .drawn').count()} of them")
        verdict = page.locator(".template-health-head").inner_text()
        print("   ", " ".join(verdict.split()), "|", " ".join(chips.all_inner_texts()))
        if "점검하는 중" in verdict:
            failures.append("the template report never arrived")
    page.keyboard.press("Escape")
    page.wait_for_timeout(500)
    for kind, message in problems:
        failures.append(f"templates: {kind} {message}")

    # The permission list belongs to the server: this screen used to keep its
    # own and had drifted, leaving templates:read — which seven routes require —
    # impossible to grant. And a revoked key is over; the list is about the keys
    # somebody is running.
    print("── what a key may do ──")
    problems.clear()
    page.goto(f"{BASE}/api-keys", wait_until="networkidle")
    page.wait_for_timeout(1500)
    page.get_by_role("button", name="새 API 키").first.click()
    page.wait_for_timeout(900)
    page.screenshot(path=f"{OUT}/ui-api-key-scopes.png")
    offered = page.locator(".scope-options label").count()
    if offered < 8:
        failures.append(f"the key screen offers {offered} permissions; the server has more")
    for wanted in ("templates:read", "presentations:write"):
        if page.locator(f".scope-options:has-text('{wanted}')").count() == 0:
            failures.append(f"the key screen does not offer {wanted}")
    print(f"   permissions offered: {offered}")
    page.keyboard.press("Escape")
    page.wait_for_timeout(500)
    for kind, message in problems:
        failures.append(f"api-keys: {kind} {message}")

    # Changing what a key may do is a screen, not just an endpoint: the point of
    # it is that somebody can widen a key without reissuing it.
    print("── changing what a key may do ──")
    problems.clear()
    made = api("/api-keys", "POST", {"name": f"화면 권한 {RUN}", "scopes": ["presentations:read"]})
    key_id = (made.get("apiKey") or {}).get("id", "")
    page.goto(f"{BASE}/api-keys", wait_until="networkidle")
    page.wait_for_timeout(1500)
    row = page.locator(".api-key-table tbody tr", has_text=f"화면 권한 {RUN}")
    if row.count() == 0:
        failures.append("a key just created is not on the key screen")
    else:
        row.first.get_by_title("권한 수정").click()
        page.wait_for_timeout(800)
        page.locator(".scope-options label", has_text="templates:read").locator("input").check()
        page.get_by_role("button", name="권한 저장").click()
        page.wait_for_timeout(1500)
        page.screenshot(path=f"{OUT}/ui-api-key-edited.png")
        after = api(f"/api-keys")
        mine = next((row for row in after if row.get("id") == key_id), {})
        if "templates:read" not in (mine.get("scopes") or []):
            failures.append(f"editing a key's permissions on screen left it at {mine.get('scopes')}")
        print(f"   after editing on screen: {mine.get('scopes')}")
    if key_id:
        api(f"/api-keys/{key_id}", "DELETE")
    for kind, message in problems:
        failures.append(f"api-keys: {kind} {message}")

    # The link is the one screen people who have no account ever see, and no
    # browser check had ever opened it. Half of a review is the reviewer being
    # told what happened to what they said, which is read here.
    print("── the link somebody was sent ──")
    problems.clear()
    reviewed = api("/presentations", "POST", {"title": f"검토 링크 {RUN}", "prompt": "점검", "language": "ko"})
    api(f"/presentations/{reviewed['id']}/source", "PUT", {"source": "# 실적\n- 매출 1,240억\n"})
    # A link with a day on it stops working when that day passes, and the list
    # used to keep it among the open ones — "3일 전까지", with a 회수 button
    # beside it — so nobody could tell it was already dead.
    dated = api(f"/presentations/{reviewed['id']}/shares", "POST", {"label": f"기한 링크 {RUN}", "days": 1})
    page.goto(f"{BASE}/presentations/{reviewed['id']}/editor", wait_until="networkidle")
    page.wait_for_timeout(2500)
    if page.get_by_role("button", name="공유").count():
        page.get_by_role("button", name="공유").first.click()
        page.wait_for_timeout(1500)
        rows = page.locator(".share-list li")
        for index in range(rows.count()):
            text = rows.nth(index).inner_text()
            if f"기한 링크 {RUN}" not in text:
                continue
            if "까지" not in text:
                failures.append(f"a link with a day on it does not say until when: {text[:60]!r}")
            if rows.nth(index).locator("button").count() == 0:
                failures.append("a link that still works cannot be revoked from the list")
        page.keyboard.press("Escape")
    api(f"/presentations/{reviewed['id']}/shares/{dated['id']}", "DELETE")
    opened = api(f"/presentations/{reviewed['id']}/shares", "POST", {})
    token = (opened.get("url") or "").rstrip("/").split("/")[-1]
    page.goto(f"{BASE}/view/{token}", wait_until="networkidle")
    page.wait_for_timeout(1800)
    if page.locator(".shared-deck-comments").count() == 0:
        failures.append("a shared link opens without anywhere to say what is wrong")
    else:
        page.locator(".shared-deck-say input").first.fill("검토자")
        page.locator(".shared-deck-say textarea").fill("이 숫자는 지난 분기 것입니다")
        page.get_by_role("button", name="남기기").first.click()
        page.wait_for_timeout(1500)
        left = api(f"/presentations/{reviewed['id']}/comments")
        if not left:
            failures.append("a remark left on the link did not reach the deck")
        else:
            api(f"/presentations/{reviewed['id']}/comments", "POST",
                {"parentId": left[0]["id"], "body": "확인했습니다. 방금 고쳤습니다"})
            page.reload(wait_until="networkidle")
            page.wait_for_timeout(1600)
            if page.locator(".shared-deck-reply").count() == 0:
                failures.append("the author answered and the link does not show it")
            else:
                page.get_by_role("button", name="답글").first.click()
                page.wait_for_timeout(500)
                page.locator(".shared-deck-answer textarea").fill("고맙습니다")
                page.locator(".shared-deck-answer button").click()
                page.wait_for_timeout(1500)
                thread = api(f"/presentations/{reviewed['id']}/comments")
                if len(thread) != 3 or sum(1 for row in thread if row.get("parentId")) != 2:
                    failures.append(f"the thread came out as {[(r.get('author'), bool(r.get('parentId'))) for r in thread]}")
                else:
                    print("   thread on the link: 의견 → 답글 → 재답글")
        page.screenshot(path=f"{OUT}/ui-shared-thread.png")
    api(f"/presentations/{reviewed['id']}", "DELETE")
    for kind, message in problems:
        failures.append(f"view: {kind} {message}")

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
