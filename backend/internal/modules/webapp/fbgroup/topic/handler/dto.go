package handler

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"erpCluvix/internal/modules/webapp/fbgroup/classify"
	"erpCluvix/pkg/errs"
)

const (
	nameMaxRunes    = 128
	keywordMaxRunes = 64
	keywordsMax     = 200
	branchesMax     = 30
)

var colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type branchInput struct {
	ID        uint     `json:"id"` // 0 = nhánh mới
	Name      string   `json:"name"`
	Keywords  []string `json:"keywords"`
	SortOrder int      `json:"sort_order"`
}

// topicInput — thân POST/PUT /fbgroup/topic. Gửi đủ trường (form sửa chủ đề luôn gửi đủ).
type topicInput struct {
	Name            string        `json:"name"`
	Color           string        `json:"color"`
	IncludeKeywords []string      `json:"include_keywords"`
	ExcludeKeywords []string      `json:"exclude_keywords"`
	MinMembers      int           `json:"min_members"`
	MinPostsPerDay  int           `json:"min_posts_per_day"`
	PublicOnly      bool          `json:"public_only"`
	SortOrder       int           `json:"sort_order"`
	Status          *int          `json:"status"`
	Branches        []branchInput `json:"branches"`
}

// cleanKeywords cắt khoảng trắng, bỏ rỗng/trùng (so theo dạng bỏ dấu), giới hạn độ dài.
func cleanKeywords(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, k := range in {
		k = strings.TrimSpace(k)
		n := classify.Normalize(k)
		if n == "" || seen[n] {
			continue
		}
		if utf8.RuneCountInString(k) > keywordMaxRunes {
			return nil, errs.NewLocalizedError("fbgroup_keyword_too_long", k, keywordMaxRunes)
		}
		seen[n] = true
		out = append(out, k)
	}
	if len(out) > keywordsMax {
		return nil, errs.NewLocalizedError("fbgroup_keywords_too_many", keywordsMax)
	}
	return out, nil
}

// normalize kiểm + làm sạch input. Trả LocalizedError (handler đưa vào response.HandleError).
func (in *topicInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > nameMaxRunes {
		return errs.NewLocalizedError("fbgroup_topic_name_invalid", nameMaxRunes)
	}
	if !colorRe.MatchString(in.Color) {
		return errs.NewLocalizedError("fbgroup_topic_color_invalid")
	}
	var err error
	if in.IncludeKeywords, err = cleanKeywords(in.IncludeKeywords); err != nil {
		return err
	}
	if len(in.IncludeKeywords) == 0 {
		return errs.NewLocalizedError("fbgroup_topic_include_required")
	}
	if in.ExcludeKeywords, err = cleanKeywords(in.ExcludeKeywords); err != nil {
		return err
	}
	if in.MinMembers < 0 || in.MinPostsPerDay < 0 {
		return errs.NewLocalizedError("invalid_data")
	}
	if in.Status != nil && *in.Status != 0 && *in.Status != 1 {
		return errs.NewLocalizedError("invalid_status")
	}
	if len(in.Branches) > branchesMax {
		return errs.NewLocalizedError("fbgroup_branches_too_many", branchesMax)
	}
	names := map[string]bool{}
	for i := range in.Branches {
		b := &in.Branches[i]
		b.Name = strings.TrimSpace(b.Name)
		if b.Name == "" || utf8.RuneCountInString(b.Name) > nameMaxRunes {
			return errs.NewLocalizedError("fbgroup_branch_name_invalid", nameMaxRunes)
		}
		if names[strings.ToLower(b.Name)] {
			return errs.NewLocalizedError("fbgroup_branch_name_duplicate", b.Name)
		}
		names[strings.ToLower(b.Name)] = true
		if b.Keywords, err = cleanKeywords(b.Keywords); err != nil {
			return err
		}
		if len(b.Keywords) == 0 {
			return errs.NewLocalizedError("fbgroup_branch_keywords_required", b.Name)
		}
	}
	return nil
}

func toJSON(v []string) []byte {
	b, _ := json.Marshal(v)
	return b
}
