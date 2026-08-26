# Ptium v1.22.2

지난 판에서 검색의 와일드카드를 고쳤으니, **검색하는 자리를 전부** 두드려 봤습니다 — 덱 ·
템플릿 · 이미지 · 저장 슬라이드 · 사용자 · 감사 기록. SQL은 전부 멀쩡했습니다.

대신 **이름이 두 가지**라는 것이 드러났습니다.

```
GET /assets?q=점검          →   0개   (제대로 걸러짐)
GET /assets?search=점검     → 113개   (전부 — 조건이 그냥 무시됨)

GET /templates?search=Rail →   5개   (제대로)
GET /templates?q=Rail      →  50개   (전부)
```

**덱 · 이미지 · 저장 슬라이드는 `q`, 템플릿 · 사용자 · 감사 기록은 `search`** 였습니다.
그리고 **읽지 않는 이름은 조용히 무시**됩니다 — 거른 줄 알았는데 전체 목록을 받고,
아무도 그 사실을 말해 주지 않습니다. **질문에 대한 틀린 답이 맞는 답처럼** 옵니다.

## 둘 다 읽습니다

이제 어느 문에서든 `q` 와 `search` 가 **같은 질문**입니다. 문서에도 그렇게 적었습니다.

```
/assets        q=0     search=0     같음
/snippets      q=0     search=0     같음
/templates     q=5     search=5     같음
/presentations q=133   search=133   같음
/admin/users   q=531   search=531   같음
/admin/audit   q=2973  search=2973  같음
```

## 검사

붙는 이름 여덟 가지를 고정했습니다(둘 다 있을 때는 `q`, 빈 문자열이면 다른 쪽, 공백만이면
빈 검색). REST 훑기는 **네 곳에서 두 이름으로 같은 것을 물어** 답이 다르면 실패합니다
(522개 점검).

전체 Go 테스트 · REST 522개 · edges 33 · deep 전부 0 failures.

## 설치

```bash
gzip -dc ptium-1.22.2.tar.gz | docker load
```
