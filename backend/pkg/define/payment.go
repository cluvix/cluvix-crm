package define

// Payment payment_for
const (
	PaymentForRefundService   = 1
	PaymentForRefundDeposit   = 2
	PaymentForDeposit         = 3
	PaymentForOrder           = 4
	PaymentForMoveDeposit     = 5
	PaymentForRefundDepositKG = 6
	PaymentForClinicIncome    = 7
	PaymentForClinicExpense   = 8
)

// Payment deposit_type
const (
	PaymentTypeDepositReserve = 1
	PaymentTypeDepositTemp    = 2
	PaymentTypeDepositKG      = 3
)

// Payment method — id cố định trong bảng payment_method (global, seed 1 lần,
// KHÔNG company-scoped, UNIQUE theo name — xem migrations/00000000000001_seed_data.up.sql).
// 1=Chuyển khoản, 2=Máy POS, 3=Chuyển cọc, 4=Tiền mặt.
const (
	PaymentMethodTransferDeposit = 3 // "Chuyển cọc" — dùng khi chuyển tiền cọc thành thanh toán (MoveDepositToPayment)
)

// OrderInstallment status
const (
	InstallmentStatusUnpaid  = 1
	InstallmentStatusPaid    = 2
	InstallmentStatusPartial = 3
)
