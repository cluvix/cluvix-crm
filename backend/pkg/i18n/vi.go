package i18n

var vi = map[string]string{
	// S-4/D-1 — bắt buộc đổi mật khẩu lần đầu (middleware AuthResponseHeaders).
	"password_same_as_old":     "Mật khẩu mới phải khác mật khẩu hiện tại",
	"password_change_required": "Bạn phải đổi mật khẩu trước khi tiếp tục sử dụng hệ thống",
	// Mobile app
	"twofa_setup_required": "Vui lòng thiết lập xác thực 2 lớp trước khi sử dụng",
	"twofa_invalid_code":   "Mã xác thực không đúng",
	"twofa_rate_limited":   "Bạn đã nhập sai mã quá nhiều lần, vui lòng thử lại sau",
	"ip_not_allowed":       "Địa chỉ IP của bạn không được phép truy cập",
	"account_not_active":   "Tài khoản không hoạt động",
	// omnichat-message-actions story-09 — thanh thao tác trên từng tin nhắn.
	"invalid_request": "Yêu cầu không hợp lệ",
	// Common
	"created":                         "Tạo mới thành công",
	"updated":                         "Cập nhật thành công",
	"deleted":                         "Xóa thành công",
	"branch_required_for_appointment": "Vui lòng chọn chi nhánh để tạo lịch hẹn",
	"not_found":                       "Không tồn tại",
	"not_valid":                       "không hợp lệ",
	"forbidden":                       "Bạn không có quyền",
	"server_error":                    "Đã có lỗi xảy ra",
	"no_data":                         "Không có dữ liệu",
	"invalid_data":                    "Dữ liệu không hợp lệ",
	"invalid_status":                  "Trạng thái không hợp lệ",
	"fk_constraint":                   "Dữ liệu đã sử dụng, không thể xoá.",
	"fk_ref_missing":                  "Dữ liệu tham chiếu không tồn tại, vui lòng kiểm tra lại thông tin.",
	"already_exists":                  "đã tồn tại",
	"data_too_long":                   "%s vượt quá độ dài cho phép",
	"version_conflict":                "Dữ liệu đã thay đổi, tải lại trang trước khi cập nhật",

	// Import
	"import_row_error":                 "Lỗi dòng %d: %s",
	"import_no_recognized_column":      "Không nhận diện được cột nào trong file — kiểm tra lại dòng tiêu đề (dòng 1 nhãn, dòng 2 tên cột)",
	"import_duplicate_column":          "Cột %s xuất hiện nhiều lần trong file, vui lòng bỏ bớt để tránh ghi nhầm giá trị",
	"import_column_required_empty":     "Cột %s bắt buộc, không được để trống",
	"import_invalid_number":            "Giá trị không hợp lệ ở cột %s: %s",
	"import_id_old_duplicated_in_file": "Giá trị cột 'id cũ' = %d bị trùng với dòng %d trong file — mỗi id cũ chỉ được dùng một lần cho mỗi thương hiệu",
	"import_id_old_taken":              "Giá trị cột 'id cũ' = %d đã thuộc dịch vụ khác (id=%d, mã %s) — đổi giá trị hoặc bỏ cột 'id cũ' khỏi file",

	// Auth
	"login_failed":         "Thông tin tài khoản hoặc mật khẩu không đúng",
	"login_rate_limit":     "Đăng nhập thất bại quá nhiều lần, vui lòng thử lại sau 5 phút",
	"token_invalid":        "Token không hợp lệ",
	"token_create_error":   "Không thể tạo token",
	"logout_success":       "Đăng xuất thành công",
	"cache_cleared":        "Đã xóa toàn bộ cache",
	"all_sessions_revoked": "Mọi access token đang sống đã bị revoke; refresh tokens đã bị xóa",

	// User
	"user_updated":               "Cập nhật nhân viên thành công",
	"user_not_found":             "Không tìm thấy nhân viên",
	"user_import_root_forbidden": "Không được phép import chỉnh sửa tài khoản quản trị gốc (id=1)",
	"user_import_multi_role":     "Nhân viên này đang có nhiều hơn 1 vai trò — import chỉ chở được 1 vai trò nên sẽ xoá mất phần còn lại. Hãy sửa vai trò trực tiếp trên form nhân sự, hoặc xoá cột Vai trò khỏi file.",
	"user_import_super_target":   "Nhân viên này có vai trò quản trị cấp cao — chỉ quản trị cấp cao mới sửa được. Hãy xoá dòng này khỏi file import.",
	"password_changed":           "Đổi mật khẩu thành công",
	"password_min":               "Mật khẩu tối thiểu 6 ký tự",
	"import_success":             "Import thành công",
	"export_no_data":             "Không có dữ liệu để export",

	// RBAC
	"permission_invalid":        "Danh sách quyền không hợp lệ",
	"permission_grant_not_held": "Bạn chỉ được cấp những quyền chính mình đang có. Bạn không có: %s",
	"permission_not_in_catalog": "Các quyền sau chưa tồn tại trong hệ thống nên KHÔNG lưu được gì: %s. Có thể backend chưa restart sau khi cập nhật danh sách quyền.",

	// Validation
	"required": "%s là bắt buộc",
	"min":      "%s tối thiểu %s ký tự",
	"max":      "%s tối đa %s ký tự",
	"email":    "%s không đúng định dạng email",
	"oneof":    "%s phải là một trong: %s",
	"alphanum": "%s chỉ chứa chữ và số",
	"numeric":  "%s phải là số",
	"gte":      "%s phải >= %s",
	"lte":      "%s phải <= %s",
	"invalid":  "%s không hợp lệ (%s)",
	"unique":   "%s đã tồn tại",

	// Field labels
	"field.username":    "Tài khoản",
	"field.password":    "Mật khẩu",
	"field.status":      "Trạng thái",
	"field.name":        "Tên",
	"field.shortcut":    "Từ khóa gõ nhanh",
	"field.phone":       "Số điện thoại",
	"field.email":       "Email",
	"field.fullname":    "Họ tên",
	"field.id":          "ID",
	"field.code":        "Mã",
	"field.address":     "Địa chỉ",
	"field.birthday":    "Ngày sinh",
	"field.gender":      "Giới tính",
	"field.branch_id":   "Chi nhánh",
	"field.customer_id": "Khách hàng",
	"field.service_id":  "Dịch vụ",
	"field.content":     "Nội dung",
	"field.description": "Mô tả",
	"field.quantity":    "Số lượng",
	"field.unit_price":  "Đơn giá",
	"field.supplier_id": "Nhà cung cấp",
	"field.coupon_code": "Mã coupon",
	"field.template_id": "Template ID",
	"field.slug":        "Slug",
	"field.key":         "Key",
	"field.client_id":   "Client ID",
	"field.color_code":  "Màu hiển thị",

	// Sentinel errors — service layer business rules
	"order_not_found":             "Không tìm thấy đơn hàng",
	"no_payable_service":          "Đơn hàng không còn dịch vụ nào cần thanh toán",
	"order_payment_success":       "Thanh toán đơn hàng thành công",
	"missing_payment_for":         "Thiếu payment_for",
	"invalid_payment_for":         "payment_for không hợp lệ cho khoản thu/chi",
	"missing_parent_id":           "Thiếu parent_id",
	"invalid_service_list":        "Danh sách dịch vụ thanh toán không hợp lệ",
	"amount_must_be_positive":     "Số tiền phải lớn hơn 0",
	"services_multiple_customers": "Các dịch vụ phải thuộc cùng một khách hàng",
	"missing_deposit_id":          "Thiếu deposit_id",
	"refund_exceeds_deposit":      "Số tiền hoàn trả lớn hơn tiền cọc",
	"refund_exceeds_escrow":       "Số tiền hoàn trả lớn hơn tiền ký gửi",
	"missing_service_used_id":     "Thiếu serviceUsed_id",
	"refund_exceeds_paid":         "Số tiền hoàn trả lớn hơn số tiền đã thanh toán",
	"deposit_not_found":           "Không tìm thấy đặt cọc",
	"deposit_customer_mismatch":   "Phiếu cọc hoặc dịch vụ không thuộc khách hàng này",
	"transfer_exceeds_deposit":    "Số tiền cần chuyển lớn hơn tiền cọc",
	"payment_not_found":           "Không tìm thấy thanh toán",
	"cannot_change_transfer_type": "Không thể thay đổi hình thức chuyển cọc",
	"deposit_remaining_negative":  "Tiền cọc còn lại không được nhỏ hơn 0",
	"deposit_used_cannot_update":  "Không cập nhật được đặt cọc đã sử dụng",
	"deposit_used_cannot_delete":  "Không xóa được đặt cọc đã sử dụng",
	"deposit_return_not_found":    "Không tìm thấy hoàn cọc",
	"refund_not_found":            "Không tìm thấy hoàn trả",
	"service_refunded_cannot_pay": "Dịch vụ đã hoàn, không thể thanh toán tiếp",
	"payment_busy_retry":          "Khách hàng đang có thao tác thanh toán khác cùng lúc, vui lòng thử lại",
	"payment_before_order_date":   "Ngày thanh toán không thể trước ngày lên đơn",
	"escrow_not_found":            "Không tìm thấy ký gửi",
	"customer_not_found":          "Không tìm thấy khách hàng",
	"customer_wrong_tenant":       "Khách hàng nằm ngoài phạm vi được phép",
	"service_has_treatment":       "Dịch vụ trong đơn đã được sử dụng, không thể xoá",
	"service_has_payment":         "Dịch vụ trong đơn hàng đã được thanh toán, không thể xoá",
	"service_delete_blocked":      "Không thể xoá đơn hàng, dịch vụ đang liên kết với: %s",
	"quantity_must_be_positive":   "Số lượng dịch vụ phải lớn hơn 0",
	"quantity_required":           "Số lượng dịch vụ là bắt buộc",

	// Request limits
	"payload_too_large":    "Dữ liệu gửi lên quá lớn (tối đa 32MB), vui lòng kiểm tra lại",
	"import_too_many_rows": "File chứa quá nhiều dòng (tối đa %d dòng), vui lòng tách nhỏ file",

	// Specialty / Department type

	// Treatment flow type

	// Business type

	// Config / service
	"import_invalid_flag":     "Cột %s chỉ nhận 0 hoặc 1, nhận được: %s",
	"import_option_not_found": "Cột %s: không tìm thấy \"%s\" trong danh mục của doanh nghiệp này (xem sheet Danh_muc)",
	"import_option_ambiguous": "Cột %s: có nhiều mục cùng tên \"%s\" — điền id để chỉ rõ mục nào",
	"import_invalid_option":   "Cột %s chỉ nhận: %s — nhận được: %s",
	"commission_out_of_range": "Hoa hồng không hợp lệ (âm, hoặc vượt 100% khi tính theo %)",
	"concurrent_request_busy": "Đang có yêu cầu khác xử lý, vui lòng thử lại sau ít giây",

	// Inventory

	// Loyalty

	// Auth
	"cache_not_configured": "Bộ nhớ đệm chưa được cấu hình",

	// Common validation
	"invalid_id":                      "ID không hợp lệ",
	"id_required":                     "Thiếu id",
	"param_id_required":               "Vui lòng truyền tham số id",
	"config_code_parent_table_exists": "Bảng dữ liệu này đã được cấu hình mã cho chi nhánh đã chọn",
	"name_required":                   "Tên không được để trống",
	"file_required":                   "File là bắt buộc",
	"excel_read_failed":               "Không đọc được file Excel: %s",
	"import_file_not_configured":      "Chưa cấu hình file import (biến môi trường IMPORT_CUSTOMER_FILE)",
	"sheet_read_failed":               "Không đọc được sheet: %s",
	"cannot_delete_no_permission":     "Bạn không được phép xóa",
	"version_missing":                 "Thiếu trường version, vui lòng tải lại trang trước khi cập nhật",
	"report_date_range_required":      "Vui lòng chọn khoảng ngày cho báo cáo.",
	"report_date_range_max_one_year":  "Khoảng ngày của báo cáo tối đa 1 năm.",
	"history_not_found":               "Lịch sử không tồn tại",
	"province_id_required":            "province_id là bắt buộc",

	// Customer
	"customer_duplicate":                "Khách bị trùng, không thể thực hiện thao tác này",
	"customer_already_exists":           "Khách này đã tồn tại trên hệ thống rồi.",
	"customer_cross_update_not_allowed": "Không được cập nhật nhân viên phụ trách sang người khác",
	"customer_no_orders":                "Khách hàng không có đơn hàng cần thanh toán",
	"customer_id_required":              "customer_id là bắt buộc",
	"upload_image_invalid":              "Không tồn tại hình ảnh hoặc file upload không hợp lệ",
	"reminder_code_config_error":        "Không tạo được mã nhắc lịch, vui lòng kiểm tra cấu hình mã",

	// S3 / File storage
	"s3_not_configured":     "S3 chưa được cấu hình",
	"image_required":        "Hình ảnh là bắt buộc",
	"image_key_not_allowed": "Ảnh không hợp lệ — chỉ được giữ ảnh hiện tại, xoá ảnh, hoặc tải file ảnh mới lên",

	// Medical record

	// Zalo / News / Game

	// Config
	"default_department_cannot_delete": "Phòng ban mặc định không được phép xóa.",
	"option_not_found":                 "Không tồn tại option này",
	"options_required":                 "options là bắt buộc",
	"assigned_user_not_in_company":     "Người dùng ID %d không hợp lệ hoặc không tồn tại",
	"code_generation_failed":           "Tạo mã dịch vụ thất bại — kiểm tra cấu hình config_code",

	// CRM / Treatment course
	"id_customer_required":      "Vui lòng chọn khách hàng",
	"image_dimension_exceeded":  "Ảnh %dx%d vượt giới hạn cho phép (tối đa %d megapixel, cạnh dài tối đa %d px)",
	"image_heic_not_supported":  "Định dạng HEIC chưa được hỗ trợ, vui lòng đổi sang JPG",
	"missing_id_or_pinned":      "Thiếu id hoặc pinned.",
	"missing_id_customer_param": "Thiếu idCustomer",

	// Auth / RBAC
	"role_name_required":            "roleName là bắt buộc",
	"rbac_name_required":            "name là bắt buộc",
	"rbac_name_exists":              "Tên đã tồn tại",
	"rbac_cannot_delete_admin_role": "Không thể xóa role admin/super",
	"password_confirm_mismatch":     "Mật khẩu xác nhận không khớp",
	"password_old_wrong":            "Mật khẩu cũ không đúng",

	// HR
	"employee_not_found": "Không tồn tại nhân viên này",

	// Marketing

	// Med

	// Inventory

	// Loyalty

	// Service / payment (service layer)
	"cannot_hash_password":               "không thể hash mật khẩu",
	"service_id_invalid":                 "ID dịch vụ không hợp lệ: %d",
	"payment_amount_too_large":           "Số tiền không được lớn hơn số tiền còn thiếu. Nhận: %s, Còn thiếu: %s",
	"total_end_below_payment":            "Không thể giảm giá trị dịch vụ (%s) xuống dưới số tiền đã thanh toán (%s)",
	"service_id_locked_by_payment":       "Không thể đổi dịch vụ vì đã có thanh toán trên dịch vụ này",
	"list_revenues_invalid":              "Danh sách tư vấn viên không hợp lệ (phải là mảng ID người dùng)",
	"order_customer_change_blocked":      "Không thể đổi khách hàng của đơn đã có dịch vụ hoặc phiếu thanh toán",
	"service_total_mismatch":             "Thành tiền dịch vụ không khớp đơn giá, số lượng và chiết khấu (tính lại: %s, gửi lên: %s)",
	"service_change_permission_denied":   "Bạn không có quyền đổi dịch vụ đã thanh toán",
	"customer_status_invalid_transition": "Không thể chuyển trạng thái từ %d sang %d",

	// Customer
	"id_and_tag_required": "id và tag là bắt buộc",
	"ids_invalid":         "ids không hợp lệ",

	// Client (partner / zalo mini-app) — common
	"data_not_found": "Không tìm thấy dữ liệu",

	// Client OAuth

	// Client appointment
	"branch_accepting_customer":       "Trạng thái nhận khách",
	"branch_not_accepting_customer":   "Trạng thái không nhận khách",
	"group_source_marked_online":      "Xác định nhóm nguồn online thành công",
	"group_source_marked_offline":     "Xác định nhóm nguồn khác thành công",
	"note_template_type_appointment":  "Loại lịch hẹn",
	"note_template_type_consultation": "Loại tư vấn",
	"source_other":                    "Nguồn khác",
	"source_unknown":                  "Không xác định",

	// Client config

	// Client customer

	// Client sync (đồng bộ từ hệ thống cũ của đối tác)

	// Client complaint

	// Client customer data

	// Client lead

	// Client news

	// Client zalo game

	// ZNS Campaign

	// Client zalo user

	// HR user contract
	"branch_id_not_allowed":        "Chi nhánh không thuộc phạm vi quyền của bạn",
	"role_not_allowed":             "Bạn không có quyền gán vai trò này (vai trò super, hoặc có quyền bạn không có)",
	"allowed_ips_invalid":          "Danh sách IP/CIDR được phép không hợp lệ",
	"self_allowed_ips_not_allowed": "Bạn không được tự đổi danh sách IP được phép của chính mình",

	// HR salary advance (tạm ứng lương)

	// HR user certificate

	// Service

	// Service combo

	// Print template
	"print_template.err_type_required":  "Loại phiếu (type) là bắt buộc",
	"print_template.err_type_invalid":   "Loại phiếu không hợp lệ",
	"print_template.err_delete_default": "Không thể xóa template mặc định. Hãy đặt template khác làm mặc định trước.",

	// Approval workflow engine — template/rule (story-03)

	// Comp plan validate (story-02, i18n bổ sung story-17)

	// Comp plan assignment (gán công thức lương/hoa hồng — story-03)

	// Comp period — tính kỳ lương/hoa hồng/KPI (story-09)

	// Comp period — chấm KPI + duyệt kỳ (story-10)
	// config/onboarding — thiết lập ban đầu DB SaaS trắng
	"onboarding_already_done":            "Hệ thống đã được thiết lập thương hiệu, vui lòng tải lại trang",
	"onboarding_in_progress":             "Hệ thống đang được thiết lập ở một phiên khác, vui lòng thử lại sau giây lát",
	"onboarding_invalid_payload":         "Dữ liệu thiết lập không hợp lệ",
	"onboarding_company_name_required":   "Vui lòng nhập tên thương hiệu",
	"onboarding_company_code_required":   "Vui lòng nhập mã thương hiệu",
	"onboarding_business_type_invalid":   "Vui lòng chọn loại hình kinh doanh hợp lệ",
	"onboarding_branch_required":         "Vui lòng thiết lập chi nhánh để bắt đầu sử dụng phần mềm",
	"onboarding_branch_name_required":    "Vui lòng nhập tên chi nhánh",
	"onboarding_branch_code_required":    "Vui lòng nhập mã chi nhánh",
	"onboarding_branch_address_required": "Vui lòng nhập địa chỉ chi nhánh",
	// config/business_profile — GET/PUT hồ sơ doanh nghiệp (story-63)
	"business_profile_timezone_required":         "Vui lòng chọn múi giờ",
	"business_profile_timezone_invalid":          "Múi giờ không hợp lệ",
	"business_profile_currency_invalid":          "Đơn vị tiền tệ không hợp lệ",
	"business_profile_allowed_ips_invalid":       "Danh sách IP/CIDR được phép không hợp lệ",
	"business_profile_require_2fa_roles_invalid": "Danh sách vai trò bắt buộc 2FA không hợp lệ",
	"business_profile_totp_trust_days_invalid":   "Số ngày tin thiết bị phải >= 0",
	// crm/attachment — file đính kèm khách hàng
	"attachment_ref_required":        "Thiếu loại đối tượng (ref_type) hoặc mã đối tượng (ref_id)",
	"attachment_ref_id_invalid":      "Mã đối tượng đính kèm (ref_id) không hợp lệ",
	"attachment_ref_type_invalid":    "Loại đối tượng đính kèm (ref_type) không được hỗ trợ",
	"attachment_customer_required":   "Không xác định được khách hàng của đối tượng %s — vui lòng cung cấp customer_id",
	"attachment_customer_id_invalid": "Mã khách hàng (customer_id) không hợp lệ",
	"attachment_file_too_large":      "File vượt quá dung lượng cho phép (tối đa %d MB)",
	"signed_url_expired":             "Liên kết xem file đã hết hạn — vui lòng mở lại file",
	// crm/dashboard + crm/dashboard_layout — bộ lọc kỳ/chi nhánh + bố cục tuỳ biến
	"crm_dashboard_range_invalid":  "Khoảng thời gian không hợp lệ: cần cả ngày bắt đầu và ngày kết thúc, ngày kết thúc phải sau ngày bắt đầu",
	"crm_dashboard_range_too_long": "Khoảng thời gian tối đa 1 năm (366 ngày)",
	"crm_dashboard_branch_invalid": "Chi nhánh không hợp lệ",
	"crm_dashboard_layout_invalid": "Bố cục dashboard không hợp lệ",
	// Group Facebook (bmad/fb-group/thiet-ke.md)
	"fbgroup_ingest_key_invalid":         "Khoá API NextCore không hợp lệ hoặc đã bị thu hồi",
	"fbgroup_ingest_key_created":         "Đã tạo khoá API mới, khoá cũ đã bị thu hồi. Khoá chỉ hiện một lần, hãy dán ngay vào NextCore",
	"fbgroup_ingest_version_unsupported": "Phiên bản dữ liệu NextCore %d chưa được hỗ trợ",
	"fbgroup_ingest_batch_too_large":     "Mỗi lô tối đa %d group",
	"fbgroup_import_file_required":       "Vui lòng chọn tệp CSV xuất từ GROUPCRAWL",
	"fbgroup_import_csv_only":            "Chỉ nhận tệp .csv",
	"fbgroup_import_no_header":           "Không đọc được dòng tiêu đề của tệp CSV",
	"fbgroup_import_missing_column":      "Tệp CSV thiếu cột %s",
	"fbgroup_import_too_many_rows":       "Tệp CSV vượt quá %d dòng",
	"fbgroup_import_done":                "Đã nhập %d group",
	"fbgroup_note":                       "Ghi chú",
	"fbgroup_bulk_limit":                 "Chọn từ 1 tới %d group",
	"fbgroup_topic_not_found":            "Chủ đề không tồn tại",
	"fbgroup_keyword_too_long":           "Từ khoá \"%s\" dài quá %d ký tự",
	"fbgroup_keywords_too_many":          "Tối đa %d từ khoá",
	"fbgroup_topic_name_invalid":         "Tên chủ đề không được trống và tối đa %d ký tự",
	"fbgroup_topic_color_invalid":        "Màu chủ đề không hợp lệ (dạng #RRGGBB)",
	"fbgroup_topic_include_required":     "Cần ít nhất 1 từ khoá gồm",
	"fbgroup_branches_too_many":          "Tối đa %d nhánh",
	"fbgroup_branch_name_invalid":        "Tên nhánh không được trống và tối đa %d ký tự",
	"fbgroup_branch_name_duplicate":      "Nhánh \"%s\" bị trùng tên",
	"fbgroup_branch_keywords_required":   "Nhánh \"%s\" cần ít nhất 1 từ khoá",
	"fbgroup_topic_name_taken":           "Chủ đề \"%s\" đã tồn tại",
	"fbgroup_topic_saved_reclassified":   "Đã lưu chủ đề và phân loại lại %d group",
	"fbgroup_reclassified":               "Đã phân loại lại %d group",
}
