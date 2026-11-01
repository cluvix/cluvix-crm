package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	paymentRepo "erpCluvix/internal/modules/webapp/crm/payment/repo"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
)

// --- Phase 4 prototype: pure unit test of beforeSave using mock Reader ---
//
// Trước Phase 4: muốn test "payment lên service đã hoàn bị reject" phải spin
// MySQL thật qua testhelpers suite (5–15s). Sau Phase 4: chạy <1ms với fake reader.
//
// Pattern này áp dụng cho mọi service hot-spot ở Phase 4.2 — xem refactor-roadmap.md.

type fakeReader struct {
	refundByID   map[int]decimal.Decimal
	planByID     map[int]*int
	snapshotByID map[uint]paymentRepo.PaymentSnapshot
	calls        int
}

func (f *fakeReader) ServiceUsedRefund(_ *gorm.DB, id int) (decimal.Decimal, error) {
	f.calls++
	if v, ok := f.refundByID[id]; ok {
		return v, nil
	}
	return decimal.Zero, nil
}

func (f *fakeReader) ServiceUsedPlanID(_ *gorm.DB, id int) (*int, error) {
	if v, ok := f.planByID[id]; ok {
		return v, nil
	}
	return nil, nil
}

func (f *fakeReader) PaymentSnapshot(_ *gorm.DB, id uint) (paymentRepo.PaymentSnapshot, error) {
	if s, ok := f.snapshotByID[id]; ok {
		return s, nil
	}
	return paymentRepo.PaymentSnapshot{}, nil
}

// Compile-time check that fakeReader satisfies the interface.
var _ paymentRepo.Reader = (*fakeReader)(nil)

// M-5 (lens-money 2026-10-18, rule "hoàn = huỷ bớt, công nợ không đổi"): dịch vụ đã hoàn MỘT PHẦN vẫn
// phải thu tiếp được phần nợ còn lại ⇒ beforeSave KHÔNG còn chặn ErrServiceRefundedCannotPay.
// (Chặn thu vượt nợ nằm ở luồng tạo/sửa dưới khoá — test integration payment_concurrency.)
func TestBeforeSave_PaymentForOrder_AllowsPartlyRefundedService(t *testing.T) {
	fake := &fakeReader{refundByID: map[int]decimal.Decimal{42: decimal.NewFromInt(100)}}
	svc := NewPaymentServiceWithReader(nil, fake)

	p := &crmPaymentDao.Payment{
		ServiceUsedId: new(42),
		PaymentFor:    new(define.PaymentForOrder),
		Amount:        decimal.NewFromInt(500),
	}

	require.NoError(t, svc.beforeSave(nil, p, true))
}

func TestBeforeSave_PaymentForMoveDeposit_AllowsPartlyRefundedService(t *testing.T) {
	fake := &fakeReader{refundByID: map[int]decimal.Decimal{7: decimal.NewFromInt(50)}}
	svc := NewPaymentServiceWithReader(nil, fake)

	p := &crmPaymentDao.Payment{
		ServiceUsedId: new(7),
		PaymentFor:    new(define.PaymentForMoveDeposit),
		Amount:        decimal.NewFromInt(200),
	}

	require.NoError(t, svc.beforeSave(nil, p, true))
}

// --- Phase 4 expand: tests for repo method PaymentSnapshot ---

func TestBeforeSave_Update_VersionConflict_Rejected(t *testing.T) {
	// FE gửi updated_at=1700000005; DB hiện tại updated_at=1700000007 → ErrVersionConflict (khoá D6).
	fake := &fakeReader{
		snapshotByID: map[uint]paymentRepo.PaymentSnapshot{
			100: {UpdatedAt: 1700000007, Amount: decimal.NewFromInt(500)},
		},
	}
	svc := NewPaymentServiceWithReader(nil, fake)

	p := &crmPaymentDao.Payment{
		ID:         100,
		UpdatedAt:  1700000005, // stale
		Amount:     decimal.NewFromInt(500),
		PaymentFor: new(define.PaymentForClinicIncome),
	}

	err := svc.beforeSave(nil, p, false /* update */)
	require.ErrorIs(t, err, errs.ErrVersionConflict)
}

func TestBeforeSave_PaymentForOrder_ZeroRefund_DoesNotEarlyReturn(t *testing.T) {
	// Refund = 0 → không reject early; flow tiếp tục tới hết beforeSave.
	// (story-37 đã bỏ setFromServiceUsed.) Test chạy với tx=nil OK.
	fake := &fakeReader{
		refundByID:   map[int]decimal.Decimal{42: decimal.Zero},
		snapshotByID: map[uint]paymentRepo.PaymentSnapshot{100: {UpdatedAt: 1700000001, Amount: decimal.NewFromInt(500)}},
	}
	svc := NewPaymentServiceWithReader(nil, fake)

	p := &crmPaymentDao.Payment{
		ID:            100,
		UpdatedAt:     1700000001,
		ServiceUsedId: new(42),
		PaymentFor:    new(define.PaymentForOrder),
		Amount:        decimal.NewFromInt(500),
	}

	err := svc.beforeSave(nil, p, false /* update */)
	// Quan trọng: refund = 0 → KHÔNG reject. Có thể có error khác (vd version mismatch
	// nếu fake không setup đủ) nhưng không phải ErrServiceRefundedCannotPay.
	assert.NotErrorIs(t, err, errs.ErrServiceRefundedCannotPay)
}

// Pure-function tests — no DB required, no build tag.
// These run on every `go test ./...` invocation.

func TestCreatePayment_NegativeAmountRejected(t *testing.T) {
	svc := NewPaymentService(nil)
	p := &crmPaymentDao.Payment{Amount: decimal.NewFromInt(-500)}
	err := svc.createPayment(nil, p, 0)
	require.ErrorIs(t, err, errs.ErrAmountMustBePositive)
}

func TestCreatePayment_ZeroAmountRejected(t *testing.T) {
	svc := NewPaymentService(nil)
	p := &crmPaymentDao.Payment{Amount: decimal.Zero}
	err := svc.createPayment(nil, p, 0)
	require.Error(t, err)
}

// --- bindPayment whitelist tests (#2 mass assignment fix) ---

func TestBindPayment_AllowedFields(t *testing.T) {
	svc := NewPaymentService(nil)
	method := 2
	params := map[string]interface{}{
		"amount":    "150000",
		"method":    float64(method),
		"note":      "test note",
		"bank_code": "VCB123",
	}
	p := svc.bindPayment(params)
	assert.True(t, p.Amount.Equal(decimal.NewFromInt(150000)), "amount should be set")
	assert.Equal(t, method, p.Method, "method should be set")
	assert.Equal(t, "test note", p.Note)
	assert.Equal(t, "VCB123", p.BankCode)
}

// TestBindPayment_BoQuaKhoaThuongHieuCu — story-37/62 (1 thương hiệu/DB): FE cũ còn gửi khoá
// `company_id`; bindPayment phải bỏ qua key đó (Payment không còn field thương hiệu) và vẫn bind các
// field hợp lệ khác.
func TestBindPayment_BoQuaKhoaThuongHieuCu(t *testing.T) {
	svc := NewPaymentService(nil)
	params := map[string]interface{}{
		"amount":     float64(100),
		"company_id": float64(999), // FE-compat: khoá cũ
	}
	p := svc.bindPayment(params)
	assert.True(t, p.Amount.Equal(decimal.NewFromInt(100)), "amount phải được bind, có: %s", p.Amount)
}

func TestBindPayment_BlocksCreatedBy(t *testing.T) {
	svc := NewPaymentService(nil)
	params := map[string]interface{}{
		"amount":     float64(100),
		"created_by": float64(42),
	}
	p := svc.bindPayment(params)
	assert.Nil(t, p.CreatedBy, "created_by must not be settable by client")
}

func TestBindPayment_BlocksUpdatedAt(t *testing.T) {
	svc := NewPaymentService(nil)
	params := map[string]interface{}{
		"amount":     float64(100),
		"updated_at": float64(99),
	}
	p := svc.bindPayment(params)
	assert.Equal(t, int64(0), p.UpdatedAt, "updated_at (khoá lạc quan) must not be settable by client on create (remains zero value)")
}

func TestBindPayment_BlocksCode(t *testing.T) {
	svc := NewPaymentService(nil)
	params := map[string]interface{}{
		"amount": float64(100),
		"code":   "FORGED-CODE",
	}
	p := svc.bindPayment(params)
	assert.Equal(t, "", p.Code, "code must not be settable by client")
}

func TestBindPayment_CustomerIDAndDepositID(t *testing.T) {
	svc := NewPaymentService(nil)
	params := map[string]interface{}{
		"amount":      float64(500),
		"customer_id": float64(7),
		"deposit_id":  float64(3),
	}
	p := svc.bindPayment(params)
	require.NotNil(t, p.CustomerId)
	assert.Equal(t, 7, *p.CustomerId)
	require.NotNil(t, p.DepositId)
	assert.Equal(t, 3, *p.DepositId)
}

// --- allocatePaymentWaterfall tests — thanh toán 1 phần cho nhiều dịch vụ ---
// Ví dụ nghiệp vụ đối chiếu: SV1 nợ 3tr (created_day cũ nhất), SV2 nợ 5tr, SV3 nợ 2tr,
// khách trả 4tr → SV1 hết nợ (dư 1tr chảy tiếp), SV2 còn nợ 4tr, SV3 chưa đụng tới.

func svcDebt(id int, totalEnd, payment int64, createdDay *int64) serviceUsedDebt {
	return serviceUsedDebt{
		ID:         id,
		CustomerID: 1,
		Payable:    decimal.NewFromInt(totalEnd),
		Payment:    decimal.NewFromInt(payment),
		CreatedDay: createdDay,
	}
}

func TestAllocatePaymentWaterfall_PartialPayment_OldestServiceFirst(t *testing.T) {
	day1, day2, day3 := int64(100), int64(200), int64(300)
	svcMap := map[int]serviceUsedDebt{
		1: svcDebt(1, 3_000_000, 0, &day1),
		2: svcDebt(2, 5_000_000, 0, &day2),
		3: svcDebt(3, 2_000_000, 0, &day3),
	}
	ids := []int{3, 1, 2} // thứ tự chọn không theo created_day — hàm phải tự sort lại

	allocations := allocatePaymentWaterfall(ids, svcMap, decimal.NewFromInt(4_000_000))

	require.Len(t, allocations, 2)
	assert.Equal(t, 1, allocations[0].ID)
	assert.True(t, allocations[0].Amount.Equal(decimal.NewFromInt(3_000_000)), "SV1 phải được trả đủ (hết nợ)")
	assert.Equal(t, 2, allocations[1].ID)
	assert.True(t, allocations[1].Amount.Equal(decimal.NewFromInt(1_000_000)), "SV2 chỉ nhận phần dư sau khi SV1 trả đủ")
	// SV3 không xuất hiện — chưa đụng tới, không tạo payment record cho amount=0.

	sum := decimal.Zero
	for _, a := range allocations {
		sum = sum.Add(a.Amount)
	}
	assert.True(t, sum.Equal(decimal.NewFromInt(4_000_000)), "tổng phân bổ phải khớp chính xác amount đầu vào — không lệch 1 đồng")
}

func TestAllocatePaymentWaterfall_ExactFullPayment_AllServicesSettled(t *testing.T) {
	day1, day2 := int64(100), int64(200)
	svcMap := map[int]serviceUsedDebt{
		1: svcDebt(1, 3_000_000, 1_000_000, &day1), // nợ 2tr
		2: svcDebt(2, 5_000_000, 0, &day2),         // nợ 5tr
	}
	ids := []int{1, 2}

	allocations := allocatePaymentWaterfall(ids, svcMap, decimal.NewFromInt(7_000_000))

	require.Len(t, allocations, 2)
	assert.True(t, allocations[0].Amount.Equal(decimal.NewFromInt(2_000_000)))
	assert.True(t, allocations[1].Amount.Equal(decimal.NewFromInt(5_000_000)))
}

func TestAllocatePaymentWaterfall_NilCreatedDayTreatedAsOldest(t *testing.T) {
	day2 := int64(200)
	svcMap := map[int]serviceUsedDebt{
		1: svcDebt(1, 1_000_000, 0, &day2), // có created_day
		2: svcDebt(2, 1_000_000, 0, nil),   // đơn cũ thiếu created_day → coi như cũ nhất
	}
	ids := []int{1, 2}

	allocations := allocatePaymentWaterfall(ids, svcMap, decimal.NewFromInt(500_000))

	require.Len(t, allocations, 1)
	assert.Equal(t, 2, allocations[0].ID, "service thiếu created_day (nil) phải được ưu tiên trả trước")
}

func TestAllocatePaymentWaterfall_SkipsAlreadySettledService(t *testing.T) {
	day1, day2 := int64(100), int64(200)
	svcMap := map[int]serviceUsedDebt{
		1: svcDebt(1, 1_000_000, 1_000_000, &day1), // đã trả đủ, nợ = 0
		2: svcDebt(2, 2_000_000, 0, &day2),
	}
	ids := []int{1, 2}

	allocations := allocatePaymentWaterfall(ids, svcMap, decimal.NewFromInt(2_000_000))

	require.Len(t, allocations, 1)
	assert.Equal(t, 2, allocations[0].ID)
	assert.True(t, allocations[0].Amount.Equal(decimal.NewFromInt(2_000_000)))
}
