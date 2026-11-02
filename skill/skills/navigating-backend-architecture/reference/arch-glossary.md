# Glossary thuật ngữ

## Mục lục

- General / Tenancy
- CRM — Customer & Lead
- CRM — Appointment & Reception
- CRM — Order & Treatment
- CRM — Payment & Receipt
- EMR (Electronic Medical Record) — Bệnh án điện tử
- Medical / Pharmacy — Dược
- Inventory (Material/Supplies) — Kho vật tư
- Loyalty — Khách hàng thân thiết
- HR — Nhân sự
- Roles / People
- Print / Reports
- Marketing / Channels
- Zalo (Vietnamese super-app integration)
- Auth / Sessions
- Misc Common Terms
- Common abbreviations / acronyms
- Notes for AI agents

---


ERP Cluvix là hệ thống quản lý phòng khám/clinic đa chi nhánh (multi-tenant) tại Việt Nam, phục vụ cả nha khoa (dental) và thẩm mỹ (beauty). Code base trộn tiếng Anh (identifier) + tiếng Việt (gorm `comment:` tags, route comments, error messages) — glossary này giúp AI agent nói tiếng Anh hiểu các identifier/comment Vietnamese mà không cần biết tiếng Việt.

Code reference dùng dạng: `model.Field` hoặc `route /path` hoặc `file:path`.

---

## General / Tenancy

| VN | EN | Code reference / Notes |
|---|---|---|
| công ty | company | `Company` model — top-level tenant; `company_id` scoping (CRITICAL multi-tenant) |
| thương hiệu | brand | Synonym với company trong nhiều comment (`Tên của thương hiệu`, `JSON array ID thương hiệu`) |
| chi nhánh | branch | `Branch` model, `branch_id` — sub-tenant under company |
| phòng khám | clinic | Used interchangeably với company; `Thu tiền phòng khám` = clinic income |
| phòng ban | department | `department_id`; HR scope (different from EMR `emr_department`) |
| người dùng | user | `hr.User` |
| nhân viên | staff / employee | `Mã nhân viên` = employee code |
| phân quyền | permission / RBAC | `/auth/rbac/*`; `JSON array ID chi nhánh được phân quyền` |
| vai trò | role | `/auth/rbac/role/*` |
| đăng nhập | login | `/auth/login` |
| đăng xuất | logout | `/auth/logout`, `/auth/logout-all` |
| đổi mật khẩu | change password | `/hr/user/change-password` |
| phiên bản | version | Optimistic-locking field, `version int64` |
| ghi chú | note | Generic note field |
| mã | code | Identifier code (e.g. `Mã nhân viên`) |
| mô tả | description | |
| trạng thái | status | |
| loại | type / kind | |
| nhóm | group / category | |
| danh mục | category | |
| danh sách | list | |
| cấu hình | configuration / config | |
| tiến trình | progress / status flow | `Tiến trình khách hàng` = customer status flow |
| đính kèm | attachment | `File đính kèm` = attached file |
| được tạo bởi ai | created by | `created_by` audit field |
| người cập nhật cuối | last updated by | `updated_by` |

---

## CRM — Customer & Lead

| VN | EN | Code reference |
|---|---|---|
| khách hàng | customer | `crm.Customer`; `/crm/customer` |
| khách hàng cũ / mới | old / new customer | `CustomerType: 1=new, 2=old` |
| bệnh nhân | patient | `is_patient`, `emr_patient_code` (BN2025000001) |
| lead | lead | `crm.CustomerLead`, `/crm/customer-lead` |
| nguồn (khách hàng) | source | `Source` model, `source_id` — marketing source |
| chiến dịch | campaign | `campain_event_id`, `Banner chiến dịch` |
| sự kiện | event | `Campain - Event` |
| nhãn | tag | `Customer.Tags` (JSON array of tag IDs) |
| nhóm khách hàng | customer category | `customer_category` table |
| tư vấn viên | consultant / advisor | `payment_user_id` — revenue assigned to this user |
| tư vấn | advise / consultation | `/crm/note/list-advise` |
| cộng tác viên (CTV) | broker / affiliate / collaborator | `Broker` model, status=15 |
| hoa hồng | commission | `BrokerCommission` |
| doanh số | revenue (gross sales) | `Customer.TotalRevenue` |
| doanh thu | actual income / paid revenue | `Customer.TotalPayment` (note: VN distinguishes doanh số vs doanh thu) |
| công nợ | debt / receivable | `Customer.TotalDebt`, `OrderTreatmentPlan.DebtsPlan` |
| đặt cọc | deposit | `payment_for=3` (FOR_DEPOSIT); `Customer.TotalDeposit` |
| đặt cọc giữ chỗ | reservation deposit | `deposit_type=1` (RESERVE) |
| đặt cọc tạm | temporary deposit | `deposit_type=2` (TEMP) |
| ký gửi | consignment | `deposit_type=3` (KG); `Customer.Consignment` |
| chuyển cọc | move deposit | `payment_for=5` (FOR_MOVE_DEPOSIT) |
| khuyến mại | promotion / bonus | `Customer.TotalBonus`, `NewPromotion` |
| khiếu nại | complaint | `CustomerCareComplaints`; status=13 |
| chăm sóc khách hàng | customer care | `CustomerCare`; Kanban board |
| tiếp nhận | receive / intake | `userRecipient_id`, `doctorRecipient_id` |
| phản hồi | feedback / response | `CustomerReviews.Feedback` |
| đánh giá | review / rating | `CustomerReviews.Rating` |
| sinh nhật | birthday | Customer birthday filter |
| dân tộc | ethnicity | `ethnicity_id` |
| nghề nghiệp | job / occupation | `job_id` |
| giới tính | gender | 1=Nam (Male), 2=Nữ (Female), 3=Khác (Other) |
| địa chỉ thường trú | permanent address | `emr_permanent_address` |
| số điện thoại | phone number | `phone` |
| người đại diện | guardian / representative | `emr_guardian_*` (cha mẹ = parent) |
| quan hệ | relationship | `emr_guardian_relationship` |
| người thân | relatives | `Customer.Relatives` (JSON) |
| tiểu sử bệnh | medical history | `Customer.MedicalHistory` (JSON array) |
| tiền sử dị ứng | allergy history | `emr_allergy` |
| tiền sử gia đình | family medical history | `emr_family_history` |
| tình trạng bệnh lý | pathology / pathological condition | `customer_pathology_id` |

---

## CRM — Appointment & Reception

| VN | EN | Code reference |
|---|---|---|
| lịch hẹn | appointment | `crm.Appointment`, `/crm/appointment` |
| đặt hẹn | book appointment | `appointment_date` |
| đặt trước | pre-booked | `BookType=DAT_TRUOC=1` |
| phát sinh | walk-in / spontaneous | `BookType=PHAT_SINH=2` |
| dự kiến | tentative / planned | `BookType=DU_KIEN=3` |
| huỷ hẹn | cancel appointment | `update-appointment-cancel`; status=4 |
| xác nhận | confirm | `update-confirm-appointment` |
| khách đến | check-in | `customer-checkin`; status=2 (CHECKED_IN) |
| đã đến | visited | customer status=8 |
| ra về | check-out | status=3 (CHECKED_OUT) |
| lượt khám | visit / encounter | `appointment_id` linked to EMR |
| lý do hẹn | appointment reason | |
| nhắc lịch | appointment reminder | `Loại thời gian nhắc lịch: 1=giờ, 2=ngày` |
| lễ tân | receptionist | `print/reception-payment` |
| tiếp đón | reception | `/emr/reception`, `emr.Reception` |
| đón tiếp tự đến | walk-in registration | `/emr/reception/walk-in` |
| số thứ tự | queue number | `emr_queue_number` |

---

## CRM — Order & Treatment

| VN | EN | Code reference |
|---|---|---|
| đơn hàng | order | Generic order |
| đơn hàng điều trị / kế hoạch điều trị | treatment order plan | `OrderTreatmentPlan`, `/crm/order-treatment-plan` (alias `/order-plan`) |
| dịch vụ | service | `Service` model |
| dịch vụ điều trị | treatment service | |
| dịch vụ sử dụng / dịch vụ đã mua | service used | `OrderServiceUsed` — child of order plan; what customer bought |
| điều trị | treatment | |
| liệu trình điều trị | treatment course | `TreatmentCourse`; Dental vs Beauty branches |
| số lần điều trị | number of treatment sessions | `numTreatment`, `numTreatmentOfService` |
| tái khai thác | re-exploitation (re-engagement) | `dayReExploitation` |
| tái khám | re-examination | `treatment-reexamination-guarantee-appointment` |
| khám | examination / consultation | `Lý do khám` = reason for visit; `Kết quả khám` = exam result |
| chẩn đoán | diagnosis | `Chẩn đoán` |
| phương pháp điều trị | treatment method | |
| kế hoạch điều trị | treatment plan | `treatment_plan_id` |
| nha khoa | dental | `create-treatment-dental` |
| thẩm mỹ | beauty | `create-treatment-beauty` |
| răng | tooth | `tooth_position`, `status_tooth` JSON |
| vị trí răng | tooth position | `Vị trí răng điều trị` |
| màu răng | tooth shade | `toothShade` |
| khoá dịch vụ | lock service | `isLock`; 1=normal, 2=locked, 3=paused |
| trả góp | installment | `OrderInstallment`, `installment_config` JSON |
| kỳ trả | installment period | `period_number` |
| đến hạn | due | `due_date` |
| labo | dental lab | `LaboOrder`, `/crm/labo-order`; `Dịch vụ đưa yêu cầu sang labo` |
| bảo hành | warranty | `CustomerWarrantyCard`, `/crm/customer-warranty-card` |
| mã bảo hành | warranty code | `CustomerWarrantyCode` |
| thẻ | card | warranty card |
| phục hình | restoration / prosthetic | `Yêu cầu phục hình chi tiết` |
| vật liệu (yêu cầu) | required material | `Yêu cầu sử dụng vật liệu` |

---

## CRM — Payment & Receipt

| VN | EN | Code reference |
|---|---|---|
| thanh toán | payment | `crm.Payment`, `/crm/payment` |
| phiếu thu | payment receipt / receipt slip | `print/payment` (`In phieu thu thanh toan`) |
| phiếu thu lễ tân | reception receipt | `print/reception-payment` |
| phiếu hoàn cọc | refund deposit slip | `print/refund-deposit` |
| phiếu hoàn ký gửi | refund consignment slip | `print/refund-consignment` |
| phiếu hoàn dịch vụ điều trị | refund treatment service slip | `print/refund-service-treatment` |
| phiếu tổng hợp | aggregated slip | `print/treatment` (`In phieu tong hop dich vu dieu tri theo khach hang`) |
| phiếu đặt hàng labo | labo order slip | `print/labo-order` |
| hoàn dịch vụ | refund service | `payment_for=1` (FOR_REFUND_SERVICE) |
| hoàn cọc | refund deposit | `payment_for=2` (FOR_REFUND_DEPOSIT); `Customer.RefundDeposit` |
| hoàn ký gửi | refund consignment | `payment_for=6` (FOR_REFUND_DEPOSIT_KG); `Customer.RefundConsignment` |
| thu tiền phòng khám | clinic income | `payment_for=7` (FOR_CLINIC_INCOME) |
| chi tiền phòng khám | clinic expense | `payment_for=8` (FOR_CLINIC_EXPENSE) |
| hình thức thanh toán | payment method | `Payment.Method` |
| phương thức thanh toán | payment card type | `card_type_id` |
| ngày thu tiền | payment date | `payment_time` |
| tổng tiền | total amount | `total` |
| tổng tiền thanh toán | total paid | `Payment.Amount` |
| đã thanh toán | already paid | `paymentPlan` |
| chiết khấu | discount | `discount`, `service_discount` |
| giảm giá khác | other discount | `discount_other` |
| coupon | coupon | `Coupon` model |
| giá bán | selling price | `Giá bán` |
| giá vốn | cost price | `Giá vốn` |
| đơn giá | unit price | `unit_price` |
| số lượng | quantity | |
| tỷ lệ | ratio / rate | `EmrInsuranceBenefitRatio` |
| tổng giảm coupon | total coupon discount | `OrderTreatmentPlan.Discount` |

---

## EMR (Electronic Medical Record) — Bệnh án điện tử

| VN | EN | Code reference |
|---|---|---|
| bệnh án | medical record | `EmrMedicalRecord`, `/emr/emr-medical-record`; `Số bệnh án` = BA{YYYY}{000001} |
| khoa | hospital department | `EmrDepartment`, `/emr/emr-department` |
| chuyên khoa | specialty | `Chuyên khoa: 1=RHM (Răng Hàm Mặt = Dentistry), 2=Mắt (Ophthalmology), ...` |
| phòng | room | `EmrRoom`, `/emr/emr-room` |
| giường | bed | `EmrBed`, `/emr/emr-bed` |
| cơ sở (y tế) | medical facility | `EmrFacility`; `Tên cơ sở KCB` = name of healthcare facility |
| Sở Y tế | Department of Health (provincial) | `EmrHealthDepartment`; `Mã cơ sở (5 số - do Sở YT cấp)` |
| Bộ Y tế | Ministry of Health (national, BYT) | `Đã báo cáo BYT` = reported to MoH |
| BHYT | health insurance (national) | `emr_insurance_*`, `emr_patient_type=1=BHYT` |
| BN | patient code abbreviation | `BN2025000001` = patient prefix |
| KCB | khám chữa bệnh = medical examination & treatment | `Mã nơi đăng ký KCB ban đầu` |
| PKĐK | đa khoa = polyclinic / general | `Loại hình: PKĐK, PKCK...` |
| PKCK | chuyên khoa = specialty clinic | |
| RHM | Răng Hàm Mặt = Odonto-Stomatology / Dentistry | `Chuyên khoa: 1=RHM` |
| chỉ số sinh tồn | vital signs | `vital_signs` JSON |
| chiều cao / cân nặng / BMI | height / weight / BMI | `emr_height`, `emr_weight`, `emr_bmi` |
| biến chứng | complication | `Biến chứng` |
| bệnh kèm theo | comorbidity | `Mã ICD bệnh kèm` |
| tiền sử bệnh | medical history | |
| ngày vào / ra viện | admission / discharge date | `Ngày vào khám`, `Ngày ra viện/kết thúc` |
| ICD | ICD diagnosis code | `EmrIcd`; ICD-10/ICD-11 |
| khoá bệnh án | lock medical record | `Người khoá`, `Thời điểm khoá bệnh án` |
| ký số | digital signature | `Phương thức ký: manual, digital` |
| chứng thư số | digital certificate | |
| số CCCD / hộ chiếu | national ID / passport | `emr_id_card_number` |

---

## Medical / Pharmacy — Dược

| VN | EN | Code reference |
|---|---|---|
| thuốc | medicine / drug | `med.Medicine`, `/med/med-medicine` |
| đơn thuốc | prescription | `med.MedPrescriptions` (template), `MedCustomerPrescription` (filled) |
| mẫu đơn thuốc | prescription template | `Mẫu đơn thuốc — master` |
| kê đơn | prescribe | `Bác sĩ kê đơn` |
| bán lẻ (thuốc) | retail sale (OTC) | `/med-customer-prescription/retail-sale`; `Tên khách hàng bán lẻ` |
| OTC / kê đơn | OTC / prescription-only | `1=Thuốc kê đơn, 0=OTC` |
| thuốc gây nghiện | controlled drug (narcotic) | `1=Thường, 2=Gây nghiện, 3=Hướng thần, 4=Tiền chất` |
| thuốc hướng thần | psychotropic drug | |
| tiền chất | precursor chemical | |
| nhà sản xuất | manufacturer | `MedManufacturer`, `/med/med-manufacturer` |
| nhà cung cấp (NCC) | supplier | `Supplier` model |
| hoạt chất | active ingredient | `Hoạt chất` |
| hàm lượng | concentration / strength | `Hàm lượng/Nồng độ` |
| dạng bào chế | dosage form | `Dạng bào chế` |
| đơn vị tính | unit of measure | `Đơn vị tính: viên (tablet), hộp (box), chai (bottle)` |
| cách dùng | usage / instruction | `MedInstruction`, `/med/med-instruction` |
| liều dùng | dosage | `Hướng dẫn sử dụng / liều dùng` |
| nhập kho | stock import | `MedStockImport`, `/med/med-stock-import` |
| xuất kho | stock export | `MedStockExport`, `/med/med-stock-export` |
| kiểm kê | stock check / inventory audit | `MedStockCheck`, `/med/med-stock-check` |
| đơn đặt hàng (dược) | purchase order | `MedPurchaseOrder`, `/med/med-purchase-order` |
| đặt hàng | order / purchase | |
| thu hồi thuốc | drug recall | `MedDrugRecall`, `/med/med-drug-recall` |
| tương tác thuốc | drug interaction | `MedDrugInteraction`, `/med/med-drug-interaction` |
| báo cáo ADR | ADR (Adverse Drug Reaction) report | `MedAdrReport`, `/med/med-adr-report` |
| lô thuốc | medicine batch | `MedBatch`; FEFO sorted by `expiry_date` |
| hạn sử dụng | expiry date | `expiry_date` |
| tồn kho | stock / inventory | `Tồn kho`, `Tồn kho thực tế`, `Tồn kho đang khoá` |
| duyệt phiếu | approve slip | `Người duyệt`, `Duyệt phiếu` |
| chuyển kho | warehouse transfer | `1=Hủy, 2=Chuyển kho, 3=Trả NCC` |
| trả NCC | return to supplier | |
| sắp hết hạn | expiring (alert) | `expiry-alert` |
| sắp hết hàng | low stock | `low-stock-alert` |
| FEFO | First-Expired-First-Out | Batch dispense rule |

---

## Inventory (Material/Supplies) — Kho vật tư

| VN | EN | Code reference |
|---|---|---|
| kho | warehouse | `Kho` |
| nguyên vật liệu (NVL) | material / supplies | `StockMaterial` |
| nhập kho (vật tư) | stock entry | `StockEntry`, `/inventory/stock-entry` |
| xuất kho / sử dụng | stock usage | `StockUsages`, `/inventory/stock-usages` |
| trả lại | stock return | `StockReturns`, `/inventory/stock-returns` |
| định mức | norm / quota | `Định mức` |
| nhật ký nhập kho | stock entry log | `StockEntryLogs` |
| nhật ký sử dụng | stock usage log | `StockUsageLogs` |
| dòng nhập kho | entry line | |
| ngày nhập kho | entry date | |
| số lượng sử dụng | quantity used | |
| số lượng trả lại | quantity returned | |
| chứng từ | document / voucher | `Chứng từ đính kèm` |

---

## Loyalty — Khách hàng thân thiết

| VN | EN | Code reference |
|---|---|---|
| coupon | coupon | `loyalty.Coupon`; `/loyalty/coupon` |
| khuyến mại | promotion | `Coupon.Type=promotion` |
| chủ coupon | coupon owner | `Chủ coupon` |
| môi giới | broker | `loyalty.Broker`; partner referral |
| hoa hồng môi giới | broker commission | `BrokerCommission`, `/loyalty/broker-commission` |
| thanh toán hoa hồng | broker payment commission | `BrokerPaymentCommission` |
| thông tin thanh toán | payment info (bank account) | `/loyalty/broker/list-payment-info` |
| đã sử dụng | used | `CouponUsedByOrderPlan` |

---

## HR — Nhân sự

| VN | EN | Code reference |
|---|---|---|
| lương | salary | `Lương cơ bản` = base salary |
| ca làm việc | work shift | `HrShift`; `Mã ca: SANG/CHIEU/DEM` (morning/afternoon/night) |
| ca qua đêm | overnight shift | |
| nghỉ phép | leave | `Mã loại phép: ANNUAL/SICK/MATERNITY` |
| phụ cấp | allowance | `HrAllowance`, `/hr/hr-allowance` |
| thưởng | bonus | `HrBonus`, `/hr/hr-bonus` |
| phạt | penalty | `Loại: 1=Thưởng, 2=Phạt` |
| khấu trừ | deduction | `Loại: 1=Thu nhập, 2=Khấu trừ` |
| thu nhập | income | |
| BHXH | social insurance | `BHXH_EMP` payroll component |
| TNCN | personal income tax | `Chịu thuế TNCN` |
| ngày công | workday | `Công tính theo ngày, full công = 1` |
| ngày công chuẩn / thực tế | standard / actual workdays | |
| đối tác | partner | `Partner` model — external API consumer |
| khoá tài khoản | lock account | `lock-partner` |
| chức vụ | position / title | `Chức vụ` |
| tăng ca | overtime | `Trạng thái ghi nhận có tính tăng ca` |

---

## Roles / People

| VN | EN | Code reference |
|---|---|---|
| bác sĩ | doctor | `doctor_id`; `Bác sĩ điều trị` (treating doctor) |
| bác sĩ tư vấn | consulting doctor | `doctorRevenue_id` |
| bác sĩ kê đơn | prescribing doctor | |
| bác sĩ tiếp nhận | receiving doctor | `doctorRecipient_id` |
| bác sĩ xử lý | handling doctor | `doctorTreatment_id` |
| kỹ thuật viên (KTV) | technician | `treatment-course-technical` |
| phụ tá | assistant | `treatment-course-assistant`; `Tính điểm cho phụ tá` |
| dược sĩ | pharmacist | (med module users) |
| direct sale | direct sale (sales rep at clinic) | `direct_sales_id` |
| telesales | telesales | `report-telesale`, `telesales-online` |
| marketing | marketing | `marketing_id` (lead source rep) |
| nhân viên kinh doanh | sales staff | `salesStaff_id` |
| kế toán | accountant | `/api/accountant/*` |
| người tiếp nhận | recipient | `userRecipient_id` |
| người xử lý | handler | `userHandler_id` |
| người duyệt | approver | |
| người giới thiệu | referrer | |

---

## Print / Reports

| VN | EN | Code reference |
|---|---|---|
| in phiếu | print slip | `In phieu …` route comments in `internal/modules/webapp/print/router.go` |
| in đơn thuốc | print prescription | `print/prescription` |
| in lịch hẹn theo giờ | print appointment by hour | `print/appointment-byhour-list` |
| in lịch hẹn theo bác sĩ | print appointment by doctor | `print/appointment-bydoctor-list` |
| báo cáo | report | `report.go` files |
| thống kê | statistics | `appointment-statistics`, `branch-statistics` |
| doanh thu theo ngày | revenue by date | `payment-by-date` |
| theo nguồn | by source | `payment-by-source` |
| theo bệnh lý | by pathology | `payment-by-pathological` |
| theo tỉnh thành | by province | `payment-by-province` |
| theo quốc gia | by country | `payment-by-country` |
| sổ nhập | book of imports | `book-import` |
| sổ xuất | book of dispensed | `book-dispensed` |
| sổ kiểm soát | book of controlled drugs | `book-controlled` |
| huỷ / huỷ phiếu | cancel slip | |
| trả huỷ | return / destroy | `return-destroy-report` |
| kiểm kê tồn kho | inventory audit | `inventory-audit-report` |
| theo dõi lô | batch tracking | `batch-tracking-report` |

---

## Marketing / Channels

| VN | EN | Code reference |
|---|---|---|
| nguồn khách | customer source | `Source` |
| nhóm nguồn | source group | `groupSource_id` |
| trang Facebook | Facebook page | `page_facebook_id`, `page-facebook-statistics` |
| chăm sóc lại | re-marketing | `CustomerReMarketing` |
| ticket | ticket | customer status=2 |
| KPI | KPI | `/cx-marketing/kpi` |
| digital facebook | digital facebook ads | `/cx-marketing/digital-facebook` |
| convert | convert (lead → ticket) | `/cx-marketing/convert` |

---

## Zalo (Vietnamese super-app integration)

| VN | EN | Code reference |
|---|---|---|
| OA (Official Account) | Zalo Official Account | `zalo_oa_config`; business account on Zalo |
| ZNS | Zalo Notification Service | `zalo_zns_template`, `ZaloZnsLog` — transactional templated SMS-like |
| Zalo Mini App | Zalo Mini App | Customer-facing app |
| tin (Zalo) | Zalo post / message | `NewPost` |
| khuyến mại (Zalo) | Zalo promotion banner | `NewPromotion` |
| slide | slide banner | `NewSlide` |
| video (Zalo) | Zalo video | `NewVideo` |
| chiến dịch game | game campaign | `ZaloGameCampaign` (Lucky-wheel etc.) |
| vòng quay | spin wheel | `1=Vòng quay, 2=Scratch card, 3=Lắc xì` |
| lắc xì | shake-to-win (Tet game) | game type 3 |
| quà tặng | gift | `ZaloGameGift` |
| lượt chơi | game play count | |
| lịch sử chơi | game history | `ZaloGameHistory` |
| đăng ký webhook | register webhook | |
| token | token | `Hạn sử dụng token`, refresh token |
| Pancake | Pancake (Vietnamese omnichannel CRM) | `ID Pancake` |

---

## Auth / Sessions

| VN | EN | Code reference |
|---|---|---|
| đăng nhập | login | rate-limited 10/IP/5min |
| đăng xuất | logout | revokes session |
| refresh token | refresh token | `RefreshToken` |
| phiên làm việc | session | `RevokeAllSessions` |
| xoá cache | clear cache | `/auth/clear-cache` |
| OTP | OTP | `Otp` field |

---

## Misc Common Terms

| VN | EN | Code reference |
|---|---|---|
| ngày sinh | birthday | `birthday`, `year_born` |
| tuổi | age | |
| địa chỉ | address | |
| thôn / phố | hamlet / street | `emr_street` |
| số nhà | house number | `emr_house_number` |
| tỉnh thành | province | `province_id` |
| phường xã | ward | `ward_id` |
| quốc gia | country | `country_id` |
| khu vực | region | `JSON array ID quốc gia thuộc khu vực` |
| URL thân thiện | friendly URL / slug | |
| hình ảnh / ảnh | image | `image`, `Avatar`, `Hình đại diện` |
| ảnh đại diện | avatar | |
| hình minh hoạ | illustration image | |
| tải lên | upload | |
| tải xuống | download | |
| nhập từ excel | import from excel | `ImportMedicineFromExcel` |
| xuất ra excel | export to excel | `/download/export` static |
| tìm kiếm | search | |
| từ khoá | keyword | `LoadByKeyword`, `load-by-keyword` |
| lọc | filter | |
| sắp xếp | sort | `SortOrder` |
| vị trí hiển thị | display position | |
| ẩn / hiện | hide / show | pattern endpoint `show-hide` cũ — đã XÓA khỏi BE, toggle qua update thường |
| hoạt động / không hoạt động | active / inactive | `1=Hoạt động, 0=Không hoạt động` |
| chờ duyệt | pending approval | `1=Chờ duyệt, 2=Active, 3=Inactive` |
| đồng ý | accepted | customer status=10 |
| không đồng ý | not accepted | customer status=9 (Lost) |
| hoàn thành | completed / done | |
| đang điều trị | being treated | `treatment_status=2 (BEING_TREATED)` |
| chưa điều trị | untreated | `treatment_status=1 (UNTREATED)` |
| dừng CS | stopped care (Trash) | customer status=1 |
| timeline / dòng thời gian | timeline | `addTimelineEvent`, `timeline-customer` |
| sao chép / chuyển | switch / clone | `create-customer-switch` (clone customer to another branch) |
| ghi chú admin | admin note | |
| lý do | reason | |
| mã vạch / QR | barcode / QR code | `Mã bảo hành (barcode, qr code)` |
| tin nhắn | message / SMS | `SmsLog`, `/crm/sms-log/send-sms` |

---

## Common abbreviations / acronyms

| Abbr | Meaning | Notes |
|---|---|---|
| BN | Bệnh Nhân = Patient (code prefix) | `BN2025000001` |
| BA | Bệnh Án = Medical Record (code prefix) | `BA{YYYY}{000001}` |
| DH | Đơn Hàng = Order (code prefix) | OrderTreatmentPlan |
| TC | Treatment Course (code prefix) | |
| DQG | Đơn Quốc Gia = National prescription submission | `med-dqg-config`; gov-mandated submission |
| ADR | Adverse Drug Reaction | `med-adr-report` |
| EMR | Electronic Medical Record | |
| ICD | International Classification of Diseases | ICD-10 / ICD-11 |
| BHYT | Bảo Hiểm Y Tế = Health Insurance (national) | `emr_insurance_*` |
| BHXH | Bảo Hiểm Xã Hội = Social Insurance | HR payroll |
| TNCN | Thu Nhập Cá Nhân = Personal Income Tax | |
| KCB | Khám Chữa Bệnh = Medical Examination & Treatment | |
| BYT | Bộ Y Tế = Ministry of Health | |
| RHM | Răng Hàm Mặt = Odonto-Stomatology / Dentistry | |
| PKĐK | Phòng Khám Đa Khoa = Polyclinic / General Clinic | |
| PKCK | Phòng Khám Chuyên Khoa = Specialty Clinic | |
| KH | Khách Hàng = Customer | |
| KN | Khiếu Nại = Complaint | `Hoàn thành KN` = complaint resolved |
| NVL | Nguyên Vật Liệu = Material / Supplies | inventory |
| NCC | Nhà Cung Cấp = Supplier | med + inventory |
| KTV | Kỹ Thuật Viên = Technician | |
| CTV | Cộng Tác Viên = Broker / Collaborator / Affiliate | |
| OTC | Over The Counter (drug) | `1=Thuốc kê đơn, 0=OTC` |
| FEFO | First Expired First Out | batch dispense order |
| KG | Ký Gửi = Consignment | `deposit_type=3` |
| OA | Official Account (Zalo) | `zalo_oa_config` |
| ZNS | Zalo Notification Service | `zalo_zns_template` |
| RBAC | Role-Based Access Control | `/auth/rbac/*` |
| FK | Foreign Key | `FK → table` in gorm comments |
| BMI | Body Mass Index | auto `emr_bmi = weight/(height/100)²` |
| KPI | Key Performance Indicator | `/cx-marketing/kpi` |
| VND / VNĐ | Vietnamese Dong (currency) | `Giá trị quy đổi (VNĐ)` |

---

## Notes for AI agents

1. **Multi-tenant**: comments mentioning `chi nhánh` / `thương hiệu` / `công ty` usually flag `branch_id` / `company_id` scoping requirements (see Hard Rules trong `CLAUDE.md`).
2. **Money**: terms like `tổng tiền`, `giá`, `chiết khấu`, `hoa hồng`, `doanh thu/số`, `công nợ`, `cọc`, `thanh toán` map to `decimal.Decimal` fields — never `float64` (Hard Rules trong `CLAUDE.md`).
3. **Optimistic locking**: most CRM models with `Phiên bản` (version) field use HTTP 409 conflict pattern (`errs.ErrVersionConflict` — xem `skill/skills/backend-hard-rules/SKILL.md`).
4. **Synonyms**: `công ty` (company) ≈ `thương hiệu` (brand) ≈ `phòng khám` (clinic) all refer to the top tenant level. `chi nhánh` (branch) is the sub-level.
5. **Status flow**: customer status uses 16 numeric codes (1=Trash → 15=Broker, +16=Contact), see `pkg/define/customer.go` and `skill/skills/working-on-crm/reference/customer.md`.
6. **Routes**: read `backend/internal/modules/<domain>/router.go` + `<resource>/route.go` — many have inline VN comments (especially module `print`, which describes each printable slip type).
