package dao

import (
	"erpCluvix/internal/models"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

type Customer struct {
	// NoStatus: bảng `customer` KHÔNG có cột `status` (bỏ ở story-12 customer-stage-pipeline) —
	// embed BaseModelWithCode sẽ kéo `BaseModel.Status` (`default:1`) vào INSERT ⇒ Unknown column.
	models.BaseModelWithCodeNoStatus
	IdOld *int ` gorm:"column:id_old;uniqueIndex;comment:ID khách hàng cũ" json:"id_old"`
	// Bước tiến trình (customer_stage) — epic customer-stage-pipeline; NOT NULL từ pha 2 (story-12).
	StageId  int    `gorm:"column:stage_id;not null;index;comment:FK → customer_stage" json:"stage_id"`
	Category *int   ` gorm:"index;comment:FK → customer_category (nhóm khách hàng)" json:"category"`
	OldCode  string ` gorm:"column:old_code;size:50;comment:Mã khách hàng từ hệ thống cũ" json:"old_code"`
	Fullname string ` gorm:"size:100" json:"fullname"`
	Nickname string ` gorm:"size:100;comment:Tên khác của khách hàng" json:"nickname"`
	Gender   *int   ` gorm:"comment:Giới tính: 1=Nam, 2=Nữ, 3=Khác" json:"gender"`
	// JSON array ID nhãn khách hàng, VD: [1, 3, 5]
	Tags                          datatypes.JSON `gorm:"type:json" json:"tags"`
	JobId                         *int           ` gorm:"column:job_id;index;comment:FK → job (nghề nghiệp)" json:"job_id"`
	EthnicityId                   *int           ` gorm:"column:ethnicity_id;index;comment:Dân tộc" json:"ethnicity_id"`
	AppointmentNotBookedReasonsId *int           ` gorm:"column:appointment_not_booked_reasons_id;index;comment:Lý do chưa ra hẹn lịch" json:"appointment_not_booked_reasons_id"`
	Birthday                      *string        ` json:"birthday"`
	YearBorn                      *int           ` gorm:"column:year_born" json:"year_born"`
	// JSON array thông tin người thân, VD: [{name, phone, relationship}]
	Relatives         datatypes.JSON   `gorm:"type:json" json:"relatives"`
	Phone             string           ` gorm:"index;size:25" json:"phone"`
	Avatar            string           ` gorm:"size:500" json:"avatar"`
	Facebook          string           ` gorm:"size:255;comment:Facebook người dùng" json:"facebook"`
	Instagram         string           ` gorm:"size:255" json:"instagram"`
	Zalo              string           ` gorm:"size:100" json:"zalo"`
	Email             string           ` gorm:"size:100" json:"email"`
	Viber             string           ` gorm:"size:100" json:"viber"`
	Address           string           ` gorm:"size:500" json:"address"`
	AddressOther      string           ` gorm:"column:address_other;size:500;comment:Địa chỉ khác của khách hàng" json:"address_other"`
	TotalRevenue      *decimal.Decimal ` gorm:"column:totalRevenue;type:decimal(20,2);default:0.00;comment:Doanh số" json:"totalRevenue"`
	TotalPayment      *decimal.Decimal ` gorm:"column:totalPayment;type:decimal(20,2);default:0.00;comment:Doanh thu" json:"totalPayment"`
	TotalDebt         *decimal.Decimal ` gorm:"column:totalDebt;type:decimal(20,2);default:0.00;comment:Công nợ" json:"totalDebt"`
	TotalDeposit      *decimal.Decimal ` gorm:"column:totalDeposit;type:decimal(20,2);default:0.00;comment:Đặt cọc" json:"totalDeposit"`
	TotalBonus        *decimal.Decimal ` gorm:"column:totalBonus;type:decimal(20,2);default:0.00;comment:Khuyến mại" json:"totalBonus"`
	RefundDeposit     *decimal.Decimal ` gorm:"column:refundDeposit;type:decimal(20,2);default:0.00;comment:Hoàn cọc" json:"refundDeposit"`
	RefundService     *decimal.Decimal ` gorm:"column:refundService;type:decimal(20,2);default:0.00;comment:Hoàn dịch vụ" json:"refundService"`
	Consignment       *decimal.Decimal ` gorm:"type:decimal(20,2);default:0.00;comment:Ký gửi" json:"consignment"`
	RefundConsignment *decimal.Decimal ` gorm:"column:refundConsignment;type:decimal(20,2);default:0.00;comment:Hoàn ký gửi" json:"refundConsignment"`
	WardId            *int             ` gorm:"column:ward_id;index" json:"ward_id"`
	ProvinceId        *int             ` gorm:"column:province_id;index" json:"province_id"`
	CountryId         *int             ` gorm:"column:country_id;index" json:"country_id"`
	BranchId          *int             ` gorm:"column:branch_id;index" json:"branch_id"`
	SourceId          *int             ` gorm:"column:source_id;index" json:"source_id"`
	PaymentUserId     *int             ` gorm:"column:payment_user_id;index;comment:FK → user (tư vấn viên, doanh số tính cho người này)" json:"payment_user_id"`
	SalesStaffId      *int             ` gorm:"column:salesStaff_id;index;comment:FK → user (nhân viên kinh doanh)" json:"salesStaff_id"`
	DataId            *int             ` gorm:"column:data_id;comment:1=Dữ liệu mới, 2=Dữ liệu cũ" json:"data_id"`
	ConnectId         *int             ` gorm:"column:connect_id;default:2;comment:1=Đã kết nối, 2=Chưa kết nối (default)" json:"connect_id"`
	CustomerType      *int             ` gorm:"column:customerType;default:1;comment:1=Khách hàng mới (default), 2=Khách hàng cũ" json:"customerType"`
	OtherPhone        string           ` gorm:"column:other_phone;size:25;comment:Số điện thoại khác" json:"other_phone"`
	LoaiMessage       string           ` gorm:"column:loai_message;size:100;comment:Loại message" json:"loai_message"`
	LinkMessage       string           ` gorm:"column:link_message;size:255;comment:Link message" json:"link_message"`
	ChatMessage       string           ` gorm:"column:chat_message;size:255;comment:Chat message" json:"chat_message"`
	Accepted          *int             ` gorm:"default:1;comment:1=Chưa đồng ý (default/khởi tạo), 2=Đã đồng ý" json:"accepted"`
	TypeServiceId     *int             ` gorm:"column:type_service_id;index;comment:Nhóm dịch vụ quan tâm" json:"type_service_id"`
	CreatedDay        *int64           ` gorm:"column:created_day;index;comment:Ngày thu thập data khách hàng (timestamp, khác created_at)" json:"created_day"`
	LastContactDate   *int64           ` gorm:"column:last_contact_date;comment:Liên hệ lần cuối" json:"last_contact_date"`
	LastVisitDate     *int64           ` gorm:"column:last_visit_date;comment:Lần ghé gần nhất" json:"last_visit_date"`

	// BHXH XML (story-13a, Bảng 1 QĐ130) — mã DMDC trực tiếp, nguồn sự thật cho XML (song song
	// ethnicity_id/job_id nội bộ). Nullable, FE story-14 nhập.
	MaDantoc     *string `gorm:"column:ma_dantoc;size:10;comment:MA_DANTOC — mã dân tộc DMDC (XML BHXH)" json:"ma_dantoc"`
	MaNgheNghiep *string `gorm:"column:ma_nghe_nghiep;size:10;comment:MA_NGHE_NGHIEP — mã nghề nghiệp DMDC (XML BHXH)" json:"ma_nghe_nghiep"`
}

func (Customer) TableName() string { return "customer" }

// Attachment — không dùng BaseModel vì table không có status/updated_by/updated_at.
type Attachment struct {
	ID         uint   `gorm:"primaryKey" json:"id" form:"-"`
	CreatedBy  *uint  `gorm:"column:created_by" json:"created_by,omitempty"`
	CreatedAt  int64  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	RefType    string `gorm:"column:ref_type;index;size:50;not null;comment:Loại đối tượng: appointment, customer..." json:"ref_type"`
	RefId      int    `gorm:"column:ref_id;index;not null;comment:ID đối tượng tương ứng với ref_type" json:"ref_id"`
	CustomerID uint   `gorm:"column:customer_id;index;not null;comment:Khách hàng sở hữu file" json:"customer_id"`
	BranchID   *uint  `gorm:"column:branch_id;comment:Chi nhánh" json:"branch_id"`
	FileName   string `gorm:"column:file_name;size:255;not null;comment:Tên file gốc" json:"file_name"`
	FileUrl    string `gorm:"column:file_url;size:500;not null;comment:URL trên cloud (S3/Minio)" json:"file_url"`
	FileType   string `gorm:"column:file_type;size:100;comment:MIME type: image/jpeg, application/pdf..." json:"file_type"`
	FileSize   *int   `gorm:"column:file_size;comment:Kích thước file (bytes)" json:"file_size"`
	Note       string `gorm:"size:500;comment:Ghi chú" json:"note"`
	SortOrder  *int   `gorm:"column:sort_order;default:0;comment:Thứ tự hiển thị" json:"sort_order"`
}

func (Attachment) TableName() string { return "attachment" }
