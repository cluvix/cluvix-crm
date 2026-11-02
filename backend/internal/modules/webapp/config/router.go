package config

import (
	"erpCluvix/internal/deps"
	// Plan 06 B1 — migrated modules:
	appointmentIsAccepted "erpCluvix/internal/modules/webapp/config/appointment_is_accepted"
	appointmentIsAcceptedHandler "erpCluvix/internal/modules/webapp/config/appointment_is_accepted/handler"
	appointmentNotBookedReasons "erpCluvix/internal/modules/webapp/config/appointment_not_booked_reasons"
	appointmentNotBookedReasonsHandler "erpCluvix/internal/modules/webapp/config/appointment_not_booked_reasons/handler"
	appointmentReason "erpCluvix/internal/modules/webapp/config/appointment_reason"
	appointmentReasonHandler "erpCluvix/internal/modules/webapp/config/appointment_reason/handler"
	bankAccount "erpCluvix/internal/modules/webapp/config/bank_account"
	bankAccountHandler "erpCluvix/internal/modules/webapp/config/bank_account/handler"
	businessProfile "erpCluvix/internal/modules/webapp/config/business_profile"
	businessProfileHandler "erpCluvix/internal/modules/webapp/config/business_profile/handler"
	cardType "erpCluvix/internal/modules/webapp/config/card_type"
	cardTypeHandler "erpCluvix/internal/modules/webapp/config/card_type/handler"
	printTemplate "erpCluvix/internal/modules/webapp/config/print_template"
	printTemplateHandler "erpCluvix/internal/modules/webapp/config/print_template/handler"

	// Plan 06 B2:
	customerCategory "erpCluvix/internal/modules/webapp/config/customer_category"
	customerCategoryHandler "erpCluvix/internal/modules/webapp/config/customer_category/handler"
	customerStage "erpCluvix/internal/modules/webapp/config/customer_stage"
	customerStageHandler "erpCluvix/internal/modules/webapp/config/customer_stage/handler"

	// Plan 06 B3:
	ethnicity "erpCluvix/internal/modules/webapp/config/ethnicity"
	ethnicityHandler "erpCluvix/internal/modules/webapp/config/ethnicity/handler"
	groupCountry "erpCluvix/internal/modules/webapp/config/group_country"
	groupCountryHandler "erpCluvix/internal/modules/webapp/config/group_country/handler"
	groupLocal "erpCluvix/internal/modules/webapp/config/group_local"
	groupLocalHandler "erpCluvix/internal/modules/webapp/config/group_local/handler"
	groupSource "erpCluvix/internal/modules/webapp/config/group_source"
	groupSourceHandler "erpCluvix/internal/modules/webapp/config/group_source/handler"
	hrDepartment "erpCluvix/internal/modules/webapp/config/hr_department"
	hrDepartmentHandler "erpCluvix/internal/modules/webapp/config/hr_department/handler"
	job "erpCluvix/internal/modules/webapp/config/job"
	jobHandler "erpCluvix/internal/modules/webapp/config/job/handler"
	locationCountry "erpCluvix/internal/modules/webapp/config/location_country"
	locationCountryHandler "erpCluvix/internal/modules/webapp/config/location_country/handler"

	// Plan 06 B4:
	locationProvince "erpCluvix/internal/modules/webapp/config/location_province"
	locationProvinceHandler "erpCluvix/internal/modules/webapp/config/location_province/handler"
	locationWard "erpCluvix/internal/modules/webapp/config/location_ward"
	locationWardHandler "erpCluvix/internal/modules/webapp/config/location_ward/handler"
	noteTemplate "erpCluvix/internal/modules/webapp/config/note_template"
	noteTemplateHandler "erpCluvix/internal/modules/webapp/config/note_template/handler"
	paymentMethod "erpCluvix/internal/modules/webapp/config/payment_method"
	paymentMethodHandler "erpCluvix/internal/modules/webapp/config/payment_method/handler"
	setting "erpCluvix/internal/modules/webapp/config/setting"
	settingHandler "erpCluvix/internal/modules/webapp/config/setting/handler"

	// Plan 06 B5:
	appointmentType "erpCluvix/internal/modules/webapp/config/appointment_type"
	appointmentTypeHandler "erpCluvix/internal/modules/webapp/config/appointment_type/handler"
	configCode "erpCluvix/internal/modules/webapp/config/config_code"
	configCodeHandler "erpCluvix/internal/modules/webapp/config/config_code/handler"
	controlFieldInTable "erpCluvix/internal/modules/webapp/config/control_field_in_table"
	controlFieldInTableHandler "erpCluvix/internal/modules/webapp/config/control_field_in_table/handler"
	options "erpCluvix/internal/modules/webapp/config/options"
	optionsHandler "erpCluvix/internal/modules/webapp/config/options/handler"
	source "erpCluvix/internal/modules/webapp/config/source"
	sourceHandler "erpCluvix/internal/modules/webapp/config/source/handler"

	// Plan 06 B6+B7:
	branch "erpCluvix/internal/modules/webapp/config/branch"
	branchHandler "erpCluvix/internal/modules/webapp/config/branch/handler"
	service "erpCluvix/internal/modules/webapp/config/service"
	serviceHandler "erpCluvix/internal/modules/webapp/config/service/handler"
	serviceType "erpCluvix/internal/modules/webapp/config/service_type"
	serviceTypeHandler "erpCluvix/internal/modules/webapp/config/service_type/handler"
	unit "erpCluvix/internal/modules/webapp/config/unit"
	unitHandler "erpCluvix/internal/modules/webapp/config/unit/handler"
	userProfile "erpCluvix/internal/modules/webapp/config/user_profile"
	userProfileHandler "erpCluvix/internal/modules/webapp/config/user_profile/handler"

	// Onboarding — màn thiết lập ban đầu DB SaaS trắng (epic onboarding-setup-wizard).
	onboarding "erpCluvix/internal/modules/webapp/config/onboarding"
	onboardingHandler "erpCluvix/internal/modules/webapp/config/onboarding/handler"

	"github.com/gin-gonic/gin"
)

func Register(authed *gin.RouterGroup, d deps.Deps) {
	// Plan 06 finalized — handlers/config/ flat package chỉ còn helper stubs.
	cfg := authed.Group("/config")
	{
		// Ethnicity — Plan 06 B3: layered module.
		ethnicity.RegisterRoutes(cfg, ethnicityHandler.Deps{DB: d.DB})

		// Onboarding — thiết lập ban đầu DB SaaS trắng. Mọi user đã đăng nhập đều tới được (story-58
		// xoá gate chọn thương hiệu); handler tự kiểm quyền.
		onboarding.RegisterRoutes(cfg, onboardingHandler.Deps{DB: d.DB, Store: d.Store, Cache: d.Cache})

		// Branch — Plan 06 B6.
		branch.RegisterRoutes(cfg, branchHandler.Deps{DB: d.DB, Cache: d.Cache})

		// BusinessProfile — màn "Hồ sơ doanh nghiệp" (story-63, D22/D23): thay CRUD company đã xoá
		// (story-62) bằng API 1 bản ghi.
		businessProfile.RegisterRoutes(cfg, businessProfileHandler.Deps{DB: d.DB, Store: d.Store, Cache: d.Cache})

		// AppointmentIsAccepted — Plan 06 B1: layered module.
		appointmentIsAccepted.RegisterRoutes(cfg, appointmentIsAcceptedHandler.Deps{DB: d.DB})

		// AppointmentNotBookedReasons — Plan 06 B1: layered module.
		appointmentNotBookedReasons.RegisterRoutes(cfg, appointmentNotBookedReasonsHandler.Deps{DB: d.DB})

		// AppointmentReason — Plan 06 B1: layered module.
		appointmentReason.RegisterRoutes(cfg, appointmentReasonHandler.Deps{DB: d.DB})

		// AppointmentType — Plan 06 B5.
		appointmentType.RegisterRoutes(cfg, appointmentTypeHandler.Deps{DB: d.DB, Cache: d.Cache})

		// CardType — Plan 06 B1: layered module.
		cardType.RegisterRoutes(cfg, cardTypeHandler.Deps{DB: d.DB})

		// ControlFieldInTable — Plan 06 B5.
		controlFieldInTable.RegisterRoutes(cfg, controlFieldInTableHandler.Deps{DB: d.DB, Cache: d.Cache})

		// CustomerCategory — Plan 06 B2: layered module.
		customerCategory.RegisterRoutes(cfg, customerCategoryHandler.Deps{DB: d.DB})

		// CustomerStage — danh mục bước tiến trình khách hàng (epic customer-stage-pipeline).
		customerStage.RegisterRoutes(cfg, customerStageHandler.Deps{DB: d.DB})

		// GroupCountry — Plan 06 B3: layered module.
		groupCountry.RegisterRoutes(cfg, groupCountryHandler.Deps{DB: d.DB})

		// GroupLocal — Plan 06 B3: layered module.
		groupLocal.RegisterRoutes(cfg, groupLocalHandler.Deps{DB: d.DB})

		// GroupSource — Plan 06 B3: layered module.
		groupSource.RegisterRoutes(cfg, groupSourceHandler.Deps{DB: d.DB})

		// HrDepartment — Plan 06 B3: layered module.
		hrDepartment.RegisterRoutes(cfg, hrDepartmentHandler.Deps{DB: d.DB})

		// Job — Plan 06 B3: layered module.
		job.RegisterRoutes(cfg, jobHandler.Deps{DB: d.DB})

		// LocationCountry — Plan 06 B3: layered module.
		locationCountry.RegisterRoutes(cfg, locationCountryHandler.Deps{DB: d.DB})

		// LocationProvince — Plan 06 B4: layered module.
		locationProvince.RegisterRoutes(cfg, locationProvinceHandler.Deps{DB: d.DB})

		// LocationWard — Plan 06 B4: layered module.
		locationWard.RegisterRoutes(cfg, locationWardHandler.Deps{DB: d.DB})

		// NoteTemplate — Plan 06 B4: layered module.
		noteTemplate.RegisterRoutes(cfg, noteTemplateHandler.Deps{DB: d.DB})

		// PaymentMethod — Plan 06 B4: layered module.
		paymentMethod.RegisterRoutes(cfg, paymentMethodHandler.Deps{DB: d.DB})

		// ServiceType — Plan 06 B6.
		serviceType.RegisterRoutes(cfg, serviceTypeHandler.Deps{DB: d.DB, Cache: d.Cache})

		// Setting — Plan 06 B4: layered module.
		setting.RegisterRoutes(cfg, settingHandler.Deps{DB: d.DB})

		// UserProfile — Plan 06 B6.
		userProfile.RegisterRoutes(cfg, userProfileHandler.Deps{DB: d.DB, Store: d.Store, Cache: d.Cache})

		// Source — Plan 06 B5.
		source.RegisterRoutes(cfg, sourceHandler.Deps{DB: d.DB, Cache: d.Cache})

		// Service — Plan 06 B7. RbacSvc: story-10 guard field byt_code (config module không có perm middleware).
		service.RegisterRoutes(cfg, serviceHandler.Deps{DB: d.DB, Cache: d.Cache, RbacSvc: d.RbacSvc})

		// ConfigCode — Plan 06 B5.
		configCode.RegisterRoutes(cfg, configCodeHandler.Deps{DB: d.DB, Cache: d.Cache})

		// Options — Plan 06 B5.
		options.RegisterRoutes(cfg, optionsHandler.Deps{DB: d.DB, Cache: d.Cache})

		// BankAccount — Plan 06 B1: layered module.
		bankAccount.RegisterRoutes(cfg, bankAccountHandler.Deps{DB: d.DB})

		// PrintTemplate — cấu hình mẫu in HTML per-company/branch.
		printTemplate.RegisterRoutes(cfg, printTemplateHandler.Deps{DB: d.DB})

		// Unit — Plan 06 B7.
		unit.RegisterRoutes(cfg, unitHandler.Deps{DB: d.DB, Cache: d.Cache})
	}
}
