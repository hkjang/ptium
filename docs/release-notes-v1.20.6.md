# Ptium v1.20.6

어제 잠긴 `.pptx` 의 거절 문구를 고쳤으니, **거절되는 나머지 경우**도 올려 봤습니다.
넷 중 셋이 **영어**로 답하고 있었습니다.

```
이름만 .pptx 로 바꾼 Word 문서 → "the package does not contain a PowerPoint presentation"
전송 중 잘린 .pptx           → "the uploaded file is not a valid PowerPoint package"
빈 파일                     → "The uploaded template is empty"
```

이 제품의 화면은 한국어입니다. **읽는 사람이 있는 자리에서는 한국어로 말합니다** — 서버 로그와
API는 영어로 쓰더라도요.

## 그리고 어느 문으로 가야 하는지 말합니다

이름만 바뀐 Word/Excel 파일은 **무엇인지 알 수 있습니다** — 압축 안의 부품 이름이
`word/document.xml` · `xl/workbook.xml` 이니까요. 그리고 **Ptium은 그 둘을 읽습니다.**
그러니 "PowerPoint 파일이 아닙니다"에서 끝낼 이유가 없습니다.

> PowerPoint 파일이 아니라 **Word 문서**입니다. 확장자를 `.docx` 로 되돌려
> **[기존 자료 가져오기]** 에 올리면 **제목이 슬라이드**가 됩니다.

> PowerPoint 파일이 아니라 **Excel 문서**입니다. 확장자를 `.xlsx` 로 되돌려
> **[기존 자료 가져오기]** 에 올리면 **시트마다 한 장**이 됩니다.

열리지 않는 나머지는 **아는 것만 말합니다** — 왜 안 열리는지는 바이트가 말해 주지 않으므로
단정하지 않고, 무엇을 해 보면 되는지를 말합니다.

> PowerPoint 파일로 열리지 않습니다. 파일이 손상됐거나 PowerPoint 파일이 아닐 수 있습니다.
> PowerPoint에서 열리는지 확인한 뒤 다시 올려 주세요.

**두 문(템플릿 · 가져오기)이 같은 말을 합니다.**

## 검사

거절 문구 다섯 가지를 고정하고, **전부 한국어인지**도 함께 봅니다. 예전 검사가 "바이트가
말하지 않는 것은 말하지 말라"고 했던 자리는 그 뜻을 지키면서 문구만 바꿨습니다 — 원인을
단정하지 않습니다.

전체 Go 테스트 · REST 489개 · edges 33 · deep 전부 0 failures. 멀쩡한 덱은 그대로
가져와집니다.

## 설치

```bash
gzip -dc ptium-1.20.6.tar.gz | docker load
```
