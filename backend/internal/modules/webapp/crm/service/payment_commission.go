// Payment commission logic — convert từ PHP Payment::afterSave.
//
// crm-standalone (D7, story-14): CHỈ còn hoa hồng + doanh số TELESALE (tư vấn viên, 1-nhiều theo
// order_service_used.listRevenues). Hoa hồng direct-sale / bác sĩ tư vấn / bác sĩ điều trị / phụ tá /
// KTV đã bỏ cùng liệu trình. Công thức + thứ tự làm tròn giữ nguyên — lưới:
// payment_telesale_parity_integration_test.go.
package service

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/pkg/define"
)

// serviceCategoryFee alias — dùng nội bộ commission flow.
const serviceCategoryFee = define.ServiceCategoryFee

// osuCommissionCtx gộp dữ liệu order_service_used cần cho hoa hồng + doanh số telesale — nạp 1 lần
// bằng loadOSUCommissionContext(), dùng chung cho handleTelesaleCommission + handleRevenueTelesale.
// Rate đã được RESOLVE SẴN (ưu tiên snapshot, fallback theo orderPlanType).
//
// bmad/order-vat D5: phiếu thu/hoàn là tiền CÓ thuế; hoa hồng + doanh số tính trên phần TRƯỚC thuế của phiếu
// (preTaxShare theo tỷ lệ totalEnd / (totalEnd + VatAmount) của dòng). Dòng không VAT ⇒ nguyên số phiếu.
type osuCommissionCtx struct {
	ListRevenues []int
	TotalEnd     decimal.Decimal
	VatAmount    decimal.Decimal

	TelesaleRate decimal.Decimal
	TelesaleType int

	ServiceCategoryID int
}

// loadOSUCommissionContext nạp 1 query duy nhất (JOIN service + order_plan) toàn bộ dữ
// liệu cần cho commission/revenue telesale của 1 order_service_used. Trả false nếu osu không tồn tại
// — caller bỏ qua hoa hồng/doanh số.
func loadOSUCommissionContext(tx *gorm.DB, serviceUsedId int) (*osuCommissionCtx, bool) {
	var row struct {
		ListRevenues []byte          `gorm:"column:list_revenues"`
		TotalEnd     decimal.Decimal `gorm:"column:total_end"`
		VatAmount    decimal.Decimal `gorm:"column:vat_amount"`

		TelesaleSnap     *decimal.Decimal `gorm:"column:telesale_snap"`
		TelesaleTypeSnap *int             `gorm:"column:telesale_type_snap"`

		TelesaleLive     decimal.Decimal `gorm:"column:telesale_live"`
		TelesaleNewLive  decimal.Decimal `gorm:"column:telesale_new_live"`
		TelesaleTypeLive int             `gorm:"column:telesale_type_live"`

		ServiceCategoryID int `gorm:"column:service_category_id"`
		OrderPlanType     int `gorm:"column:order_plan_type"`
	}

	if err := tx.Raw(`
		SELECT
			osu.listRevenues AS list_revenues,
			COALESCE(osu.totalEnd, 0) AS total_end,
			osu.vat_amount AS vat_amount,
			osu.commission_telesale_snap AS telesale_snap,
			osu.commission_telesale_type_snap AS telesale_type_snap,
			COALESCE(s.commission_telesale, 0) AS telesale_live,
			COALESCE(s.commission_telesale_new, 0) AS telesale_new_live,
			COALESCE(s.commission_telesale_type, 1) AS telesale_type_live,
			COALESCE(s.serviceCategory_id, 1) AS service_category_id,
			COALESCE(otp.orderPlanType, 1) AS order_plan_type
		FROM order_service_used osu
		JOIN service s ON s.id = osu.service_id
		LEFT JOIN order_plan otp ON otp.id = osu.order_plan_id
		WHERE osu.id = ?
	`, serviceUsedId).Scan(&row).Error; err != nil {
		return nil, false
	}

	ctx := &osuCommissionCtx{
		TotalEnd:          row.TotalEnd,
		VatAmount:         row.VatAmount,
		ServiceCategoryID: row.ServiceCategoryID,
	}
	if len(row.ListRevenues) > 0 {
		_ = json.Unmarshal(row.ListRevenues, &ctx.ListRevenues)
	}

	isOld := row.OrderPlanType == define.OrderPlanTypeOld

	if row.TelesaleSnap != nil {
		ctx.TelesaleRate = *row.TelesaleSnap
		if row.TelesaleTypeSnap != nil {
			ctx.TelesaleType = *row.TelesaleTypeSnap
		} else {
			ctx.TelesaleType = define.DiscountTypeNormal
		}
	} else if isOld {
		ctx.TelesaleRate, ctx.TelesaleType = row.TelesaleLive, row.TelesaleTypeLive
	} else {
		ctx.TelesaleRate, ctx.TelesaleType = row.TelesaleNewLive, row.TelesaleTypeLive
	}

	return ctx, true
}

// distributeCommission tính hoa hồng theo type (VND/%) trên totalEnd + amount thanh toán, rồi chia
// đều cho `dem` người qua splitEvenLastRemainder.
func distributeCommission(amount, totalEnd, rate decimal.Decimal, commissionType, dem int) []decimal.Decimal {
	hundred := decimal.NewFromInt(100)
	total := decimal.Zero
	if commissionType == define.DiscountTypeNormal {
		if totalEnd.IsPositive() {
			percent := amount.Div(totalEnd).Mul(hundred).Round(2)
			// /investigate 2026-07-29: thiếu .Div(hundred) khiến hoa hồng VND bị tính gấp 100
			// lần (percent đã ở thang 0-100, phải quy về tỷ lệ 0-1 trước khi nhân rate).
			total = percent.Div(hundred).Mul(rate).Round(0)
		}
	} else {
		total = amount.Div(hundred).Mul(rate).Round(0)
	}

	return splitEvenLastRemainder(total, dem)
}

// splitEvenLastRemainder chia đều `total` cho `dem` người — người cuối nhận phần dư để
// SUM(shares) luôn khớp đúng total, không lệch vài đồng do Round(0) từng người độc lập.
func splitEvenLastRemainder(total decimal.Decimal, dem int) []decimal.Decimal {
	if dem <= 0 {
		return nil
	}
	demDec := decimal.NewFromInt(int64(dem))
	base := total.Div(demDec).Truncate(0)
	remainder := total.Sub(base.Mul(demDec))

	shares := make([]decimal.Decimal, dem)
	for i := range shares {
		shares[i] = base
	}
	shares[dem-1] = shares[dem-1].Add(remainder)
	return shares
}

// --- OrderCommissionTelesale (hoa hồng tư vấn viên 1-nhiều) ---

// handleTelesaleCommission ghi nhận hoa hồng cho từng tư vấn viên trong listRevenues:
// xóa cũ → tính mới → insert.
func (s *PaymentService) handleTelesaleCommission(tx *gorm.DB, ctx *osuCommissionCtx, paymentID uint, serviceUsedId, parentId, customerId int,
	branchId int, amount decimal.Decimal, paymentOfDate *int64) {

	// Xóa cũ — luôn chạy trước, kể cả khi list rỗng (member bị gỡ hết) — tránh
	// để lại row commission cũ của member đã bị xoá khỏi listRevenues.
	tx.Exec("DELETE FROM order_commission_telesale WHERE payment_id = ?", paymentID)

	dem := len(ctx.ListRevenues)
	if dem == 0 {
		return
	}
	if ctx.ServiceCategoryID == serviceCategoryFee {
		return
	}
	if ctx.TelesaleRate.IsZero() {
		return
	}

	// Hoa hồng trên phần TRƯỚC thuế của phiếu (D5); cột amount cũng ghi số trước thuế.
	amount = preTaxShare(amount, ctx.TotalEnd, ctx.VatAmount)
	shares := distributeCommission(amount, ctx.TotalEnd, ctx.TelesaleRate, ctx.TelesaleType, dem)

	for i, telesaleId := range ctx.ListRevenues {
		if err := tx.Exec(`INSERT INTO order_commission_telesale
			(customer_id, order_plan_id, order_service_used_id, payment_id,
			 revenue_id, branch_id,
			 amount, commissionTelesale, confirm_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			customerId, parentId, serviceUsedId, paymentID,
			telesaleId, branchId,
			amount, shares[i], paymentOfDate).Error; err != nil {
			return
		}
	}
}

// --- order_revenue_telesale (doanh số, KHÔNG cần rate) ---
//
// Khác handleTelesaleCommission (chỉ ghi khi commissionRate > 0 và serviceCategory != Fee): hàm dưới
// LUÔN ghi nhận doanh số cho mỗi payment FOR_ORDER/FOR_MOVE_DEPOSIT (và dòng ÂM cho FOR_REFUND_SERVICE — M-13),
// không phụ thuộc cấu hình hoa hồng.
// revenue = amount thanh toán chia đều cho số member trong list (người cuối nhận phần dư).

func handleRevenueTelesale(tx *gorm.DB, ctx *osuCommissionCtx, paymentID uint, serviceUsedId, parentId, customerId int,
	branchId int, amount decimal.Decimal, paymentOfDate *int64) {
	// DELETE luôn chạy trước — kể cả khi list rỗng (member bị gỡ hết) — tránh
	// để lại row doanh số cũ của member đã bị xoá khỏi listRevenues.
	tx.Exec("DELETE FROM order_revenue_telesale WHERE payment_id = ?", paymentID)

	if len(ctx.ListRevenues) == 0 {
		return
	}

	// Doanh số = phần TRƯỚC thuế của phiếu (bmad/order-vat D5); cột amount cũng ghi số trước thuế.
	amount = preTaxShare(amount, ctx.TotalEnd, ctx.VatAmount)
	// lens-money M-18: chia đều, người cuối nhận phần dư ⇒ Σ doanh số = đúng số tiền (trước thuế) của phiếu (bản cũ
	// Round(0) từng người: 1.000.001 / 3 ⇒ 333.334 × 3 = 1.000.002).
	shares := splitEvenLastRemainder(amount, len(ctx.ListRevenues))
	for i, id := range ctx.ListRevenues {
		revenuePerMember := shares[i]
		if err := tx.Exec(`INSERT INTO order_revenue_telesale
			(customer_id, order_plan_id, order_service_used_id, payment_id,
			 revenue_id, branch_id, amount, revenue, confirm_time, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			customerId, parentId, serviceUsedId, paymentID,
			id, branchId, amount, revenuePerMember, paymentOfDate,
			time.Now().Unix(), time.Now().Unix()).Error; err != nil {
			return
		}
	}
}

// resyncRevenueForOSU tính lại order_revenue_telesale cho TẤT CẢ payment đã có của 1
// order_service_used — gọi khi staff sửa đơn (đổi listRevenues, hoặc đổi VAT dòng ⇒ đổi phần trước thuế) sau khi
// dịch vụ đã có payment. amount của mỗi payment giữ nguyên (không đổi số tiền đã thanh toán), chỉ chia lại theo
// list member + tỷ lệ trước thuế mới nhất.
// Idempotent: handleRevenueTelesale tự DELETE+INSERT theo payment_id, an toàn gọi lại nhiều lần.
// ctx (list member) nạp 1 lần cho toàn bộ payment — list không đổi giữa các payment của cùng osu.
func resyncRevenueForOSU(tx *gorm.DB, serviceUsedId, parentId, customerId int) {
	ctx, ok := loadOSUCommissionContext(tx, serviceUsedId)
	if !ok {
		return
	}
	var payments []struct {
		ID            uint            `gorm:"column:id"`
		Amount        decimal.Decimal `gorm:"column:amount"`
		BranchId      int             `gorm:"column:branch_id"`
		PaymentOfDate *int64          `gorm:"column:payment_of_date"`
	}
	tx.Table("payment").
		Select("id, amount, branch_id, payment_of_date").
		Where("serviceUsed_id = ? AND payment_for IN ?", serviceUsedId,
			// gồm cả phiếu hoàn (dòng doanh số âm, M-13) — đổi tư vấn viên thì phần thu hồi cũng chuyển theo.
			[]int{define.PaymentForOrder, define.PaymentForMoveDeposit, define.PaymentForRefundService}).
		Find(&payments)

	for _, p := range payments {
		handleRevenueTelesale(tx, ctx, p.ID, serviceUsedId, parentId, customerId,
			p.BranchId, p.Amount, p.PaymentOfDate)
	}
}
