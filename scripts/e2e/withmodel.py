"""Generation with a real provider, which is the path a deployment uses.

Every other sweep runs offline, so the writer this one exercises — the model,
the repair passes, the notes the deck keeps about what it did — is checked by
nothing. It switches the provider on, generates, and puts it back the way it
found it, whatever happens.

    PTIUM_URL=http://localhost:8099 python3 scripts/e2e/withmodel.py
"""
import argparse
import atexit
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
import uuid

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/") + "/api/v1"
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
RUN = uuid.uuid4().hex[:6]
failures = []
checks = 0


def stopped_early(kind, error, trace):
    failures.append(f"the sweep stopped early: {kind.__name__}: {error}".replace("\n", " ")[:300])
    sys.__excepthook__(kind, error, trace)


def report():
    print()
    print(f"{checks} checks, {len(failures)} failures")
    for failure in failures:
        print(" ✗ " + failure)


sys.excepthook = stopped_early
atexit.register(report)


def call(method, path, body=None, timeout=600):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"X-Ptium-Dev-Secret": SECRET}
    if data:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            payload = response.read()
            return response.status, (json.loads(payload)["data"] if payload else None)
    except urllib.error.HTTPError as error:
        return error.code, error.read()[:300]


def provider(name):
    return call("PUT", "/admin/settings", {"values": {"ai.provider": name}})[0]


parser = argparse.ArgumentParser()
parser.add_argument("--minutes", type=float, default=12.0, help="how long to wait for all of it")
options = parser.parse_args()

settings = call("GET", "/admin/settings")[1]
rows = settings.get("settings") if isinstance(settings, dict) else settings
was = next((row.get("value") for row in rows or [] if row.get("key") == "ai.provider"), "fallback")
configured = {row.get("key"): row.get("value") for row in rows or []}
if not str(configured.get("ai.base_url") or "").strip():
    print("no provider is configured on this deployment; nothing to check")
    sys.exit(0)

BRIEFS = [
    ("ko", f"자동화 승인 {RUN}",
     "물류센터 자동화(AMR 20대) 도입 승인 요청. 투자 18억, 3년 내 회수, 인력 재배치 12명. 임원 대상."),
    ("en", f"Warehouse automation {RUN}",
     "Approval for warehouse automation (20 AMR units). Investment $1.8M, payback in 3 years, "
     "12 staff redeployed. For the executive committee."),
]
SCRIPTS = {"ko": r"[가-힣]", "en": r"[A-Za-z]"}

print("── generating with the provider this deployment is set up for ──")
print("   provider:", json.dumps(provider("openai-compatible")))
deadline = time.time() + options.minutes * 60
try:
    for language, title, prompt in BRIEFS:
        status, deck = call("POST", "/presentations", {"title": title, "prompt": prompt,
                                                       "requestedSlideCount": 8, "language": language})
        checks += 1
        if status != 201:
            failures.append(f"a deck could not be asked for: {status} {deck!r}")
            continue
        call("POST", f"/presentations/{deck['id']}/generate", {})
        state = {}
        while time.time() < deadline:
            time.sleep(3)
            state = call("GET", f"/presentations/{deck['id']}")[1] or {}
            if state.get("status") in ("completed", "failed"):
                break
        checks += 1
        if state.get("status") != "completed":
            failures.append(f"{language}: generation ended as {state.get('status')!r}: "
                            f"{str(state.get('errorMessage'))[:120]}")
            call("DELETE", f"/presentations/{deck['id']}")
            continue
        slides = state.get("slides") or []
        checks += 1
        if len(slides) < 4:
            failures.append(f"{language}: the model wrote {len(slides)} slide(s) for a brief asking 8")
        measured = call("GET", f"/presentations/{deck['id']}/inspect")[1] or {}
        checks += 1
        if measured.get("defects"):
            details = [f"{f.get('slide')} {f.get('slot')} {f.get('kind')}" for f in (measured.get("findings") or [])
                       if not f.get("advisory")]
            failures.append(f"{language}: a generated deck has {measured.get('defects')} defect(s): {details}")
        source = (call("GET", f"/presentations/{deck['id']}/source")[1] or {}).get("source", "")
        # Written in the language it was asked for. Names are not the deck's own
        # words: an image is named by whoever uploaded it, and a layout by
        # whoever made the template — this deployment's builtin templates name
        # their layouts in Korean, so an English deck refers to them in Korean
        # and is right to.
        prose = "\n".join(line for line in source.splitlines()
                          if not line.startswith("::image") and not line.startswith("@layout"))
        checks += 1
        if not re.search(SCRIPTS[language], prose):
            failures.append(f"{language}: the deck is not written in the language it was asked for")
        if language == "en":
            checks += 1
            if re.search(r"[가-힣]", prose):
                failures.append("en: a deck asked for in English carries Korean in its words")
        # A number the brief never gave is kept and said, not deleted silently.
        given = set(re.findall(r"\d[\d,.]*", prompt))
        used = {match for match in re.findall(r"\d[\d,.]*", prose)}
        introduced = {value for value in used if value not in given and len(value) > 1}
        notes = " ".join(state.get("generationNotes") or [])
        checks += 1
        if introduced and not notes.strip():
            failures.append(f"{language}: the deck introduced numbers the brief never gave "
                            f"({sorted(introduced)[:4]}) and said nothing about it")
        print(f"   {language}: {len(slides)} slides, {measured.get('defects')} defect(s), "
              f"{measured.get('advisories')} advisory(ies), "
              f"{'said what it introduced' if notes.strip() else 'nothing introduced'}")
        call("DELETE", f"/presentations/{deck['id']}")
    # Another draft of one slide, which is the other thing the model is for. It
    # is offered and not saved: the author decides, and a deck that revised
    # itself behind their back would be the worst of both.
    print("── another draft of one slide ──")
    status, drafting = call("POST", "/presentations", {"title": f"다시 쓰기 {RUN}", "prompt": "점검",
                                                       "language": "ko"})
    if status == 201:
        call("PUT", f"/presentations/{drafting['id']}/source",
             {"source": "# 현황과 문제\n- 전환 대상은 42개 시스템이며 이관 기간은 18개월로 예상됩니다\n"
                        "- 운영 비용은 매년 12% 늘고 있습니다\n- 장애 복구는 평균 4시간이 걸립니다\n!notes 노트\n"})
        held = data = call("GET", f"/presentations/{drafting['id']}")[1] or {}
        before_slide = json.dumps((held.get("slides") or [{}])[0], ensure_ascii=False)
        for name, ask in (("shorten", {"action": "shorten"}),
                          ("in English", {"instruction": "이 슬라이드만 영어로 다시 써 주세요"})):
            status, draft = call("POST", f"/presentations/{drafting['id']}/slides/1/revise", ask)
            checks += 1
            if status != 200:
                failures.append(f"a revision asked for as {name} answered {status}: {str(draft)[:120]}")
                continue
            drafted = (draft or {}).get("source") or ""
            checks += 1
            if drafted.strip() == "":
                failures.append(f"a revision asked for as {name} came back with nothing written")
            checks += 1
            if (draft or {}).get("applied"):
                failures.append(f"a revision asked for as {name} saved itself without being asked to")
            if name == "in English":
                checks += 1
                if not re.search(r"[A-Za-z]{4,}", drafted):
                    failures.append("a slide asked for in English came back without a word of it")
        after_slide = json.dumps(((call("GET", f"/presentations/{drafting['id']}")[1] or {}).get("slides")
                                  or [{}])[0], ensure_ascii=False)
        checks += 1
        if after_slide != before_slide:
            failures.append("the slide changed on the server although no revision was applied")
        print("   drafts came back, and the deck is as it was")
        call("DELETE", f"/presentations/{drafting['id']}")
finally:
    restored = provider(was if isinstance(was, str) else "fallback")
    print("   provider put back:", json.dumps(was), restored)

sys.exit(1 if failures else 0)
