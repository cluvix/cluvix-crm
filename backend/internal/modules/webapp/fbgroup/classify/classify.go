// Package classify — gắn chủ đề, nhánh và tỉnh cho group Facebook dựa trên TÊN group.
//
// Hàm thuần (không DB) để unit test được bằng tên group thật. Cả tên group lẫn từ khoá đều đi qua
// Normalize (chữ thường, bỏ dấu, đ→d, ký tự không phải chữ/số thành khoảng trắng) rồi so khớp THEO
// TỪ: từ khoá "me bau" khớp "hoi me bau ha noi" nhưng không khớp "me baulo". Nhờ đó các cách viết
// "mẹ & bé", "mẹ - bé", "MẸ BÉ" cùng về một dạng "me be".
package classify

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Normalize trả tên/từ khoá về dạng so khớp: chữ thường, bỏ dấu, đ→d, mọi ký tự không phải chữ
// hoặc số thành 1 khoảng trắng, không khoảng trắng đầu/cuối.
func Normalize(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	var b strings.Builder
	b.Grow(len(out))
	space := true // bỏ khoảng trắng đầu
	for _, r := range out {
		switch {
		case r == 'đ' || r == 'Đ':
			r = 'd'
		default:
			r = unicode.ToLower(r)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Matcher — tập từ khoá đã chuẩn hoá, so khớp theo từ.
type Matcher struct {
	words []string // dạng " me bau " (có khoảng trắng 2 đầu để so theo từ)
}

// NewMatcher chuẩn hoá danh sách từ khoá; từ khoá rỗng sau chuẩn hoá bị bỏ.
func NewMatcher(keywords []string) Matcher {
	m := Matcher{}
	seen := map[string]bool{}
	for _, k := range keywords {
		n := Normalize(k)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		m.words = append(m.words, " "+n+" ")
	}
	return m
}

// Empty — matcher không có từ khoá nào.
func (m Matcher) Empty() bool { return len(m.words) == 0 }

// MatchPadded — padded là tên đã Normalize và bọc " … ".
func (m Matcher) MatchPadded(padded string) bool {
	for _, w := range m.words {
		if strings.Contains(padded, w) {
			return true
		}
	}
	return false
}

// Branch — nhánh trong chủ đề.
type Branch struct {
	ID      uint
	Matcher Matcher
}

// Rule — luật của 1 chủ đề. Branches xếp sẵn theo thứ tự xét (sort_order).
type Rule struct {
	TopicID  uint
	Include  Matcher
	Exclude  Matcher
	Branches []Branch
}

// Match — 1 chủ đề group khớp. BranchID = 0 nghĩa là không thuộc nhánh nào.
type Match struct {
	TopicID  uint
	BranchID uint
}

// Classify trả các chủ đề tên group khớp: khớp ≥1 từ khoá gồm VÀ không khớp từ khoá loại trừ nào.
// Nhánh là nhánh ĐẦU TIÊN (theo thứ tự trong Rule.Branches) có từ khoá khớp.
func Classify(normalizedName string, rules []Rule) []Match {
	padded := " " + normalizedName + " "
	out := make([]Match, 0, 2)
	for _, r := range rules {
		if r.Include.Empty() || !r.Include.MatchPadded(padded) || r.Exclude.MatchPadded(padded) {
			continue
		}
		m := Match{TopicID: r.TopicID}
		for _, b := range r.Branches {
			if b.Matcher.MatchPadded(padded) {
				m.BranchID = b.ID
				break
			}
		}
		out = append(out, m)
	}
	return out
}
