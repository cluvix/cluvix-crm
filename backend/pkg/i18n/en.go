package i18n

var en = map[string]string{
	// S-4/D-1 — bắt buộc đổi mật khẩu lần đầu (middleware AuthResponseHeaders).
	"password_same_as_old":     "New password must be different from the current password",
	"password_change_required": "You must change your password before continuing",
	// Mobile app
	"twofa_setup_required": "Please set up two-factor authentication before continuing",
	"twofa_invalid_code":   "Invalid verification code",
	"twofa_rate_limited":   "Too many incorrect attempts, please try again later",
	"ip_not_allowed":       "Your IP address is not allowed",
	"account_not_active":   "Account is not active",
	// omnichat-message-actions story-09 — per-message action bar.
	"invalid_request": "Invalid request",
	// Common
	"created":                         "Created successfully",
	"branch_required_for_appointment": "Please select a branch to create an appointment",
	"updated":                         "Updated successfully",
	"deleted":                         "Deleted successfully",
	"not_found":                       "Not found",
	"not_valid":                       "is invalid",
	"forbidden":                       "Access denied",
	"server_error":                    "Internal server error",
	"no_data":                         "No data",
	"invalid_data":                    "Invalid data",
	"invalid_status":                  "Invalid status",
	"fk_constraint":                   "Data is in use, cannot delete.",
	"fk_ref_missing":                  "Referenced data does not exist, please check your input.",
	"already_exists":                  "already exists",
	"data_too_long":                   "%s exceeds maximum length",
	"version_conflict":                "Data has changed, please reload before updating",

	// Import
	"import_row_error":                 "Error at row %d: %s",
	"import_no_recognized_column":      "No recognizable column in the file — check the header rows (row 1 labels, row 2 field names)",
	"import_duplicate_column":          "Column %s appears more than once; remove the duplicate to avoid writing the wrong value",
	"import_column_required_empty":     "Column %s is required and cannot be empty",
	"import_invalid_number":            "Invalid value in column %s: %s",
	"import_id_old_duplicated_in_file": "Old id %d in column 'id cũ' duplicates row %d in the file — each old id may be used once per brand",
	"import_id_old_taken":              "Old id %d in column 'id cũ' already belongs to another service (id=%d, code %s) — change it or remove the 'id cũ' column from the file",

	// Auth
	"login_failed":         "Invalid username or password",
	"login_rate_limit":     "Too many failed login attempts, please try again after 5 minutes",
	"token_invalid":        "Invalid token",
	"token_create_error":   "Cannot create token",
	"logout_success":       "Logged out successfully",
	"cache_cleared":        "Cache cleared",
	"all_sessions_revoked": "All active tokens revoked; refresh tokens deleted",

	// User
	"user_updated":               "User updated successfully",
	"user_not_found":             "User not found",
	"user_import_root_forbidden": "Importing changes to the root admin account (id=1) is not allowed",
	"user_import_multi_role":     "This staff member currently has more than one role — the import file carries only one, so the rest would be deleted. Edit the roles on the staff form, or remove the Role column from the file.",
	"user_import_super_target":   "This staff member has a super administrator role — only a super administrator can edit them. Remove this row from the import file.",
	"password_changed":           "Password changed successfully",
	"password_min":               "Password must be at least 6 characters",
	"import_success":             "Import successful",
	"export_no_data":             "No data to export",

	// RBAC
	"permission_invalid":        "Invalid permission list",
	"permission_grant_not_held": "You can only grant permissions you hold yourself. Not held: %s",
	"permission_not_in_catalog": "These permissions do not exist in the system yet, nothing was saved: %s. The backend may not have been restarted after the permission list was updated.",

	// Validation
	"required": "%s is required",
	"min":      "%s must be at least %s characters",
	"max":      "%s must be at most %s characters",
	"email":    "%s must be a valid email",
	"oneof":    "%s must be one of: %s",
	"alphanum": "%s must contain only letters and numbers",
	"numeric":  "%s must be a number",
	"gte":      "%s must be >= %s",
	"lte":      "%s must be <= %s",
	"invalid":  "%s is invalid (%s)",
	"unique":   "%s already exists",

	// Field labels
	"field.username":    "Username",
	"field.password":    "Password",
	"field.status":      "Status",
	"field.name":        "Name",
	"field.shortcut":    "Shortcut",
	"field.phone":       "Phone",
	"field.email":       "Email",
	"field.fullname":    "Full name",
	"field.id":          "ID",
	"field.code":        "Code",
	"field.address":     "Address",
	"field.birthday":    "Birthday",
	"field.gender":      "Gender",
	"field.branch_id":   "Branch",
	"field.customer_id": "Customer",
	"field.service_id":  "Service",
	"field.content":     "Content",
	"field.description": "Description",
	"field.quantity":    "Quantity",
	"field.unit_price":  "Unit price",
	"field.supplier_id": "Supplier",
	"field.coupon_code": "Coupon code",
	"field.template_id": "Template ID",
	"field.slug":        "Slug",
	"field.key":         "Key",
	"field.client_id":   "Client ID",
	"field.color_code":  "Display color",

	// Sentinel errors — service layer business rules
	"order_not_found":             "Order not found",
	"no_payable_service":          "This order has no service left to pay",
	"order_payment_success":       "Order payment successful",
	"missing_payment_for":         "Missing payment_for",
	"invalid_payment_for":         "Invalid payment_for for this payment",
	"missing_parent_id":           "Missing parent_id",
	"invalid_service_list":        "Invalid payment service list",
	"amount_must_be_positive":     "Amount must be greater than 0",
	"services_multiple_customers": "Services must belong to the same customer",
	"missing_deposit_id":          "Missing deposit_id",
	"refund_exceeds_deposit":      "Refund amount exceeds deposit",
	"refund_exceeds_escrow":       "Refund amount exceeds escrow",
	"missing_service_used_id":     "Missing serviceUsed_id",
	"refund_exceeds_paid":         "Refund amount exceeds amount paid",
	"deposit_not_found":           "Deposit not found",
	"deposit_customer_mismatch":   "The deposit or service does not belong to this customer",
	"transfer_exceeds_deposit":    "Transfer amount exceeds deposit",
	"payment_not_found":           "Payment not found",
	"cannot_change_transfer_type": "Cannot change deposit transfer type",
	"deposit_remaining_negative":  "Remaining deposit cannot be negative",
	"deposit_used_cannot_update":  "Cannot update a used deposit",
	"deposit_used_cannot_delete":  "Cannot delete a used deposit",
	"deposit_return_not_found":    "Deposit return not found",
	"refund_not_found":            "Refund not found",
	"service_refunded_cannot_pay": "Service has been refunded, cannot continue payment",
	"payment_busy_retry":          "Another payment for this customer is being processed, please try again",
	"payment_before_order_date":   "Payment date cannot be before the order date",
	"escrow_not_found":            "Escrow not found",
	"customer_not_found":          "Customer not found",
	"customer_wrong_tenant":       "Customer is outside your allowed scope",
	"service_has_treatment":       "Service has already been used, cannot delete",
	"service_has_payment":         "Service has been paid, cannot delete",
	"service_delete_blocked":      "Cannot delete order, service is linked to: %s",
	"quantity_must_be_positive":   "Service quantity must be greater than 0",
	"quantity_required":           "Service quantity is required",

	// Business type

	// Flow

	// Specialty

	// Request limits
	"payload_too_large":    "Payload too large (max 32MB), please check your request",
	"import_too_many_rows": "Import file has too many rows (max %d), please split the file",

	// Config / service
	"import_invalid_flag":     "Column %s only accepts 0 or 1, got: %s",
	"import_option_not_found": "Column %s: \"%s\" not found in the catalog (see the Danh_muc sheet)",
	"import_option_ambiguous": "Column %s: multiple entries named \"%s\" — enter the id to disambiguate",
	"import_invalid_option":   "Column %s only accepts: %s — got: %s",
	"commission_out_of_range": "Invalid commission (negative, or over 100% when type is percent)",
	"concurrent_request_busy": "Another request is in progress, please try again shortly",

	// Inventory

	// Loyalty

	// Auth
	"cache_not_configured": "Cache is not configured",

	// Common validation
	"invalid_id":                      "Invalid ID",
	"id_required":                     "Missing id",
	"param_id_required":               "Please provide id parameter",
	"config_code_parent_table_exists": "This data table already has a code configuration for the selected branch",
	"name_required":                   "Name is required",
	"file_required":                   "File is required",
	"excel_read_failed":               "Cannot read Excel file: %s",
	"import_file_not_configured":      "Import file is not configured (IMPORT_CUSTOMER_FILE environment variable)",
	"sheet_read_failed":               "Cannot read sheet: %s",
	"cannot_delete_no_permission":     "You do not have permission to delete",
	"version_missing":                 "Missing version field, please reload the page before updating",
	"report_date_range_required":      "Please select a date range for the report.",
	"report_date_range_max_one_year":  "The report date range must not exceed 1 year.",
	"history_not_found":               "History not found",
	"province_id_required":            "province_id is required",

	// Customer
	"customer_duplicate":                "Duplicate customer, cannot perform this action",
	"customer_already_exists":           "This customer already exists in the system.",
	"customer_cross_update_not_allowed": "Cannot reassign customer to another staff in charge",
	"customer_no_orders":                "Customer has no pending orders",
	"customer_id_required":              "customer_id is required",
	"upload_image_invalid":              "Image not found or invalid file upload",
	"reminder_code_config_error":        "Cannot create reminder code, please check code configuration",

	// S3 / File storage
	"s3_not_configured":     "S3 is not configured",
	"image_required":        "Image is required",
	"image_key_not_allowed": "Invalid image — you can only keep the current image, remove it, or upload a new image file",

	// Medical record

	// Zalo / News / Game

	// Config
	"default_department_cannot_delete": "Default department cannot be deleted.",
	"option_not_found":                 "Option not found",
	"options_required":                 "options is required",
	"assigned_user_not_in_company":     "User ID %d is invalid or does not exist",
	"code_generation_failed":           "Failed to generate service code — check config_code setup",

	// CRM / Treatment course
	"id_customer_required":      "Please select a customer",
	"image_dimension_exceeded":  "Image %dx%d exceeds the allowed limit (max %d megapixels, max %d px per side)",
	"image_heic_not_supported":  "HEIC format is not supported, please convert to JPG",
	"missing_id_or_pinned":      "Missing id or pinned.",
	"missing_id_customer_param": "Missing idCustomer",

	// Auth / RBAC
	"role_name_required":            "roleName is required",
	"rbac_name_required":            "name is required",
	"rbac_name_exists":              "Name already exists",
	"rbac_cannot_delete_admin_role": "Cannot delete the admin/super role",
	"password_confirm_mismatch":     "Password confirmation does not match",
	"password_old_wrong":            "Old password is incorrect",

	// HR
	"employee_not_found": "Employee not found",

	// Marketing

	// Med

	// Inventory

	// Loyalty

	// Service / payment (service layer)
	"cannot_hash_password":               "cannot hash password",
	"service_id_invalid":                 "Invalid service ID: %d",
	"payment_amount_too_large":           "Amount cannot exceed remaining amount. Received: %s, Remaining: %s",
	"total_end_below_payment":            "Cannot reduce service value (%s) below amount already paid (%s)",
	"service_id_locked_by_payment":       "Cannot change service — this service already has payment recorded",
	"list_revenues_invalid":              "Invalid consultant list (must be an array of user IDs)",
	"order_customer_change_blocked":      "Cannot change the customer of an order that already has services or payments",
	"service_total_mismatch":             "Service amount does not match unit price, quantity and discounts (expected: %s, received: %s)",
	"service_change_permission_denied":   "You do not have permission to change a paid service",
	"customer_status_invalid_transition": "Cannot transition status from %d to %d",

	// Customer
	"id_and_tag_required": "id and tag are required",
	"ids_invalid":         "ids are invalid",

	// Client (partner / zalo mini-app) — common
	"data_not_found": "Data not found",

	// Client OAuth

	// Client appointment
	"branch_accepting_customer":       "Now accepting customers",
	"branch_not_accepting_customer":   "Not accepting customers",
	"group_source_marked_online":      "Group source marked as online successfully",
	"group_source_marked_offline":     "Group source marked as offline successfully",
	"note_template_type_appointment":  "Appointment type",
	"note_template_type_consultation": "Consultation type",
	"source_other":                    "Other source",
	"source_unknown":                  "Unknown",

	// Client config

	// Client customer

	// Client sync (sync from partner's legacy system)

	// Client complaint

	// Client customer data

	// Client lead

	// Client news

	// Client zalo game

	// Client zalo user

	// ZNS Campaign

	// HR user contract
	"branch_id_not_allowed":        "Branch is not within your scope",
	"role_not_allowed":             "You are not allowed to assign this role (super role, or it has permissions you do not hold)",
	"allowed_ips_invalid":          "Invalid allowed IP/CIDR list",
	"self_allowed_ips_not_allowed": "You cannot change your own allowed IP list",

	// HR salary advance

	// HR user certificate

	// Service

	// Service combo

	// Print template
	"print_template.err_type_required":  "Template type is required",
	"print_template.err_type_invalid":   "Invalid template type",
	"print_template.err_delete_default": "Cannot delete default template. Set another template as default first.",

	// Approval workflow engine — template/rule (story-03)

	// Comp plan validate (story-02, i18n added story-17)

	// Comp plan assignment (story-03)

	// Comp period — payroll/commission/KPI period calculation (story-09)

	// Comp period — KPI scoring + period approval (story-10)
	// config/onboarding — initial setup for a blank SaaS database
	"onboarding_already_done":            "The system already has a brand set up, please reload the page",
	"onboarding_in_progress":             "The system is being set up in another session, please try again shortly",
	"onboarding_invalid_payload":         "Invalid setup data",
	"onboarding_company_name_required":   "Please enter the brand name",
	"onboarding_company_code_required":   "Please enter the brand code",
	"onboarding_business_type_invalid":   "Please select a valid business type",
	"onboarding_branch_required":         "Please set up a branch to start using the software",
	"onboarding_branch_name_required":    "Please enter the branch name",
	"onboarding_branch_code_required":    "Please enter the branch code",
	"onboarding_branch_address_required": "Please enter the branch address",
	// config/business_profile — GET/PUT hồ sơ doanh nghiệp (story-63)
	"business_profile_timezone_required":         "Please select a timezone",
	"business_profile_timezone_invalid":          "Invalid timezone",
	"business_profile_currency_invalid":          "Invalid currency",
	"business_profile_allowed_ips_invalid":       "Invalid allowed IP/CIDR list",
	"business_profile_require_2fa_roles_invalid": "Invalid required-2FA role list",
	"business_profile_totp_trust_days_invalid":   "Trust-device days must be >= 0",
	// crm/attachment — customer attachments
	"attachment_ref_required":        "ref_type and ref_id are required",
	"attachment_ref_id_invalid":      "Invalid attachment ref_id",
	"attachment_ref_type_invalid":    "Unsupported attachment ref_type",
	"attachment_customer_required":   "Cannot determine the customer of %s — please provide customer_id",
	"attachment_customer_id_invalid": "Invalid customer_id",
	"attachment_file_too_large":      "File exceeds the allowed size (max %d MB)",
	"signed_url_expired":             "The file link has expired — please open the file again",
	// crm/dashboard + crm/dashboard_layout — period/branch filter + customizable layout
	"crm_dashboard_range_invalid":  "Invalid period: both start and end dates are required and the end must be after the start",
	"crm_dashboard_range_too_long": "The period can be at most 1 year (366 days)",
	"crm_dashboard_branch_invalid": "Invalid branch",
	"crm_dashboard_layout_invalid": "Invalid dashboard layout",
	// Facebook groups (bmad/fb-group/thiet-ke.md)
	"fbgroup_ingest_key_invalid":         "The NextCore API key is invalid or has been revoked",
	"fbgroup_ingest_key_created":         "A new API key was created and the old one revoked. The key is shown only once, paste it into NextCore now",
	"fbgroup_ingest_version_unsupported": "NextCore payload version %d is not supported",
	"fbgroup_ingest_batch_too_large":     "At most %d groups per batch",
	"fbgroup_import_file_required":       "Please choose the CSV file exported from GROUPCRAWL",
	"fbgroup_import_csv_only":            "Only .csv files are accepted",
	"fbgroup_import_no_header":           "Cannot read the CSV header row",
	"fbgroup_import_missing_column":      "The CSV file is missing column %s",
	"fbgroup_import_too_many_rows":       "The CSV file exceeds %d rows",
	"fbgroup_import_done":                "Imported %d groups",
	"fbgroup_note":                       "Note",
	"fbgroup_bulk_limit":                 "Select between 1 and %d groups",
	"fbgroup_topic_not_found":            "Topic not found",
	"fbgroup_keyword_too_long":           "Keyword \"%s\" is longer than %d characters",
	"fbgroup_keywords_too_many":          "At most %d keywords",
	"fbgroup_topic_name_invalid":         "Topic name is required and at most %d characters",
	"fbgroup_topic_color_invalid":        "Invalid topic color (use #RRGGBB)",
	"fbgroup_topic_include_required":     "At least 1 include keyword is required",
	"fbgroup_branches_too_many":          "At most %d branches",
	"fbgroup_branch_name_invalid":        "Branch name is required and at most %d characters",
	"fbgroup_branch_name_duplicate":      "Branch \"%s\" is duplicated",
	"fbgroup_branch_keywords_required":   "Branch \"%s\" needs at least 1 keyword",
	"fbgroup_topic_name_taken":           "Topic \"%s\" already exists",
	"fbgroup_topic_saved_reclassified":   "Topic saved and %d groups reclassified",
	"fbgroup_reclassified":               "Reclassified %d groups",
}
