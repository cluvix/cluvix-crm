package classify

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Hội Mẹ Bầu Hà Nội":                 "hoi me bau ha noi",
		"HỘI THANH LÝ ĐỒ DÙNG MẸ & BÉ":      "hoi thanh ly do dung me be",
		"  Đau Dạ Dày - Trào Ngược!! ":      "dau da day trao nguoc",
		"Mẹ  va bé  tại . Czech ✅":          "me va be tai czech",
		"Tiêu hoá":                          "tieu hoa",
		"Tiêu hóa":                          "tieu hoa", // dấu đặt khác vị trí vẫn ra một dạng
		"Lần Đầu Mang Thai 🤰🏻 Và Làm Mẹ 🤱🏻": "lan dau mang thai va lam me",
		"": "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, muốn %q", in, got, want)
		}
	}
}

// Luật rút gọn của 3 chủ đề seed (migration 20261027200500) — đủ để chốt hành vi so khớp.
func seedRules() []Rule {
	return []Rule{
		{TopicID: 1,
			Include: NewMatcher([]string{"mẹ bầu", "bà bầu", "mang thai", "sau sinh", "bỉm sữa", "mẹ bỉm", "làm mẹ"}),
			Branches: []Branch{
				{ID: 11, Matcher: NewMatcher([]string{"sau sinh", "ở cữ"})},
				{ID: 12, Matcher: NewMatcher([]string{"mang thai", "mẹ bầu", "bà bầu"})},
				{ID: 13, Matcher: NewMatcher([]string{"bỉm sữa", "mẹ bỉm"})},
			}},
		{TopicID: 2,
			Include: NewMatcher([]string{"vì con", "nuôi con", "mẹ và bé", "mẹ & bé", "mẹ vs bé", "mẹ bé", "cho bé"})},
		{TopicID: 3,
			Include: NewMatcher([]string{"dạ dày", "dạ giày", "trào ngược", "tiêu hoá"}),
			Exclude: NewMatcher([]string{"áo dạ", "dạ hội"}),
			Branches: []Branch{
				{ID: 31, Matcher: NewMatcher([]string{"trào ngược"})},
				{ID: 32, Matcher: NewMatcher([]string{"tiêu hoá", "đại tràng"})},
			}},
	}
}

func TestClassify(t *testing.T) {
	type want struct{ topic, branch uint }
	cases := []struct {
		name string
		want []want
	}{
		{"Hội các mẹ bầu thông thái❤️", []want{{1, 12}}},
		{"Hội Mẹ Bỉm Sữa Nuôi Con", []want{{1, 13}, {2, 0}}}, // thuộc 2 chủ đề
		{"HỘI THANH LÝ ĐỒ DÙNG MẸ & BÉ CAO CẤP", []want{{2, 0}}},
		{"Shiper Vĩnh Phúc (mẹ vs bé)", []want{{2, 0}}},
		{"CHIA SẺ KINH NGHIỆM CHỮA BỆNH DẠ DÀY , TRÀO NGƯỢC DẠ DÀY", []want{{3, 31}}},
		{"BÌNH VỊ NAM BỆNH VIỆN 354 THUỐC ĐẶC TRỊ CÁC BỆNH VỀ DẠ GIÀY", []want{{3, 0}}},
		{"BỆNH TIÊU HÓA", []want{{3, 32}}},
		// lạc đề thật lấy từ kho — không được khớp chủ đề nào
		{"Hội Mê Dạ Tweed - Thanh Lý - Trao đổi các hãng nổi tiếng", nil},
		{"HỘI THANH LÝ ĐẦM DẠ HỘI", nil},
		{"Hội đồng hương Ba Vì", nil},
		{"Skincare Da Hỗn Hợp, Thiên Dầu, Mụn Nhạy Cảm", nil},
		// so theo TỪ: "mẹ bé" không được khớp nhầm vào giữa từ khác
		{"Shop mẹbéyêu", nil},
	}
	rules := seedRules()
	for _, c := range cases {
		got := Classify(Normalize(c.name), rules)
		if len(got) != len(c.want) {
			t.Errorf("%q: được %v, muốn %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i].TopicID != c.want[i].topic || got[i].BranchID != c.want[i].branch {
				t.Errorf("%q: được %v, muốn %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestClassifyExcludeWins(t *testing.T) {
	// có từ khoá gồm ("dạ dày") nhưng dính từ loại trừ ⇒ bỏ
	r := []Rule{{TopicID: 1, Include: NewMatcher([]string{"dạ dày"}), Exclude: NewMatcher([]string{"áo dạ"})}}
	if got := Classify(Normalize("Thanh lý áo dạ - đau dạ dày"), r); len(got) != 0 {
		t.Errorf("từ loại trừ phải thắng, được %v", got)
	}
}

func TestProvince(t *testing.T) {
	cases := map[string]string{
		"HỎI ĐÁP MẸ BẦU HÀ NỘI":                        "Hà Nội",
		"Mẹ Bỉm Sữa Sài Gòn":                           "TP.HCM",
		"Hội mẹ bầu mẹ bỉm BMT - ĐăkLăk":               "Đắk Lắk",
		"Hội Mang Thai Và Nuôi Con Nhỏ Tại Thanh Hoá":  "Thanh Hoá",
		"Cộng Đồng Mẹ Bỉm Sữa Đà Lạt":                  "Lâm Đồng",
		"Hội các mẹ bầu thông thái":                    "",
		"Hội Mê Dạ Tweed Cao Cấp Chính Hãng Hà Nội ✅️": "Hà Nội",
	}
	for in, want := range cases {
		if got := Province(Normalize(in)); got != want {
			t.Errorf("Province(%q) = %q, muốn %q", in, got, want)
		}
	}
}
