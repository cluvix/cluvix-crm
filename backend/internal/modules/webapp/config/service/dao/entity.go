package dao

import (
	"erpCluvix/internal/models"

	"github.com/shopspring/decimal"
)

type Service struct {
	models.BaseModelWithCode
	IdOld             *int             ` gorm:"column:id_old;uniqueIndex" json:"id_old"`
	Name              string           ` gorm:"size:255;not null" json:"name"`
	CodeOld           string           ` gorm:"column:code_old;size:25;comment:Mã dịch vụ cũ" json:"code_old"`
	SortOrder         *int             ` gorm:"column:SortOrder;comment:Vị trí hiển thị" json:"SortOrder"`
	ServiceTypeId     *int             ` gorm:"column:service_type_id;index" json:"service_type_id"`
	MinPrice          *decimal.Decimal ` gorm:"column:min_price;type:decimal(20,2);comment:Giá dịch vụ thấp nhất" json:"min_price"`
	MaxPrice          *decimal.Decimal ` gorm:"column:max_price;type:decimal(20,2);comment:Giá dịch vụ cao nhất" json:"max_price"`
	UnitId            *int             ` gorm:"column:unit_id;index;comment:Đơn vị tính của dịch vụ" json:"unit_id"`
	ServiceCategoryId *int             ` gorm:"column:serviceCategory_id;default:1;comment:Dịch vụ hay sản phẩm" json:"serviceCategory_id"`
	Child             *int             ` gorm:"default:0;comment:Thuộc răng trẻ em hay người lớn" json:"child"`
	IsInstallment     *int             ` gorm:"column:is_installment;default:0;comment:Dịch vụ được trả góp hay không?" json:"is_installment"`
	TimeReminder      *int             ` gorm:"column:time_reminder;default:0;comment:Thời gian nhắc lịch hẹn sau khi sử dụng dịch vụ" json:"time_reminder"`
	// Field code commission_telesale* giữ nguyên (không migration đổi tên cột) — chỉ nhãn hiển thị
	// FE/export/import đổi "Telesale" → "Tư vấn viên" (đúng vai trò thực tế nhận hoa hồng này).
	CommissionTelesale     *decimal.Decimal ` gorm:"column:commission_telesale;type:decimal(20,2);default:0.00;comment:Hoa hồng cho tư vấn viên đối với khách hàng cũ" json:"commission_telesale"`
	CommissionTelesaleNew  *decimal.Decimal ` gorm:"column:commission_telesale_new;type:decimal(20,2);default:0.00;comment:Hoa hồng cho tư vấn viên đối với khách hàng mới" json:"commission_telesale_new"`
	CommissionTelesaleType *int             ` gorm:"column:commission_telesale_type;default:1;comment:Loại hoa hồng cho tư vấn viên" json:"commission_telesale_type"`
}

func (Service) TableName() string { return "service" }
