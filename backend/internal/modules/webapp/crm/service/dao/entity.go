package dao

import (
	"erpCluvix/internal/models"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

type OrderCommissionTelesale struct {
	ID                 uint            `gorm:"primaryKey" json:"id" form:"-"`
	CustomerId         int             ` gorm:"column:customer_id;index;not null;comment:Khách hàng" json:"customer_id"`
	OrderPlanId        int             ` gorm:"column:order_plan_id;index;not null;comment:Đơn hàng" json:"order_plan_id"`
	OrderServiceUsedId int             ` gorm:"column:order_service_used_id;index;not null;comment:Dịch vụ sử dụng" json:"order_service_used_id"`
	PaymentId          *int            ` gorm:"column:payment_id;index;comment:Thanh toán" json:"payment_id"`
	RevenueId          *int            ` gorm:"column:revenue_id;index;comment:Tư vấn viên" json:"revenue_id"`
	BranchId           int             ` gorm:"column:branch_id;index;not null;comment:Chi nhánh" json:"branch_id"`
	Amount             decimal.Decimal ` gorm:"column:amount;type:decimal(20,2);default:0.00;comment:Số tiền thanh toán trigger hoa hồng" json:"amount"`
	CommissionTelesale decimal.Decimal ` gorm:"column:commissionTelesale;type:decimal(20,2);default:0.00;comment:Số tiền hoa hồng" json:"commissionTelesale"`
	ConfirmCommission  *int            ` gorm:"column:confirm_commission;default:0;comment:Xác nhận tính hoa hồng" json:"confirm_commission"`
	ConfirmTime        *int64          ` gorm:"column:confirm_time;comment:Thời gian tính hoa hồng" json:"confirm_time"`
	ServiceIdSnapshot  *int            ` gorm:"column:service_id_snapshot;comment:service_id snapshot tại thời điểm ghi nhận hoa hồng, NULL = chưa từng đổi dịch vụ" json:"service_id_snapshot"`
}

func (OrderCommissionTelesale) TableName() string { return "order_commission_telesale" }

type OrderServiceUsed struct {
	models.BaseModelWithCode
	OldCode             string           ` gorm:"column:old_code;size:50;comment:Mã dịch vụ đơn hàng cũ (sync đối tác)" json:"old_code"`
	CustomerId          int              ` gorm:"column:customer_id;index;not null;comment:Khách hàng" json:"customer_id"`
	AppointmentId       *int             ` gorm:"column:appointment_id;index;comment:Lịch hẹn" json:"appointment_id"`
	BranchId            *int             ` gorm:"column:branch_id;index;comment:Chi nhánh" json:"branch_id"`
	OrderPlanId         int              ` gorm:"column:order_plan_id;index;not null;comment:ID kế hoạch sử dụng dịch vụ" json:"order_plan_id"`
	ServiceId           int              ` gorm:"column:service_id;index;not null;comment:ID dịch vụ sử dụng" json:"service_id"`
	ListRevenues        datatypes.JSON   `gorm:"column:listRevenues;type:json;comment:JSON array tư vấn viên phụ trách (1-nhiều)" json:"listRevenues"`
	SourceId            *int             ` gorm:"column:source_id;index;comment:Dịch vụ về từ nguồn nào" json:"source_id"`
	Quantity            *int             ` gorm:"comment:Số lượng sản" json:"quantity"`
	UnitPrice           *decimal.Decimal ` gorm:"column:unit_price;type:decimal(20,2);default:0.00;comment:Đơn giá sản phẩm" json:"unit_price"`
	ServiceDiscount     *decimal.Decimal ` gorm:"column:service_discount;type:decimal(20,2);default:0.00;comment:Chiết khấu theo dịch vụ" json:"service_discount"`
	ServiceDiscountType *int             ` gorm:"column:service_discount_type;default:1;comment:Loại chiết khấu theo dịch vụ" json:"service_discount_type"`
	DiscountOther       *decimal.Decimal ` gorm:"column:discount_other;type:decimal(20,2);default:0.00;comment:Giảm giá khác" json:"discount_other"`
	NoteDiscount        string           ` gorm:"column:note_discount;comment:Ghi chú lý do chiết khấu" json:"note_discount"`
	RefundServiceUsed   *decimal.Decimal ` gorm:"column:refundServiceUsed;type:decimal(20,2);default:0.00;comment:Hoàn trả dịch vụ" json:"refundServiceUsed"`
	Total               *decimal.Decimal ` gorm:"type:decimal(20,2);default:0.00;comment:Tổng tiền sau khi khấu trừ service_discount" json:"total"`
	DiscountServiceLock decimal.Decimal  ` gorm:"column:discountServiceLock;type:decimal(20,2);default:0.00;comment:Giảm trừ do khoá dịch vụ" json:"discountServiceLock"`
	RankDiscountPercent *decimal.Decimal ` gorm:"column:rank_discount_percent;type:decimal(5,2);default:0.00;comment:Giảm trừ theo hạng thành viên" json:"rank_discount_percent"`
	RankDiscountAmount  *decimal.Decimal ` gorm:"column:rank_discount_amount;type:decimal(20,2);default:0.00;comment:Số tiền giảm trừ theo hạng thành viên" json:"rank_discount_amount"`
	InstallmentConfig   datatypes.JSON   `gorm:"column:installment_config;type:json;comment:Cấu hình trả góp (JSON)" json:"installment_config"`
	TotalEnd            *decimal.Decimal ` gorm:"column:totalEnd;type:decimal(20,2);default:0.00;comment:Tổng tiền sau khi khấu trừ service_discount, discount_other" json:"totalEnd"`
	Payment             *decimal.Decimal ` gorm:"type:decimal(20,2);default:0.00;comment:Số tiền đã thanh toán" json:"payment"`
	Note                string           ` gorm:"comment:Ghi chú" json:"note"`
	PaymentStatus       *int             ` gorm:"column:payment_status;default:1;comment:Trạng thái thanh toán" json:"payment_status"`
	ActivatedAt         *int64           ` gorm:"column:activated_at;comment:Thời điểm dịch vụ bắt đầu có hiệu lực" json:"activated_at"`
	DayComplete         *int64           ` gorm:"column:dayComplete;comment:Ngày hoàn thành dịch vụ" json:"dayComplete"`
	DayReExploitation   *int64           ` gorm:"column:dayReExploitation;comment:Ngày tái khai thác" json:"dayReExploitation"`
	CreatedDay          *int64           ` gorm:"column:created_day;index;comment:Thời gian tạo - Có thể chỉnh sửa" json:"created_day"`
	IsLock              int              ` gorm:"column:isLock;not null;default:1;comment:Trạng thái khoá dịch vụ. 1: bình thường, 2: Khoá, 3: Tạm ngưng" json:"isLock"`

	// Snapshot commission rates từ service tại thời điểm lên đơn.
	// NULL = chưa có snapshot (đơn cũ), fallback sang JOIN service khi tính hoa hồng.
	CommissionTelesaleSnap     *decimal.Decimal ` gorm:"column:commission_telesale_snap;type:decimal(20,2)" json:"commission_telesale_snap"`
	CommissionTelesaleTypeSnap *int             ` gorm:"column:commission_telesale_type_snap" json:"commission_telesale_type_snap"`
}

func (OrderServiceUsed) TableName() string { return "order_service_used" }
