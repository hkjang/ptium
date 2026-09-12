package korean

import (
	"strings"
	"unicode/utf8"
)

// Whether a phrase is a heading or half of a sentence.
//
// A deck built from somebody's own brief cuts its headings out of that brief's
// sentences, and the cut lands wherever the splitting rule put it. Ten ordinary
// briefs measured through the running product produced these on the title line:
//
//	재고 회전율 개선 근거를 담고
//	하반기 목표를 제안
//	책임을 나누고 우선순위를 정
//	만들고 적용 일정을 안내
//
// One of the four was reported by anything. The others were drawn, fitted,
// scored and shipped.
//
// Two checks knew about this — one in the writer, one in the measurement — and
// each carried its own hand-written list of verbs, so a verb nobody had thought
// of went through both. 담고 was not on either list. The lists are replaced
// here by the rule the language already has.
//
// The rule: a word carrying a case marker is waiting for a predicate. 근거를
// wants a verb; 목표를 wants a verb. If the word after it is not a finished
// predicate, the phrase stopped before it said what it was going to say. That
// is why 안전 사고, 물류 창고 and 분기 매출 보고 are headings and 매출을 보고 is
// not: the same syllable 고, with and without something in front of it waiting.

// objectMarkers are the two endings that make a word the object of a verb that
// has to come after it.
//
// Only these two, and only on a stem of two syllables or more. The first list
// tried here held every case marker the language has, and it read 결과, 평가,
// 회의, 경로 and 성과 as marked words — 과 and 가 and 의 and 로 are the last
// syllable of ordinary nouns far more often than they are markers, so
// "결과 보고" and "협력사 평가 기준" came back as cut sentences. 은 and 는 were
// worse: they are also how a verb attaches to the noun it describes, so
// "남은 질문" and "매출을 늘리는 방법" were both reported.
//
// 을 and 를 do not collide: 가을 and 마을 are the only common nouns that end in
// 을, and both have a one-syllable stem, which is the floor below.
var objectMarkers = []string{"을", "를"}

// clauseMarkers are longer and collide with nothing, so one syllable of stem is
// enough under them.
var clauseMarkers = []string{"에게", "에서", "으로", "부터", "까지", "처럼", "보다", "한테"}

// finalEndings close a sentence. A predicate wearing one has finished.
//
// Kept deliberately short. Every ending here is one a heading really ends on —
// 늘렸습니다, 줄인다, 시작하자, 어떻게 할까 — and each one added is a word this
// will stop looking at, so a noun that happens to end the same way stops being
// reported. 자 is not here on purpose: 투자, 이자 and 조사 are nouns a heading
// ends on far more often than 하자 is a heading's last word.
var finalEndings = []string{
	"다", "요", "까", "죠", "네", "습니다", "ㅂ니다", "세요", "십시오", "시오",
	"는가", "은가", "ㄴ가", "음", "함", "됨", "임", "기",
}

// CutPhrase reports whether a phrase stops in the middle of what it was saying.
//
// It answers only for the shape the language makes plain: a case marker with
// nothing finished after it. A phrase it does not recognise is a phrase, which
// is the safe way round — this drives what a reader is told and, in the writer,
// what gets thrown away and rebuilt.
func CutPhrase(phrase string) bool {
	words := strings.Fields(strings.TrimSpace(phrase))
	if len(words) < 2 {
		return false
	}
	last := strings.TrimRight(words[len(words)-1], " .·—-,、。")
	before := words[len(words)-2]
	if last == "" || !endsInCaseMarker(before) {
		return false
	}
	// A word of one syllable after a marker is a verb somebody cut the ending
	// off — "…과제를 정" — whatever it is. There is no one-syllable word that
	// finishes a clause.
	if utf8.RuneCountInString(last) < 2 {
		return true
	}
	return !finishesAPredicate(last)
}

// endsInCaseMarker says whether a word is holding a marker, which only counts
// when there is enough word underneath it to be a word.
func endsInCaseMarker(word string) bool {
	trimmed := strings.TrimRight(word, " .·—-,、。")
	for _, marker := range objectMarkers {
		if stem, found := strings.CutSuffix(trimmed, marker); found &&
			utf8.RuneCountInString(stem) >= 2 && hangul(stem) {
			return true
		}
	}
	for _, marker := range clauseMarkers {
		if stem, found := strings.CutSuffix(trimmed, marker); found &&
			utf8.RuneCountInString(stem) >= 1 && hangul(stem) {
			return true
		}
	}
	return false
}

func finishesAPredicate(word string) bool {
	for _, ending := range finalEndings {
		if strings.HasSuffix(word, ending) {
			return true
		}
	}
	return false
}

// hangul reports whether the last character of a stem is Korean, so that a
// marker is only read as a marker on a Korean word. "Q&A를" is one; "Gulf" ends
// in an f and nothing else.
func hangul(stem string) bool {
	last, _ := utf8.DecodeLastRuneInString(stem)
	return last >= 0xAC00 && last <= 0xD7A3
}

// cutVerbTails are what a verb looks like when the sentence it belonged to was
// cut off after it. Each is read as a verb only in the one position this file
// looks at it — straight after an object — where no noun belongs.
//
// 서 is here with a length floor, because 문서 and 순서 are nouns and 확보해서
// is not. 어, 아, 여 and 게 are deliberately absent: 부여, 참여, 공유 and 무게
// are ordinary words and the endings are rare enough in this position not to
// be worth them.
var cutVerbTails = []string{"고", "며", "서", "려", "면서", "지만", "거나", "도록"}

// cutStems are verbs with the ending taken clean off, which no rule about
// syllables can tell from a noun: 세우 is not a word, and neither is 줄여.
var cutStems = map[string]bool{
	"세우": true, "세워": true, "줄이": true, "줄여": true, "늘리": true, "늘려": true,
	"높이": true, "높여": true, "낮추": true, "낮춰": true, "바꾸": true, "바꿔": true,
	"맞추": true, "맞춰": true, "이루": true, "이뤄": true, "만들": true, "만드": true,
	"하": true, "되": true, "받": true, "얻": true, "삼": true,
}

// TrimToPhrase turns a heading cut out of a sentence back into a heading.
//
// A heading is a noun phrase. What the cut leaves behind is an object still
// wearing its marker and, after it, either the verb it was waiting for or the
// wreck of one. Two different repairs, and which applies is decided by the word
// itself:
//
//	하반기 목표를 제안       →  하반기 목표 제안     the verb is a noun too; drop the marker
//	재고 회전율 개선 근거를 담고 →  재고 회전율 개선 근거  the verb is only a verb; drop it
//	다음 분기 과제를 정       →  다음 분기 과제       and a syllable is not a word at all
//
// It reports whether it changed anything, so a caller can tell a heading it
// repaired from one that was never cut.
func TrimToPhrase(heading string) (string, bool) {
	if !CutPhrase(heading) {
		return heading, false
	}
	words := strings.Fields(strings.TrimSpace(heading))
	last := strings.TrimRight(words[len(words)-1], " .·—-,、。")
	kept := words[:len(words)-1]
	// The object loses the marker that was holding it to a verb.
	for _, marker := range objectMarkers {
		if stem, found := strings.CutSuffix(kept[len(kept)-1], marker); found &&
			utf8.RuneCountInString(stem) >= 2 && hangul(stem) {
			kept[len(kept)-1] = stem
			break
		}
	}
	if !onlyAVerb(last) {
		kept = append(kept, last)
	}
	if len(kept) == 0 {
		return heading, false
	}
	trimmed := strings.Join(kept, " ")
	if utf8.RuneCountInString(strings.Join(strings.Fields(trimmed), "")) < 2 {
		return heading, false
	}
	return trimmed, true
}

// onlyAVerb says whether a word straight after an object can only be the verb
// that object was waiting for, so that nothing is lost by dropping it.
func onlyAVerb(word string) bool {
	if utf8.RuneCountInString(word) < 2 || cutStems[word] {
		return true
	}
	for _, tail := range cutVerbTails {
		if !strings.HasSuffix(word, tail) {
			continue
		}
		if tail == "서" && utf8.RuneCountInString(word) < 3 {
			continue
		}
		return true
	}
	return false
}

// BeginsMidClause reports a phrase that starts after its own beginning.
//
// "만들고 적용 일정을 안내" was the heading on four slides of one deck, cut out
// of "협력사 평가 기준을 다시 만들고 적용 일정을 안내합니다". Trimming the tail
// cannot mend it: what is missing is in front.
//
// The first word has to be a verb for this to be true, and 보고, 참고 and 사고
// all begin a heading perfectly well. What separates them is what is left when
// the ending comes off: 만들 and 검토하 are verbs waiting to be finished, 보 and
// 참 are single syllables that are nothing on their own.
func BeginsMidClause(phrase string) bool {
	words := strings.Fields(strings.TrimSpace(phrase))
	if len(words) < 2 {
		return false
	}
	// Not the last word: a verb there is the phrase's own ending and CutPhrase
	// answers for it. One in the middle means the phrase carries the join
	// between two clauses, so one of them is only half here.
	for index := 0; index < len(words)-1; index++ {
		if aVerbForm(words[index], index > 0 && endsInCaseMarker(words[index-1])) {
			return true
		}
	}
	return false
}

// aVerbForm says whether a word ending in a joining ending is really a verb.
//
// 보고, 참고, 사고 and 창고 all end in 고 and are nouns. Two things tell a verb
// from them: what is left when the ending comes off — 만들 and 검토하 are verbs
// waiting to be finished, 보 and 참 are single syllables that are nothing on
// their own — and whether the word before it is an object, which only a verb
// can follow.
func aVerbForm(word string, afterAnObject bool) bool {
	for _, tail := range cutVerbTails {
		stem, found := strings.CutSuffix(word, tail)
		if !found || !hangul(stem) || stem == "" {
			continue
		}
		if afterAnObject || cutStems[stem] {
			return true
		}
		for _, built := range []string{"하", "되", "시키"} {
			if strings.HasSuffix(stem, built) && utf8.RuneCountInString(stem) >= 2 {
				return true
			}
		}
	}
	return false
}
