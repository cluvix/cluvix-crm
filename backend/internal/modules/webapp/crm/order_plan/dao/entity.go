package dao

import (
	"erpCluvix/internal/models"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

type OrderPlan struct {
	models.BaseModelWithCode
	OldCode       string           ` gorm:"column:old_code;size:50;comment:Mã đơn hàng cũ (sync đối tác)" json:"old_code"`
	OrderPlanType *int             ` gorm:"column:orderPlanType;default:1;comment:Đơn hàng mới hay cũ" json:"orderPlanType"`
	CustomerId    int              ` gorm:"column:customer_id;index;not null;comment:Khách hàng" json:"customer_id"`
	AppointmentId *int             ` gorm:"column:appointment_id;comment:Lịch hẹn" json:"appointment_id"`
	BranchId      *int             ` gorm:"column:branch_id;index;comment:Chi nhánh" json:"branch_id"`
	DiscountOther *decimal.Decimal ` gorm:"column:discount_other;type:decimal(20,2);default:0.00;comment:Giảm giá khác của đơn hàng" json:"discount_other"`
	PaymentPlan   *decimal.Decimal ` gorm:"column:paymentPlan;type:decimal(20,2);default:0.00;comment:Tổng thanh toán cho đơn hàng" json:"paymentPlan"`
	DebtsPlan     *decimal.Decimal ` gorm:"column:debtsPlan;type:decimal(20,2);default:0.00;comment:Công nợ đơn hàng" json:"debtsPlan"`
	ListRevenues  datatypes.JSON   `gorm:"column:listRevenues;type:json;comment:JSON array tư vấn viên phụ trách (1-nhiều)" json:"listRevenues"`
	SourceId      *int             ` gorm:"column:source_id;index;comment:Nguồn khách hàng" json:"source_id"`
	CreatedTime   *int64           ` gorm:"column:created_time;index;comment:Ngày nhập đơn hàng" json:"created_time"`
	VatRate       *string          ` gorm:"column:vat_rate;size:4;comment:Mức VAT của đơn: KCT, 0, 5, 8, 10 (%). NULL = không VAT" json:"vat_rate"`
	Note          string           ` gorm:"comment:Ghi chú" json:"note"`
}

func (OrderPlan) TableName() string { return "order_plan" }
