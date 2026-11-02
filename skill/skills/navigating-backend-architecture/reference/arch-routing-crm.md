# Routes CRM + CustomerCare

## Mục lục

  - CRM (internal/modules/webapp/crm/router.go) — authed (danh sách resource đầy đủ + số endpoint: xem `router.go`, KHÔNG hardcode)

---


> ⚠ Path viết trước restructure 2026-07-28: `internal/modules/<domain>` đọc là `internal/modules/webapp/<domain>` (CRM standalone chỉ còn nhóm `webapp/`).

> Index tổng + auth groups: `arch-routing.md`. Nguồn: code tại 2026-06-10.

### CRM (internal/modules/webapp/crm/router.go) — authed (danh sách resource đầy đủ + số endpoint: xem `router.go`, KHÔNG hardcode)

**Path prefix:** `/api/crm`. Chưa doc chi tiết trong file này: `attachment` (crm/router.go), `psych_quiz` (→ Xem `skill/skills/working-on-crm/SKILL.md`) — route xem code.

#### Customer (`/customer`)
- GET    `/api/crm/customer` → `ListCustomer` (internal/modules/webapp/crm/customer/handler/customer.go:103).
- GET    `/api/crm/customer/search-global` → `SearchGlobal` (internal/modules/webapp/crm/customer/handler/customer.go:817).
- GET    `/api/crm/customer/load-by-keyword` → `LoadCustomerByKeyword` (internal/modules/webapp/crm/customer/handler/customer.go:857).
- GET    `/api/crm/customer/get-number-customer-by-phone` → `GetNumberCustomerByPhone` (internal/modules/webapp/crm/customer/handler/customer.go:942).
- GET    `/api/crm/customer/for-affiliate` → `ForAffiliate` (internal/modules/webapp/crm/customer/handler/customer.go:984).
- GET    `/api/crm/customer/get-customer-by-code` → `GetCustomerByCode` (internal/modules/webapp/crm/customer/handler/customer.go:163).
- GET    `/api/crm/customer/:id` → `GetCustomer` (internal/modules/webapp/crm/customer/handler/customer.go:179).
- POST   `/api/crm/customer` → `CreateCustomer` (internal/modules/webapp/crm/customer/handler/customer.go:210).
- PUT    `/api/crm/customer/:id` → `UpdateCustomer` (internal/modules/webapp/crm/customer/handler/customer.go:616).
- DELETE `/api/crm/customer/:id` → `DeleteCustomer` (internal/modules/webapp/crm/customer/handler/customer.go:1022).
- POST   `/api/crm/customer/:id/add-tag` → `AddTag` (internal/modules/webapp/crm/customer/handler/customer.go:748).
- POST   `/api/crm/customer/:id/remove-tag` → `RemoveTag` (internal/modules/webapp/crm/customer/handler/customer.go:783).
- POST   `/api/crm/customer/:id/update-macro` → `UpdateMacro` (internal/modules/webapp/crm/customer/handler/customer.go:684).
- POST   `/api/crm/customer/:id/update-appointment-not-booked-reasons` → `UpdateAppointmentNotBookedReasons` (internal/modules/webapp/crm/customer/handler/customer.go:704).
- POST   `/api/crm/customer/:id/create-avatar` → `CreateAvatar` (internal/modules/webapp/crm/customer/handler/customer.go:894).

#### Appointment (`/appointment`)
- GET    `/api/crm/appointment` → `ListAppointment` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-schedule` → `AppointmentSchedule` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-schedule-byhour` → `AppointmentScheduleByHour` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-schedule-bydoctor` → `AppointmentScheduleByDoctor` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-schedule-bydoctor-hour` → `AppointmentScheduleByDoctorHour` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-schedule-byhour-list` → `AppointmentScheduleByHourList` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-schedule-bydoctor-hour-list` → `AppointmentScheduleByDoctorHourList` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-overview` → `AppointmentOverview` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/get-appointment-chart-status` → `GetAppointmentChartStatus` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/appointment-date-in-day` → `AppointmentDateInDay` (internal/modules/webapp/crm/appointment/handler/handler.go).
- GET    `/api/crm/appointment/:id` → `GetAppointment` (internal/modules/webapp/crm/appointment/handler/handler.go).
- POST   `/api/crm/appointment` → `CreateAppointment` (internal/modules/webapp/crm/appointment/handler/handler.go).
- POST   `/api/crm/appointment/customer-checkin` → `CustomerCheckin` (internal/modules/webapp/crm/appointment/handler/handler.go) — inline wrapper with query param.
- PUT    `/api/crm/appointment/:id/update-appointment` → `UpdateAppointment` (internal/modules/webapp/crm/appointment/handler/handler.go).
- PUT    `/api/crm/appointment/:id/customer-checkin` → `CustomerCheckin` (internal/modules/webapp/crm/appointment/handler/handler.go).
- POST   `/api/crm/appointment/:id/add-note` → `AddNote` (internal/modules/webapp/crm/appointment/handler/handler.go).
- PUT    `/api/crm/appointment/:id/update-appointment-cancel` → `UpdateAppointmentCancel` (internal/modules/webapp/crm/appointment/handler/handler.go).
- PUT    `/api/crm/appointment/:id/update-confirm-appointment` → `UpdateConfirmAppointment` (internal/modules/webapp/crm/appointment/handler/handler.go).
- DELETE `/api/crm/appointment/:id` → `DeleteAppointment` (internal/modules/webapp/crm/appointment/handler/handler.go).

#### Order Plan (`/order-plan`)
> Không còn route danh sách đơn (`GET ""`) trong CRM — chỉ chi tiết + tạo/sửa/xoá đơn.
- GET    `/api/crm/order-plan/:id` → `GetOrderPlan` (internal/modules/webapp/crm/order_plan/handler/handler.go).
- POST   `/api/crm/order-plan/create-order` → `CreateOrderPlan` (internal/modules/webapp/crm/order_plan/handler/handler.go).
- PUT    `/api/crm/order-plan/:id/update-order` → `UpdateOrderPlan` (internal/modules/webapp/crm/order_plan/handler/handler.go).
- GET    `/api/crm/order-plan/:id/print-order-plan` → 301 redirect to `/api/print/order-plan/:id`.
- DELETE `/api/crm/order-plan/:id` → `DeleteOrderPlan` (internal/modules/webapp/crm/order_plan/handler/handler.go).

#### Payment (`/payment`)
- GET    `/api/crm/payment/:id` → `GetPayment` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/create-payment-order` → `CreatePaymentOrder` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment` → `CreatePaymentClinic` (internal/modules/webapp/crm/payment/handler/handler.go).
- PUT    `/api/crm/payment/:id` → `UpdatePaymentClinic` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/create-payment-customer` → `CreatePaymentCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/create-deposit-customer` → `CreateDepositCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/create-refund-deposit-customer` → `CreateRefundDepositCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/create-refund-kg-customer` → `CreateRefundKgCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/create-refund-order-plan` → `CreateRefundOrderPlan` (internal/modules/webapp/crm/payment/handler/handler.go).
- POST   `/api/crm/payment/:id/move-deposit-to-payment` → `MoveDepositToPayment` (internal/modules/webapp/crm/payment/handler/handler.go).
- PUT    `/api/crm/payment/:id/move-deposit-to-payment` → `MoveDepositToPayment` (internal/modules/webapp/crm/payment/handler/handler.go) — alias.
- PUT    `/api/crm/payment/:id/update-payment-customer` → `UpdatePaymentCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- PUT    `/api/crm/payment/:id/update-deposit-customer` → `UpdateDepositCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- DELETE `/api/crm/payment/:id/delete-payment-customer` → `DeletePaymentCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- DELETE `/api/crm/payment/:id/delete-deposit-customer` → `DeleteDepositCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- DELETE `/api/crm/payment/:id/delete-refund-customer` → `DeleteRefundCustomer` (internal/modules/webapp/crm/payment/handler/handler.go).
- DELETE `/api/crm/payment/:id/delete-refund-consignment` → `DeleteRefundConsignment` (internal/modules/webapp/crm/payment/handler/handler.go).
- DELETE `/api/crm/payment/:id/delete-refund-order-plan` → `DeleteRefundOrderPlan` (internal/modules/webapp/crm/payment/handler/handler.go).

#### Note (`/note`)
- GET    `/api/crm/note/get-all-note` → `GetAllNote` (internal/modules/webapp/crm/note/handler/handler.go).
- GET    `/api/crm/note/list-advise` → `ListAdvise` (internal/modules/webapp/crm/note/handler/handler.go).
- GET    `/api/crm/note/:id` → `GetNote` (internal/modules/webapp/crm/note/handler/handler.go).
- POST   `/api/crm/note` → `CreateNote` (internal/modules/webapp/crm/note/handler/handler.go).
- POST   `/api/crm/note/:id/pin-note` → `PinNote` (internal/modules/webapp/crm/note/handler/handler.go).
- DELETE `/api/crm/note/:id` → `DeleteNote` (internal/modules/webapp/crm/note/handler/handler.go).

#### Reminder (`/reminder`)
- GET    `/api/crm/reminder` → `ListReminder` (internal/modules/webapp/crm/reminder/handler/handler.go).
- GET    `/api/crm/reminder/reminder-of-customer` → `ReminderOfCustomer` (internal/modules/webapp/crm/reminder/handler/handler.go).
- GET    `/api/crm/reminder/:id` → `GetReminder` (internal/modules/webapp/crm/reminder/handler/handler.go).
- POST   `/api/crm/reminder` → `CreateReminder` (internal/modules/webapp/crm/reminder/handler/handler.go).
- PUT    `/api/crm/reminder/:id` → `UpdateReminder` (internal/modules/webapp/crm/reminder/handler/handler.go).
- DELETE `/api/crm/reminder/:id` → `DeleteReminder` (internal/modules/webapp/crm/reminder/handler/handler.go).

#### Dashboard (`/crm-dashboard`)
- GET    `/api/crm/crm-dashboard/payment-in-month` → `CrmPaymentInMonth` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/quick-starts` → `CrmQuickStarts` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/additional-stats` → `CrmAdditionalStats` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/compare-appointment` → `CrmCompareAppointment` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/appointment-accepted-stats` → `CrmAppointmentAcceptedStats` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/overview-appointment` → `CrmOverviewAppointment` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/overview-online` → `CrmOverviewOnline` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/estimate-price-marketing` → `CrmEstimatePriceMarketing` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/report-statistics` → `CrmReportStatistics` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/customer-status-marketing` → `CrmCustomerStatusMarketing` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/status-customer-overview` → `CrmStatusCustomerOverview` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/report-customer-by-region` → `CrmReportCustomerByRegion` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/marketing-view` → `CrmMarketingView` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/payment-branch-overview` → `CrmPaymentBranchOverview` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/overview-service-type-accepted` → `CrmOverviewServiceTypeAccepted` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/overview-chart-source` → `CrmOverviewChartSource` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/overview-chart-event` → `CrmOverviewChartEvent` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/overview-service-type-interest` → `CrmOverviewServiceTypeInterest` (internal/modules/webapp/crm/dashboard/handler/handler.go).
- GET    `/api/crm/crm-dashboard/service-used-statistics` → `CrmServiceUsedStatistics` (internal/modules/webapp/crm/dashboard/handler/handler.go).

#### Report (`/report`)
- GET    `/api/crm/report/appointment-statistics` → `AppointmentStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/branch-statistics` → `BranchStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/source-statistics` → `SourceStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/group-source-statistics` → `GroupSourceStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/pathology-statistics` → `PathologyStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/province-statistics` → `ProvinceStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/country-statistics` → `CountryStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/category-statistics` → `CategoryStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/page-facebook-statistics` → `PageFacebookStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/campain-event-statistics` → `CampainEventStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/interested-statistics` → `InterestedStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/service-type-statistics` → `ServiceTypeStatistics` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/revenue` → `Revenue` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/telesales-online` → `TelesalesOnline` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/doctor-advise` → `DoctorAdvise` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/direct-sale-advise` → `DirectSaleAdvise` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/effective-advise` → `EffectiveAdvise` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/report/compare-appointment` → `CrmCompareAppointment` (internal/modules/webapp/crm/dashboard/handler/handler.go) — wired separately in router.go.
- GET    `/api/crm/report/treatment-reexamination-guarantee-appointment` → `CrmTreatmentReexaminationGuaranteeAppointment` (internal/modules/webapp/crm/dashboard/handler/handler.go) — wired separately in router.go.

#### Cross-Cutting Detail Endpoints (customer_detail carve-outs)
- GET    `/api/crm/appointment/get-by-customer` → `GetAppointmentByCustomer` (internal/modules/webapp/crm/customer_detail/handler/handler.go).
- GET    `/api/crm/payment/get-payment-detail-for-customer` → `GetPaymentDetailForCustomer` (internal/modules/webapp/crm/customer_detail/handler/handler.go).
- GET    `/api/crm/treatment-services/list-service-used-customer` → `ListServiceUsedCustomer` (internal/modules/webapp/crm/customer_detail/handler/handler.go).
- GET    `/api/crm/treatment-services/report-service-used` → `ReportServiceUsed` (internal/modules/webapp/crm/report/handler/handler.go).
- GET    `/api/crm/treatment-course/get-treatment-course-of-customer` → `GetTreatmentCourseOfCustomer` (internal/modules/webapp/crm/customer_detail/handler/handler.go).
- POST   `/api/crm/treatment-course/list-folder` → `ListFolder` (internal/modules/webapp/crm/customer_detail/handler/handler.go).
- GET    `/api/crm/customer/:id/timeline-customer` → `TimelineCustomer` (internal/modules/webapp/crm/customer_detail/handler/handler.go).
- GET    `/api/crm/customer-care` → `ListCustomerCare` (internal/modules/webapp/crm/customer_detail/handler/handler.go).

#### Utility Endpoints
- GET    `/api/crm/exchange-rate/currency` → `ExchangeRateCurrency` (internal/modules/webapp/crm/exchange_rate/handler/handler.go).
- POST   `/api/crm/tinymce/upload-image` → `TinymceUploadImage` (internal/modules/webapp/crm/tinymce/handler/handler.go).

#### IndexedDB Sync (config module, CRM path)
- GET    `/api/crm/indexeddb/sync` → `Sync` (internal/modules/webapp/config/indexeddb/handler/handler.go) — URL giữ nguyên /crm/indexeddb/sync (backward compat) nhưng module ở config/indexeddb/.

---
