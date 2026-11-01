package define

// Mức VAT của đơn hàng — order_plan.vat_rate (bmad/order-vat D1). Giá dịch vụ CHƯA gồm VAT; VAT tính theo
// dòng = round(totalEnd × % / 100) vào order_service_used.vat_amount, cộng vào số phải thu.
// NULL = đơn trước khi có VAT ⇒ không VAT.
const (
	VatRateKCT = "KCT" // Không chịu thuế — KHÁC 0% về khai thuế, cùng 0 đồng tiền thuế
	VatRate0   = "0"
	VatRate5   = "5"
	VatRate8   = "8" // mức giảm tạm thời — chỉ cho đơn tới hết VatRate8LastDay
	VatRate10  = "10"
)

// VatRatePercent — % thuế của từng mức (KCT ⇒ 0). Khoá của map cũng là danh sách mức hợp lệ.
var VatRatePercent = map[string]int64{
	VatRateKCT: 0,
	VatRate0:   0,
	VatRate5:   5,
	VatRate8:   8,
	VatRate10:  10,
}

// VatRate8LastDay — ngày cuối (theo múi giờ thương hiệu, yyyy-mm-dd) mà ngày đơn còn được chọn mức 8%:
// giảm thuế tạm thời tới hết 31/12/2026 (tra 2026-11-01, nguồn thứ cấp). Nhà nước gia hạn ⇒ chỉ sửa hằng này
// + `VAT_RATE_8_LAST_DAY` ở FE (define/order-vat.define.ts).
const VatRate8LastDay = "2026-12-31"
