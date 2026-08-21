"""What PowerPoint sees.

Ptium writes the .pptx itself, so the only honest check is to read one back with
a library that is not ours. python-pptx parses the same OOXML PowerPoint does: if
it finds a real table, real notes and real placeholders, so will PowerPoint — and
anything malformed enough to make PowerPoint offer to repair the file tends to
make python-pptx raise here first.

    pip install python-pptx
    python3 scripts/e2e/package.py
"""
import json, os, sys, time, urllib.request, urllib.error

BASE = os.environ.get("PTIUM_URL", "http://localhost:8099").rstrip("/") + "/api/v1"
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
RUN = str(int(time.time()))[-6:]
failures = []

try:
    from pptx import Presentation
except ImportError:
    print("python-pptx is not installed: pip install python-pptx")
    sys.exit(0)


def call(method, path, body=None, raw=False):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"X-Ptium-Dev-Secret": SECRET}
    if data:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(request) as response:
        payload = response.read()
    if raw:
        return payload
    return json.loads(payload)["data"] if payload else None


source = """# 패키지 점검 {run}
@cover
> 내보낸 파일을 다른 도구로 읽습니다

# 분기 실적
::table 분기 실적
- 항목 | 1분기 | 2분기
- 매출 | 120억 | 140억
- 영업이익 | 12억 | 18억
::
!notes 표는 편집 가능한 표여야 합니다.

# 핵심 지표
::kpi 한눈에
- 전환 대상 | 42개
- 예상 절감 | 18%
::
- 요점 하나
""".format(run=RUN)

deck = call("POST", "/presentations", {"title": f"패키지 점검 {RUN}", "prompt": "패키지",
                                       "requestedSlideCount": 3, "language": "ko"})
call("PUT", f"/presentations/{deck['id']}/source", {"source": source})
package = call("GET", f"/presentations/{deck['id']}/export?format=pptx", raw=True)
path = os.path.join(os.environ.get("TMPDIR", "/tmp"), f"ptium-package-{RUN}.pptx")
with open(path, "wb") as handle:
    handle.write(package)
print(f"exported {len(package):,} bytes → {path}")

try:
    read = Presentation(path)
except Exception as error:
    print(f" ✗ python-pptx cannot open the export: {error}")
    sys.exit(1)

print(f"slides: {len(read.slides)}  size: {read.slide_width}x{read.slide_height}")
if len(read.slides) != 3:
    failures.append(f"the package holds {len(read.slides)} slides, expected 3")

tables, numbered, noted, titles = 0, 0, 0, []
cover_numbered = False
for index, slide in enumerate(read.slides, 1):
    for shape in slide.shapes:
        if shape.has_table:
            tables += 1
            table = shape.table
            cells = [[cell.text for cell in row.cells] for row in table.rows]
            print(f"  slide{index}: table {len(table.rows)}x{len(table.columns)} {cells[0]}")
            if cells[0] != ["항목", "1분기", "2분기"]:
                failures.append(f"the table's header reads {cells[0]}")
            if len(table.rows) != 3:
                failures.append(f"the table has {len(table.rows)} rows, expected 3")
            # A grouped table is a table nobody can click into.
            if shape.shape_type == 6:
                failures.append("the table is inside a group")
        if shape.is_placeholder and shape.placeholder_format.type is not None:
            if str(shape.placeholder_format.type).startswith("SLIDE_NUMBER"):
                numbered += 1
                if index == 1:
                    cover_numbered = True
        if shape.has_text_frame and shape.text_frame.text.strip() and not titles:
            pass
    if slide.shapes.title is not None and slide.shapes.title.text.strip():
        titles.append(slide.shapes.title.text.strip())
    if slide.has_notes_slide and slide.notes_slide.notes_text_frame.text.strip():
        noted += 1

print(f"titles: {titles}")
print(f"tables: {tables}  numbered slides: {numbered}  slides with notes: {noted}")
if tables != 1:
    failures.append(f"found {tables} real tables, expected 1")
# The cover never carries one, and a layout that opts out of the master's shapes
# — the shipped closing page paints its own full-bleed background — has no place
# for one either. What must hold is that ordinary content slides are numbered and
# the cover is not.
if numbered < 1:
    failures.append("no slide carries a page number")
if cover_numbered:
    failures.append("the cover carries a page number")
if noted < 1:
    failures.append("no slide carries speaker notes")
if len(titles) != 3:
    failures.append(f"read {len(titles)} slide titles, expected 3")

print()
print(f"{len(failures)} failures")
for failure in failures:
    print(" ✗ " + failure)
sys.exit(1 if failures else 0)
