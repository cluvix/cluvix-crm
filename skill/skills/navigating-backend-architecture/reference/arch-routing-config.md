# Routes Config

## Mục lục

  - Config (internal/modules/webapp/config/router.go) — authed (số resource: `ls internal/modules/webapp/config/`, KHÔNG hardcode)

---


> ⚠ Path viết trước restructure 2026-07-28: `internal/modules/<domain>` đọc là `internal/modules/webapp/<domain>` (CRM standalone chỉ còn nhóm `webapp/`).

> Index tổng + auth groups: `arch-routing.md`. Nguồn: code tại 2026-06-10.

### Config (internal/modules/webapp/config/router.go) — authed (số resource: `ls internal/modules/webapp/config/`, KHÔNG hardcode)

**Path prefix:** `/api/config`. Mỗi resource là 1 subfolder `internal/modules/webapp/config/<resource>/` (route.go + handler/ + dao/).

**CRUD chuẩn (5 routes)** = GET list / GET `:id` / POST / PUT `:id` / DELETE `:id`.
**CRUD+POST-update (6 routes)** = CRUD chuẩn + POST `:id` (UpdateViaPOST, dùng cho FormData).

#### Resources CRUD chuẩn (5 routes mỗi cái)
appointment_is_accepted, appointment_not_booked_reasons, appointment_reason, appointment_time_frame, broker_catagory, broker_type, campain_event, card_type, consent_form_customer, coupon_catagory, customer_care_classification, customer_care_result, customer_care_status, customer_care_type, customer_category, customer_medical_history, customer_pathology, diagnostic, ethnicity, faq, faq_category, group_country, group_local, hr_department, hr_holiday, hr_leave_policy, hr_payroll_config, hr_position, hr_shift, job, location_country, location_province, macro, material_type, note_analysis, note_template, payment_method, service_combo, service_combo_type, service_material, setting, sms_template, sms_type, stock_usage_reasons, supplier, supplier_service, supplier_status, supplier_template, treatment_category, warehouse, warranty_card_status, warranty_card_type, warranty_code, config_code, zalo_zns_template, ai_config, ai_prompt, ai_provider, emr_bed, emr_department, emr_facility, emr_health_department, emr_room.

#### Resources CRUD+POST-update (6 routes mỗi cái)
consent_form, customer_care_channel, customer_care_complaints_type, group_source, location_ward, new_category, service_treatment_step, sms_option.

#### Resources có endpoint đặc biệt
- **BankAccount** (bank_account/) — 6: CRUD + GET `/bank-account/random`.
- **AppointmentType** (appointment_type/) — 7: CRUD + POST `appointment-type-from-zalo-change` + POST `type-revenue-change`.
- **ConfigCallCenter** (config_call_center/) — 13: CRUD + get-call-center-active, get-config-call-center-by-id, get-config-voip24h, get-config-stringe, update-config-call-center-stringee, update-config-call-center-voip24h, active, in-active.
- **ControlFieldInTable** (control_field_in_table/) — 3: GET `:id/get-field-customer`, POST `:id/update-field-customer`, POST `:id/re-build`.
- **Material** (material/) — 7: CRUD + load-by-keyword + POST import-material-from-excel.
- **Options** (options/) — 3: get-options, get-option/:id, PUT update-option/:id.
- **ReferralCard** (referral_card/) — 4: List, Get, Create, Delete.
- **Source** (source/) — 7: CRUD + overview-chart-source + POST is-zalo-source.
- **TreatmentCourseType** (treatment_course_type/) — 8: CRUD + import-from-excel + handle-max-point-change + change-create-labo.
- **ZaloOaConfig** (zalo_oa_config/) — 2: GET get-config-zalo-oa, POST update-config-zalo-oa.
- **Branch** (branch/) — 8: CRUD + UpdateViaPOST + POST status-open-appointment + POST status-open-appointment-zaloapp.
- **ServiceType** (service_type/) — 9: CRUD + UpdateViaPOST + suggestions + bulk-insert + import-from-excel.
- **Unit** (unit/) — 8: CRUD + suggestions + import-from-excel + bulk-insert.
- **Company** (company/) — 6: CRUD + UpdateViaPOST.
- **FacebookPage** (facebook_page/) — 12: CRUD + UpdateViaPOST + get-page + list-user-on-pancake + list-page-facebook-on-pancake + set-page + upload + update-page-facebook-from-pancake.
- **Service** (service/) — 11: CRUD + UpdateViaPOST + load-service-by-type + load-by-keyword + load-list-service-by-keyword + load-service + import-service-from-excel.
- **UserProfile** (user_profile/) — 6: get-user-profile, get-overview-user, GET `:id`, switch-profile, update-user-profile, create-avatar-user.
- **EmrIcd** (emr_icd/) — 9: CRUD + chapters + chapter-detail + all + POST import.
- **DigitalSignature** (digital_signature/) — 2: GET `/digital-signature/config`, POST `/digital-signature/config` (Upsert) — cấu hình ký số CA (pkg/ca), thay thế emr_ca_config cũ.

#### Mount ngoài `/api/config` (handler vẫn thuộc module Config)
- **IndexedDB** (indexeddb/) — GET `/api/crm/indexeddb/sync` (legacy path) — chi tiết: `arch-routing-crm.md`.

#### Notes
- Endpoint `show-hide` (ToggleStatus) đã bị XÓA khỏi toàn bộ BE (grep `show-hide` internal/modules = 0 kết quả) — toggle status giờ đi qua PUT/POST update thường.
- ServiceMaterial import: POST `/service-material/import-service-material` wire trong Material handler (cross-resource).
