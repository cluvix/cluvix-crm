package classify

import "strings"

// provinceAliases — tên tỉnh/thành hiển thị → các cách viết hay gặp trong tên group (dạng đã
// Normalize). Thứ tự quan trọng: tỉnh đứng trước thắng khi tên group nhắc nhiều tỉnh.
// Chỉ phủ các tỉnh/thành hay xuất hiện trong tên group; tỉnh khác để trống (không đoán).
var provinceAliases = []struct {
	Name    string
	Aliases []string
}{
	{"TP.HCM", []string{"sai gon", "saigon", "tp hcm", "tphcm", "hcm", "ho chi minh"}},
	{"Hà Nội", []string{"ha noi", "hanoi"}},
	{"Đà Nẵng", []string{"da nang", "danang"}},
	{"Hải Phòng", []string{"hai phong"}},
	{"Cần Thơ", []string{"can tho"}},
	{"Huế", []string{"hue", "thua thien hue"}},
	{"Thanh Hoá", []string{"thanh hoa"}},
	{"Nghệ An", []string{"nghe an"}},
	{"Nam Định", []string{"nam dinh"}},
	{"Thái Bình", []string{"thai binh"}},
	{"Đắk Lắk", []string{"dak lak", "daklak", "dac lac", "bmt", "buon ma thuot"}},
	{"Lâm Đồng", []string{"lam dong", "da lat", "dalat", "bao loc"}},
	{"Bắc Ninh", []string{"bac ninh"}},
	{"Hải Dương", []string{"hai duong"}},
	{"Quảng Ninh", []string{"quang ninh", "ha long"}},
	{"Bình Dương", []string{"binh duong"}},
	{"Đồng Nai", []string{"dong nai", "bien hoa"}},
	{"Bà Rịa-Vũng Tàu", []string{"vung tau", "ba ria"}},
	{"Khánh Hoà", []string{"khanh hoa", "nha trang"}},
	{"Vĩnh Long", []string{"vinh long"}},
	{"Quảng Nam", []string{"quang nam", "hoi an", "tam ky"}},
	{"Thái Nguyên", []string{"thai nguyen"}},
	{"Phú Thọ", []string{"phu tho", "viet tri"}},
	{"Vĩnh Phúc", []string{"vinh phuc"}},
	{"Ninh Bình", []string{"ninh binh"}},
	{"Gia Lai", []string{"gia lai", "pleiku"}},
	{"Long An", []string{"long an"}},
	{"An Giang", []string{"an giang"}},
	{"Kiên Giang", []string{"kien giang", "phu quoc"}},
	{"Hà Tĩnh", []string{"ha tinh"}},
	{"Bắc Giang", []string{"bac giang"}},
	{"Bình Định", []string{"binh dinh", "quy nhon"}},
	{"Bình Thuận", []string{"binh thuan", "phan thiet"}},
	{"Tiền Giang", []string{"tien giang", "my tho"}},
	{"Bến Tre", []string{"ben tre"}},
	{"Hưng Yên", []string{"hung yen"}},
	{"Quảng Ngãi", []string{"quang ngai"}},
	{"Quảng Bình", []string{"quang binh"}},
	{"Lào Cai", []string{"lao cai", "sa pa", "sapa"}},
}

// Province trả tỉnh đầu tiên được nhắc trong tên group (đã Normalize), "" nếu không có.
func Province(normalizedName string) string {
	padded := " " + normalizedName + " "
	for _, p := range provinceAliases {
		for _, a := range p.Aliases {
			if strings.Contains(padded, " "+a+" ") {
				return p.Name
			}
		}
	}
	return ""
}
