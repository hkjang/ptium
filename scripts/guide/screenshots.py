"""The screens the guides show, taken from a running Ptium.

Every picture in docs/USER_GUIDE.md and docs/ADMIN_GUIDE.md comes from here, so
a guide never carries a mock-up or a screen of some other release. The target
is a throwaway deployment named by variables this script alone reads — never
the PTIUM_URL the e2e sweeps share — because it fills the account it signs in
with sample decks and pictures, and stops rather than guess when they are unset.

    export PTIUM_GUIDE_URL=http://localhost:18099
    export PTIUM_GUIDE_DEV_SECRET=…        # the deployment's DEV_AUTH_SECRET
    python3 scripts/guide/screenshots.py docs/assets/guide

The deployment needs development auth on, with the dev account carrying the
administrator role (DEV_AUTH_ROLES=ptium-admin,user), and a DEV_AUTH_EMAIL and
DEV_AUTH_NAME that may appear in print: nothing here reads a real directory.
It changes no service setting; the sample data it adds carries a marker so a
second run reuses it instead of adding more.
"""
import io
import json
import os
import sys
import time
import urllib.request
import uuid

from playwright.sync_api import sync_playwright

BASE = os.environ.get("PTIUM_GUIDE_URL", "").rstrip("/")
SECRET = os.environ.get("PTIUM_GUIDE_DEV_SECRET", "")
OUT = sys.argv[1] if len(sys.argv) > 1 else "docs/assets/guide"
MARK = "[가이드]"

if not BASE or not SECRET:
    sys.exit("set PTIUM_GUIDE_URL and PTIUM_GUIDE_DEV_SECRET to a deployment you can fill with sample data")
if "localhost" not in BASE and "127.0.0.1" not in BASE and os.environ.get("PTIUM_GUIDE_REMOTE_OK") != "1":
    sys.exit(f"{BASE} is not local; set PTIUM_GUIDE_REMOTE_OK=1 if it really is a deployment you may fill")


def call(method, path, body=None, files=None):
    """One request with the dev header; JSON in, the data envelope out."""
    headers = {"X-Ptium-Dev-Secret": SECRET}
    if files is not None:
        boundary = "guide" + uuid.uuid4().hex
        buffer = io.BytesIO()
        for name, value in (body or {}).items():
            buffer.write(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"\r\n\r\n{value}\r\n".encode())
        for name, (filename, content, kind) in files.items():
            buffer.write(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"; filename=\"{filename}\"\r\n"
                         f"Content-Type: {kind}\r\n\r\n".encode())
            buffer.write(content)
            buffer.write(b"\r\n")
        buffer.write(f"--{boundary}--\r\n".encode())
        data = buffer.getvalue()
        headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"
    else:
        data = json.dumps(body).encode() if body is not None else None
        if data:
            headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + "/api/v1" + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request) as response:
            payload = response.read()
    except urllib.error.HTTPError as error:
        payload = error.read()
        print(f"  {method} {path} -> {error.code}: {payload[:200]!r}")
        return None
    return json.loads(payload)["data"] if payload else None


def wait_for(deck_id, seconds=180):
    for _ in range(seconds):
        state = call("GET", f"/presentations/{deck_id}") or {}
        if state.get("status") in ("ready", "completed", "failed"):
            return state
        time.sleep(1)
    return state


# ---------------------------------------------------------------- sample data

BRIEFS = [
    ("AI 코딩 도구 도입 성과 보고", "사내 개발팀의 AI 코딩 도구 도입 성과를 경영진에게 보고하는 덱. "
     "개발 속도 32% 개선, 12개월 ROI 2.4배, 내년 전사 확대 방안을 포함해 주세요.", 8),
    ("2026 하반기 클라우드 전환 로드맵", "온프레미스 계약이 9월에 끝나고 재계약가는 22% 인상됩니다. "
     "전환 대상 42개 시스템, 예상 절감 18%, 3단계 이행 일정을 임원에게 보고합니다.", 7),
    ("데모 회사 신입사원 온보딩", "데모 회사의 신입사원 첫 주 온보딩 안내. 조직 소개, 보안 수칙, "
     "업무 도구 세 가지, 첫 달 목표를 담아 주세요.", 6),
    ("분기 고객 만족도 조사 결과", "2분기 고객 만족도 조사 결과를 CS 팀장들에게 공유합니다. "
     "응답 1,240명, NPS 41, 주요 불만 세 가지와 개선 과제를 담아 주세요.", 6),
]

SALES_CSV = "지역,매출\n서울,1200\n부산,980\n대구,640\n인천,570\n광주,410\n"


def seed():
    have = call("GET", "/presentations?limit=50") or []
    mine = {d["title"]: d for d in have if MARK in (d.get("title") or "")}
    made = []
    for title, prompt, count in BRIEFS:
        name = f"{title} {MARK}"
        if name in mine:
            made.append(mine[name])
            continue
        deck = call("POST", "/presentations/generate",
                    {"title": name, "prompt": prompt, "language": "ko", "slideCount": count,
                     "audience": "경영진", "tone": "professional"})
        if deck:
            print("  generating", name)
            made.append(deck)
    for deck in made:
        state = wait_for(deck["id"])
        print("  ", state.get("title"), state.get("status"), state.get("slideCount"))
    imported_name = f"지역별 매출 {MARK}"
    if imported_name not in mine:
        imported = call("POST", "/presentations/import", {"name": imported_name, "language": "ko"},
                        files={"file": ("지역별 매출.csv", SALES_CSV.encode("utf-8"), "text/csv")})
        if imported:
            print("  imported", imported_name)
    assets = call("GET", "/assets") or []
    if not any(MARK in (a.get("name") or "") for a in assets):
        call("POST", "/assets", {"name": f"데모 회사 로고 {MARK}", "tags": "로고"},
             files={"file": ("logo.svg", LOGO.encode("utf-8"), "image/svg+xml")})
        call("POST", "/assets", {"name": f"제품 화면 {MARK}", "tags": "제품컷"},
             files={"file": ("product.svg", PRODUCT.encode("utf-8"), "image/svg+xml")})
        print("  uploaded two pictures")
    keys = call("GET", "/api-keys") or []
    if not any(MARK in (k.get("name") or "") for k in keys):
        call("POST", "/api-keys", {"name": f"보고서 자동화 {MARK}", "scopes": ["presentations:read", "presentations:write"]})
        call("POST", "/api-keys", {"name": f"MCP 클라이언트 {MARK}", "scopes": ["mcp:use", "presentations:read"]})
        print("  made two API keys")
    decks = call("GET", "/presentations?limit=50") or []
    ready = [d for d in decks if MARK in (d.get("title") or "") and (d.get("slideCount") or 0) > 0]
    ready.sort(key=lambda d: d.get("slideCount") or 0, reverse=True)
    return ready


LOGO = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 240" width="240" height="240">
<rect width="240" height="240" rx="48" fill="#1d4ed8"/><circle cx="120" cy="120" r="64" fill="none" stroke="#fff" stroke-width="18"/>
<text x="120" y="212" font-family="sans-serif" font-size="28" fill="#fff" text-anchor="middle">DEMO</text></svg>"""
PRODUCT = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400" width="640" height="400">
<rect width="640" height="400" fill="#f1f5f9"/><rect x="40" y="40" width="560" height="320" rx="16" fill="#fff" stroke="#cbd5e1"/>
<rect x="72" y="80" width="200" height="24" rx="6" fill="#1d4ed8"/><rect x="72" y="130" width="496" height="14" rx="4" fill="#e2e8f0"/>
<rect x="72" y="160" width="420" height="14" rx="4" fill="#e2e8f0"/><rect x="72" y="220" width="140" height="100" rx="8" fill="#93c5fd"/>
<rect x="250" y="220" width="140" height="100" rx="8" fill="#60a5fa"/><rect x="428" y="220" width="140" height="100" rx="8" fill="#3b82f6"/></svg>"""


# ---------------------------------------------------------------- the screens

def main():
    os.makedirs(OUT, exist_ok=True)
    print("seeding", BASE)
    decks = seed()
    if not decks:
        sys.exit("no generated deck to show; look at the deployment's log")
    deck = decks[0]
    print("showing", deck["title"])

    with sync_playwright() as play:
        browser = play.chromium.launch()
        shot = 0

        def snap(page, name, settle=1.5):
            nonlocal shot
            page.wait_for_load_state("networkidle")
            time.sleep(settle)
            page.screenshot(path=os.path.join(OUT, name + ".png"))
            shot += 1
            print("  ", name)

        # The sign-in screen is what a person sees with no session at all.
        anonymous = browser.new_context(viewport={"width": 1440, "height": 900}, locale="ko-KR")
        page = anonymous.new_page()
        page.goto(BASE + "/login")
        snap(page, "login")
        anonymous.close()

        context = browser.new_context(viewport={"width": 1440, "height": 900}, locale="ko-KR",
                                      extra_http_headers={"X-Ptium-Dev-Secret": SECRET})
        page = context.new_page()

        def go(path, name, settle=1.5, before=None):
            page.goto(BASE + path)
            page.wait_for_load_state("networkidle")
            if before:
                before()
            snap(page, name, settle)

        go("/dashboard", "dashboard")
        go("/presentations", "presentations")
        go("/create", "create", before=lambda: fill_brief(page))
        click(page, "button", "디자인 고르기", "create-designs", settle=4)
        go("/templates", "templates", settle=3)
        go("/images", "images")
        go(f"/presentations/{deck['id']}/editor", "editor", settle=4, before=lambda: pick_slide(page, 2))
        open_tab(page, "코드", "editor-code")
        click(page, "title", "이 덱의 체크포인트를 열어 되돌립니다", "editor-history")
        click(page, "title", "말로 시킵니다", "editor-command")
        click(page, "title", "계정이 없는 사람도 볼 수 있는 링크를 만듭니다", "editor-share")
        open_quality(page)
        go("/profile", "profile")
        go("/api-keys", "api-keys")
        go("/guide", "guide")
        go("/docs", "docs")

        go("/admin", "admin-overview", settle=3)
        go("/admin/settings", "admin-settings", settle=3)
        click(page, "button", "AI 모델", "admin-settings-ai")
        click(page, "button", "OIDC · SSO", "admin-settings-oidc")
        click(page, "button", "생성 정책", "admin-settings-generation")
        click(page, "button", "보안 · 키", "admin-settings-security")
        go("/admin/users", "admin-users")
        go("/admin/usage", "admin-usage", settle=3)
        go("/admin/errors", "admin-errors")
        go("/admin/audit", "admin-audit")
        go("/admin/designs", "admin-designs", settle=3)
        go("/admin/tidy", "admin-tidy", settle=3)
        browser.close()
    print(f"{shot} screens in {OUT}")


def fill_brief(page):
    """The brief screen with a brief in it, the way a person leaves it."""
    box = page.locator("textarea").first
    if box.count():
        box.fill("사내 개발팀의 AI 코딩 도구 도입 성과를 경영진에게 보고하는 8장짜리 덱. "
                 "개발 속도 32% 개선, 12개월 ROI, 내년 전사 확대 방안을 포함해 주세요.")
        time.sleep(2)


def pick_slide(page, number):
    """A body slide on the canvas: the cover shows a title and little else."""
    thumb = page.locator(".slide-thumbnail").nth(number - 1)
    if thumb.count():
        thumb.click()
        time.sleep(1.5)


def click(page, how, label, name, settle=2):
    """Press one control — a button by its text, or anything by its title —
    take the screen it opens, and close it again with Escape."""
    if how == "button":
        target = page.locator("button", has_text=label).first
    else:
        target = page.locator(f'[title^="{label}"]').first
    if target.count() == 0:
        print("   (no", label, "; skipped)")
        return
    target.click()
    time.sleep(settle)
    page.screenshot(path=os.path.join(OUT, name + ".png"))
    print("  ", name)
    page.keyboard.press("Escape")
    time.sleep(0.5)


def open_tab(page, label, name):
    """A side tab of the editor, if the release has a control by that name."""
    button = page.get_by_role("button", name=label, exact=True)
    if button.count() == 0:
        button = page.get_by_role("tab", name=label, exact=True)
    if button.count() == 0:
        print("   (no", label, "tab; skipped)")
        return
    button.first.click()
    time.sleep(2)
    page.screenshot(path=os.path.join(OUT, name + ".png"))
    print("  ", name)


def open_quality(page):
    """The quality score's breakdown, opened from the badge in the header."""
    badge = page.locator("button", has_text="품질").first
    if badge.count() == 0:
        print("   (no quality badge; skipped)")
        return
    badge.click()
    time.sleep(2)
    page.screenshot(path=os.path.join(OUT, "editor-quality.png"))
    print("   editor-quality")
    page.keyboard.press("Escape")


if __name__ == "__main__":
    main()
