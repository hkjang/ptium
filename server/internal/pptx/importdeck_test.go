package pptx

import "testing"

// A line that is only the bullet its own design drew is not a point.
//
// A deck imported from a real company file came back with points reading "•"
// and "**•**": the glyph the original layout drew beside each line, read as a
// paragraph of its own.
func TestALineThatIsOnlyItsBulletIsNotAPoint(t *testing.T) {
	t.Parallel()
	for _, decoration := range []string{"•", "▪", "·", "-", "—", "**•**", " • ", "*•*", "‣", "※"} {
		if !saysNothing(decoration) {
			t.Errorf("%q was read as a point", decoration)
		}
	}
	for _, words := range []string{"• 핵심 요구사항", "비용 절감", "-30% 절감", "A-B 테스트", "**중요**", "3.5%"} {
		if saysNothing(words) {
			t.Errorf("%q was dropped as decoration", words)
		}
	}
}
