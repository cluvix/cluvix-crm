# Hiệu năng endpoint danh sách ở quy mô lớn

> Đọc khi được hỏi "màn này chậm khi nhiều dữ liệu", trước khi tối ưu bất kỳ handler list nào,
> và TRƯỚC KHI kết luận một endpoint "đã nhanh" dựa trên DB dev.
> Mọi số đo dưới đây lấy trên DB clone schema y hệt, 524.288 lịch hẹn / 100.000 khách, MySQL 9.7,
> máy rảnh. ~40ms trong mỗi số là overhead kết nối `mysql` client.

> ⚠ **LỊCH SỬ — mọi số đo trong file này lấy trên schema ERP CÒN cột `company_id` (trước D15/story-62).**
> CRM standalone là 1 thương hiệu/DB, schema mới (migration schema mới) KHÔNG còn `company_id`
> ở bảng nào ⇒ (1) các bẫy RIÊNG của cột này (ref-lookup trên `company_id`, chỉ mục 1 cột `company_id`,
> JOIN thiếu vế company, index_merge với `company_id`) không còn tồn tại — đừng áp lại bản vá "thêm vế
> company"; (2) các nguyên lý còn lại (LEFT JOIN không bị loại, `IN (SELECT …)`, `COUNT` của `Paginate`,
> `LIKE '%kw%'`, mất early-exit…) vẫn đúng, nhưng SỐ ĐO và tên chỉ mục `idx_*_company_*` là của schema cũ
> — cần con số thì đo lại bằng `/check-scale` trên bench dựng từ migration mới, KHÔNG suy từ số dưới đây.
> Nhiều module được đo (kế toán, workspace, EMR, hoa hồng bác sĩ, warranty, care…) cũng đã xoá khỏi CRM.

## Mục lục

- [Luật số 0 — không kết luận hiệu năng trên DB dev](#luật-số-0--không-kết-luận-hiệu-năng-trên-db-dev)
- [MySQL KHÔNG loại-bỏ-left-join](#mysql-không-loại-bỏ-left-join)
- [`IN (SELECT …)` biến bộ lọc ngày thành vô dụng](#in-select--biến-bộ-lọc-ngày-thành-vô-dụng--semi-join-đổi-bảng-dẫn)
- [`COUNT(*)` của `pagination.Paginate` chạy trên MỌI list](#count-của-paginationpaginate-chạy-trên-mọi-list)
- [GROUP BY kèm cột của bảng ĐƯỢC JOIN — khoá sort phình](#group-by-kèm-cột-của-bảng-được-join--khoá-sort-phình-lên-varchar)
- [`LIKE '%kw%'` xuyên join = quét toàn bảng](#like-kw-xuyên-join--quét-toàn-bảng)
- [⚠ ngram KHÔNG phải lúc nào cũng thắng — thua `LIKE` ở từ khoá toàn chữ](#-ngram-không-phải-lúc-nào-cũng-thắng--nó-thua-like-ở-từ-khoá-toàn-chữ)
- [Cách sửa tìm kiếm đã đo — và 4 biến thể đã loại](#cách-sửa-tìm-kiếm-đã-đo--và-4-biến-thể-đã-loại)
- [Cost model chấm SAI ref-lookup trên `company_id`](#cost-model-chấm-sai-ref-lookup-trên-company_id--bẫy-riêng-của-db-per-customer)
  - [Dashboard Kế toán — cột phủ SẴN CÓ vẫn bị bỏ, và cột không có chỉ mục nào](#dashboard-kế-toán--cột-phủ-sẵn-có-vẫn-bị-bỏ-và-cột-không-có-chỉ-mục-nào)
  - [Công nợ khách hàng — chỉ mục đúng đã có, câu TRANG vẫn cược PRIMARY và thua](#công-nợ-khách-hàng--chỉ-mục-đúng-đã-có-câu-trang-vẫn-cược-primary-và-thua)
  - [Công nợ đơn hàng — 5 bảng dẫn xuất gom TOÀN BẢNG, chạy 2 lần](#công-nợ-đơn-hàng--5-bảng-dẫn-xuất-gom-toàn-bảng-chạy-2-lần-câu-trang-tính-lại-mọi-đơn-chỉ-để-lấy-20)
  - [Công nợ điều trị — chỉ mục HÀM cho `totalEnd != payment`](#công-nợ-điều-trị--điều-kiện-totalend--payment-không-có-đường-vào--chỉ-mục-hàm-và-phải-viết-đúng-dạng)
  - [Công nợ theo buổi — điều kiện là BIỂU THỨC, 24% dòng thoả](#công-nợ-theo-buổi--điều-kiện-là-biểu-thức-24-dòng-thoả-không-chỉ-mục-nào-cứu-vá-bằng-bớt-lượt--đổi-bảng-dẫn)
  - [Công nợ trả góp — bảng dẫn xuất gom TOÀN BẢNG chỉ để đếm kỳ](#công-nợ-trả-góp--bảng-dẫn-xuất-gom-toàn-bảng-chỉ-để-đếm-kỳ-và-câu-trang-từ-khoá-cược-dừng-sớm)
  - [Công nợ sản phẩm — bảng dẫn xuất gom TOÀN BẢNG payment theo mốc ngày](#công-nợ-sản-phẩm--bảng-dẫn-xuất-gom-toàn-bảng-payment-theo-mốc-ngày-bộ-lọc-ngày-không-giảm-việc-gì)
  - [Thu theo chi nhánh — lần thứ N khuôn COUNT/SUM kéo JOIN](#thu-theo-chi-nhánh--lần-thứ-n-khuôn-countsum-kéo-join-cộng-tra-id-trước-cho-trang-sâu)
  - [Dashboard Workspace — `COALESCE(cột ngày, cột ngày)` trong WHERE giết mọi chỉ mục](#dashboard-workspace--coalescecột-ngày-cột-ngày-trong-where-giết-mọi-chỉ-mục)
  - [Thu/chi phòng khám — hint có CỔNG theo chi nhánh VÀ độ rộng dải ngày](#thuchi-phòng-khám--hint-có-cổng-theo-chi-nhánh-và-độ-rộng-dải-ngày)
  - [Đơn hàng Kế toán — COUNT của `Paginate` tính subquery tương quan cho MỌI dòng](#đơn-hàng-kế-toán--count-của-paginate-tính-subquery-tương-quan-cho-mọi-dòng)
  - [Phiếu thu Kế toán — COUNT + SUM trả tiền 4 LEFT JOIN mà bộ lọc không dùng](#phiếu-thu-kế-toán--count--sum-trả-tiền-4-left-join-mà-bộ-lọc-không-dùng)
  - [Báo cáo Doanh thu theo ngày — baseSQL UNION chạy 2 lần, 3 nhánh cùng dính ref-lookup](#báo-cáo-doanh-thu-theo-ngày--basesql-union-chạy-2-lần-3-nhánh-cùng-dính-ref-lookup)
  - [Báo cáo Doanh thu theo chi nhánh — 7 bảng dẫn xuất chạy 2 lần, 4 lượt quét payment cùng dải ngày](#báo-cáo-doanh-thu-theo-chi-nhánh--7-bảng-dẫn-xuất-chạy-2-lần-4-lượt-quét-payment-cùng-dải-ngày)
  - [9 báo cáo doanh thu nhóm theo chiều — cùng khuôn, thêm bẫy dẫn từ `customer`](#9-báo-cáo-doanh-thu-nhóm-theo-chiều--cùng-khuôn-thêm-bẫy-dẫn-từ-customer)
  - [Doanh thu theo dịch vụ / nhóm dịch vụ — chạy 2 lần, bảng thực thu JOIN vào từng dòng osu](#doanh-thu-theo-dịch-vụ--nhóm-dịch-vụ--chạy-2-lần-bảng-thực-thu-join-vào-từng-dòng-osu)
  - [Doanh số theo nhân viên — bộ lọc ngày không có chỉ mục nào để vào](#doanh-số-theo-nhân-viên--bộ-lọc-ngày-không-có-chỉ-mục-nào-để-vào)
- [Bảng được JOIN không bị scope `company_id` ⇒ optimizer chọn sai bảng dẫn](#bảng-được-join-không-bị-scope-company_id--optimizer-chọn-sai-bảng-dẫn)
- [Query GROUP BY không phân trang KHÔNG miễn nhiễm](#query-group-by-không-phân-trang-không-miễn-nhiễm)
- [Query có pre-filter chọn lọc ⇒ không dính 3 nút thắt trên](#query-có-pre-filter-chọn-lọc--không-dính-3-nút-thắt-trên)
- [Chỉ mục 1 cột `company_id` vừa là thủ phạm vừa là thứ PHẢI GIỮ](#chỉ-mục-1-cột-company_id-vừa-là-thủ-phạm-vừa-là-thứ-phải-giữ)
- [Tool AI `getCustomerBenchmark` — cohort toàn chi nhánh](#tool-ai-getcustomerbenchmark--hai-câu-cohort-toàn-chi-nhánh-và-cái-bẫy-cache-theo-khách)
- [Đã đo và KHÔNG có vấn đề — đừng bench lại](#đã-đo-và-không-có-vấn-đề--đừng-bench-lại)
- [List CON của một cha (`WHERE <fk> = ?`) cũng miễn nhiễm](#list-con-của-một-cha-where-fk---cũng-miễn-nhiễm--chi-phí-là-osố-con-không-obảng)
- [Xác nhận lần 2 khuôn list-con — `reminder-of-customer`](#xác-nhận-lần-2-khuôn-trên--reminder-of-customer-và-mẹo-đọc-đồng-hồ-không-bị-sàn-overhead)
- [Xác nhận lần 3 khuôn list-con — `check-up-general-of-customer`, và bảng KHÔNG có `company_id`](#xác-nhận-lần-3-khuôn-list-con--check-up-general-of-customer--và-cái-giá-của-bảng-không-có-company_id)
- [Xác nhận lần 4 khuôn list-con — `appointment/get-by-customer`](#xác-nhận-lần-4-khuôn-list-con--appointmentget-by-customer-order-by-không-phải-id)
- [⚠ Khuôn list-con KHÔNG tự động miễn nhiễm — `list-service-used-customer`](#-khuôn-list-con-không-tự-động-miễn-nhiễm--list-service-used-customer-là-ca-đối-chứng)
- [Xác nhận lần 5 khuôn list-con — `get-treatment-course-of-customer` có `Sort:` nhưng vẫn không đau](#xác-nhận-lần-5-khuôn-list-con--get-treatment-course-of-customer-có-sort-nhưng-vẫn-không-đau)
- [Endpoint không-SQL: `list-folder` — trục scale là S3, và cái đáng lo là CẮT IM LẶNG](#endpoint-không-sql-list-folder--trục-scale-là-s3-và-cái-đáng-lo-là-cắt-im-lặng)
- [Vì sao `prescription-of-customer` KHOẺ còn `get-payment-detail-for-customer` HỎNG](#vì-sao-prescription-of-customer-khoẻ-còn-get-payment-detail-for-customer-hỏng--khác-đúng-một-chỗ)
- [Endpoint 1-dòng-mỗi-khách: `get-warranty-code` — cả 3 câu đều là const/index lookup](#endpoint-1-dòng-mỗi-khách-get-warranty-code--cả-3-câu-đều-là-constindex-lookup)
- [Chỉ mục composite ĐÚNG cột thì list-con miễn nhiễm cả với `ORDER BY` không phải `id` — `care-of-customer`](#chỉ-mục-composite-đúng-cột-thì-list-con-miễn-nhiễm-cả-với-order-by-không-phải-id--care-of-customer)
- [Chuỗi con trên cột MÃ không cứu được bằng chỉ mục](#chuỗi-con-trên-cột-mã-không-cứu-được-bằng-chỉ-mục--lần-thứ-hai-đo-ra-cùng-kết-luận)
- [⚠ `mysqldump` KHÔNG bảo toàn stopword của FULLTEXT ⇒ bench báo SÓT GIẢ](#-mysqldump-không-bảo-toàn-trạng-thái-stopword-của-fulltext--bench-báo-sót-giả)
- [`ORDER BY` bảng gốc + JOIN làm mất early-exit của `LIMIT`](#order-by-bảng-gốc--join-làm-mất-early-exit-của-limit)
- [Đo trên máy đang có phiên khác chạy bench](#đo-trên-máy-đang-có-phiên-khác-chạy-bench)
- [Quy trình đo bắt buộc](#quy-trình-đo-bắt-buộc)
- [`GROUP BY <bảng>.id` vô điều kiện — no-op nhưng trả tiền trong `COUNT`](#group-by-bảngid-vô-điều-kiện--no-op-ngữ-nghĩa-nhưng-trả-tiền-thật-trong-count)
- [Đo bằng SỐ DÒNG InnoDB đọc khi máy quá nhiễu](#đo-bằng-số-dòng-innodb-đọc-khi-máy-quá-nhiễu-để-tin-thời-gian)
- [⛔ Export `?_export=1` — vòng batch OFFSET đọc gấp 28 lần, và BỎ SÓT dòng im lặng](#-export-_export1--vòng-batch-offset-đọc-gấp-28-lần-và-bỏ-sót-dòng-im-lặng)
- [Helper ngram dùng chung — đừng đẻ bản sao thứ N](#helper-ngram-dùng-chung--đừng-đẻ-bản-sao-thứ-n)

---

## ⛔ Luật số 0-A — DỰNG BENCH TỪ MIGRATION, KHÔNG `mysqldump` TỪ DEV

Đọc trước cả Luật số 0. Vi phạm luật này thì mọi số đo phía sau đều đo sai đối tượng.

`crm_dev_db` (DB dev) là **DB CŨ có từ trước hệ migration** — user xác nhận 2026-09-09. Khách hàng
mới thì DB được `cmd/migrate` dựng từ 0 nên **sạch hơn hẳn**. Hai schema KHÁC NHAU thật sự, và
`mysqldump --no-data crm_dev_db` bê nguyên cái cũ vào bench.

Đo cụ thể ngày 2026-09-09, chỉ tính **10 bảng hay bench nhất**: khách hàng có **16 chỉ mục** mà
bench-clone-từ-dev KHÔNG có, trong đó có đúng những chỉ mục ngày mà người ta hay kết luận "thiếu":

```
appointment          ← appointment_date        ·  created_at
customer             ← created_day · job_id · userZaloID
order_service_used   ← created_day
order_treatment_plan ← created_time
payment              ← payment_time · payment_of_date · deposit_id
treatment_course     ← created_time · percent_success · treatment_timeEnd
                       · (category_id,created_time)
                       · (company_id,created_time,branch_id,point_for_assistant,service_used_id)
```

⇒ Bench dev-shape **thiếu chỉ mục ⇒ đo ra CHẬM GIẢ ⇒ phóng đại con số "trước khi vá"**, và tệ nhất
là ship một chỉ mục mà khách đã có sẵn. Chiều ngược lại cũng có (dev thừa chỉ mục/cột ⇒ đo NHANH
GIẢ, hoặc code viết theo dev rồi chết ở khách — đã xảy ra với `warehouse.branch_id`).

**Cách dựng bench ĐÚNG:**
```bash
mysql -uroot -e "CREATE DATABASE cluvix_<x>_tmp DEFAULT CHARACTER SET utf8mb4"
go run cmd/migrate/main.go -cmd=up -db=cluvix_<x>_tmp     # ← schema của KHÁCH, không phải của dev
# rồi mới seed dữ liệu (copy từ dev / nhân bản), CHỈ những cột có ở schema này
```
Trước khi tin bất kỳ kết luận cũ nào: `bash skill/skills/_tooling/verify-schema-drift.sh crm_dev_db`.

⚠ Kết luận **cấu trúc** (gộp N subquery, bỏ join thừa, sửa lỗi scope, singleflight, gộp lượt
aggregate) KHÔNG phụ thuộc hình dạng chỉ mục nên vẫn đứng vững. Kết luận **về chỉ mục** và **con số
tuyệt đối trước/sau** thì phụ thuộc — phải đo lại trên schema migration.

## Luật số 0 — không kết luận hiệu năng trên DB dev

DB dev có 569 lịch hẹn ⇒ endpoint trả 7ms và che sạch mọi vấn đề bên dưới. Cùng endpoint đó ở
524k dòng có đường đi mất 6.8 giây. **Số đo trên dev không chứng minh được gì về scale.**

Đo `run.log` TRƯỚC để biết nút thắt nằm ở SQL hay Go: `pkg/logger/gorm.go` in `[SLOW SQL <t>] @ <path>`
cho mọi query ≥200ms. Tổng thời gian request trừ đi tổng SLOW SQL = phần của code Go.

## MySQL KHÔNG loại-bỏ-left-join

`LEFT JOIN` vào bảng mà điều kiện join là khoá chính KHÔNG thể làm đổi số dòng — về mặt logic bỏ
được khi không cột nào của bảng đó được đọc. MySQL **không** làm tối ưu này (khác SQL Server/Oracle)
nên vẫn join đủ mọi dòng, kể cả trong `COUNT(*)`.

| | 524k dòng |
|---|---|
| `COUNT(*)` có `LEFT JOIN customer` | 1091–1272ms |
| `COUNT(*)` bỏ join | **641–773ms** |

⇒ Chỉ gắn join khi thật sự có thứ đọc cột bảng đó. Mẫu đã áp: `needsCustomerJoin()` trong
`crm/appointment/handler/list_search.go`.

⚠ Bỏ join phải rà HẾT consumer của cùng query — `_export=1` đưa nguyên query sang
`export.ExportAppointment`, hàm đó select `customer.code/fullname/phone` và join
`location_province ON prov.id = customer.province_id`. Bỏ join ⇒ export chết với
`Unknown column 'customer.code'`. Ép join tại MỘT điểm quyết định duy nhất; để hàm export tự join
thì khi filter customer cũng có mặt sẽ thành `Not unique table/alias: 'customer'`.

⚠ Thêm filter mới đụng `customer.*` mà quên khai báo vào danh sách "cần join" ⇒ `Unknown column`.

Repo còn **12+ handler gắn ≥4 `LEFT JOIN` vô điều kiện rồi `Paginate`** — cao nhất
`crm/treatment_service/handler/treatment_service.go` (20 join), `accountant/payment/handler/payment_report.go` (17).

## `IN (SELECT …)` biến bộ lọc ngày thành vô dụng — semi-join đổi bảng DẪN

Anh em của hai bẫy "optimizer chọn sai bảng dẫn" ở dưới, nhưng nguyên nhân nằm ở **cách viết điều
kiện**, không ở chỉ mục. `col IN (SELECT id FROM <bảng danh mục> WHERE <cờ>)` được MySQL 9.7 viết
lại thành **semi-join** ⇒ INNER JOIN lấy chính bảng danh mục (7 dòng) làm bảng **DẪN**, rồi tra
`Index lookup on appointment using fk-appointment_appointment_type_id` cho từng giá trị:

```
-> Sort: appointment.id DESC, limit input to 15 row(s) per chunk
    -> Nested loop inner join  (cost=71768 rows=70283)
        -> Table scan on appointment_type  (cost=1.7 rows=7)          <- bảng DẪN
        -> Index lookup on appointment using fk-appointment_appointment_type_id
```

Hệ quả nặng hơn vẻ ngoài: một khi bảng dẫn là bảng danh mục, `idx_appointment_company_date`
(dẫn đầu `company_id, appointment_date`) **không dùng được nữa**, nên **bộ lọc NGÀY do người dùng
chọn không giảm được một dòng đọc nào**. Đo trên `GET /crm/report/appointment-consultation`,
bench 872.284 lịch hẹn / 57.384 lịch tư vấn / 7 năm dữ liệu (2026-09-09): dù lọc 9 ngày, 1 tháng,
3 tháng hay 1 năm thì đều đọc **~57.384 dòng** — bằng đúng số lịch tư vấn TỪ TRƯỚC TỚI NAY.

Cách chữa: đọc danh sách id ra Go (bảng danh mục vài dòng, ~0,1ms) rồi truyền vào `IN ?` dưới
dạng **hằng số**. Optimizer lúc đó thấy một điều kiện thường và tự chọn được đường ngày/PK.

| @872.284 lịch hẹn | `IN (SELECT …)` + join | `IN (1,3,6)`, bỏ join |
|---|---|---|
| SELECT trang 1, lọc THÁNG HIỆN TẠI | 16.528 dòng · 0,119–0,366s | **254 dòng · 0,001–0,002s** |
| SELECT trang 1, KHÔNG lọc ngày | 57.384 dòng · 0,203–1,017s | **254 dòng · 0,001–0,003s** |
| SELECT trang 1, lọc THÁNG 9/2020 (cũ) | 4.482 dòng · 0,020–0,027s | **931 dòng · 0,004s** |
| COUNT, lọc tháng hiện tại | 20.448 dòng · 0,008–0,034s | **9.584 dòng · 0,003–0,011s** |
| COUNT, KHÔNG lọc ngày | **2.616.852 dòng** · 0,504–1,360s | 872.284 dòng · 0,211–0,379s |

Chú ý dòng cuối: `COUNT(*)` derived của `pagination.Paginate` đọc **2,6 triệu dòng trên bảng 872k**
— semi-join nhân bảng chính với bảng danh mục. Đây là ca xấu nhất và nó là **ca người dùng bấm
"xoá bộ lọc ngày"**, không phải ca hiếm.

⚠ Đường `IN (<hằng số>)` phải xử lý danh sách RỖNG cho đúng: GORM sinh `IN (NULL)` ⇒ 0 dòng, đúng
bằng ngữ nghĩa subquery rỗng. "Tối ưu" bằng cách bỏ luôn điều kiện khi danh sách rỗng ⇒ report trả
**toàn bộ** lịch hẹn, sai nghiệp vụ mà nhìn vẫn như đang chạy — đã neo bằng test riêng.

⚠ Bảng danh mục ở đây (`appointment_type`) **không có cột `company_id`** nên bản vá giữ nguyên
ngữ nghĩa. Bảng danh mục CÓ `company_id` thì đọc id phải qua `tenant.ScopedDB*`, và lúc đó bản vá
KHÔNG còn là đổi-cách-viết thuần tuý — phải đối chiếu kết quả lại từ đầu.

⚠ `FORCE INDEX (PRIMARY)` để ép quét ngược PK **đã đo và LOẠI**: 1,727s (so với 0,266s nguyên
trạng) — semi-join vẫn buộc materialize rồi sort, ép chỉ mục chỉ làm mất luôn đường ngày.

Áp dụng: `crm/report/handler/appointment_consultation.go` (`buildAppointmentConsultationQuery`).
Cùng lượt đó bỏ `LEFT JOIN customer` khỏi đường list theo mẫu ở mục trên — endpoint này lấy thông
tin khách bằng query thứ 2 theo `customer_id`, join chỉ tồn tại cho `_export=1`.

Mẫu này lặp lại được: `grep -rn "IN (SELECT" --include='*.go' internal/` trước khi kết luận một
list "đã có chỉ mục đúng mà vẫn chậm".

## `COUNT(*)` của `pagination.Paginate` chạy trên MỌI list

```go
db.DB.Raw("SELECT COUNT(*) FROM (?) AS cnt", query.Session(&gorm.Session{})).Scan(&total)
```

Bọc NGUYÊN query — cả danh sách cột lẫn `ORDER BY` lẫn mọi join — vào derived table rồi đếm.
**298 endpoint** dùng `pagination.Paginate`, chỉ 14 dùng `PaginateWithTotal` (nhận total từ ngoài,
bỏ qua COUNT). Mọi chi phí join/filter ở trên bị trả **hai lần** mỗi lần load danh sách.

Handler có dòng tổng (`sumPoint`, `totalTotalEnd`…) gọi builder **lần thứ 2** rồi
`Offset(-1).Limit(-1)` ⇒ **3 lượt quét bảng chính mỗi lần mở màn**, không phải 2. Chỉ đo COUNT
sẽ báo thiếu ~1/3 chi phí thật.

## GROUP BY kèm cột của bảng ĐƯỢC JOIN — khoá sort phình lên VARCHAR

`GROUP BY <bảng>.fk_id, <bảng_join>.name, <bảng_join>.code` nhìn vô hại: hai cột kia **phụ thuộc
HÀM** vào `fk_id` (join theo khoá chính) nên nhóm ra y hệt. Nhưng chúng nằm trong **khoá sắp xếp**
của filesort ⇒ mỗi dòng phải mang theo 2 chuỗi VARCHAR và so sánh chuỗi trên TOÀN BỘ dòng khớp.
Bỏ chúng đi thì khoá sort còn 1 INT — cùng số dòng phải sort, nhưng rẻ hơn hẳn.

Đo `GET /crm/treatment-services/report-service-used` @898.720 `order_service_used`
(782.032 dòng `isLock = 1`), ca người dùng bấm dấu **×** xoá bộ lọc ngày (2026-09-09):

| | Dòng sort | Thời gian |
|---|---|---|
| `GROUP BY service_id, service.name, service.code` | 782.052 | 40,6–49,3s |
| `GROUP BY service_id` | 782.052 | **10,0–13,4s** |

`ONLY_FULL_GROUP_BY` đang BẬT và câu vẫn hợp lệ — MySQL suy ra phụ thuộc hàm qua đẳng thức join
`service.id = order_service_used.service_id`. Đối chiếu 7 bộ lọc: chuỗi ghép TOÀN BỘ nhóm × 13 cột
KHỚP tuyệt đối.

⚠ **Chỉ mục KHÔNG cứu được ca này — đã đo và LOẠI.** `idx_osu_scope_service
(company_id, isLock, service_id, customer_id)` để mong bỏ hẳn filesort: (a) không có chỉ mục
6,4–6,9s · (b) có chỉ mục, optimizer **KHÔNG chọn** (898.720 dòng, sort 782.052, 2m21–2m39) ·
(c) FORCE INDEX 1m35–2m09, VẪN sort 782.052. Lý do: chừng nào GROUP BY còn cột của bảng được join
thì thứ tự chỉ mục của bảng gốc vô dụng; mà khi đã bỏ 2 cột đó rồi thì bản KHÔNG chỉ mục đã đủ
nhanh. Gate (b) ≥ (a) ⇒ không ship, tránh phình bảng.

⚠ **Bỏ `LEFT JOIN` ở câu GROUP BY này cũng KHÔNG phải bản vá — đã đo và LOẠI.** Bỏ
`LEFT JOIN service_type` (chết hoàn toàn ở đường list; `export.ExportTreatmentServices` mới đọc
`service_type.name`) và `LEFT JOIN customer`: số dòng đọc KHÔNG đổi (898.720) và thời gian không
tốt lên (1 join 2m22–2m32 vs 3 join 1m14–1m44). Ở endpoint này chi phí nằm TRỌN trong filesort của
GROUP BY, không ở join — khác hẳn `appointment-consultation` cùng phiên, nơi bỏ join ăn 2x. Đừng
áp máy móc mẫu "bỏ join vô điều kiện" mà không đo.

### `COUNT(*)` của `Paginate` trên câu GROUP BY = đếm NHÓM ⇒ `COUNT(DISTINCT <khoá nhóm>)`

Cùng endpoint, cùng bench. `pagination.Paginate` bọc NGUYÊN câu — 13 cột tổng + `GROUP BY` +
`ORDER BY` — vào `COUNT(*) FROM (…)`, tức dựng lại trọn bảng dẫn xuất chỉ để đếm số dòng của nó.
Con số cần lấy chỉ là **số nhóm**:

| | Dòng đọc | Sort | Thời gian |
|---|---|---|---|
| `COUNT(*)` bọc bảng dẫn xuất (`Paginate`) | 898.720 | 782.568 | 33,0–71,8s |
| `COUNT(DISTINCT service_id)` + `PaginateWithTotal` | 898.720 | **0** | **1,77–1,88s** |

⇒ 19–40×. Đối chiếu số nhóm trên 7 bộ lọc: 536/131/0/293/1/0/371 — khớp tuyệt đối.

⚠ Câu `COUNT(DISTINCT …)` phải dựng từ CÙNG builder (giữ nguyên join + bộ lọc), không phải viết
tay bản rút gọn: bộ lọc `source_id` đi qua `customer`, `service_type_id` đi qua `service`. Các
join này đều theo khoá chính nên không nhân dòng ⇒ số nhóm không đổi (đã đối chiếu ở ca `source`).

### Hai câu tổng chạy riêng trên cùng bộ lọc — gộp một lượt

Handler này chạy `totals` (9 hàm SUM/COUNT) rồi chạy tiếp `COUNT(DISTINCT customer_id)` thành câu
thứ hai. Cùng bảng, cùng bộ lọc, không GROUP BY ⇒ gộp được vào một lượt quét:

| | Thời gian |
|---|---|
| 2 câu riêng (`totals` 10,0–11,9s + `customerCount` 5,8–16,5s) | 15,8–28,4s |
| gộp 1 câu | **2,3–2,8s** |

⚠ Bẫy khi gộp: `customerCount` là số khách **PHÂN BIỆT trên toàn bộ bộ lọc**, KHÔNG phải tổng
`customerCount` của từng nhóm cộng lại (khách mua 2 dịch vụ bị đếm 2 lần). Đã neo bằng test có
phân bố cố ý: 2 khách × 2 dịch vụ ⇒ đúng là 2, sai là 3.

**Tổng kết một lần mở màn (ca không lọc ngày, @898.720 dòng):** ~90–150s → **~14–18s**.

### …và vì ~14–18s vẫn không dùng được, endpoint này có TRẦN DẢI NGÀY (user chốt 2026-09-09)

Ba câu còn lại vẫn quét trọn bảng — đây là aggregate toàn bảng, không bộ lọc nào chặn trên. Nên
`ReportServiceUsed` nay bắt buộc có `created_day` gte/lte và chặn trần `define.ReportMaxRangeSec`
(366 ngày), cùng khuôn với `report/appointment-statistics`. Vượt trần ⇒ **422 tường minh**, KHÔNG
tự cắt bớt dải ngày (cắt im lặng = trả thiếu số liệu mà người xem báo cáo không có cách nào biết).

Ba chi tiết dễ làm sai, đều đã neo bằng test:

- **Guard phải đặt TRƯỚC nhánh `_export=1`** — export chạy đúng câu này qua con trỏ, không có lý
  do được miễn trần. Mutation "miễn trần cho export" ⇒ test đỏ đúng ca đó.
- **Đọc được CẢ HAI khuôn param**: `filter[created_day][gte]` (field thường) và `created_day[gte]`
  (khuôn field tiền tố `_`) — `query.getOperatorValue` chấp nhận cả hai, guard không được hẹp hơn
  chính bộ lọc nó gác, nếu không caller dùng khuôn thứ hai sẽ ăn 422 oan.
- **Phải sửa kèm nút xoá ngày ở FE**: `clearCreateDay()` trước đây set `null` ⇒ sau khi có guard
  chỉ đổi một màn treo lấy một toast 422. Nay reset về mặc định (đầu tháng → hôm nay).

⚠ Rà consumer khi thêm guard: `internal/contracttest` có case `crm_report_service_used` gọi
endpoint KHÔNG kèm dải ngày ⇒ 422 ⇒ case decimal không bao giờ chạm tới response thật. Đã vá.

## `LIKE '%kw%'` xuyên join = quét toàn bảng

`query.ApplyKeywordFilter` sinh `col LIKE '%kw%'` cho từng cột. Dấu `%` đứng đầu ⇒ không index nào
dùng được. Với `*KeySearch` trỏ sang bảng đã join, MySQL quét toàn bộ bảng chính theo thứ tự
`ORDER BY`, join bảng kia từng dòng, lọc `LIKE` tới khi đủ `LIMIT`:

| | 524k dòng |
|---|---|
| từ khoá CÓ khớp | 1351ms |
| từ khoá hiếm / KHÔNG khớp | **6814ms** |

⚠ Ca xấu là ca thường: `SELECT` thoát sớm khi đủ 15 dòng, nhưng **`COUNT` của pagination không bao
giờ thoát sớm được** ⇒ mọi lần gõ tìm kiếm đều trả giá quét toàn bảng.

Repo có **127 mục `*KeySearch` trỏ `customer.*`** — cùng một mẫu, chờ áp cùng bản vá.

## ⚠ ngram KHÔNG phải lúc nào cũng thắng — nó THUA `LIKE` ở từ khoá toàn chữ

Phần trên trình bày ngram như bản vá; đây là chiều ngược lại, bắt buộc đọc trước khi áp cho một ô
tìm kiếm mới. Âm tiết tiếng Việt dùng chung bigram (`ng`, `uy`, `th`…) nên posting list của một
cụm toàn chữ gần bằng cả bảng ⇒ phrase search phải giao các danh sách khổng lồ. Chữ số thì ngược
lại: entropy cao, cực chọn lọc.

Đo trên 615.992 khách (2026-09-07), TỔNG một lần mở danh sách (SELECT trang 1 + COUNT):

| từ khoá | ký tự | `LIKE` | ngram | |
|---|---|---|---|---|
| `Nguy` | 4 | 1,206s | 5,353s | ngram thua 4,4× |
| `Nguyễn` | 6 | 1,483s | 2,597s | ngram thua 1,8× |
| `KH4` · `091` | 3 | 1,25–1,46s | 1,67–1,93s | ngram thua |
| `0913` · `KH40` | 4 | 1,12–1,63s | **0,455–0,646s** | ngram thắng 2,5× |
| `0913353112` | 10 | 2,594s | **0,403s** | ngram thắng 6,4× |
| gõ sai, không khớp | 9 | 2,596s | **0,214s** | ngram thắng 12× |

⛔ **Không dò trước được độ chọn lọc để rẽ nhánh**: InnoDB FULLTEXT KHÔNG thoát sớm theo `LIMIT` —
probe `MATCH … LIMIT 10001` cho `"Nguy"` vẫn tốn 1,87s, tức bản thân phép dò đã đắt hơn cả đường
`LIKE` mà nó định thay.

⇒ Cổng đã đo (`crm/customer/handler/list_search.go`): đi ngram khi từ khoá **có ít nhất 1 chữ số VÀ
≥4 ký tự**; ngoài cổng giữ nguyên `LIKE` ⇒ không ca nào chậm đi. Biên 4 ký tự là số đo, không phải
ước lượng: 3 ký tự thua, 4 ký tự thắng. Nới cổng (vd để ăn ca "gõ sai tên") PHẢI bench lại ≥500k
dòng trước — bảng trên nói rõ nó kéo theo `"Nguyễn"` chậm 1,8×.

## Cách sửa tìm kiếm đã đo — và 4 biến thể đã loại

Cách DUY NHẤT nhanh: **tra id ra trước ở Go** (FULLTEXT ngram) rồi lọc bằng danh sách literal —
MySQL dùng index_merge.

⚠ Chỉ áp cho cột nằm ở bảng **được JOIN**. Cột của **chính bảng chính** giữ `LIKE` inline:
bảng chính dù sao cũng phải quét một lượt cho scope + range ngày, tra thêm FULLTEXT rồi
materialize id là việc thừa.

```go
// custIDs ← MATCH(...) AGAINST('"kw"' IN BOOLEAN MODE) trên customer
// apptIDs ← MATCH(code) AGAINST('"kw"' IN BOOLEAN MODE) trên appointment
q.Where("appointment.customer_id IN ? OR appointment.id IN ?", custIDs, apptIDs)   // 36ms
```

| Biến thể ĐÃ THỬ VÀ LOẠI — đừng làm lại | |
|---|---|
| `MATCH(code) … OR customer_id IN (SELECT … MATCH …)` | 1811–2968ms (FULLTEXT trong `OR` ⇒ tụt về quét bảng) |
| `id IN ((MATCH …) UNION (SELECT …))` | 3601ms |
| Đổi join thành `IN (subquery)` nhưng vẫn `LIKE` | 1520ms |
| `customer_id IN (SELECT … MATCH …) OR code LIKE 'kw%'` | 1108ms |
| Ngram trên cột `note` của CHÍNH bảng chính để cả 5 vế đều `IN (...)` | COUNT 5,8s / 6,4s — so với 1,6–2,5s khi để `LIKE` inline (594k dòng) |
| Cap danh sách id cho cột text tự do của bảng chính | từ khoá thường gặp khớp >10.000 dòng ⇒ luôn vượt trần ⇒ luôn rơi về đường chậm |

Ràng buộc bắt buộc của đường nhanh:
- Vượt trần id (`keywordIDCap`) ⇒ **rơi về đường `LIKE` cũ**, KHÔNG cắt bớt danh sách (cắt bớt = trả
  thiếu kết quả im lặng).
- Từ khoá < `ngram_token_size` (mặc định 2) ⇒ rơi về `LIKE`.
- Cả hai danh sách rỗng ⇒ phải `Where("1 = 0")`; bỏ qua điều kiện = trả về toàn bộ như không tìm gì.
- Lỗi câu `MATCH` (vd 1191 thiếu chỉ mục) ⇒ rơi về `LIKE` — nhờ đó `DROP INDEX` là cách lùi an toàn.
- Cột `id` kiểu INT không ngram được ⇒ giữ `LIKE` riêng, chỉ chạy khi từ khoá toàn số.
- **Danh sách id dài hơn `eq_range_index_dive_limit` (mặc định 200) làm MySQL BỎ index.** Quá trần,
  optimizer ngừng index-dive và ước lượng theo thống kê ⇒ chọn quét toàn bảng rồi lọc `IN` trong bộ
  nhớ. Đo: `customer_id IN (8.333 id)` trên 524k dòng ⇒ plan `Index lookup … company_id … rows=259349`
  + `Filter`, 1,36s cho COUNT và 1,36s cho tra id — trong khi cùng câu đó với danh sách ngắn là ~0ms.
  `keywordIDCap` chặn được "trả thiếu im lặng" nhưng KHÔNG chặn được cái này (trần đang là 10.000).

Bẫy correctness của chính chỉ mục ngram: xem `patterns-gotchas.md` §FULLTEXT `ngram`.

## Cost model chấm SAI ref-lookup trên `company_id` — bẫy riêng của DB-per-customer

> ⚠ Lịch sử (D15): schema mới không còn `company_id` — mục này chỉ giải thích số đo cũ, không áp dụng cho CRM standalone.

Ở mô hình 1 DB = 1 khách, **mọi dòng có cùng `company_id`**. Một index UNIQUE dẫn đầu bằng
`company_id` (vd `appointment.code = UNIQUE(company_id, code)`) vì thế cho ref lookup khớp
TOÀN BẢNG — nhưng cost model của MySQL chấm ref lookup rẻ hơn range scan và chọn nhầm:

```
-> Index lookup on a using code (company_id = 1)   (cost=3397  rows=260870)   <- chọn
-> Index range scan on a using idx_appointment_company_date (cost=36445 rows=80988)  <- đúng
```

Điều kiện kích hoạt: query chỉ có `company_id = ?` + range ngày, và đọc **một cột nằm ngoài**
index composite ⇒ mất tính covering ⇒ cost range scan phồng lên đủ để thua ref lookup giả rẻ.

Cách chữa đã đo (threshold 56 ngày của `appointment-schedule-bydoctor`, 524k dòng):

| | 524k dòng |
|---|---|
| Nguyên trạng (optimizer chọn `code`) | 750–1490ms (median ~1050ms) |
| `FORCE INDEX (idx_appointment_company_date)` | 130–240ms |
| **Thêm cột thiếu vào ĐUÔI index composite ⇒ covering** | **30–40ms** |

⇒ Chữa bằng **cột phủ**, không bằng `FORCE INDEX`: nhanh hơn 5x nữa, và hint cứng trong code Go
sẽ vỡ im lặng nếu index bị đổi tên. Chi phí: `+doctorAppointment_id` vào đuôi
`idx_appointment_company_date` = +2MB / 524k dòng (199→201MB, ~+1%), không thêm secondary index
mới. Migration: `20260906222500_extend_appointment_company_date_index_doctor.up.sql`.

⚠ Trước khi kết luận "index này đã đủ", đọc `EXPLAIN FORMAT=TREE` xem có chữ **`Covering`** hay
không. `Index range scan` mà thiếu `Covering` nghĩa là còn fetch row — và ở ngưỡng nào đó
optimizer sẽ bỏ nó để quay về ref lookup giả rẻ ở trên.

### Khi thủ phạm là UNIQUE — không nới đuôi được, phải ĐẢO THỨ TỰ CỘT

Cách chữa "thêm cột phủ vào đuôi composite" ở trên chỉ dùng được khi index thủ phạm là index
thường. Thủ phạm là **UNIQUE** (`customer.id_old = UNIQUE(company_id, id_old)`) thì nới đuôi =
đổi ràng buộc, không làm được. Và **thêm một composite phủ MỚI cũng vô ích** — optimizer vẫn bám
UNIQUE (đo 615.992 khách, 2026-09-07: `(company_id,status,branch_id)` và `(status,company_id,branch_id)`
đều để nguyên 1,84s).

Cách ăn: **đảo thứ tự cột của chính UNIQUE đó** để `company_id` không còn dẫn đầu. Ràng buộc unique
KHÔNG đổi (vẫn unique trên đúng cặp cột); chỉ mất đường tra theo `company_id` đơn lẻ — đúng thứ cần
chặn. `customer`: **1,472s → 0,170s**, `EXPLAIN` chuyển sang `Covering index scan`.

⚠ **Thứ tự hai việc không đảo được:** UNIQUE đó có thể đang gánh chỉ mục cho FK `company_id`. Phải
`ADD` composite phủ TRƯỚC, đảo UNIQUE SAU — ngược lại là `ERROR 1553 … needed in a foreign key
constraint` (chi tiết: `writing-mysql-migrations/reference/migration-idempotent.md`).

⚠ Index thôi CHƯA đủ: `pagination.Paginate` bọc nguyên query (danh sách cột + join) vào derived
table nên COUNT mất tính covering. Phải tách bộ lọc khỏi phần hiển thị rồi `PaginateWithTotal`.
Chỉ đổi index mà giữ `Paginate` chỉ xuống 0,839s; đủ cả hai mới 0,170s.

Phạm vi còn lại của cùng bệnh: **34 UNIQUE dẫn đầu `company_id` trên 25 bảng** — `appointment` (×2),
`order_service_used`, `order_treatment_plan`, `payment`, `treatment_course`, `service`…

### Quét toàn repo 2026-09-09 — điều kiện kích hoạt cái bẫy này HẸP hơn tưởng

Đã soi HẾT: **33 chỗ** viết `IN (SELECT` trong nguồn (`grep -rn "IN (SELECT" --include='*.go' internal/ pkg/`)
cộng **80 chỗ** `IN (?)` (GORM sinh subquery lúc chạy — grep nguồn KHÔNG thấy). Phân loại:

| Nhóm | Số | Kết luận |
|---|---|---|
| `LEFT JOIN (SELECT …)` — derived table, grep bắt nhầm | 3 | không liên quan |
| Comment/tài liệu | 9 | — |
| Cascade `DELETE`/`UPDATE` khi XOÁ khách (`customer/service/delete.go`, `inventory/service/material_hooks.go`) | 14 | một phát, `WHERE <fk> IN (SELECT id FROM <bảng> WHERE customer_id = ?)` — chọn lọc theo 1 khách, đo 263 dòng đọc. KHÔNG đụng |
| Đường ĐỌC, đã đo từng cái | 9 | 2 chỗ có nút thắt (đã vá), 7 chỗ KHÔNG |
| `IN (?)` với đối số là **slice Go** (không phải subquery) | ~70 | là hằng số sẵn, an toàn |
| `IN (?)` với đối số là `*gorm.DB` (subquery thật) | ~10 | đo 3 nhóm đại diện, không chỗ nào có nút thắt |

**Điều kiện để cái bẫy nổ — phải có ĐỦ hai vế:**

1. Bảng trong subquery **NHỎ** (bảng danh mục: `appointment_type` 7 dòng). Bảng LỚN thì cost model
   không bao giờ chọn nó làm bảng dẫn — đã đo: `customer_id IN (SELECT id FROM customer …)` ở
   615.992 khách luôn cho `Single-row index lookup on customer using PRIMARY`, tức MySQL tự chọn
   đúng. Đây là lý do 7/9 chỗ đường-đọc không sao.
2. Điều kiện danh mục **CHỌN LỌC** trên bảng chính (6,6% dòng khớp). Khớp gần hết thì optimizer
   vẫn quét ngược PK và thoát sớm — đo `service_material.service_id IN (SELECT id FROM service …)`
   @193.800 dòng: `Handler_read_prev=19`, **0,001–0,002s**, không cần sửa.

**Số đo của 7 chỗ ĐÃ ĐO VÀ KHÔNG CÓ VẤN ĐỀ** (đừng bench lại) — bench 615.992 khách / 872.284 lịch
hẹn / 596.680 `order_treatment_plan` / 372.504 `payment` / 787.696 `treatment_course` /
449.360 `order_service_used` / 167.246 `hr_payroll`:

| Chỗ | Số đo | Vì sao không sao |
|---|---|---|
| `accountant/dashboard` `hrPayrollScope` (×2) | 3–6ms, đọc thêm 324 dòng | `hr_payroll` có **UNIQUE(user_id, month)** ⇒ chặn trên bằng *nhân viên × tháng*; lọc `month = ?` còn ≤ số nhân viên. `user` nhỏ, `JSON_CONTAINS` không index được nhưng chỉ quét 323 dòng |
| `report_payment_by_date` nhánh `appointment` | 98.404 dòng đọc — **y hệt** cả 3 cách viết | chi phí nằm ở range ngày trên `appointment`, subquery không đổi plan |
| `report_payment_by_date` nhánh `payment` | 372.504 dòng — y hệt cả 3 cách viết | plan là `Index lookup on payment using code (company_id = 1)` (bẫy ref-lookup ở mục trên), không phải semi-join |
| `ai/service/segment_trend_benchmark` cohort | 787.696 dòng, warm 0,30–0,39s — y hệt `IN`/`JOIN`/`EXISTS` | aggregate GROUP BY toàn bộ `treatment_course` của 1 chi nhánh: chi phí là bản chất câu hỏi, đổi cách viết không mua được gì. ⚠ cache lạnh 5–32s — nợ RIÊNG, không thuộc lớp bug này |
| `crm/customer_detail` `treatment_plan_id IN (SELECT … WHERE appointment_id = ?)` | 79 dòng đọc | nằm trong một vế `OR` ⇒ không thể thành semi-join; ngoài ra đã có `customer_id = ?` chặn trên |
| `crm/customer` audit trước khi xoá (`payment.serviceUsed_id IN (…)`) | 263 dòng, `IN` = `JOIN` | chọn lọc theo 1 khách |
| `config/service_material` list (`service_id IN (?)` qua GORM) | SELECT 0,001–0,002s (`Handler_read_prev=19`); COUNT 194.446 dòng/0,17–0,21s | thiếu vế (2) ở trên. COUNT quét hết là chi phí của `Paginate`, không phải subquery |

### Chỗ THỨ HAI thật sự có nút thắt — và nó nổ theo kiểu KHÁC

`report_payment_by_date.go` + bản song sinh `export/report_revenue_by_date.go`: bộ lọc `source_id`
sinh `<bảng>.customer_id IN (SELECT id FROM customer WHERE source_id IN (…))`. Ở nhánh
`order_treatment_plan`, MySQL **materialize tập khách rồi hash-join** ⇒ bảng chính mất range scan
theo ngày:

| Nhánh `order_treatment_plan`, lọc 1 tháng @596.680 dòng | Dòng đọc | Thời gian |
|---|---|---|
| `customer_id IN (SELECT …)` | `Table scan` **596.680** | 429–622ms (cache lạnh **10.546ms**) |
| `EXISTS (SELECT 1 FROM customer c WHERE c.id = otp.customer_id AND …)` | `Index range scan … idx_otp_company_created_time` **29.816** | 230–271ms |

⚠ **Cái đắt nhất không phải 20× dòng đọc mà là plan DAO ĐỘNG**: chạy `ANALYZE TABLE` rồi đo lại
3 vòng, cách `IN (SELECT …)` rơi vào table scan **2/3 lượt**, ra range scan 1/3 lượt. Cách `EXISTS`
ra range scan **11/11 lượt**. Một câu query lúc nhanh lúc chậm gấp 20 lần là thứ không debug được
từ log.

⚠⚠ **`EXPLAIN FORMAT=TREE` (không ANALYZE) in ra cây GIỐNG HỆT NHAU cho hai cách viết** — chỉ
`EXPLAIN ANALYZE` mới lộ khác biệt (nó thực thi thật). Đừng kết luận "hai cách như nhau" từ
`EXPLAIN` thường; đây là lần thứ nhất trong repo bắt được `EXPLAIN` nói dối kiểu này.

Đối chiếu kết quả: 7 bộ lọc source × 3 nhánh (appointment · order_treatment_plan · payment), cả
số dòng lẫn tổng tiền KHỚP tuyệt đối. Hai nhánh `appointment`/`payment` đo ra TRUNG TÍNH nhưng vẫn
đổi cùng khuôn để chỉ có MỘT điểm quyết định. Alias dùng `src_c` (không đụng alias nào có sẵn).
4 unit test thuần (không cần DB) neo khuôn, mutation-verified.

### Dashboard Kế toán — cột phủ SẴN CÓ vẫn bị bỏ, và cột không có chỉ mục nào

`/accountant/accountant-dashboard/{kpi,revenue-trend,receivable,alerts}` (/check-scale 2026-09-14).
Bench dựng TỪ MIGRATION: 616.000 khách (6.160 đang nợ) / 600.000 payment / 650.000 osu / 600.000
`order_installment` / 60.000 `stock_entry` / 18.000 `hr_payroll`. Đo trên máy load 4–19 ⇒ kết luận
dựa trên dòng đọc của `EXPLAIN ANALYZE`, thời gian chỉ tham khảo.

**Trước vá, cache lạnh** (toàn chi nhánh / 1 chi nhánh): kpi **1.880** / 166ms · revenue-trend 6T
674 / 154ms · **receivable 3.986** / 420ms · alerts 95 / 85ms. Người test 1 chi nhánh KHÔNG thấy gì.

**Nút thắt 1 — `customer.totalDebt` không nằm trong chỉ mục nào.** `SUM(totalDebt) WHERE totalDebt > 0`
và `ORDER BY totalDebt DESC LIMIT 10` tra `idx_customer_scope_status (company_id…)` ⇒ **616.000/616.000
dòng** để lấy 6.160 dòng, 1,85s MỖI câu, chạy **3 lần/lần mở trang** (kpi 1 + receivable 2). Vá:
`idx_customer_total_debt (totalDebt, company_id, branch_id)` — migration `20260914131225`.

| | Trước | Sau |
|---|---|---|
| SUM toàn chi nhánh | 1.854ms / 616.000 | **1,6ms / 6.160** (`Covering index range scan`) |
| TOP10 toàn chi nhánh | 1.895ms | **0,10ms** (range scan ngược, dừng ở dòng 10) |
| SUM 1 chi nhánh | 122ms / 36.235 | 1,3ms |
| Gate FORCE INDEX | — | tự chọn = ép dùng, cùng plan ⇒ ĐẠT |
| Đối kháng 1/3 khách có nợ (204.160) | 3.479ms | toàn CN **60ms**; 1 CN optimizer tự lui plan cũ 109ms = đường cũ |
| Giá ghi UPDATE 2.054 khách theo PK | 3,28–3,44s | 3,29–3,88s (trong nhiễu) · +19,6MB/616k dòng |

⚠ Đo giá ghi bằng `WHERE id % 300 = 7` cho 5,0–5,5s CẢ HAI bên — chi phí quét bảng lấn át hoàn toàn
phần bảo trì chỉ mục. Phải UPDATE theo dải PK mới đo được đúng thứ cần đo.

**Nút thắt 2 — NGOẠI LỆ của luật "chữa bằng cột phủ, không bằng FORCE INDEX" ở trên.** Câu trend
nhiều tháng (`payment_time BETWEEN <6–12 tháng> AND payment_for IN (4,5)`) đã CÓ SẴN chỉ mục phủ
đúng hình dạng `idx_payment_company_time (company_id, payment_time, payment_for, deposit_type,
branch_id, amount)` — optimizer VẪN chọn `code (company_id)` và đọc 600.000 dòng. Thêm một chỉ mục
phủ MỚI `(payment_time, payment_for, company_id, branch_id, amount)` cũng bị bỏ y hệt (auto 583ms,
ép 69ms). Khác ca `appointment-schedule-bydoctor`: vế scope ở đây là `(company_id = ? OR company_id
IS NULL)` ⇒ optimizer chấm ref_or_null trên `code` rẻ hơn. Cột phủ không cứu được ⇒ `FORCE INDEX`
CÓ CỔNG (`paymentTrendIndexHint`, chỉ khi `branchActiveId = 0`):

| payment_time 12 tháng | auto | FORCE `idx_payment_company_time` |
|---|---|---|
| 17 chi nhánh (IN) | 561ms / 600.000 | **105ms** / 287.256 mục phủ |
| 8 chi nhánh (IN) | 545ms | **86ms** |
| 2 chi nhánh (IN) | 741ms | **190ms** |
| **1 chi nhánh (`=`)** | **43ms** (`fk-payment-branch_id`) | 73ms — TỆ HƠN ⇒ cổng tắt hint |
| 1 tháng (câu KPI), toàn CN | 5,8ms (tự chọn `idx-payment-payment_time`) | 3,9ms — không cần hint |

Đối chiếu 6 tháng × toàn CN + `payment_for=8` × 1 CN: số dòng và tổng tiền khớp tuyệt đối.
⚠ Rủi ro của hint cứng: chỉ mục bị đổi tên/xoá ⇒ Error 1176, dashboard 500. Test
`TestPaymentTrend_*` neo tên trong SQL — ai đổi migration chỉ mục đó phải sửa cả hằng
`paymentCompanyTimeIndex`.

**Sau vá (spike gọi thẳng handler, cache lạnh)** toàn chi nhánh / 1 chi nhánh: kpi **63** / 63ms ·
revenue-trend 6T 151 / 129ms · 12T 270 / 132ms · **receivable 361** / 323ms · alerts 158 / 140ms.
Phần còn lại là `order_installment` (table scan 600k, 145–211ms) — không phải nút thắt, chi phí O(số
kỳ trả góp), DB dev hiện 0 dòng.

**Đã đo và KHÔNG có vấn đề** (đừng bench lại): `stock_entry` 42–52ms @60.000 (không chỉ mục
`created_at`, DB dev 0 dòng) · `hr_payroll` 2–10ms @18.000 · câu "đến hạn 7 ngày" 13–24ms.

### Công nợ khách hàng — chỉ mục đúng đã có, câu TRANG vẫn cược PRIMARY và thua

`GET /accountant/customer/get-customer-debts` (+ `?_export=1`), /check-scale 2026-09-16 (2 lượt). Bench
dựng TỪ MIGRATION, 524.288 khách, tên/SĐT/chi nhánh chép từ dev. Hai phân bố: **thật 0,72%** khách nợ
(= DB dev 554/76.999 ⇒ 3.771) và **đối kháng 1/3** (177.277). SQL lấy từ spike gọi handler
(`spike/custdebts_scale`). Code: `accountant/customer_debts/handler/handler.go`.

**3 cơ chế độc lập, cùng hiện ra ở một endpoint:**

1. **Câu trang cược PRIMARY và thua.** `ORDER BY customer.id DESC LIMIT 20` quét ngược khoá chính mong đủ
   20 dòng; khách nợ hiếm nên thua mỗi khi bộ lọc loại gần hết (phân bố thật): 1 CN lọc ngày hẹp
   **524.288 dòng / 696–705ms**, từ khoá không khớp 669–719ms. Không cần JOIN để thua — chỉ cần predicate
   hiếm. Ép `idx_customer_total_debt`: 408 / 1,7ms.
2. **Câu tổng rơi vào bẫy ref-lookup `company_id`** khi nhiều khách nợ (đối kháng, toàn CN): tra
   `idx_customer_scope_status (company_id = 1)` 1.350ms. Chỉ mục nợ cũ không phủ 2 cột tiền ⇒ nới thành
   `(totalDebt, company_id, branch_id, totalRevenue, totalPayment)` (migration `20260916235911`, giữ tên) ⇒
   covering range scan **72ms**; phân bố thật 30,6 → 1,7ms. Giá ghi UPDATE 2 cột tiền 2.000 khách theo PK:
   3.815/3.831 → 3.899/3.910ms (+~2%; bảng `customer` vốn nặng vì FK con).
3. **Đọc cột rộng cho mọi khách nợ chỉ để sắp xếp.** Câu trang ép chỉ mục vẫn tra dòng đủ 12 cột cho
   3.771 khách rồi bỏ 3.751. Tra id trước (chỉ mục nợ + PK ngầm ⇒ covering) rồi nạp ≤20 dòng theo PK: toàn
   CN trang 1 29,7 → 1,4ms, trang 150 32,5 → 1,8ms.

**Kế hoạch trong handler** (D = số khách nợ trong phạm vi):

| Request | D | Câu tổng | Câu trang |
|---|---|---|---|
| không từ khoá, không ngày | (= COUNT, không câu dò) | ép, covering | tra id covering — kể cả D lớn |
| có từ khoá / ngày | ≤ 10.000 | **1 lượt** stream id + 3 cột tiền, COUNT/SUM/cắt trang ở Go | (cùng lượt) |
| có từ khoá, không ngày | > 10.000 | **1 lượt** như trên | (cùng lượt) |
| có lọc ngày | > 10.000 | để optimizer (chỉ mục ngày) | để optimizer |
| COUNT ≤ offset | | | không chạm DB |

Câu dò `COUNT(*) FROM (SELECT 1 … FORCE INDEX … LIMIT 10001)` covering 1,0–1,5ms; PHẢI ép (không ép:
1 CN rơi vào `fk-customer_branch_id` 55.606 dòng / 255ms). Câu dò lỗi hoặc câu tổng lỗi 1176 (chỉ mục bị
xoá) ⇒ lùi về đường cũ. Tra id covering kể cả khi D lớn vì đường cược dao động 0,1ms–1,5s còn tra id có
trần: đối kháng 1 CN trang 1 cược 149ms / tra id 40ms · toàn CN trang 1 0,1ms / 55ms · offset 100.000
**1.500ms / 115–123ms**. Ngày giữ cổng vì đối kháng + ngày hẹp: optimizer dùng chỉ mục ngày 1,6ms, ép chỉ
mục nợ 556ms.

**Biến thể LOẠI có số đo:**

| Cách | Số đo | Vì sao loại |
|---|---|---|
| `FORCE INDEX` vô điều kiện cho câu trang | đối kháng toàn CN không lọc 0,1 → 578ms | nhiều khách nợ thì thảm hoạ |
| `IGNORE INDEX FOR ORDER BY (PRIMARY)` | đối kháng 1.256–1.329ms | lùi về ref-lookup `idx_customer_scope_status` |
| + `IGNORE INDEX (idx_customer_scope_status)` | 960–1.034ms | bẫy chuyển sang `idx_customer_company_created_day` — đuổi bắt vô tận |
| từ khoá: câu tổng + câu trang 2 lượt riêng | đối kháng từ khoá hiếm toàn CN 1.180ms | gộp 1 lượt stream: 586ms |
| Cổng câu trang theo `COUNT ≤ offset+perPage` cho từ khoá | — | thay bằng 1 lượt, nhánh không còn đường tới |

**Endpoint, spike, lượt ấm — HEAD → bản cuối.** Phân bố thật:

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| không lọc | 36,5 → **3,2ms** | 6,4 → 4,1ms |
| lọc ngày hẹp | **635 → 3,2ms** | 12,2 → 13,9–15,1ms |
| "Nguyễn" | 322–329 → 4,2ms | 82 → 17ms |
| `0913` / mã KH | 620–625 → 3,4–3,7ms | 302–652 → 14–15ms |
| từ khoá không khớp / hiếm | **620–624 → 3,0–3,4ms** | **654–663 → 13,4ms** |
| trang 150 / 5000 | 3,5–3,8 → 1,4ms | 16 → 1,9–5,4ms |

Đối kháng (177.277 khách nợ):

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| không lọc | 331 → **80ms** | **1.500 → 110ms** |
| trang 5000 | 307 → 42ms | **3.039 → 161ms** |
| "Nguyễn" / `0913` | 311–316 → 122–127ms | 1.594–1.607 → 599–625ms |
| từ khoá hiếm / không khớp | 309–311 → 118–120ms | **2.200–2.271 → 585–587ms** |
| lọc ngày hẹp | 12,2 → 14,7ms | 9,6 → 13,5ms |
| toàn CN + `filter[branch_id]` 1 chi nhánh | — | 24 → **68ms** ⚠ chậm hơn |
| … + từ khoá không khớp | — | 46 → **59ms** ⚠ chậm hơn |

Export 1 CN không lọc 170ms / 55.606 dòng → 6,5ms / 408. Body sha1 khớp HEAD **48/48** (2 phân bố × 2 chế
độ chi nhánh × 12 ca); refactor cuối khớp lượt trước 24/24. 8 test (DryRun SQL + hàm thuần), mutation
**6/6**. Migration thử trên DB rỗng: dựng từ đầu · nâng từ dạng cũ · chạy lại no-op · thiếu hẳn chỉ mục.

**Còn lại (đều CHỈ ở phân bố đối kháng, không vá được mà không đổi hình dạng chỉ mục):**
- Toàn CN + lọc 1 chi nhánh chậm đi 24 → 68ms: `branch_id` đứng thứ 3 trong chỉ mục nợ nên range quét
  mọi mục khách nợ (177k) rồi mới lọc chi nhánh. Chỉ mục dẫn đầu `branch_id` sẽ phá TOP10/SUM toàn chi
  nhánh của Dashboard Kế toán ⇒ không đổi.
- Từ khoá toàn CN còn ~600ms: LIKE phải tra 177k dòng. Đường ngram tra id khách (`pkg/query/ngram.go`)
  là hướng tiếp — CHƯA đo, và theo §"ngram KHÔNG phải lúc nào cũng thắng" nó thua ở từ khoá toàn chữ.

### Công nợ đơn hàng — 5 bảng dẫn xuất gom TOÀN BẢNG, chạy 2 lần; câu trang tính lại mọi đơn chỉ để lấy 20

`GET /accountant/order-plan/get-order-plan-debts` (+ `?_export=1`), /check-scale 2026-09-17. Bench dựng TỪ
MIGRATION: 600.000 đơn / 900.000 dịch vụ / 375.000 phiếu thu / 600.000 khách — tỉ lệ theo DB dev (1,5 dịch
vụ/đơn, 0,62 phiếu thu gắn dịch vụ/đơn). SQL lấy từ spike gọi handler (`spike/opdebts_scale`).

**Cơ chế.** Mỗi đơn cần 3 con số trong kỳ [F, T]: phải thu đầu kỳ (dịch vụ tạo < F trừ đã thu < F của chính
các dịch vụ đó), doanh thu trong kỳ, đã thu trong kỳ (mọi dịch vụ). Bản cũ viết thành 3 bảng dẫn xuất
`GROUP BY treatment_plan_id` trên **toàn bộ** `order_service_used` (một cái không có WHERE nào), mỗi cái JOIN
một bảng dẫn xuất gom `payment` riêng, rồi `LEFT JOIN` cả 3 vào `order_treatment_plan` + `GROUP BY` +
`HAVING`. Toàn khối chạy **2 lần** (câu tổng bọc nó + câu trang). Bộ lọc NGÀY không giảm việc gì: phải thu
đầu kỳ cần cả lịch sử ⇒ 1 ngày / tháng / 1 năm đều 13–15s.

**Bản vá (`order_plan_debts/handler`, hàm `debtQueries`):**
- **Câu tổng đi từ đơn TRONG PHẠM VI** → dịch vụ (fk `treatment_plan_id`) → phiếu thu qua
  `LEFT JOIN LATERAL` (1 lượt, 2 SUM có điều kiện theo `payment_of_date`), gom theo đơn. Phiếu thu phải gom
  theo TỪNG dịch vụ trước — JOIN thẳng sẽ nhân `totalEnd` khi dịch vụ có nhiều phiếu.
- **Câu trang tính từng đơn bằng `JOIN LATERAL` tương quan theo đơn**, duyệt `id DESC` ⇒ dừng khi đủ trang.
- COUNT ≤ offset ⇒ không chạm DB; `customer` chỉ JOIN khi có từ khoá (và export); câu tổng lỗi không còn
  bị nuốt. Export dùng chung câu trang (cột `A.*`).

**Tầng SQL** (1 CN = chi nhánh 1, 35.294 đơn nợ; toàn CN = 600.000):

| | Cũ | Mới |
|---|---|---|
| Câu tổng 1 CN | 6,5s | **0,41s** |
| Câu tổng toàn CN | 10,3s | 5,3–5,9s (bản chất phải tính mọi đơn) |
| Trang 1 toàn CN | 5,92s (gom + sắp xếp) | **0,03s** |
| Trang 201 / trang cuối toàn CN | 5,48 / 6,17s | 0,25 / 3,28s |

**Biến thể LOẠI có số đo:**

| Cách | Số đo | Vì sao loại |
|---|---|---|
| Gộp 3 bảng dẫn xuất thành 1 lượt gom theo dịch vụ trên toàn bảng rồi mới lọc phạm vi | 1 CN 4,8s, toàn CN 6,0s | vẫn gom 600k nhóm cho 35k đơn cần |
| 1 lượt + `COUNT/SUM OVER ()` để tổng và trang chung 1 lần chạy | toàn CN **18s** | sắp xếp 600k dòng rộng — tệ hơn 2 lượt riêng |
| Chỉ mục phủ `osu (treatment_plan_id, created_day, totalEnd)` + `payment (serviceUsed_id, payment_for, company_id, payment_of_date, amount)` | 1 CN 0,41 → 0,33s, toàn CN 5,9 → 5,3s | ~20%, chìm trong nhiễu; 2 bảng ghi liên tục |

**Endpoint, spike, HEAD → bản vá:**

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| tháng này (mặc định FE) | **14,7s → 369ms** | 16,3s → 5,1s |
| 1 ngày / 1 năm | 13,2 / 14,6s → 371ms | 16,2 / 18,1s → 5,1s |
| xoá bộ lọc ngày | **16,8s → 369ms** | **25,9s → 5,1s** |
| trang 200 | 12,9s → 406ms | 16,3s → 5,1s |
| "Nguyễn" | 12,7s → 285ms | 14,1s → 2,3s |
| từ khoá không khớp / mã đơn | 355–362 → 175ms | 3,3–15,5s → 2,0–3,6s |
| + lọc 1 chi nhánh | — | 12,6s → **301ms** |

Body sha1 khớp HEAD **19/19**. ⚠ Ca "xoá ngày" toàn CN lệch 1 dòng ở lượt đầu (599.992 vs 599.993) — KHÔNG
phải bug: không có `lte` thì handler lấy `to = time.Now()`, 2 bản chạy cách nhau vài phút và seed có dịch vụ
đặt giờ tương lai; chạy lại với `lte` cố định ⇒ khớp cả 2 chế độ. Bài học: đối chiếu body phải CỐ ĐỊNH mọi
mốc thời gian mặc định. Tầng SQL: số dòng + 4 tổng + checksum từng dòng khớp 5/5 bộ lọc (gồm F > T).
3 test DryRun, mutation 4/4. ⚠ Lượt mutation đầu báo `o3` "xanh" nhưng thật ra **không biên dịch được** —
grep chỉ bắt `--- FAIL` nên sót `build failed`; phải grep cả hai.

**Lượt 2 cùng ngày — vá nốt toàn chi nhánh và từ khoá** (bench dựng lại y hệt, mốc `now` seed cố định):

1. **Câu tổng chia khoảng id đơn, chạy song song 4 luồng** (`totals.go`). Công nợ mọi đơn là việc không
   bớt được bằng chỉ mục, nhưng COUNT/SUM cộng dồn được qua các khoảng rời nhau. SQL thật, toàn CN tháng
   này: 1 luồng 5,25s · **4 luồng 1,64s** · 8 luồng 1,53s — khớp tuyệt đối cả 3. ⚠ Tổng việc DB phải làm
   KHÔNG giảm, chỉ dồn vào 4 kết nối (pool app 200). Cổng: câu dò có trần `LIMIT 100001` ≥ 100k đơn trong
   phạm vi (1 CN @35k đơn: 64–68ms, chạy 1 luồng), hoặc từ khoá phải đi đường cũ (luôn nặng ⇒ bỏ dò).
   `-race -p 1` sạch. ⚠ **Bẫy đo**: spike đặt `SetMaxOpenConns(1)` ⇒ 4 khoảng xếp hàng tuần tự
   (1,2 → 2,5 → 3,7 → 5,0s) và trông như "song song không giúp gì" — spike phải có pool > số luồng.
2. **Từ khoá qua ngram — CHỈ khi xem toàn chi nhánh và không lọc `branch_id`** (`keyword.go`): tra id khách
   `MATCH(search_ngram) AND LIKE` 4 cột (trần 10.000) → tra id ĐƠN ứng viên → câu tổng/trang chỉ tra theo
   khoá chính. Tầng SQL toàn CN: đường cũ JOIN customer + LIKE 5 cột ~2.000ms với MỌI từ khoá; ngram khách
   180–230ms (hiếm) / 274–498ms (phổ biến, vượt trần); LIKE mã đơn không JOIN 87–103ms.
   - Tra id đơn phải **TÁCH 2 câu** (`customer_id IN` · `code LIKE`) rồi hợp ở Go — gộp
     `customer_id IN … OR code LIKE` thì optimizer chọn plan kém: SĐT hiếm 659–674ms → endpoint 844 → 276ms.
   - Không tra id đơn trước: câu tổng 673ms + câu trang 330ms (cược `id DESC` không thoát sớm vì hiếm).
   - **LOẠI ngram ở phạm vi 1 chi nhánh** (đường cũ vốn rẻ): không khớp 175 → 212ms · mã đơn 176 → 256ms ·
     "Nguyễn" (vượt trần, tốn MATCH rồi vẫn về đường cũ) 285 → 645ms.

**Endpoint sau lượt 2** (HEAD = trước cả lượt 1 → bản cuối):

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| tháng này / 1 năm / xoá ngày | 13,2–17,0s → **0,48–0,54s** | 16,3–26,7s → **1,9–2,2s** |
| trang 200 | 13,1s → 0,59s | 16,5s → 2,0s |
| "Nguyễn" / "Thảo" | 12,8s → **184–192ms** | 14,1–14,2s → 1,39–1,57s |
| từ khoá 1 ký tự (đường cũ) | 12,9s → 305ms | 15,5s → 1,75s |
| mã đơn / SĐT hiếm | 377–406ms → 92–116ms | 15,7s → **281–303ms** |
| không khớp | 382–405ms → 97–131ms | 3,3–4,0s → **244–262ms** |
| + lọc 1 chi nhánh | — | 12,7s → 400ms |

Body sha1 khớp HEAD **26/28** — 2 ca lệch là "xoá ngày" KHÔNG có `lte` (`to = time.Now()` trôi giữa 2
lượt); cùng ca với `lte` cố định khớp cả 2 chế độ. 11 test (DryRun SQL + hàm thuần + song song -race),
mutation **8/8** (mọi bản mutation đều biên dịch được — đã kiểm cả `build failed`).

**Còn lại:** toàn chi nhánh không từ khoá ~2s và từ khoá phổ biến vượt trần ~1,4–1,75s — muốn thấp hơn
phải lưu sẵn công nợ theo đơn (cập nhật khi ghi đơn/dịch vụ/phiếu thu), không còn là việc của query.

### Công nợ điều trị — điều kiện `totalEnd != payment` không có đường vào ⇒ chỉ mục HÀM, và phải viết đúng dạng

`GET /accountant/treatment-services/list-treatment-debts` (+ `?_export=1`), /check-scale 2026-09-17. Bench dựng
TỪ MIGRATION: 900.000 dịch vụ / 600.000 khách / 221.538 liệu trình, danh mục dịch vụ chép từ dev; dịch vụ còn nợ
**0,53%** (4.752; DB dev 624/112.340 = 0,56%). Ca đối kháng: 1/3 dịch vụ còn nợ (300.000).

**Cơ chế.** "Còn nợ" = `totalEnd != payment` — so sánh 2 cột, không chỉ mục thường nào dùng được. Optimizer dẫn từ
`service` (615 dịch vụ loại 1/3) rồi tra `fk-order_service_used-service_id` **857k lượt tra dòng** để lọc ra 4.752.
Ba câu của màn (COUNT bọc `SELECT … GROUP BY id ORDER BY` của `Paginate` · câu trang · `totalDeposit`) mỗi câu
3,0–3,1s, toàn chi nhánh không lọc ngày cả request **13,7s**.

**Vá:**
1. **Chỉ mục hàm** `idx_osu_debt_day ((totalEnd <> payment), created_day)` — migration `20260917093917`. Gate
   FORCE INDEX: optimizer TỰ chọn (auto = ép). Đếm 3.104 → **78ms**, trang 3.081 → **73ms**, totalDeposit
   2.996 → **71ms**, kết quả khớp tuyệt đối.
2. ⛔ **Điều kiện PHẢI viết `(order_service_used.totalEnd <> order_service_used.payment) = 1`.** Viết
   `totalEnd != payment` hay `totalEnd <> payment` trần thì MySQL KHÔNG khớp chỉ mục hàm (đo trên câu đếm đơn:
   dạng trần quét `idx_osu_appt_day_cover` 150–326ms · dạng `(…) = 1` / `= TRUE` tra `idx_osu_debt_day` 8,8ms).
   Ngữ nghĩa NULL không đổi. Test DryRun neo đúng chuỗi này.
3. Bỏ `GROUP BY order_service_used.id` (id là PK, 2 JOIN đều 1:1 theo PK ⇒ không dòng trùng), đếm thẳng +
   `PaginateWithTotal`, COUNT ≤ offset không chạm DB, `LEFT JOIN customer` chỉ khi có từ khoá / export, lỗi câu
   totalDeposit hết bị nuốt. Consumer export: 2 hàm load dùng `q` làm `IN (subquery)` ⇒ bỏ GROUP BY không đổi.

**Vận hành chỉ mục hàm (MySQL 9.7):** thêm được KHÔNG dựng lại bảng (900k dòng 2,2–2,5s) nhưng **không cho
`LOCK=NONE`** ("ADD COLUMN col...VIRTUAL, ADD INDEX(col). Try LOCK=SHARED") ⇒ ghi bị chặn trong lúc dựng. Đừng
gộp với thao tác khác trong cùng `ALTER` / đừng thêm lúc đã có chỉ mục hàm khác cần hidden column mới: MySQL từ
chối INPLACE ("INPLACE ADD or DROP of virtual columns cannot be combined…") và rơi về COPY — dựng lại bảng + kiểm lại
TOÀN BỘ FK (bench báo 1452 vì bảng `branch` rỗng).

**Biến thể LOẠI:** chỉ mục phủ `((totalEnd <> payment), created_day, company_id, branch_id, service_id, customer_id)`
— 78–82ms, không nhanh hơn (chi phí còn lại không ở chỗ đọc dòng).

**Endpoint, spike, HEAD → bản vá** (phân bố thật):

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| tháng này (mặc định FE) | 235 → **21ms** | 649 → **3ms** |
| 1 năm | 314 → 52ms | **12,1s → 20ms** |
| xoá bộ lọc ngày | 322 → 63ms | **13,7s → 105ms** |
| xoá ngày, trang 200 | 307 → 143ms | 10,8s → 101ms |
| từ khoá không lọc ngày (không khớp / mã / SĐT / "Nguyễn") | 309–505 → 66–182ms | **11,1–13,7s → 139–169ms** |
| tháng này + từ khoá | 54–64 → 7–11ms | 56–59 → 4–5ms |

Đối kháng 300.000 dòng nợ (HEAD → vá): toàn CN 1 năm 16,2 → **2,6s**, xoá ngày 28,3 → 15,0s, trang 200
43,5 → 13,9s, từ khoá không lọc ngày ~20–27s → 14–22s (ngang); 1 CN mọi ca ≤ HEAD. Body sha1 khớp HEAD
**24/24** (thật) + **24/24** (đối kháng) + refactor 24/24. 3 test DryRun, mutation 3/3. Migration: áp 2 lần
(lần 2 no-op 0,02s) + `cmd/migrate` từ đầu trên DB dùng-một-lần (version 20260917093917, dirty 0).

**Lượt 2 cùng ngày — vá nốt phân bố đối kháng + `totalDeposit`:**

1. **Ép `idx_osu_debt_day` CÓ CỔNG** (toàn chi nhánh, không lọc `branch_id`). Khi nợ chiếm tỉ lệ lớn optimizer bỏ
   chỉ mục, quay về dẫn từ `service`, và **plan câu trang LẬT** giữa các lượt đo cùng câu (0,2ms ↔ 4.335–4.506ms).
   Đối kháng toàn CN không lọc ngày: đếm auto 3.287–4.353 / ép 762–818ms · trang lật / ép 0,3ms · totalDeposit
   auto 7.209–7.432 / ép 1.585–1.691ms. **Cổng vì 1 CN ép chậm hơn**: xoá ngày auto 495 / ép 882ms, từ khoá auto
   414–527 / ép 1.006–1.033ms. Trước khi ép kiểm `information_schema.STATISTICS` (~1ms) — ép chỉ mục vắng mặt
   (DB chưa migrate) = Error 1176 = 500.
2. **`totalDeposit` dùng chung `debtsQuery`** ⇒ áp ĐÚNG bộ lọc danh sách (trước thiếu từ khoá + `filter[company_id]`
   dù comment ghi "cùng filters" ⇒ tìm 1 khách vẫn cộng cọc của mọi khách còn nợ). Dạng
   `SUM(totalDeposit) FROM customer WHERE id IN (SELECT customer_id …)` = cùng tập khách với `DISTINCT` cũ.
   **Đổi số hiển thị** ở ca có từ khoá (vd "Nguyễn" toàn CN 36,7 tỷ → 2,78 tỷ; không khớp → 0).

**LOẠI (đo trên đối kháng):** chỉ mục phủ `((totalEnd <> payment), created_day, company_id, branch_id, service_id,
customer_id)` — đếm 762ms (≈ 818), **trang ÉP 578ms** (tệ hơn 0,3ms: cột chen giữa làm mất thứ tự đọc ngược),
totalDeposit 1.622ms (≈). ⚠ Thêm chỉ mục hàm THỨ HAI khi đã có một cái ⇒ MySQL rơi về COPY (dựng lại bảng +
kiểm lại FK); muốn đổi hình dạng phải DROP rồi ADD 2 câu riêng.

**Endpoint cuối, HEAD → bản vá** (danh sách body khớp 24/24 ở CẢ 2 phân bố; totalDeposit đổi đúng 12 ca có từ khoá):

| | thật 1 CN | thật toàn CN | đối kháng 1 CN | đối kháng toàn CN |
|---|---|---|---|---|
| tháng này | 55 → 3ms | 57 → 3ms | 64 → 17ms | 108 → 35ms |
| 1 năm | 324 → 11ms | 13,8s → **18ms** | 549 → 353ms | 15,9s → **0,72s** |
| xoá ngày | 329 → 25ms | 9,8s → **82ms** | 1.060 → 446ms | 25,1s → **2,1s** |
| xoá ngày trang 200 | 312 → 28ms | 10,2s → 90ms | 984 → 457ms | 25,7s → 2,1s |
| từ khoá không lọc ngày | 314–328 → 23–35ms | 9,0–10,2s → 131–158ms | 595–657 → 451–576ms | 20,3–20,5s → 3,3–4,7s |

Tổng 8 test DryRun, mutation 6/6 (ép chỉ mục · cổng chi nhánh · điều kiện dạng `= 1` · GROUP BY · JOIN customer ·
bộ lọc totalDeposit). Contract test `integration` biên dịch sạch.

**Còn lại:** đối kháng toàn CN + từ khoá không lọc ngày 3,3–4,7s (LIKE qua JOIN customer trên 285k dòng nợ) —
chỉ ở phân bố 1/3 dịch vụ chưa thanh toán đủ; phân bố thật 131–158ms.

### Công nợ theo buổi — điều kiện là BIỂU THỨC, 24% dòng thoả: không chỉ mục nào cứu, vá bằng bớt lượt + đổi bảng dẫn

`GET /accountant/treatment-services/list-treatment-session-debts` (+ `?_export=1`), /check-scale 2026-09-17. Bench dựng
TỪ MIGRATION: 900.000 dịch vụ (856.810 loại 1/3) / 600.000 khách; dòng thoả điều kiện **24,1%** (206.591; DB dev
25.737/108.831 = 23,6% — dày vì tính cả dịch vụ trả trước chưa dùng hết buổi).

**Cơ chế.** Điều kiện `HAVING debtByUnitPrice != 0 OR debtByAllocated != 0` trên ALIAS (không GROUP BY) — biểu thức
`LEAST(nts × unit_price, totalEnd) − payment` / chia đều theo buổi. 3 lượt quét mỗi request (câu tổng bọc bảng dẫn
xuất · COUNT của `Paginate` · câu trang `ORDER BY customer_id, id`), optimizer dẫn từ `service` rồi tra
`fk-order_service_used-service_id` 857k dòng ngẫu nhiên. Toàn CN xoá ngày **38–55s** cache lạnh (~6s ấm).

**Bẫy — ĐỪNG chuyển điều kiện sang WHERE.** Viết biểu thức vào `WHERE` đổi ước lượng: câu trang lọc tháng 1 CN chuyển
sang cược thoát sớm trên `fk-order_service_used-customer_id`, quét **888.725 dòng / ~1s** (HAVING: chỉ mục ngày,
19ms); câu tổng WHERE auto 5,1–6,9s (tệ hơn HEAD). ⇒ giữ HAVING cho danh sách.

**Bẫy GORM — `Having()` nằm TRONG clause GROUP BY.** `delete(stmt.Clauses, "HAVING")` là no-op (câu dò giữ HAVING ⇒
Error 1054 alias ⇒ âm thầm không bao giờ chia luồng — bắt được khi đọc log); ngược lại `cleanIDSubQuery` của export
(xoá GROUP BY) xoá LUÔN HAVING ⇒ câu lấy id trả mọi dịch vụ trong phạm vi (đúng kết quả, thừa việc). ⚠ Suýt báo
nhầm "export hỏng cột liệu trình" vì kiểm chứng bằng SQL tay còn giữ HAVING — code thật không giữ.

**Vá:**
1. **Câu tổng 1 lượt** (COUNT gộp, `PaginateWithTotal`), HAVING giữ nguyên, **dẫn từ order_service_used**
   (`STRAIGHT_JOIN service`), JOIN customer chỉ khi có từ khoá. Cache ấm: toàn CN không lọc 2.245+2.256 → 2.166ms,
   1 năm 6.705 → 1.401ms, tháng 86 → 56ms, 1 CN không lọc 304 → 187ms. Kết quả khớp tuyệt đối.
2. **Chia 4 khoảng id song song** khi phạm vi ≥ 100.000 dòng dịch vụ (câu dò có trần, chỉ đếm `order_service_used`,
   không JOIN service — kèm JOIN thì 1 năm 903ms): toàn CN không lọc 2.166 → 884ms, 1 năm 1.401 → 852ms. Tháng này
   chia KHÔNG lợi (56 → 65ms), 1 CN tháng TỆ hơn (35 → 72ms) ⇒ cổng.
3. **Câu trang ép `fk-order_service_used-customer_id` + STRAIGHT_JOIN** khi không lọc ngày + toàn CN + không từ
   khoá + sort mặc định: 1.378 → **15ms**, trang 200 1.378 → 44ms. Cổng vì ép ở ca khác chậm hơn: 1 CN + từ khoá không
   khớp 192 → 1.065ms, 1 CN 1 năm 131 → 687ms.
4. **Câu trang có từ khoá + không lọc ngày + toàn CN ⇒ STRAIGHT_JOIN (không ép chỉ mục).** Lượt A/B đầu thiếu cổng
   này: từ khoá hiếm toàn CN **HỒI QUY 6,1 → 14,0s** — câu tổng mới đổi đường truy cập (quét tuần tự osu) nên câu trang
   (vẫn dẫn từ service, tra ngẫu nhiên) mất phần cache mà câu tổng CŨ vô tình làm ấm sẵn. Bài học: đổi plan một câu có
   thể làm chậm câu KHÁC trong cùng request qua cache — A/B phải đo cả request, không chỉ từng câu.
5. Export: điều kiện nợ dạng WHERE (không LIMIT nên không dính bẫy cược) ⇒ câu lấy id của `cleanIDSubQuery` đúng tập.

**Endpoint, HEAD → bản vá** (2 lượt xen kẽ; body sha1 khớp HEAD **24/24**):

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| tháng này | 60–62 → 44ms | 96–98 → 69ms |
| 1 năm | 345–393 → 307–333ms | 3,9–29,7s → **2,06–2,19s** |
| xoá ngày | 462–551 → 366–386ms | 38,1–55,4s → **0,96–0,98s** |
| xoá ngày trang 200 | 479–584 → 367–387ms | 37,0–39,3s → 1,07–1,18s |
| từ khoá không khớp / mã / SĐT (không lọc ngày) | 493–554 → 211–230ms | 5,8–7,5s → 2,4–4,2s |
| "Nguyễn" (không lọc ngày) | 444–565 → 335–362ms | 34,7–43,1s → **3,8–4,2s** |

8 test DryRun, mutation 7/7 (một mutation — export quay về HAVING — ban đầu XANH vì test gọi thẳng `debtsQuery` với
cờ đã đặt sẵn; tách `exportQuery` để test đi đúng đường của handler).

**Lượt 2 cùng ngày — vá nốt từ khoá toàn CN + trang 1 năm:**

1. **Từ khoá qua ngram** (`keyword.go`, CHỈ toàn CN + không lọc ngày): tra id khách `MATCH(search_ngram) AND LIKE`
   (fullname/code/phone) → tra id dịch vụ ứng viên TÁCH 2 câu (`customer_id IN` · `code LIKE`, trần 10.000) → câu tổng +
   câu trang lọc `id IN`, không LIKE, không JOIN customer; tập rỗng ⇒ `1 = 0`. Tầng SQL: ngram khách 13–241ms (hiếm) /
   308–523ms (phổ biến, vượt trần); `customer_id IN (6)` 12ms; mã LIKE không JOIN 242–298ms. Endpoint toàn CN không khớp /
   mã / SĐT **5,9–8,2s → 0,29–0,52s**. Từ khoá phổ biến vượt trần về đường cũ, phạt ~+0,5s ("Nguyễn" 3,8 → 4,3–4,8s so
   với lượt 1; so HEAD 34,6–41,1s vẫn thắng).
2. **Câu trang song song** khi câu tổng đã chia khoảng (phạm vi ≥100k) + sort mặc định + `offset+perPage ≤ 2000` + không
   thuộc ca ép chỉ mục customer_id: mỗi khoảng lấy top (offset+perPage) theo (customer_id, id), trộn trong Go
   (`mergeTopPage`; top-k toàn cục ⊆ hợp top-k từng khoảng). Tầng SQL 1 năm toàn CN: 1 luồng 1.128–1.136ms (lạnh
   6.158ms) → 4 luồng **560–599ms**, trang 1/10/100 khớp tuyệt đối; tháng này KHÔNG lợi (37–39 → 40–45ms) nên gắn vào
   cổng của câu tổng.

**Endpoint cuối, HEAD → bản vá** (2 lượt xen kẽ, máy tải 3,5–4,5; body sha1 khớp **32/32**):

| | 1 chi nhánh | toàn chi nhánh |
|---|---|---|
| tháng này | 61–63 → 44–46ms | 101–105 → 72ms |
| 1 năm / trang 10 | 343–365 → 298–304ms | 4,0–21,6s → 1,7–3,4s (nhiễu) |
| xoá ngày / trang 200 | 462–543 → 353–369ms | 38,6–74,8s → **1,05–1,29s** |
| không khớp / mã / SĐT (không lọc ngày) | 503–527 → 212–228ms | 5,9–8,2s → **0,29–0,52s** |
| "Nguyễn" / "Thảo" (không lọc ngày) | 451–469 → 341–400ms | 34,6–54,7s → 4,0–6,5s |
| 1 ký tự (đường cũ) | 450–466 → 366–375ms | 47,5–53,9s → 4,8–7,4s |
| 1 năm + "Nguyễn" | 346–349 → 251–252ms | 3,9–6,0s → 2,6–3,7s |

Tổng 12 test DryRun, mutation 12/12 (5 mới: tập id rỗng phải `1 = 0` · cổng ngram theo chi nhánh · thứ tự trộn · lát
cắt offset · đường id không còn LIKE; 7 cũ chạy lại trên source mới).

**Còn lại:** từ khoá PHỔ BIẾN / 1 ký tự toàn CN không lọc ngày 4–7s (vượt trần ngram ⇒ LIKE qua JOIN customer trên 857k
dịch vụ) — muốn thấp hơn phải chia song song cả đường LIKE hoặc đổi thiết kế tìm kiếm; 1 năm toàn CN 1,7–3,4s (tổng
~0,85s + trang ~0,6s, còn lại là nhiễu máy).

### Công nợ trả góp — bảng dẫn xuất gom TOÀN BẢNG chỉ để đếm kỳ, và câu trang từ khoá cược dừng sớm

`GET /accountant/installment/list-debts` (+ `?_export=1`), /check-scale 2026-09-17. Bench dựng TỪ MIGRATION:
750.000 kỳ `order_installment` (188.288 còn mở `status IN (1,3)`, 76.804 quá hạn) / 900.000 dịch vụ / 600.000 khách
(tên thật copy từ dev). ⚠ DB dev **0 kỳ trả góp** ⇒ không có phân bố thật để neo.

**Cơ chế — 3 cái, độc lập:**

1. **`LEFT JOIN (SELECT order_service_used_id, COUNT(*) … GROUP BY order_service_used_id) tp`** chỉ để lấy cột
   "tổng số kỳ" — gom TOÀN BẢNG ~570ms, gắn vào query gốc nên cả câu tổng (không đọc cột nào của tp) lẫn câu
   trang đều trả. Lọc 1 ngày (617 dòng) vẫn 1,25–1,38s. Comment cũ ghi "pre-aggregate thay correlated subquery
   per row" — đúng cho export, SAI cho trang 20 dòng. Vá: câu tổng bỏ tp + `order_treatment_plan` (FK thật,
   `treatment_plan_id` NOT NULL ⇒ JOIN inner không loại dòng) + `customer` (chỉ khi từ khoá) + ORDER BY; trang
   tính `total_periods` bằng 1 câu `IN (≤20 dịch vụ)` (14–25ms); export đổi sang subquery tương quan
   `NULLIF(COUNT(*), 0)` — giữ đúng NULL của LEFT JOIN cũ (file export có ô `"%v/%v"`).
2. **Không chỉ mục `(status, due_date)`** ⇒ 1 năm (27.654 kỳ) quét trọn 750k. Chỉ mục phủ
   `idx_oi_status_due_cover (status, due_date, order_service_used_id, amount, paid_amount)` (migration
   `20260917121929`, +27,6MB). Gate: câu tổng 1 năm (a) 210 · (b) 84–92 · (c) ép 78–84ms ⇒ ĐẠT.
   ⚠ **KHÔNG lọc ngày optimizer vẫn chọn nó** (quét trọn chỉ mục theo `due_date`) rồi tra `order_service_used`
   theo thứ tự NGẪU NHIÊN — thua quét bảng, nơi id kỳ trả góp đi cùng thứ tự id dịch vụ (locality): câu tổng
   toàn CN 278–313 vs 391–414ms, có từ khoá 650–990ms vs 1,6–2,5s. ⇒ `IGNORE INDEX` có cổng (không lọc ngày +
   `information_schema` xác nhận chỉ mục tồn tại, tránh Error 1176). Câu trang không áp (ngang nhau).
3. **Câu trang có từ khoá cược dừng sớm**: `Sort` 188.288 kỳ trước, rồi mới tra osu → otp → customer từng dòng
   theo thứ tự ngày để lọc LIKE ⇒ từ khoá hiếm/không khớp không bao giờ gom đủ 20: **1,6–2,2s cho 0 dòng**, trong
   khi cùng khối việc theo thứ tự bảng (câu tổng) 0,65–0,99s. Vá: từ khoá ⇒ MỘT lượt `Rows()` (id + tiền) cộng tổng
   trong Go + gom id ≤ 10.000; câu trang lọc `order_installment.id IN (…)` với ĐÚNG ORDER BY cũ. Vượt trần ⇒ câu
   trang như cũ (khớp nhiều thì cược dừng sớm thắng: "Nguyễn" 16.401 kỳ, trang 245–320ms), tổng vẫn cộng đủ.

**Đo endpoint (spike gọi thẳng handler, cùng máy, xen kẽ):**

| Ca | 1 chi nhánh HEAD → vá | Toàn chi nhánh HEAD → vá |
|---|---|---|
| tháng này (mặc định FE) | 1.332–2.065 → **9ms** | 1.246–2.116 → **8–11ms** |
| 1 ngày | 1.250–2.063 → 4ms | 1.261–1.380 → 4–6ms |
| 1 năm | 1.353–1.859 → **88–107ms** | 1.571–1.839 → **95–114ms** |
| 1 năm trang 100 | 1.385–1.925 → 45ms | 1.550–1.716 → 103–137ms |
| xoá ngày | 1.960–2.268 → 386–410ms | 2.517–3.704 → **495ms** |
| xoá ngày trang 200 | 1.939–2.356 → 381ms | 2.682–3.537 → 493ms |
| xoá ngày + từ khoá không khớp | 990–1.065 → 292ms | 2.327–3.433 → **562–622ms** |
| xoá ngày + mã dịch vụ | 1.030–1.183 → 295ms | 3.773–5.944 → **566ms** |
| xoá ngày + "Nguyễn" (vượt trần) | 1.890–2.153 → 302ms | 2.131–3.133 → 781–836ms |
| 1 năm + "Thảo" | 1.411–2.777 → 80ms | 1.511–2.568 → 268–288ms |
| lọc `status` 3, xoá ngày | 1.416–1.493 → 150–165ms | 1.665–1.789 → 159–166ms |
| export toàn CN "Nguyễn" / 1 năm "Thảo" / xoá ngày | | 10,7s → 1,7s · 12,6s → 0,65s · 7,9 → 6,0s |

Đóng góp riêng của chỉ mục (bản vá có/không chỉ mục, xen kẽ 3 lượt): 1 năm 297–329 → 105–114ms, tháng 24 → 10ms,
1 năm trang 100 322–333 → 103–129ms; không ca nào tệ hơn >10% ("1 năm + Thảo" 245–261 → 268–288ms).

**LOẠI (có số):**

| Cách | Số đo | Vì sao loại |
|---|---|---|
| Chỉ mục `(status, order_service_used_id, due_date, amount, paid_amount)` | câu tổng không cần cổng (xoá ngày 242ms), NHƯNG trang 1 năm 200ms (A: 65) · trang từ khoá 1,5s | giữ locality cho câu tổng, mất đường vào ngày cho câu trang |
| `status + 0` thay `IGNORE INDEX` (tránh 1176) | từ khoá 1.513ms (không đổi) | optimizer chuyển sang quét trọn CHỈ MỤC PHỦ, vẫn tra osu ngẫu nhiên |
| ngram cho từ khoá (khuôn các màn công nợ khác) | tra khách 14–475ms + osu theo mã 246–279ms + osu theo khách 176–646ms, "Thảo"/"Nguyễn" vượt trần 10.000 | lượt 1 lượt quét đã 290–620ms cho MỌI từ khoá, ngram chỉ cộng thêm |

Đối chiếu: body JSON **32/32** (16 ca × 2 chế độ) cả khi CÓ và KHÔNG có chỉ mục (DB chưa migrate ⇒ không 500);
file export **14/14** (xlsx đọc theo ô, csv theo byte). Test DryRun 7 (5 mới + 2 cũ sửa), mutation **8/8**.

~~**Còn lại:** xoá ngày toàn CN ~0,5s và từ khoá phổ biến ~0,8s … snapshot `branch_id` … `idx-order_installment-status`
cố ý chưa bỏ.~~ → đã xử ở Lượt 2.

#### Lượt 2 (cùng ngày) — chia khoảng id song song, bỏ chỉ mục thừa, vá hồi quy dashboard

**Không snapshot `branch_id` vào `order_installment`** (cách lượt 1 gợi ý): `osu.branch_id` đổi được sau khi tạo kỳ ⇒
bản sao lệch ⇒ scope chi nhánh SAI im lặng. Đi đường không đổi schema:

1. **Chia 4 khoảng id song song** (`parallel.go`) khi KHÔNG lọc ngày và khoảng id ≥ 100.000: câu tổng, lượt quét từ
   khoá (id gộp lại, vượt trần 10.000 ⇒ nil — `mergeKeywordIDs`, không cắt bớt), câu trang top-(offset+perPage) mỗi
   khoảng rồi trộn trong Go (trần offset+perPage ≤ 2.000). Bộ so sánh dựng từ CHÍNH chuỗi `query.SortClause` (chỉ
   nhận `due_date`/`id`, cột lạ ⇒ về 1 câu) nên `sort=` người dùng chọn cũng song song được; `due_date` là chuỗi
   `dd/mm/yyyy` ⇒ so theo năm-tháng-ngày.
   - ⚠ **Luồng cuối nặng nhất**: id lớn = kỳ tạo gần đây = hạn tương lai = gần như đều còn mở (luồng cuối 134 vs
     ~90ms mỗi luồng khác). 8 luồng không đỡ thêm (xoá ngày toàn CN 217–228 vs 192–214ms) ⇒ giữ 4.
   - ⚠ **Trong luồng BẮT BUỘC `STRAIGHT_JOIN`**: 1 CN + lọc status optimizer dẫn từ `fk-order_service_used-branch_id`,
     quét trọn dịch vụ của chi nhánh ở MỖI luồng — 4 luồng đều ~200ms bằng nhau tăm tắp là dấu hiệu khoảng id vô dụng.
     Luồng cuối: auto 163 · STRAIGHT 55ms.
   - ⚠ **Cổng `IGNORE INDEX` phụ thuộc có chia luồng hay không**: đo câu ĐƠN thì 1 CN bỏ chỉ mục tệ hơn (status 3:
     118–124 → 182–191ms, aging 190–195 → 253–266ms) — nhưng trong LUỒNG, `id BETWEEN` chỉ đi gọn PRIMARY khi bỏ chỉ
     mục phủ: 1 CN từ khoá không khớp áp 136–150 · không áp 359–379ms. Kết luận đo trên câu đơn KHÔNG áp được cho
     câu đã chia khoảng. Cổng: chia luồng ⇒ luôn áp; 1 luồng ⇒ chỉ khi toàn chi nhánh.
2. **Bỏ `idx-order_installment-status`** (migration `20260917124053`): tiền tố thừa của `idx_oi_status_due_cover`. Tra
   ĐỘNG theo danh sách cột, chỉ bỏ khi chỉ mục phủ đã có (đã thử cả 2 nhánh trên bench). −16MB/750k dòng.
3. **Hồi quy do chính chỉ mục lượt 1 ở consumer khác**: dashboard `receivable` aging quét MỌI kỳ còn mở (không lọc
   ngày) ⇒ cùng bẫy mất locality — toàn CN 331–364 → 451–486ms khi có chỉ mục. Vá `IGNORE INDEX` có cổng (toàn CN +
   chỉ mục tồn tại). Đo nốt các consumer còn lại: `alerts` quá hạn 213–241 / đến hạn 7 ngày 21–26ms, danh sách kỳ theo
   khách (`treatment_service.go`) 11–16ms — không đổi. ⇒ **Thêm chỉ mục cho bảng nào thì đo MỌI câu đọc bảng đó**, không
   chỉ endpoint đang vá.

**Đo endpoint (bench dựng lại y hệt lượt 1; r0 = bản gốc, r1 = lượt 1, lượt 2 hai lượt chạy):**

| Ca | 1 CN r0 → r1 → lượt 2 | Toàn CN r0 → r1 → lượt 2 |
|---|---|---|
| xoá ngày | 1.865 → 365 → **199–246** | 3.171 → 482 → **212–240** |
| xoá ngày trang 50 / 200 / 500 | 1.899 → 366 → 245–338 · 2.033 → 368 → 314–366 · 2.219 → 385 → 333–369 | 3.372 → 485 → 244–266 · 3.360 → 494 → 340–392 · 3.552 → 523 → 404–407 |
| từ khoá không khớp | 1.021 → 280 → **134–162** | 3.127 → 560 → **308–332** |
| mã dịch vụ / SĐT | 1.028–2.134 → 280–282 → 142–155 | 2.828–4.345 → 566–636 → 334–352 |
| "Nguyễn" (vượt trần) / trang 30 | 1.883 → 286 → 157–164 · 2.125 → 292 → 174–178 | 2.974 → 760 → 443–453 · 2.845 → 798 → 584–597 |
| lọc status 3 | 1.394 → 213 → 98–121 | 1.674 → 245 → 91–92 |
| `sort=-due_date` / trang 50 | 1.868 → 365 → 214–238 · 1.906 → 368 → 267–358 | 2.875 → 510 → 237–241 · 2.882 → 466 → 239–256 |
| `sort=id` / `sort=-id` trang 10 | 1.844 → 364 → 209–226 · 1.876 → 368 → 229 | 2.848 → 502 → 225–230 · 2.865 → 472 → 249–252 |
| có lọc ngày (tháng / 1 năm) | không đi đường song song — ngang lượt 1 | ngang lượt 1 |

**LOẠI (có số):**

| Cách | Số đo | Vì sao loại |
|---|---|---|
| 8 luồng thay 4 | xoá ngày toàn CN 192–214 vs 217–228ms; "Nguyễn" 391–436 vs 448–462 | lợi ~10% mà gấp đôi kết nối |
| Trần trang song song 20.000 thay 2.000 | toàn CN trang 200: 262–279 vs 345–379; **1 CN trang 500: 448–471 vs 332–342ms** | mỗi luồng trả tới offset+perPage dòng ⇒ 1 CN tệ hơn |
| `IGNORE INDEX` cả câu trang trong luồng | ngang hoặc tệ hơn (toàn CN status 3: 118–135 vs 95–99ms) | không lợi |
| Snapshot `branch_id` vào bảng kỳ | không đo | `osu.branch_id` đổi được ⇒ lệch scope im lặng |

Đối chiếu body JSON **44/44** (22 ca × 2 chế độ, thêm sort/trang sâu/từ khoá trang 30) so với BẢN GỐC, cả lượt 1 lẫn
lượt 2 (2 lần chạy). Test DryRun: 11 trong package + 2 dashboard; mutation lượt 2 **11/11**, chạy lại 8 mutation lượt
1 trên code mới **8/8**; `-race -p 1` sạch + 8 lượt chạy thật đường song song dưới `-race`: 0 race.

**Còn lại:** xoá ngày toàn CN ~0,2s, từ khoá phổ biến toàn CN ~0,45s, trang sâu (> 2.000 dòng) 0,35–0,4s — sàn của
"đi qua 188k kỳ còn mở + tra dịch vụ từng kỳ". Không còn đường không-đổi-schema nào đã đo mà rẻ hơn.

### Công nợ sản phẩm — bảng dẫn xuất gom TOÀN BẢNG payment theo mốc ngày, bộ lọc ngày không giảm việc gì

`GET /accountant/product-sale/get-product-sale-debts` (+ `?_export=1`), /check-scale 2026-09-17. Bench dựng TỪ
MIGRATION: 900.000 dịch vụ (180.000 dòng sản phẩm `serviceCategory_id = 2`, **25% còn nợ** — cao hơn thực tế, dùng
như ca đối kháng) / 900.000 phiếu thu (810.000 thuộc `payment_for IN (1,4,5)`) / 600.000 khách tên thật.

**Cơ chế:** 2 bảng dẫn xuất `SELECT SUM(amount) … FROM payment WHERE payment_of_date < from … GROUP BY
serviceUsed_id` ("trước kỳ" = gần như TOÀN BỘ lịch sử) và `… BETWEEN from AND to`, LEFT JOIN vào mọi dòng sản phẩm,
chạy 2 lần (câu tổng bọc nguyên query có ORDER BY + câu trang). Bộ lọc ngày chỉ đổi mốc, không thu hẹp dòng nào ⇒
lọc 1 ngày cũng 11,6s (1 CN) / 13,0s (toàn CN). Toàn CN tháng này: câu tổng 20,5s + câu trang 11,1s. Lỗi câu tổng
bị nuốt (không kiểm `Error`).

**Vá (không thêm chỉ mục — `idx_payment_service_for (serviceUsed_id, payment_for)` có sẵn):**
1. Phiếu thu tính bằng **subquery tương quan theo từng dòng** thay bảng dẫn xuất; `created_day <= to` đẩy vào WHERE
   (dòng tạo sau `to` / `created_day` NULL vốn bị HAVING loại).
2. **Lượt quét** chia 4 khoảng id song song, stream `(id, 4 cột kỳ)` về Go cộng tổng + gom id ≤ 10.000. Lọc ở WHERE
   lớp bọc ngoài — ⚠ HAVING theo alias TÍNH LẠI subquery (229.576 lượt cho 180.000 dòng). JOIN customer/đơn hàng chỉ
   khi có từ khoá.
3. **Câu trang**: tập id ≤ trần ⇒ `id IN`; vượt trần ⇒ `STRAIGHT_JOIN` dẫn từ `order_service_used` theo id giảm dần,
   dừng khi đủ trang (optimizer tự chọn thì dẫn từ `service`, tính cả 180.000 dòng rồi mới xếp: toàn CN 5,3–6,3s →
   18–26ms). ⚠ 1 CN không cứu được bằng STRAIGHT: optimizer vẫn đi `fk-…-branch_id` và đẩy điều kiện HAVING lên TRƯỚC
   JOIN service ⇒ tính subquery cho 52.275 dịch vụ của chi nhánh (420–520ms) — nhưng 1 CN hầu như luôn dưới trần id.
4. **Export** dạng bọc ngoài (lớp trong tính mỗi subquery 1 lần/dòng, lớp ngoài lọc + `totalEndDebt` + ORDER BY) +
   STRAIGHT_JOIN: dạng HAVING tính subquery tới 4 lần/dòng. 1 lượt đọc toàn CN tháng này 7,1–7,7 → 4,8s.

**Đo endpoint (spike gọi thẳng handler):**

| Ca | 1 CN HEAD → vá | Toàn CN HEAD → vá |
|---|---|---|
| tháng này (mặc định FE) | 10.188 → **142–177ms** | 12.961 → **1.333–1.347ms** |
| 1 ngày / 1 năm | 11.590 / 10.727 → 172–184 / 150–161ms | 12.977 / 12.360 → 1.326–1.362 / 1.359–1.410ms |
| xoá ngày (thật, `to = now`) | 8.455 → 287–320ms | 13.547 → 1.240–1.256ms |
| xoá ngày trang 200 | 6.730 → 317–368ms | 13.334 → 1.396–1.433ms |
| từ khoá không khớp (xoá ngày) | 538 → 129–130ms | 5.424 → 1.769–1.992ms |
| "Nguyễn" / SĐT / tên sản phẩm (xoá ngày) | 6.496–7.043 → 135–155ms | 11.358–12.272 → 1.691–2.204ms |
| mã dịch vụ hiếm (xoá ngày) | 543 → 131ms | 11.854 → 1.934–2.399ms |
| 1 năm + "Thảo" | 7.907 → 147ms | 12.434 → 1.623–1.896ms |
| lọc chi nhánh 3 (tháng) | 1,5 → 1,4–1,8ms (rỗng) | 8.789 → 146–150ms |
| **export** tháng này / xoá ngày | 9.884 → 1.376 · 6.828 → 1.070ms | 15.244 → 10.329 · 15.480 → 10.806ms |
| **export** "Nguyễn" / 1 năm + "Thảo" | 7.172 → 647 · 7.811 → 580ms | 11.859 → 5.086 · 13.733 → 4.778ms |

**LOẠI (có số):**

| Cách | Số đo | Vì sao loại |
|---|---|---|
| Gộp 2 subquery thành 1 `LEFT JOIN LATERAL … SUM(CASE …)` | toàn CN tháng 3,2–3,6s (ngang); xoá ngày 2,8 → 3,3–3,4s | không lợi, xoá ngày tệ hơn |
| Chỉ mục phủ `payment (serviceUsed_id, payment_for, company_id, payment_of_date, amount)` | (a) 2,7–3,1s · (b) auto 2,9–3,0s · (c) ép 2,0s; +37,6MB | optimizer không tự chọn; bảng payment ghi nhiều |
| Câu trang song song theo khoảng | không đo | luồng chứa id lớn nhất đã đủ trang — 4 luồng chỉ nhân việc |
| Câu trang dạng bọc ngoài (như export) | — | lớp ngoài ORDER BY … LIMIT phải tính mọi dòng, mất dừng sớm |

Đối chiếu: body JSON **32/32** (16 ca × 2 chế độ; các ca "xoá ngày" gửi `lte` cố định để body không trôi theo
`time.Now()`, riêng 1 ca thật không gửi gì); file export **10/10**. 7 test integration có sẵn + 1 test biên mới
(dòng tạo ĐÚNG `from`/`to`, sau `to`) xanh trên CẢ HEAD lẫn bản vá; 8 test DryRun. Mutation **14/14** (lỗi
`created_day < from` → `<=` lọt cả bản gốc lẫn test cũ — test biên thêm để bắt). `-race`: 6 lượt chạy thật đường
song song, 0 race.

~~**Còn lại:** toàn CN ~1,3s và từ khoá ~1,7–2,4s … export toàn CN ~10s …~~ → Lượt 2.

#### Lượt 2 (cùng ngày) — từ khoá qua tập ứng viên ngram, export bỏ COUNT bọc query

1. **Từ khoá (toàn chi nhánh)** qua `keyword.go`: tập ứng viên = hợp 4 nhánh ứng với 7 cột LIKE, MỖI nhánh 1 câu
   đơn KHÔNG JOIN — khách `MATCH(search_ngram) AND LIKE` (13–507ms) → dịch vụ theo `customer_id`; sản phẩm
   `service.name LIKE` (15–90ms); đơn hàng `order_treatment_plan.code LIKE` (59–67ms; viết kèm JOIN sản phẩm+scope
   1,7–13,5s); mã dịch vụ `order_service_used.code LIKE` (152–202ms; kèm JOIN 1,2–9,8s). Lượt quét lọc `id IN` +
   ĐỦ bộ lọc cũ, không JOIN khách. Vượt trần (khách > 10.000, ứng viên > 20.000) / từ khoá không an toàn cho ngram /
   lỗi ⇒ đường cũ. 1 chi nhánh không áp (đường cũ đã 130–160ms).
2. **`pkg/export` `Config.CountRows`** (additive, ~80 hàm khác không đổi): handler truyền lượt quét song song thay
   `SELECT COUNT(*) FROM (<query export>)`.
3. ⚠ **Test integration phải chạy `SchemaAll`**: baseline thiếu `customer.search_ngram` ⇒ MATCH lỗi ⇒ đường ngram
   âm thầm rơi về LIKE ⇒ test ngram XANH GIẢ (5/6 mutation lọt). Kèm: từ khoá test có `_` (`X_1`) bị
   `NgramPhraseSafe` từ chối đúng thiết kế — cũng xanh giả. Test nay gọi thẳng `debtsKeywordCandidates` và đòi `ok`,
   và kiểm dòng TỔNG (câu trang tự lọc LIKE nên danh sách đúng dù lượt quét bỏ lọc ứng viên).

| Ca (toàn CN, trừ ghi chú) | HEAD | lượt 1 | lượt 2 |
|---|---|---|---|
| mã dịch vụ hiếm (xoá ngày / tháng) | 12.230 / 15.659ms | 1.934–2.399ms | **236 / 216ms** |
| không khớp (xoá ngày / tháng) | 5.514 / 6.843ms | 1.769–1.992ms | **337 / 446ms** |
| mã đơn hiếm | 5.785ms | — | **448ms** |
| SĐT / tên sản phẩm | 12.193 / 12.086ms | 1.759 / 2.204ms | 713 / 899ms |
| "Nguyễn" / "Thảo" (vượt trần ⇒ đường cũ) | 12.557 / 13.047ms | 1.691–2.024ms | 2.157–2.304ms (phạt tra ngram) |
| export tháng này / xoá ngày | 16.199 / 18.631ms | 10.329 / 10.806ms | **6.920 / 8.313ms** |
| export "Nguyễn" / không khớp | 12.532 / 2.942ms | 5.086 / 2.274ms | 4.844 / **703ms** |
| 1 CN export tháng / xoá ngày | 8.611 / 7.190ms | 1.376 / 1.070ms | **812 / 750ms** |

⚠ Lượt 2 đo lúc máy có tải: ca không từ khoá toàn CN ra 1,5–2,4s (lượt 1 đo 1,25–1,35s trên cùng code) — so sánh
các ca từ khoá theo cùng lượt chạy. Đối chiếu body **38/38** (19 ca × 2 chế độ), export **10/10**. 16 test (8 DryRun
+ 8 integration); mutation lượt 2 **7/7** (5 nhánh ngram + cổng + `CountRows`).

**LOẠI:** tăng số luồng (khuôn công nợ trả góp: 8 luồng chỉ ~10%, gấp đôi kết nối — không đo lại).

**Còn lại:** mặc định toàn CN ~1,3s và từ khoá phổ biến ~2,2s là sàn "tính phiếu thu cho 180k dòng sản phẩm";
export toàn CN ~7–8s là 1 lượt đếm + 1 con trỏ đọc toàn bộ. Nhanh hơn cần lưu sẵn "đã thu" theo dòng (đổi schema).

### Thu theo chi nhánh — lần thứ N khuôn COUNT/SUM kéo JOIN, cộng tra id trước cho trang sâu

`GET /accountant/payment-branch/list` (+ `?_export=1`, export nhiều sheet theo chi nhánh), /check-scale 2026-09-17.
Bench dựng TỪ MIGRATION: 600.000 phiếu (90% `payment_for = 4`, 4% `amount = 0` — theo phân bố dev) trải 3 năm /
600.000 khách tên thật / 300.000 dịch vụ. ⚠ Giữa lượt dựng bench, MySQL bị `SHUTDOWN` từ bên ngoài (log
`Received SHUTDOWN from user`, không phải phiên này) ⇒ seed bị cắt, phải dựng lại.

**Cơ chế (cùng bệnh `GET /accountant/payment` 2026-09-14):** dòng tổng `SUM` kéo `LEFT JOIN customer` +
`LEFT JOIN order_service_used` + ORDER BY; `Paginate` đếm bằng COUNT bọc NGUYÊN câu SELECT có JOIN ⇒ 2 lượt × JOIN
mỗi dòng; lỗi câu SUM bị nuốt. Toàn CN xoá ngày: SUM 4,1s + COUNT 3,8–4,0s.

**Vá (không thêm chỉ mục):**
1. Câu tổng COUNT+SUM 1 lượt, không JOIN, không ORDER BY; `FORCE INDEX (idx_payment_company_time)` CHỈ khi toàn
   chi nhánh (1 năm 643–721 → 86–99ms; 1 CN không lợi).
2. Câu trang **tra id trước** trên riêng `payment` rồi mới JOIN 20 dòng. Hint câu tra id khi toàn CN + có dải
   `payment_time` + (offset ≥ 2.000 hoặc `sort` người dùng chọn) — ⚠ trang 1 mặc định ép thì TỆ hơn (dừng sớm theo
   PRIMARY 13–28ms · ép 108–278ms).
3. Từ khoá: tra id khách qua ngram (`MATCH AND LIKE` fullname/code/phone) ⇒ `customer_id IN … OR payment.code LIKE`,
   không JOIN — CHỈ toàn CN + (không lọc ngày hoặc dải > 31 ngày); tháng này JOIN thắng (45–67 vs 190–590ms), 1 CN
   lẫn lộn. Từ khoá luôn đi MỘT lượt stream (id, amount) cộng tổng + gom id ≤ 10.000 rồi trang tra theo id; toàn CN
   không lọc ngày chia 4 khoảng id song song ("Nguyễn" vượt trần ⇒ JOIN+LIKE: 2,5 → 1,1–1,2s).
4. Export: `pkg/export` `ExportMultiSheet` nay tôn trọng `Config.CountRows`; handler đếm bằng đúng đường của màn
   danh sách.

| Ca | 1 CN HEAD → vá | Toàn CN HEAD → vá |
|---|---|---|
| tháng này (mặc định FE) | 27–29 → **4–5ms** | 90–113 → **5–20ms** |
| 1 năm | 416–622 → **52–55ms** | 2.921–3.405 → **77–90ms** |
| xoá ngày | 839–1.164 → **52–54ms** | 6.236–7.385 → **179–203ms** |
| xoá ngày trang 500 | 724–1.400 → 66–69ms | 7.181–8.020 → 480–540ms |
| xoá ngày `sort=-created_at` | 802–1.062 → 88–95ms | 6.880–9.947 → 849–912ms |
| từ khoá không khớp / mã phiếu (xoá ngày) | 608–946 → 201–234ms | 7.359–8.056 → **112–272ms** |
| SĐT / "Nguyễn" (xoá ngày) | 470–606 → 206–237ms | 5.254–6.178 → 843–2.047ms |
| 1 năm + "Thảo" | 232–600 → 105–111ms | 2.587–3.770 → 1.149–1.996ms |
| lọc `payment_for` 3 (tháng) | 78–218 → 61–71ms | 90–98 → 3–4ms |
| export toàn CN 1 năm / không khớp xoá ngày | | 4.032 → 2.512 · 2.475 → 1.446ms |

**LOẠI (có số):** `FORCE INDEX` cho câu trang 1 mặc định (tệ hơn, xem trên); tra id qua ngram ở tháng này / 1 CN (tệ
hơn hoặc lẫn lộn); chia luồng khi có dải ngày (1 năm "Thảo" FORCE 858–905 · 4 luồng 816–840ms — không lợi).

Đối chiếu body JSON **32/32** (16 ca × 2 chế độ, đo 3 lượt), export **10/10**. 4 test DryRun + 1 test `pkg/export`;
mutation **9/9**.

**Còn lại:** sắp xếp `created_at` toàn CN không lọc ngày ~0,9s (filesort 556k dòng, `created_at` không có chỉ mục
phủ); từ khoá phổ biến vượt trần toàn CN 1,6–2,0s; trang 500 toàn CN không lọc ngày ~0,5s.

### Dashboard Workspace — `COALESCE(cột ngày, cột ngày)` trong WHERE giết mọi chỉ mục

`GET /workspace/dashboard/revenue-by-branch-payment` + anh em cùng trang `revenue-by-payment-method`, /check-scale
2026-09-17. Bench dựng TỪ MIGRATION: 600.000 phiếu trải 3 năm, 0,8% thiếu `payment_of_date` (đúng tỉ lệ DB dev
379/46.564). ⚠ Seed lần đầu hỏng (`payment.branch_id` NOT NULL) ⇒ bảng rỗng, mọi số 0,3ms — dòng đối chứng
`ĐỐI CHỨNG payment=0` bắt được; không có dòng đó thì đã kết luận "không có nút thắt".

**Cơ chế:** `WHERE COALESCE(p.payment_of_date, p.payment_time) BETWEEN ? AND ?` — bọc cột trong hàm ⇒ không chỉ mục
nào dùng được ⇒ toàn chi nhánh quét TRỌN bảng mỗi lần mở dashboard, kể cả xem "hôm nay" (538–716ms mỗi endpoint).
1 chi nhánh không đau (37–60ms) vì đi `fk-payment-branch_id`. `payment_for` lọc trong `CASE` thay vì WHERE.

**Vá:** `((payment_of_date BETWEEN ? AND ?) OR (payment_of_date IS NULL AND payment_time BETWEEN ? AND ?))` — tương
đương từng dòng — hằng `paymentDateWhere` dùng chung 2 handler; `payment_for IN (…)` đẩy vào WHERE (loại khác chỉ
cộng 0 và nhóm chỉ có loại đó đã bị `HAVING > 0` loại sẵn). Không thêm chỉ mục.

| Ca (endpoint chi nhánh / phương thức) | Toàn CN HEAD → vá | 1 CN HEAD → vá |
|---|---|---|
| hôm nay | 538–716 / 547 → **10–27 / 9ms** | 37–47 / 37–61 → 8–10 / 8–11ms |
| tháng này (mặc định FE) | 560–616 / 556–711 → **35–39 / 30–31ms** | 38–48 / 38–48 → 20–30 / 18–22ms |
| tháng trước | 582–614 / 570–574 → 61–64 / 52ms | 42–48 / 39–52 → 40–42 / 40–41ms |
| 1 năm / 3 năm (FE không gửi) | ngang: ~0,9–1,2s / 1,4–2,0s | ngang |

**LOẠI (có số):** `UNION ALL` 2 nhánh `payment_of_date` / `payment_time` (tháng ngang, 3 năm 1,71 → 1,93–2,02s);
thêm `FORCE INDEX` từng nhánh (3 năm 2,62s, 1 CN 3 năm 107 → 800ms). Dải 1–3 năm là cộng 1/3–1/2 bảng — FE chỉ gửi
"hôm nay"/"tháng này" nên không đáng thêm chỉ mục phủ trên `payment`.

**Kèm sửa thứ tự:** `ORDER BY branch_name, thuc_thu DESC` không có khoá phụ ⇒ dòng hoà (cùng chi nhánh + cùng thực
thu) ra thứ tự tuỳ plan ⇒ thứ tự CỘT hình thức trên FE (dựng theo lần xuất hiện đầu) có thể nhảy giữa các lần tải.
Thêm `method_id` (cả 2 endpoint). Đối chiếu: nội dung **20/20** khớp (chuẩn hoá thứ tự), 19/20 khớp byte — ca lệch
duy nhất là đúng một cặp dòng hoà; bản mới chạy 2 lần khớp byte 20/20. 2 test DryRun, mutation **6/6**.

~~**Còn cùng bệnh, CHƯA vá:** `report_payment_by_method.go`, `fund/cashbook.go` + `balance.go`~~ → đã vá cùng ngày:

#### Lượt 2 — báo cáo theo phương thức, sổ quỹ, tồn quỹ

Bench riêng @600.000 phiếu (đặt cọc `payment_for = 3` để `payment_of_date` NULL như `beforeSave` ⇒ 4%) / 90 quỹ
(85 gắn chi nhánh × hình thức + 5 quỹ công ty) / 50.000 `fund_transaction`.

- **`GET /accountant/report/payment-by-method`**: cùng `COALESCE` ⇒ viết OR; kèm bắt lỗi SQL (trước bị nuốt ⇒ báo cáo rỗng
  HTTP 200). Tháng này 594–700 → **47–50ms**; 1–3 năm ngang (~0,76–0,84s / ~1,2–1,4s).
- **Tồn quỹ = cộng TOÀN BỘ lịch sử tới ngày D** ⇒ viết lại điều kiện ngày KHÔNG giúp (776–842ms mọi cách viết, kể cả bỏ qua
  chỉ mục). Hai việc có ích:
  - `balanceOfFund` (số dư đầu kỳ sổ quỹ + snapshot kiểm quỹ) trước gom MỌI (branch, method) + MỌI quỹ rồi chọn 1 (~780 +
    95ms) ⇒ lọc đúng quỹ trong SQL (method + branch_id nếu có; `fund_transaction` theo `fund_id` / quỹ đích): 48–64ms quỹ
    chi nhánh, 137–146ms quỹ công ty, 12–14ms giao dịch quỹ. Kèm trả `error` thay vì nuốt — snapshot kiểm quỹ lưu vĩnh
    viễn, lỗi SQL mà thành số 0 là ghi sai số hệ thống.
  - `GET /accountant/fund/balance` (mọi quỹ): chia 4 khoảng id song song khi xem toàn chi nhánh — 1 nhóm (branch, method) nằm
    ở nhiều khoảng nên PHẢI cộng dồn, không gán đè. 764–869 → **389–435ms**; 1 chi nhánh không chia (51–59ms).

| Endpoint (toàn chi nhánh) | HEAD (2 lượt) → vá |
|---|---|
| report tháng này | 594–700 → **47–50ms** |
| fund/balance hôm nay / 1 năm trước | 812–869 / 764–817 → **390–435 / 389–430ms** |
| cashbook quỹ chi nhánh tháng này / 1 năm | 836–926 / 778–854 → **73–77 / 76–81ms** |
| cashbook quỹ công ty tháng này / 1 năm | 925–1.065 / 900–964 → **162–172 / 274–284ms** |
| cashbook 1 chi nhánh (tài khoản bị scope) | 89–96 → 72–77ms |

Đối chiếu body **11/11** (2 lượt). 5 test DryRun (fund 4 + report 1), mutation **8/8** — `nuốt lỗi câu payment` ban đầu
XANH GIẢ (DryRun làm câu `fund_transaction` phía sau cũng lỗi ⇒ hàm vẫn trả lỗi), phải kiểm thêm "dừng ngay, không chạy
tiếp". `-race` trên spike đường song song: 0 race.

**Còn lại:** tồn quỹ toàn chi nhánh ~0,4s là cộng toàn lịch sử phiếu — nhanh hơn nữa cần lưu sẵn số dư theo kỳ (đổi
schema); báo cáo theo phương thức dải 1–3 năm ~0,8–1,3s (FE mặc định tháng).

### Thu/chi phòng khám — hint có CỔNG theo chi nhánh VÀ độ rộng dải ngày

`GET /accountant/payment/clinic-revenue-expenses` (/check-scale 2026-09-14). Bench dựng TỪ MIGRATION:
600.000 payment / **57.455 phiếu thu-chi** (`payment_for` 7/8, 10%) trải 24 tháng, 17 chi nhánh. ⚠ DB dev
có **0** phiếu thu/chi ⇒ không có phân bố thật để neo. Số ms lấy từ `EXPLAIN ANALYZE` trên SQL NGUYÊN
VĂN của handler lúc load 3–6; lượt spike cuối bị `mediaanalysisd` đẩy load lên 102 nên chỉ dùng dòng đọc.

**Nút thắt** — cùng bẫy ref-lookup ở trên: optimizer tra `code (company_id = 1)` đọc **600.000/600.000**
dòng rồi mới lọc `payment_for IN (7,8)`, trả **3 lần**/lần mở màn (COUNT của `Paginate` + SELECT + SUM).
Đường FE đi tới được: nút xoá bộ lọc ngày gọi `clearStartTime()` ⇒ `payment_time = null`.

| Toàn chi nhánh | Trước | Sau |
|---|---|---|
| KHÔNG lọc ngày — COUNT / SELECT / SUM | 674 / 868 / 1.512ms · 600.000 dòng | 162–201 / 200 / 171ms · 57.455 |
| … COUNT+SUM gộp 1 lượt | (2 lượt) 324ms | **159ms** |
| KHÔNG lọc, page=2000 | 4.135ms | 593ms |
| Dải 24 tháng | 677 / 758 / 617ms | 228 / 223 / 198ms |
| Tháng này (mặc định FE) | 29–39ms · 12.312 | **10–13ms · 1.232** (gộp: 4,8ms) |

**Cổng — hint THẮNG hay THUA tuỳ chi nhánh + dải ngày** (1 chi nhánh, A = nguyên trạng, C = FORCE
`idx_payment_company_time`, COUNT/SELECT/SUM):

| 1 chi nhánh | A | C |
|---|---|---|
| 1 tháng | 20 / 47 / 14ms | **2 / 2 / 2ms** |
| 3 tháng | 37 / 43 / 38ms | 8 / 10 / 9ms |
| 6 tháng | 38 / 44 / 38ms | 19 / 21 / 20ms |
| 12 tháng | 39 / 46 / 40ms | 39 / 44 / 42ms (hoà) |
| 24 tháng | 40 / 49 / 44ms | **80 / 147 / 113ms — TỆ hơn** |
| KHÔNG lọc | 299 / 172 / 109ms | **453 / 228 / 79ms — TỆ hơn** |

⇒ `clinicPaymentTable`: hint khi `branchActiveId = 0`, hoặc 1 chi nhánh có ĐỦ `gte`+`lte` và dải ≤
`define.ReportMaxRangeSec`; còn lại để optimizer tự chọn `fk-payment-branch_id`. Dòng InnoDB handler đọc
(spike, cả 3 câu): 1 chi nhánh tháng này **105.888 → 208**, toàn chi nhánh tháng này **36.942 → 2.526**,
1 chi nhánh không lọc 105.888 → 70.593 (cố ý không hint).

**Biến thể LOẠI có số đo:**

| Cách | Số đo | Vì sao loại |
|---|---|---|
| FORCE `idx_payment_for` (có sẵn) | tháng này 29–39 → **97–123ms**; không lọc 156–237ms | hỏng đường mặc định |
| Chỉ mục mới `(payment_for, company_id, branch_id, payment_time, amount)` | 0,2–3ms mặc định; không lọc auto SELECT **752ms**, FORCE 159ms | nhanh hơn C nhưng +25,6MB/600k + chi phí ghi trên `payment` (20 chỉ mục, bảng ghi nhiều nhất); C đã đủ |
| C + bỏ 3 `LEFT JOIN` ở COUNT/SUM | COUNT **Error 1054** (COUNT của `Paginate` bọc SELECT đọc cột `order_service_used`); SUM 171 → 165ms | không làm được / không mua được gì |

**Sửa kèm (user chốt):** `_keyword` trước đó AND riêng `payment.note LIKE` với nhóm khách/mã ⇒ tìm theo nội
dung phiếu gần như luôn ra 0 (đo: "tiền điện" 0 dòng dù đối chứng SQL độc lập có **154**); dòng Tổng lại
thiếu điều kiện note. Nay note nằm TRONG nhóm OR, list + Tổng + export dùng CHUNG `clinicPaymentFilters`.

**Đối chiếu:** 18 ca (9 bộ lọc × 2 chế độ chi nhánh) — **16 khớp tuyệt đối** (count + tổng tiền + danh
sách id), 2 ca từ khoá lệch ĐÚNG chủ đích (0 → 154 · 0 → 9, khớp SQL độc lập). 3 test DryRun, mutation
5/5 đỏ (1 mutant đầu không biên dịch được — đã cấy lại bằng mutant hợp lệ).

### Đơn hàng Kế toán — COUNT của `Paginate` tính subquery tương quan cho MỌI dòng

`GET /accountant/order-plan` (/check-scale 2026-09-14; trước đó là `GET /crm/order-plan`, đã dời module vì
chỉ màn Kế toán › Đơn hàng dùng). Bench dựng TỪ MIGRATION: 600.000 đơn / 900.000 dịch vụ / 300.000 khách,
17 chi nhánh, 24 tháng (dev 74.585 đơn/15 tháng ⇒ ×8). Load máy 15–21 suốt phiên ⇒ đọc xu hướng.

**Nút thắt 1 — COUNT vật chất hoá câu SELECT kèm subquery.** Câu SELECT có
`COALESCE((SELECT SUM(osu.totalEnd) … WHERE osu.treatment_plan_id = otp.id), 0) AS total`. `Paginate` bọc
nguyên câu vào `COUNT(*) FROM (…)` ⇒ MySQL **Materialize** bảng dẫn xuất và tính subquery cho mọi đơn khớp
(`EXPLAIN`: `Aggregate: sum(osu.totalEnd) … loops=600000`) chỉ để đếm. Vá: đếm THẲNG trên bộ lọc +
`PaginateWithTotal`.

| COUNT, toàn chi nhánh | Gốc | Đếm thẳng |
|---|---|---|
| Không lọc (nút xoá ngày FE) | 3.280ms | 118ms |
| Không lọc, page=2000 | 5.422ms | 118ms |
| 1 năm | 3.432ms | 644ms → **84ms** (+`USE INDEX`) |
| Tháng này (mặc định FE) | 66ms | 3,9ms |

Số đếm khớp 18/18 bộ lọc. ⚠ Khuôn này đáng quét toàn repo: MỌI list có subquery tương quan trong
`Select` rồi đi qua `pagination.Paginate` đều trả chi phí đó ở vế COUNT.

**Nút thắt 2 — early-exit trên PRIMARY lúc thắng lúc thua.** `ORDER BY id DESC LIMIT 20` + từ khoá hiếm ⇒
quét ngược cả 600.000 đơn (484ms tháng này). Dữ liệu dày (toàn chi nhánh, không từ khoá) thì early-exit rẻ
0ms và `USE INDEX` lại TỆ hơn (tháng này 17ms, 1 năm 485ms). ⇒ 2 cổng riêng (`orderPlanIndexHints`):

| | Không hint | `USE INDEX (idx_otp_company_created_time)` |
|---|---|---|
| COUNT 1 CN 6 tháng / 12 tháng / 24 tháng / không lọc | 54 / 55 / 54 / 58ms | 35 / 60 / 121 / 92ms |
| SELECT 1 CN 1 tháng / 6 tháng | 64 / 69ms | 5 / **451ms** |
| SELECT toàn CN, từ khoá KHÔNG khớp 1T / 3T | 525 / 656ms | 120 / 351ms |
| SELECT toàn CN, từ khoá CÓ khớp 1T / 3T | 25 / 28ms | 59 / 147ms |

⇒ COUNT hint khi không lọc theo 1 chi nhánh (hoặc bật xem chéo chi nhánh), hoặc dải ngày ≤ 183 ngày;
SELECT hint chỉ khi dải ≤ 31 ngày VÀ (có từ khoá hoặc đang lọc 1 chi nhánh). `FORCE INDEX` LOẠI: COUNT toàn
CN từ khoá 24 tháng 3.094ms (USE 2.150, không hint 2.404).

**Sau vá, endpoint cache lạnh** (toàn CN / 1 CN): tháng này 67 → **6–7ms** / 139 → **9–13ms** · 1 năm
1.779 → **75–80ms** / 232 → 103ms · không lọc 3.095 → **102–111ms** / 450–2.275 → 101ms · page=2000
3.156–3.773 → **126–137ms** · từ khoá không khớp tháng này 519–543 → **90ms** / 78–415 → **9,5ms**.
Đối chiếu trước/sau 18/18 ca khớp (count + danh sách id). 3 test (DryRun + bảng cổng + tên chỉ mục khớp
migration), mutation 4/4 đỏ.

**Vá tiếp cùng ngày — từ khoá qua FULLTEXT ngram** (`accountant/order_plan/handler/list_search.go`). Ca còn
lại là toàn chi nhánh + từ khoá + KHÔNG lọc ngày = 4,4–6,1s (`LIKE '%kw%'` xuyên `customer` trên 600k đơn).
Áp khuôn `crm/appointment`: tra id khách trước bằng `MATCH(search_ngram) AND LIKE`, lọc `customer_id IN (…)`,
bỏ JOIN; cột bảng chính `code/note` giữ `LIKE` inline. Bench dựng lại từ migration, đối chứng chỉ mục ngram
`MATCH∧LIKE = LIKE` 9/9 từ khoá TRƯỚC khi đo. Đo ĐẦU-CUỐI trên handler thật (A cũ · B mới), 60 ca × count +
danh sách id khớp tuyệt đối:

| Ca | A | B |
|---|---|---|
| Toàn CN, không lọc ngày: không khớp / SĐT 10 số | 4.506 / 4.237ms | **1.252 / 1.244ms** |
| … tên đầy đủ / ghi chú đơn / nickname | 2.346 / 2.123 / 2.133ms | **825 / 620 / 726ms** |
| Toàn CN, 12 tháng: không khớp / tên / SĐT | 2.846 / 1.252 / 2.280ms | 1.067 / 497 / 957ms |
| 1 CN, không lọc ngày: không khớp / ghi chú / SĐT | 349 / 232 / 324ms | 94 / 103 / 256ms |
| ⚠ Toàn CN, 3 tháng: tên đầy đủ | 324ms | **978–2.064ms (TỆ hơn)** |
| ⚠ 1 CN, 3 tháng SĐT / 12 tháng tên | 105 / 218ms | **230 / 273ms (TỆ hơn)** |
| ⚠ "Nguyễn" (37.500 khách > trần 10.000 ⇒ rơi về A SAU khi đã tốn MATCH) | 297–2.355ms | +200–500ms |
| Tháng này (mọi chế độ) | 10–95ms | ngang |

⇒ Cổng `orderPlanKeywordUseNgram` = vùng B thắng rõ: **KHÔNG lọc ngày** (mọi chế độ), hoặc **toàn chi nhánh
với dải > 183 ngày**. Tháng này / 1 chi nhánh có dải / toàn CN ≤ 183 ngày giữ đường cũ. ⚠ Khoản phạt từ khoá
phổ biến ở vùng bật cổng là cái giá đã chấp nhận (không dò trước được độ phổ biến — FULLTEXT không thoát sớm
theo LIMIT). Sàn còn lại ~0,6–1,3s là `LIKE` inline trên `code/note` 600k đơn. 5 test (bảng cổng · điều kiện
0 khách vẫn giữ 2 vế bảng chính · tháng này đi đường cũ · không lọc ngày đi ngram · MATCH lỗi 1191 cấy qua
callback GORM ⇒ rơi về LIKE), mutation 5/5 đỏ.

Test đường lui khi MATCH lỗi: xem `testing-backend/SKILL.md` §"Test DryRun".

### Phiếu thu Kế toán — COUNT + SUM trả tiền 4 LEFT JOIN mà bộ lọc không dùng

`GET /accountant/payment` (/check-scale 2026-09-14). Bench dựng TỪ MIGRATION: 600.000 payment (97% `payment_for=4`)
/ 300.000 khách / 400.000 đơn / 600.000 osu, 17 chi nhánh, 24 tháng (dev 46.564 phiếu). Load 3–7.

**Nút thắt:** COUNT của `Paginate` bọc câu SELECT có 4 `LEFT JOIN` (customer · order_treatment_plan · osu ·
service), câu SUM riêng cũng mang đủ 4 JOIN ⇒ 2 lượt × 4 lần tra PK mỗi dòng dù bộ lọc mặc định chỉ chạm
`payment`. `EXPLAIN ANALYZE` không lọc ngày: `Index lookup on payment using code (company_id = 1)` 600.000 dòng
rồi 4 × `Single-row index lookup … loops=588000` = 3,7s; cùng bộ lọc KHÔNG JOIN: `Covering index lookup …
idx_payment_company_time` 188ms.

| Toàn chi nhánh trừ khi ghi | Trước: COUNT + SUM (dòng đọc/câu) | Sau: 1 câu gộp |
|---|---|---|
| Tháng này (mặc định FE) | 0,22 + 0,08s · 48.547 | **12–29ms** · 9.863 |
| 1 chi nhánh, tháng này | 17 + 19ms | 27–30ms (nhiễu) |
| KHÔNG lọc ngày (nút xoá ngày FE) | **3,34 + 3,24s** · 2.568.001 | **0,19–0,21s** · 600.001 |
| 1 chi nhánh, không lọc ngày | 0,43 + 0,43s · 151.052 | 75–84ms · 35.294 |
| Trang 500 (OFFSET 9.980), SELECT | 74ms | không đổi |
| Tháng này + từ khoá không khớp | 49 + 53 + 53ms (cả SELECT) | không đổi — từ khoá cần JOIN |
| **Không lọc ngày + từ khoá không khớp** | 2,21 + 3,58 + 2,82s | 2,28–2,50 + 3,13–3,89s — **CÒN NỢ** |

**Vá:** `paymentFilters` trả kèm tập bảng mà bộ lọc CÓ HIỆU LỰC tham chiếu (đếm biểu thức WHERE trước/sau mỗi
bộ lọc — không DryRun, không parse param) ⇒ `withPaymentJoinsFor` chỉ JOIN bảng đó; COUNT + SUM 1 câu +
`PaginateWithTotal`. Câu SELECT trang giữ nguyên. Thiếu JOIN = Error 1054 ⇒ test neo mọi cột lọc thuộc bảng biết nối.

**Đối chiếu:** 16 bộ lọc (từ khoá, `source_id`, `service_type`, `service_id`, `_listRevenues`, `created_time`,
cột customer, phân trang, 5 ca 0 dòng) chạy HANDLER THẬT trên bench — body JSON khớp byte-for-byte. 3 test, mutation 4/4.
⚠ Test dùng driver giả trả 0 dòng, KHÔNG DryRun: dưới DryRun `Scan` câu tổng lỗi ⇒ handler return trước
`Paginate` ⇒ mutant "quay lại Paginate" XANH GIẢ.

**Vá tiếp cùng ngày (lượt 2)** — bench dựng lại: 600.000 phiếu (85% for=4 · 8% cọc · 3% hoàn cọc · 3% hoàn dịch vụ),
300.000 khách / 400.000 đơn / 300.000 lịch hẹn. Đo handler THẬT: A = HEAD (qua `go test -overlay`, không đụng
working tree) · B = code mới, đo liền nhau cùng mức load 5–11. Body JSON 2 bên khớp byte-for-byte mọi ca, trừ 1 ca
lệch đúng chủ đích (dưới).

1. **`expandPaymentItems` — thủ phạm ẩn của MỌI list payment.** `appointment.id IN (SELECT MAX(id) … GROUP BY customer_id)`
   ⇒ quét 300.000 lịch hẹn, dò subquery đã materialize từng dòng (`loops=300001`): **330–1.180ms cho 20 khách**. JOIN bảng
   dẫn xuất: 16 dòng / 0,1ms. Hoàn cọc tháng này 1.634 → **7ms**, danh sách phiếu tháng này 118 → 17ms.
2. **Gộp dòng tổng + bỏ JOIN thừa cho 6 list** (`paymentAggregate`): cọc trang 50 **3.960 → 97ms** · lễ tân không lọc ngày
   **5.927 → 160ms** · hoàn cọc/dịch vụ (`list-refund-deposit-service`) 2.047 → 137ms · ký gửi không lọc 289 → 53ms.
3. **Từ khoá qua FULLTEXT ngram** (`list_search.go`, khuôn Đơn hàng Kế toán; cột khách ⇒ id qua `MATCH ∧ LIKE`, `order_treatment_plan.code`
   ⇒ id qua `LIKE` 66–83ms, cột `payment.*` giữ LIKE inline). Đối chứng chỉ mục `MATCH ∧ LIKE = LIKE` 7/7 từ khoá trước khi đo.

| `GET /accountant/payment` + từ khoá | A | B |
|---|---|---|
| Toàn CN, không lọc ngày: không khớp / mã phiếu / SĐT / "Nguyễn"* | 7.835 / 7.683 / 4.315 / 5.372 | **1.063 / 879 / 1.046 / 2.416** |
| Toàn CN, 12 / 6 / 3 tháng, không khớp | 4.797 / 3.131 / 2.133 | **602 / 328 / 226** |
| Toàn CN, 3 tháng, mã KH / "Nguyễn"* | 2.262 / 1.887 | 944 / 1.106 |
| ⚠ Toàn CN, tháng này: mã KH / SĐT / "Nguyễn"* | 128 / 131 / 226 | **244 / 284 / 392 (TỆ hơn)** |
| 1 CN, không lọc ngày: không khớp / "Linh"* / SĐT | 1.433 / 1.117 / 1.220 | 889 / 426 / 1.146 |
| ⚠ 1 CN, 3 tháng / 12 tháng: "Nguyễn"* | 223 / 323 | **413 / 454 (TỆ hơn)** |

(*) vượt trần 10.000 id ⇒ rơi về đường cũ sau khi đã tốn MATCH. ⇒ Cổng `paymentKeywordGate`: bật khi KHÔNG lọc ngày (mọi
chế độ) hoặc nhiều chi nhánh với dải > 31 ngày; "1 chi nhánh" = chi nhánh active HOẶC `filter[branch_id]` đúng 1 giá trị.
4. **List cọc MKT, lọc ngày hẹn**: bảng dẫn xuất `MAX(id) GROUP BY customer_id` trên TOÀN BỘ `appointment` (materialize
   300.000 nhóm) → subquery tương quan theo khách: 631 → **18ms** (SQL), endpoint 1.112 → 41ms; đối chiếu 3 bộ lọc khớp.

**Sửa đúng/sai kèm:** (a) dòng tổng `get-customer-deposit` thiếu bộ lọc `deposit_type` — lọc loại 2 (0 dòng) mà tổng vẫn
**8,76 tỷ**, nay 0 (ca lệch duy nhất trong đối chiếu); (b) export `ListPayment` bỏ qua bộ lọc `service_type`; (c) lỗi SQL
của các câu SUM riêng bị nuốt ⇒ nay `FailQuery`.

**Test:** 16 test trong package, mutation 9/9 (cổng tắt/bật · MATCH lỗi không lui · không vế nào ⇒ `1 = 1` · quay lại `Paginate` · tổng
cọc bỏ `deposit_type` · `expandPaymentItems` quay lại `IN` · MKT quay lại bảng dẫn xuất · bỏ qua `branch_id` 1 giá trị).

**Lượt 3 — các list còn lại dùng chung bộ lọc** (bench dựng lại, thêm 48.000 phiếu thu/chi `payment_for` 7/8; A = HEAD qua
overlay · B = code mới; body JSON khớp byte-for-byte 50/50 ca):

- **`get-list-receipts-of-customer`** (lịch sử thu, FE mặc định tháng này, KHÔNG lọc theo khách): cùng khuôn COUNT bọc
  SELECT 4 JOIN + SUM riêng ⇒ `paymentAggregate`. Không lọc ngày **5.570 → 177ms**, trang 50 6.144 → 181ms, tháng này
  120 → 5ms; từ khoá (ngram): mã KH không lọc ngày **14.153 → 1.396ms**, không khớp 7.243 → 551ms, SĐT 6.279 → 989ms.
- **`clinic-revenue-expenses` KHÔNG đi ngram** (chặn bằng `slices.Equal(cols, paymentKeySearch)`): bảng con ~8% phiếu, đã
  có FORCE INDEX nên LIKE vốn rẻ; ngram chỉ thêm chi phí MATCH — "Nguyễn" 3 tháng 74 → **354ms**, không lọc ngày 617 → 834ms.
  ⇒ Đường nhanh chỉ áp cho bộ cột ĐÃ ĐO; bộ cột khác phải bench riêng trước khi mở.
- **`get-customer-deposit-mkt`**: bảng dẫn xuất "phiếu cọc mới nhất mỗi khách" bị tính 3 lần (COUNT của `Paginate` bọc
  SELECT + trang + SUM riêng) ⇒ COUNT + SUM gộp 1 câu: không lọc ngày **819 → 411ms**, trang 50 814 → 392ms, lọc ngày
  hẹn 1 năm 2.398 → 691ms (kèm subquery tương quan ở trên), từ khoá SĐT không lọc ngày 2.263 → 267ms.
- **Lượt 4 — câu TRANG của list cọc MKT chọn chiến lược theo tổng vừa đếm** (`mktPageStrategy`). "Mới nhất mỗi khách"
  viết `NOT EXISTS (phiếu mới hơn cùng khách)` thì trang nông thoát sớm (chi phí ∝ offset); bảng dẫn xuất thì chi phí ∝
  tổng, không đổi theo trang. Đo riêng câu trang (best-of-2):

  | | Bảng dẫn xuất | NOT EXISTS |
  |---|---|---|
  | Toàn CN không lọc ngày (12.215): trang 1 / 10 / 20 / 50 | 225 / 232 / 234 / 232ms | **28 / 136** / 247 / 590ms |
  | 1 CN (909): trang 1 / 10 / 50 | 36 / 40 / 39ms | 39 / 128 / 262ms |
  | Lọc ngày hẹn 1 năm (6.095): trang 1 / 10 / 50 | 351 / 337 / 338ms | **22 / 40 / 153ms** |

  ⇒ `NOT EXISTS` khi `offset + perPage ≤ tổng/40`; tổng = 0 hoặc offset ≥ tổng ⇒ câu `1 = 0`. Câu TỔNG giữ bảng dẫn xuất
  (NOT EXISTS ở đó CHẬM hơn 0,31 → 0,51s; gộp tổng vào trang bằng window function cũng chậm hơn, 273.001 dòng đọc).
  Endpoint (HEAD → mới): không lọc ngày 694 → **166ms** · ngày hẹn 1 năm 1.963 → **304ms** · trang 50 686 → 365ms ·
  trang vượt tổng 564 → 147ms · từ khoá không khớp 381 → 128ms. Bản `NOT EXISTS` KHÔNG cổng từng làm trang 50 686 → 1.146ms,
  trang vượt tổng 2.583 → 4.946ms ⇒ cổng là bắt buộc.
- ⛔ `NOT EXISTS` thay `GROUP BY customer_id` phải so khách bằng `<=>`: GROUP BY gom mọi phiếu KHÔNG gắn khách vào MỘT nhóm.
  Đo với 2.572 phiếu cọc không khách: `<=>` khớp tuyệt đối (12.215 dòng · SUM(id) · SUM(id²)); `=` trả **14.786** dòng.
- Điều kiện cấp phiếu dùng CHUNG cho câu chính và subquery `p2` (`mktPaymentConds(c, q, alias)`) — thêm bộ lọc chỉ ở một nơi
  là trang và dòng tổng lệch nhau im lặng. Điều kiện cấp khách không lặp trong `p2` (cùng khách ⇒ cùng kết quả).
- Đối chiếu handler thật: 16/16 body khớp. Test package 21, mutation 5/5 (`<=>`→`=` · p2 bỏ điều kiện phiếu · bỏ NOT EXISTS ·
  ngưỡng ·40→·1 · bỏ nhánh rỗng).
- Test: package còn 18 test sau lượt 3; mutation lượt 3 thêm 3/3 (bỏ chặn thu/chi · MKT / lịch sử thu quay lại `Paginate`).

### Báo cáo Doanh thu theo ngày — baseSQL UNION chạy 2 lần, 3 nhánh cùng dính ref-lookup

`GET /accountant/report/payment-by-date` (/check-scale 2026-09-14, lượt 2 — lượt 1 ngày 2026-09-09 chỉ vá bộ lọc
`source_id`). Bench dựng TỪ MIGRATION: 600.000 lịch hẹn (3/7 loại tính doanh thu) / 400.000 `order_treatment_plan` /
600.000 osu / 600.000 payment, 17 chi nhánh, 24 tháng. Load 6–9 (máy có phiên khác) ⇒ lấy lượt 2, số dao động ±30%.

**Hai nút thắt chồng nhau:**
1. baseSQL (3 nhánh `UNION ALL`, mỗi nhánh `GROUP BY` ngày) chạy **HAI lần** — một câu `COUNT/SUM` bọc nó cho dòng
   tổng, một câu `LIMIT/OFFSET` bọc nó cho trang. Kết quả chỉ 1 dòng/ngày (24 tháng = 734 dòng) ⇒ tải hết 1 lần,
   cộng tổng + cắt trang trong Go (decimal, NULL = 0 như `COALESCE`).
2. Cả 3 nhánh chọn sai bảng dẫn: `order_treatment_plan`/`payment` tra `code (company_id = 1)` đọc toàn bảng (mục trên);
   `appointment` dẫn từ `appointment_type` (7 dòng) qua chỉ mục FK rồi lọc ngày trên 257.000 dòng.

| Nhánh (`EXPLAIN ANALYZE`, lượt 2) | 1 năm | Không lọc ngày | Tháng này | Đúng 1 chi nhánh |
|---|---|---|---|---|
| appointment + `FORCE (idx_appointment_company_date)` | 400 → 218ms | hoà (513 / 502–614) | 15–61 → 3,5ms | — |
| order_treatment_plan + `FORCE (idx_otp_company_created_time)` | 1.294 → 739ms | 1.936–2.822 → 1.732–1.844 | hoà | hoà |
| payment + `FORCE (idx_payment_company_time)` | 729 → 540ms | **939 → 1.178–1.243 (TỆ)** | hoà | **74–129 → 267–486 (TỆ)** |

⇒ `paymentByDateIndexHints`: không hint khi lọc ĐÚNG 1 chi nhánh (optimizer tự chọn `fk-…-branch_id`); payment chỉ
hint khi có mốc ngày bắt đầu — chỉ mục covering thiếu `deposit_after` nên quét cả bảng qua nó vẫn tra PK từng dòng.

**Toàn request** (handler thật, lượt 2, A = HEAD qua `go test -overlay`):

| Ca | Trước | Sau |
|---|---|---|
| 1 ngày | 6ms | 3ms |
| tháng này (mặc định FE) | 98ms | 45ms |
| 1 năm | 5.322ms | 1.360ms |
| không lọc ngày | 10.154ms | 2.842ms |
| không lọc ngày, trang 30 | 7.639ms | 2.704ms |
| không lọc ngày, `per-page=200` trang 4 | 7.533ms | 2.733ms |
| chỉ có `lte` | 4.516ms | 1.664ms |
| 1 năm, lọc 2 chi nhánh | 2.815ms | 386ms |
| lọc 1 chi nhánh, không lọc ngày | 698ms | 292ms |
| tháng này, lọc nguồn khách | 176ms | 60ms |

**Đối chiếu:** body JSON **byte-for-byte** 14/14 ca (gồm trang vượt cuối ⇒ `items: []`, cửa sổ rỗng ⇒ tổng `"0"`).
Test `report_payment_by_date_scale_test.go` chạy CHÍNH handler trên driver giả; mutation 6/6 đỏ (bỏ cổng 1 chi nhánh,
hint payment vô điều kiện, cộng đè, nuốt lỗi parse, bỏ kẹp offset, chạy baseSQL 2 lần).

**LOẠI có số đo:** `IGNORE INDEX (code)` — payment rơi sang `id_old (company_id)`, cùng bẫy (701–840ms); chỉ mục mới
`order_service_used (treatment_plan_id, totalEnd)` cho subquery doanh số — 739 → 539–605ms, không đáng một chỉ mục trên
bảng ghi nhiều; subquery tương quan → `LEFT JOIN` + `SUM` theo ngày — 655–1.859ms, dao động, không hơn.

**Export (vá cùng ngày):** bản song sinh `export/report_revenue_by_date.go` dùng CHUNG cổng hint (`export.PaymentByDateIndexHints`
— hàm nằm ở package export vì report import export). Nó còn trả thêm một lượt baseSQL qua câu
`SELECT COUNT(*) FROM (<query>)` của `pkg/export.Export` (chỉ để chặn "không có dữ liệu" + chọn Excel/CSV) ⇒ thêm
`Config.BoundedRows`: báo cáo chắc chắn < `CSVThreshold` dòng thì bỏ COUNT, ghi thẳng Excel, 0 dòng trả đúng lỗi cũ và
không để lại file. Export: tháng 93 → 46ms · 1 năm 4.050 → 1.211ms · không lọc ngày 5.573 → 2.319ms · 1 năm 1 chi nhánh
428 → 216ms. Nội dung sheet (`xl/worksheets/*` + `sharedStrings`) khớp byte 4/4 ca. KHÔNG bật `BoundedRows` cho export dạng
danh sách — vượt ngưỡng mà vẫn ghi Excel là giữ cả file trong RAM (~7,5 KB/dòng, mục Export ở dưới).

### Báo cáo Doanh thu theo chi nhánh — 7 bảng dẫn xuất chạy 2 lần, 4 lượt quét payment cùng dải ngày

`GET /accountant/report/payment-by-branch` (/check-scale 2026-09-14). Cùng bench mục trên + 17 chi nhánh. FE mặc định
`created_at` = đầu tháng → cuối hôm nay; nút "xoá ngày bắt đầu" gửi `gte=0`.

**Nút thắt** (`EXPLAIN ANALYZE` từng bảng dẫn xuất, 1 năm): A lịch hẹn ~0,5s · B osu **1,2s** (table scan + tra PK
customer + service) · C doanh thu **1,5s** (payment table scan + tra PK osu/customer/service) · D/E/F/G — **4 lượt**
quét `payment` cùng dải `payment_time`, ~1,05s cộng lại. Rồi toàn bộ baseSQL chạy **HAI lần** (câu `COUNT/SUM` bọc nó
cho dòng tổng + câu `LIMIT/OFFSET`).

**Sửa (không đổi kết quả):**
1. Một câu `WITH t AS (baseSQL) SELECT t.*, COALESCE(SUM(t.x) OVER (), 0) AS rpt_sum_x … ORDER BY` (helper dùng chung `queryPaymentReportOnce`) — dòng tổng bằng
   window function với ĐÚNG biểu thức câu tổng cũ (giữ kiểu/scale, kể cả `tbKhach` chia decimal), phân trang trong Go
   (kết quả ≤ số chi nhánh). Tập rỗng ⇒ mọi chỉ số `"0"` như `COALESCE` cũ.
2. D/E/F/G gộp MỘT lượt `SUM(CASE WHEN payment_for … THEN … END)`: 1,05s → 815–832ms.
3. `LEFT JOIN service` (PK, không nhân dòng) chỉ khi lọc nhóm dịch vụ: B 1.216 → 790ms · C 1.464 → 1.078ms.

| Ca (handler thật, lượt 2) | Trước | Sau |
|---|---|---|
| 1 ngày | 13ms | 10ms |
| tháng này (mặc định FE) | 439ms | 196ms |
| 1 năm | 7.502ms | 2.836ms |
| xoá ngày bắt đầu (`gte=0`) | 14.619ms | 4.777ms |
| 1 năm, lọc 2 nguồn khách | 15.359ms (câu tổng 13,3s — plan khác câu trang) | 1.315ms |
| 1 năm, lọc nhóm dịch vụ | 9.142ms | 1.569ms |
| 1 năm, 1 chi nhánh | 950ms | 386ms |

**Đối chiếu:** body JSON byte-for-byte 11/11 ca (gồm sort theo tên, trang 2 `per-page=5`, cửa sổ rỗng). Test
`report_payment_by_branch_scale_test.go` chạy CHÍNH handler trên driver giả; mutation 4/4 đỏ.

**Đo, KHÔNG ship — đổi hành vi, cần người quyết:** tenant đang lọc qua `customer.company_id` (LEFT JOIN customer bắt buộc
⇒ tra PK customer mọi dòng, và loại dòng không gắn khách). Lọc theo `company_id` của CHÍNH bảng dữ liệu (khuôn đã vá ở
`report_payment_order_by_service.go` 2026-07-28 vì khách đổi company làm lệch số): D 784 → ~640ms · B 1.216 → 194–456ms.
Đổi thì số liệu của khách đã chuyển company và phiếu không gắn khách sẽ đổi. 9 endpoint anh em: mục ngay dưới.

### 9 báo cáo doanh thu nhóm theo chiều — cùng khuôn, thêm bẫy dẫn từ `customer`

`GET /accountant/report/payment-by-{source,group-source,pathological,province,page-facebook,country,campain-event,interested,category}`
(/check-scale 2026-09-14). Cùng bench + danh mục chiều: 30 nguồn / 6 nhóm nguồn / 25 bệnh lý / 63 tỉnh / 15 page / 40 quốc
gia / 20 sự kiện / 18 nhóm quan tâm / 6 phân loại, 20% khách NULL ở chiều tuỳ chọn. FE mặc định đầu tháng → hôm nay.

**Cùng khuôn by-branch** (8/9 chạy baseSQL 2 lần — `interested` vốn đã 1 lần; 4 lượt quét payment; JOIN service vô điều
kiện) ⇒ áp CHUNG `queryPaymentReportOnce` + `paymentReportServiceJoin`, gộp lượt quét payment: 7 chiều-trên-khách còn 1
lượt; `source`/`group-source` còn 2 (D+G nhóm theo `osu.source_id` qua INNER JOIN osu, E+F theo `customer.source_id`).

**Bẫy riêng của 7 chiều nằm trên `customer`** (tỉnh, quốc gia, bệnh lý, page, sự kiện, nhóm quan tâm, phân loại): điều kiện
`customer.<chiều> IS NOT NULL` biến LEFT JOIN thành JOIN ⇒ optimizer được đảo thứ tự và chọn dẫn từ `customer (company_id)`
(DB-per-customer ⇒ cả 300.000 khách) rồi tra osu/payment theo từng khách: nhánh B 3.128ms, D 2.896ms (1 năm). Sau khi đã
chạy 1 lần + gộp quét, 1 năm VẪN ~9s ⇒ `paymentReportCustomerJoin` trả `STRAIGHT_JOIN` (ép dẫn bằng bảng dữ liệu, kết quả
như JOIN): B 938ms · D 952ms, tháng hoà. **Có cổng:** lọc theo khách (nguồn/marketing/sự kiện/nhóm nguồn/nhóm vùng/nhóm quốc
gia) thì để optimizer tự chọn — ép thứ tự làm ca lọc 2/30 nguồn 787 → 1.950ms (bộ lọc chọn lọc trên customer làm dẫn từ
customer rẻ hơn). KHÔNG áp cho source/group-source: customer ở đó chỉ mang vế tenant, không có vế loại NULL.

| Ca (handler thật, lượt 2) | 7 chiều-trên-khách (vd province) trước → sau | source / group-source trước → sau | interested trước → sau |
|---|---|---|---|
| tháng này (mặc định FE) | 490 → 116ms | 401 / 393 → 111 / 108ms | 230 → 113ms |
| 1 năm | 19.522–33.561 → 3.155–3.256ms | 6.573 / 5.861 → 2.495 / 2.452ms | 9.596 → 3.165ms |
| xoá ngày bắt đầu | 22.411–30.390 → 5.456–5.683ms | 9.060 / 8.607 → 3.908 / 4.732ms | 11.151 → 5.581ms |
| 1 năm, lọc 2 chi nhánh | 2.813–3.310 → 1.183–1.208ms | 2.959 / 2.790 → 1.360 / 1.349ms | 1.398 → 1.193ms |
| 1 năm, lọc nhóm dịch vụ | 7.331–7.915 → 1.707–1.756ms | 4.593 / 4.194 → 2.028 / 2.043ms | 3.665 → 1.719ms |
| 1 năm, lọc 2 nguồn | 1.782–2.183 → 608–773ms | 2.467 / 2.338 → 1.025 / 1.009ms | 1.053 → 768ms |

**Đối chiếu:** body JSON byte-for-byte **78/81** (9 endpoint × 9 ca); 3 ca lệch đều là `province` — cùng `_meta`, cùng
`summary`, cùng DÃY khoá sắp xếp, các dòng chung giống từng trường, chỉ đổi chỗ các tỉnh BẰNG NHAU `totalAccepted` ở biên
trang (SQL cũ `ORDER BY` không có khoá phụ ⇒ thứ tự dòng bằng nhau vốn không xác định). Test
`report_payment_by_dimension_scale_test.go` (9 endpoint, driver giả); mutation 5/5 đỏ.

**Cần người quyết (không đổi):** `group-source` nhánh thực thu dùng `payment_for IN (Order, MoveDeposit)`, 8 báo cáo còn lại
(và by-branch) dùng `(Order, Deposit)` — có từ trước đợt tái cấu trúc 2026-07-28, git không cho biết chủ đích.

### Doanh thu theo dịch vụ / nhóm dịch vụ — chạy 2 lần, bảng thực thu JOIN vào từng dòng osu

`GET /accountant/report/get-payment-order-by-service` (+ `?_export=1`) và `…-by-service-type` (/check-scale 2026-09-14). Cùng
bench (646 dịch vụ / 18 nhóm). FE mặc định `_created_at` đầu tháng → hôm nay; "xoá ngày bắt đầu" bỏ hẳn bộ lọc.

**Nút thắt** (`EXPLAIN ANALYZE`, 1 năm, ~6s/lượt): (1) baseSQL chạy **2 lần** (câu tổng + câu trang); export cũng 2 lần
(COUNT của `pkg/export` + con trỏ). (2) Bảng thực thu `act_pay` (1 dòng/dịch vụ) bị JOIN vào **từng dòng osu** (298.358 lần
tra) rồi `MAX` — ~1,9s. (3) Dẫn từ `customer (company_id)` vì vế tenant nằm trên customer — ~1,9s. (4) 2 bảng dẫn xuất payment
tra `code (company_id)` đọc cả 600.000 dòng — ~0,6s mỗi bảng.

**Sửa:** `queryReportOnce` (generic hoá helper của payment-by-*: dòng tổng `sumTotal*` bằng window function) · `act_pay` JOIN
SAU GROUP BY (checksum 646 dòng khớp tuyệt đối — MAX của một giá trị = chính nó) · export bật `BoundedRows` (1 dòng/dịch vụ).

| Biến thể 1 lượt, 1 năm (`EXPLAIN ANALYZE` lượt 2) | Thời gian | Quyết định |
|---|---|---|
| gốc | 5.971ms | — |
| STRAIGHT_JOIN customer | 4.884ms | LOẠI — sau khi JOIN act_pay sau GROUP BY chỉ còn 4.596ms (trong nhiễu), không đáng thêm cổng |
| act_pay JOIN sau GROUP BY | 4.608ms | GIỮ |
| + FORCE INDEX ngày cho `pay` | 4.476ms | LOẠI — trong nhiễu |

| Ca (handler thật, lượt 2) | service trước → sau | service-type trước → sau | export service trước → sau |
|---|---|---|---|
| tháng này (mặc định FE) | 220 → 124ms | 238 → 116ms | 248 → 126ms |
| 1 năm | 10.930 → 6.390ms | 11.789 → 5.475ms | 10.896 → 5.139ms |
| xoá ngày bắt đầu | 18.312 → 16.534ms (lượt đo nhiễu; `gte=0` 16.337 → 10.824ms) | 17.947 → 8.244ms | 17.733 → 7.545ms |
| 1 năm, trang 3 sort tên | 12.177 → 4.214ms | 11.536 → 5.373ms | — |
| 1 năm, lọc 2 chi nhánh | 4.501 → 2.199ms | 5.136 → 2.332ms | — |
| 1 năm, lọc 2 nguồn | 3.047 → 1.609ms | 3.078 → 1.520ms | — |

**Đối chiếu:** body JSON byte-for-byte 20/27 ca + nội dung sheet export 3/3 khớp byte. 7 ca lệch đều của `service` sort theo
`created_at` / `totalActualPayment` — `_meta` + 3 tổng giống hệt, dãy khoá sắp xếp giống hệt, mọi dòng chung giống từng trường,
chỉ đổi chỗ dòng BẰNG khoá (646 dịch vụ chỉ có ~13 giá trị `MAX(created_day)` khác nhau/trang ⇒ tie dày đặc; SQL cũ không có khoá
phụ). Test `report_payment_order_by_service_scale_test.go`; mutation 3/3 đỏ.

**Còn lại:** ~4,5s/lượt ở 1 năm là 2 bảng dẫn xuất payment tổng hợp cả dải + quét osu cả dải — bản chất câu hỏi. Sort mặc định
`-created_at` có tie dày đặc ⇒ thứ tự dòng bằng nhau đổi giữa các lần tải (từ trước khi vá) — thêm khoá phụ `service_id` là
đổi hành vi hiển thị, chưa làm.

### Doanh số theo nhân viên — bộ lọc ngày không có chỉ mục nào để vào

`GET /accountant/revenue-{doctor-consultant,telesale,direct-sale}` + `…/report-revenue` (/check-scale 2026-09-15). Code chung
`accountant/revenue_staff` (3 bảng cùng schema, khác mỗi cột nhân viên). Bench dựng TỪ MIGRATION: mỗi bảng 600.000 dòng /
600.000 osu / 300.000 khách, 17 chi nhánh, 40 nhân viên, 24 tháng. ⚠ DB dev: bác sĩ 0 dòng · telesale 5.396 · direct sale 262 ⇒
không có phân bố thật để neo.

**Nút thắt:** chỉ mục 1 cột `company_id`/`branch_id` + `(<nhân viên>, confirm_time)` ⇒ bộ lọc `confirm_time` mặc định FE (đầu
tháng → hôm nay) không có đường vào. Toàn chi nhánh: list 3 câu (SUM bọc bảng dẫn xuất · COUNT của `Paginate` · trang), tổng hợp
3 câu (SUM · COUNT bọc GROUP BY · trang GROUP BY) — đều `Index lookup … idx_*_company_id (company_id = 1)` **600.000/600.000**;
SUM + COUNT list còn mang `LEFT JOIN order_service_used` không đọc cột nào. Từ khoá: `LIKE '%kw%'` xuyên JOIN customer + osu.

**Vá:**
1. `(confirm_time, id, company_id, branch_id, revenue, <nhân viên>)` × 3 bảng (migration `20260915124101`) — `id` TƯỜNG MINH ở
   vị trí 2 khớp `ORDER BY confirm_time DESC, id DESC` (hậu tố PK ngầm nằm cuối không giữ thứ tự tie); phủ câu tổng list; cột
   nhân viên ở ĐUÔI phủ GROUP BY tổng hợp mà không đổi tiền tố.
2. List: COUNT + SUM 1 câu không JOIN + `PaginateWithTotal`. Tổng hợp: GROUP BY chạy MỘT lần, tổng + phân trang trong Go.
3. `FORCE INDEX` có cổng (`Spec.aggregateTable`) cho câu tổng list + GROUP BY; câu trang không hint.
4. Từ khoá (khuôn Phiếu thu Kế toán): id khách `MATCH ∧ LIKE` tra TRƯỚC (vượt trần 10.000 ⇒ thôi, khỏi quét osu), id dịch vụ
   `code LIKE` (~100ms cố định), lọc `IN` không JOIN; cổng `keywordUseFast`.

| Chỉ mục, toàn CN (`EXPLAIN ANALYZE` lượt 2), tháng / năm / không lọc | câu tổng list (auto · FORCE) | câu trang (auto) | GROUP BY tổng hợp (auto · FORCE) | Quyết định |
|---|---|---|---|---|
| không có | 538 / 519 / 530ms | 555 / 589 / 637ms | 21 / 586 / 612ms | — |
| **`(confirm_time, id, company_id, branch_id, revenue, <nhân viên>)`** | 3,65 · 3,73 / 587 · **96** / 495 · **143** | **3,1 / 3,2 / 0,14** | 5,6 · 6,2 / 572 · **144** / 578 · **227** | GIỮ |
| như trên, KHÔNG có cột nhân viên | như trên | như trên | 21 · 21 / 586 · **550** / 612 · 244 | LOẠI |
| `(confirm_time, company_id, branch_id, revenue)` | 3,75 · 3,68 / 515 · 96 / 503 · 147 | 2,7 / **637** / 271 (FORCE; mất thứ tự tie ⇒ Sort) | — | LOẠI |
| `(confirm_time)` một cột | — · 17,5 / — · **564** / — · 160 (không phủ) | 1,8 / 2,2 / 0,13 | — | LOẠI |

**Cổng hint** (auto → FORCE): ép khi xem nhiều chi nhánh (3 CN: năm 531 → 74 · không lọc 476 → 108ms). TỆ hơn khi ép ⇒ tắt: đúng 1
chi nhánh (câu tổng năm 35 → 64 · không lọc 34 → 98ms; GROUP BY 39 → 77 · 39 → 114ms) · lọc nhân viên (tháng 0,6 → 21 · năm
13 → 493ms) · có từ khoá (đường JOIN năm 1,86 → 5,2s; đường id: 0,2ms → 1,5s).

**Cổng từ khoá** (handler thật, đường JOIN → đường id): thắng ở toàn chi nhánh mọi dải (tháng tên 555 → 209 · năm 10.627 → 190 ·
không lọc tên 16.444 → 187, mã KH 14.974 → 230, không khớp 14.120 → 89ms) và ở 1 chi nhánh dải rộng (năm 644 → 269 · không lọc
942 → 329ms); THUA ở 1 chi nhánh dải ≤ 31 ngày (tên 170 → 218 · SĐT 168 → 322ms) ⇒ ca đó giữ JOIN (đo lại sau cổng: ~111ms). Vượt
trần ("Nguyễn" 21.428 khách) phạt +190–330ms — chấp nhận.

| Cả request (handler thật; H = HEAD đã có chỉ mục, qua `-overlay`; A = HEAD không chỉ mục) | trước | sau |
|---|---|---|
| List toàn CN, tháng này (bác sĩ / telesale / direct sale) | A 1.651–1.701 · H 164–177 / 328 / 305ms | **8 / 8 / 9** |
| List toàn CN, 1 năm | 1.733 / 1.771 / 1.778 | **89 / 89 / 88** |
| List toàn CN, không lọc ngày | 2.271 / 2.337 / 2.550–2.671 | **119 / 118 / 119** |
| List toàn CN, `page=2000` | 3.128 / 3.197 / 5.488–5.920 | 1.013–1.074 |
| List toàn CN, lọc 1 CN không lọc · 2 CN năm | 535–645 · 1.186–1.221 | 173–189 · 87–89 |
| List 1 CN: tháng / năm / không lọc | 150–161 / 232–236 / 264–270 | 44–68 / 79–102 / 78–97 |
| Tổng hợp toàn CN: tháng / năm / không lọc | 14–15 / 1.665–1.699 / 1.611–1.640 | **7 / 133–144 / 203–210** |
| Tổng hợp 1 CN: năm | 112–149 | 40–51 |
| List toàn CN, từ khoá tên: tháng / năm / không lọc | 555–772 / 10.627 / 16.444 | **187–209 / 190 / 187** |
| List toàn CN, từ khoá không khớp: năm / không lọc | 8.010 / 14.120 | **90 / 89** |
| ⚠ Từ khoá vượt trần, toàn CN không lọc | 2.659 | 2.985 (đường JOIN — còn lại) |

**Đối chiếu:** sha1 nguyên body JSON — HEAD vs bản vá **120/120** (6 endpoint × 10–13 bộ lọc × 2 chế độ chi nhánh, lặp lại trên bản
có cổng: 120/120); đường JOIN vs đường id **24/24** ca từ khoá nặng (vượt trần, không dấu, kèm lọc chi nhánh/nhân viên). Chỉ mục
ngram đối chứng `MATCH ∧ LIKE = LIKE` 13/13 từ khoá (có dấu/không dấu/hoa/thường) TRƯỚC khi đo. Test `revenue_staff_test.go`
(driver giả, 10 test), mutation 13/13.

⚠ `ApplyFilter`/`GetFilterValues` đọc mảng dạng có chỉ số `filter[x][in][0]=`; `filter[x][in][]=` bị bỏ qua IM LẶNG — dấu hiệu
khi bench: dòng đọc trùng khít ca không lọc.

**Còn lại:** `page=2000` toàn CN ~1,0s (chi phí OFFSET) · từ khoá vượt trần dải rộng đi đường JOIN · `workspace/wsrevenue` luôn
khoá `WHERE <nhân viên> = <user>` ⇒ O(số dòng của 1 người), không đo.

#### Hoa hồng bác sĩ tư vấn — cùng bệnh, dùng chung `revenue_staff`

`GET /accountant/commission-doctor-consultant` + `/report-commission` (/check-scale 2026-09-15). Bảng `order_commission_doctor_consultant`
có đúng bộ chỉ mục của 3 bảng doanh số trước khi vá ⇒ cùng bẫy `idx_ocdc_company_id`, nhưng tệ hơn: 1 chi nhánh cũng chậm
(list 1 ngày 377ms). Bench dựng từ migration, 600.000 dòng / 17 chi nhánh / 100 bác sĩ / 24 tháng, phân bố lấy theo 2 bảng hoa hồng
anh em trên dev (telesale 3.579 · direct sale 13.398 dòng; `confirm_commission` = 0 mọi dòng). ⚠ Bảng bác sĩ trên dev **0 dòng**.

**Vá:** chỉ mục `idx_ocdc_confirm_id_cover (confirm_time, id, company_id, branch_id, commissionDoctorConsultant, amount,
doctor_consultant_id)` (migration `20260915140921`, 32,6MB) + handler dùng `revenue_staff.CommissionDoctorConsultant`. `Spec` nay có
`Sums` (cột CHÍNH `commissionDoctorConsultant`/`totalCommission` + `amount`/`totalAmount`). Cổng hint + cổng từ khoá dùng chung —
đo lại trên bảng này ra cùng hình dạng (GROUP BY 1 chi nhánh 1 năm ép 36,9 → 70,4ms; lọc bác sĩ 3,95 → 59,6ms).

| Biến thể LOẠI | Số đo | Vì sao |
|---|---|---|
| Bỏ `amount` khỏi đuôi chỉ mục | GROUP BY toàn CN 1 năm ép 533ms (giữ amount 159) · 1 CN tháng auto 140ms (giữ 2,8) | GROUP BY tổng hợp không còn phủ |

| Cả request (handler thật) | Trước (không chỉ mục, load 4–13) | Sau (load 22–84, cặp xen kẽ) |
|---|---|---|
| List toàn CN: tháng / 1 năm / không lọc | 1.989–3.055 / 2.648–2.937 / 2.941–3.078ms | **11 / 111–114 / 157–169ms** |
| List toàn CN `page=2000` | 3.065–3.343 | 1.164–1.193 |
| List 1 CN: tháng / 1 năm / không lọc | 382–399 / 491–509 / 547–566 | **11–13 / 56–59 / 204–224** |
| Tổng hợp toàn CN: tháng / 1 năm / không lọc | 1.427–1.430 / 1.539–1.552 / 1.541–1.547 | **9–10 / 183–194 / 322–324** |
| Tổng hợp 1 CN: tháng / 1 năm | 442–507 / 511–616 | **4–6 / 54–55** |
| List toàn CN từ khoá tên / không khớp, không lọc ngày | 14.955 / 12.749 | **224 / 108** (load ~90–220) |
| ⚠ Từ khoá vượt trần, toàn CN không lọc | 5.948 | 2.931 (đường JOIN — còn lại) |

**Đối chiếu:** sha1 nguyên body JSON HEAD vs bản vá **66/66** (list 14 + tổng hợp 7 bộ lọc × 2 chế độ + 12 ca từ khoá nặng × 2) và lặp
lại cặp xen kẽ 42/42. ⚠ Lượt đo sau vá đầu tiên trùng lúc load 222 (ca 1 CN 1 năm ra 4,5–6,6s, `page=2000` 27s) — số vô nghĩa, đã đo lại
theo cặp H/N xen kẽ; đừng trích số lượt đó. Test `revenue_staff_test.go` 11 test, mutation 4/4 riêng phần hoa hồng (tổng hợp thiếu cột
SUM · dòng tổng cứng `revenue` · ORDER BY sai cột · sai tên chỉ mục).

#### Hoa hồng direct sale — lần thứ 6, chỉ mục KHÔNG kèm `amount`

`GET /accountant/commission_direct-sale` + `/report-commission` (/check-scale 2026-09-15). Bảng `order_commission_directSale` (chữ
`S` hoa — tên bảng khác quy ước) cùng bộ chỉ mục 1 cột ⇒ toàn chi nhánh mọi câu tra `fk-order_commission_directSale-company_id`
600.000/600.000 dòng (list SUM 556 · COUNT 542 · trang 534ms; tổng hợp 3 câu 508–517ms); 1 chi nhánh 148–154ms mỗi câu. Bench dựng từ
migration, 600.000 dòng / 17 chi nhánh / 116 direct sale / 24 tháng, phân bố theo dev (13.398 dòng, 116 nhân viên, 17 chi nhánh,
không NULL).

**Vá:** `idx_ocds_confirm_id_cover (confirm_time, id, company_id, branch_id, commissionDirectSale, direct_sales_id)` (migration
`20260915180237`, 26,6MB) + handler dùng `revenue_staff.CommissionDirectSale` với **1 cột SUM**. Khác 2 bảng hoa hồng anh em: màn
tổng hợp (và export tổng hợp) KHÔNG trả tiền thu, nên không kèm `amount` — chỉ mục nhỏ hơn 6MB mà không câu nào mất phủ. Cổng hint
đo lại ra đúng hình dạng (không hint → FORCE): câu tổng toàn CN 1 năm 507 → 88ms · không lọc 486 → 139ms; GROUP BY 547 → 134 ·
574 → 224ms; 3 chi nhánh 1 năm 460 → 60ms — tệ hơn khi ép: 1 CN GROUP BY 1 năm 36,6 → 63,6ms · lọc direct sale 1 năm 4,7 → 54,4ms.

| Cả request (handler thật) | Trước (không chỉ mục, load 2–4) | Sau (load 2–3) |
|---|---|---|
| List toàn CN: 1 ngày / tháng / 1 năm / không lọc | 1.469–1.507 / 1.513 / 2.175–2.180 / 2.725ms | **1,9–2,1 / 8,8–9,1 / 85 / 119–120ms** |
| List toàn CN `page=2000` | 3.044–3.121 | 1.133–1.146 |
| List 1 CN: 1 ngày / tháng / 1 năm / không lọc | 380–396 / 378–380 / 477–479 / 520 | **1,8 / 6,6–6,7 / 45 / 161–181** |
| Tổng hợp toàn CN: tháng / 1 năm / không lọc | 1.413–1.570 / 1.637–1.721 / 1.509–1.552 | **5,6–6,1 / 121 / 199–208** |
| Tổng hợp 1 CN: tháng / 1 năm | 371–372 / 378–396 | **3,3–3,6 / 37** |
| List toàn CN từ khoá tên / không khớp / mã KH, không lọc ngày | 14.874 / 14.367 / 14.625 | **253 / 89 / 229** |
| ⚠ Từ khoá vượt trần (`Nguyễn` / `Hải`), toàn CN không lọc | 6.002 / 6.101 | 2.842 / 2.784 (đường JOIN — còn lại) |
| ⚠ 1 CN active + từ khoá + bộ lọc khiến kết quả rỗng / hẹp (lọc CN khác / direct sale) | 1,0 / 32–55 | 253 / 303–321 — đường nhanh trả ~200ms tra id ngram TRƯỚC |

**Đối chiếu:** sha1 nguyên body HEAD vs bản vá **86/86** (list 14 + tổng hợp 7 bộ lọc + 12 ca từ khoá nặng + 10 ca từ khoá CÓ khớp, × 2
chế độ); 10 ca từ khoá có khớp ra **17/20** body có dữ liệu (tên có/không dấu ra CÙNG body ở cả 2 bản); 28/86 ca body rỗng ở cả 2 bản. Test `revenue_staff_test.go` 11 test; mutation 4/4 — test đỏ: sai tên chỉ mục · sai cột nhân viên · cột SUM
chính đổi sang `amount` (cả 3 qua `TestCoverIndexMatchesMigration`); đối chiếu body đỏ 12/14 ca list toàn CN (2 ca trùng là body rỗng, tổng vốn bằng 0): list trả `totalCommission` = 0.

**Ngoài phạm vi, chưa đo:** màn workspace "Hoa hồng direct sale của tôi" gọi endpoint riêng `GET /workspace/commission-direct-sale`,
KHÔNG đi qua handler này.

#### Hoa hồng bác sĩ điều trị — chỉ mục ngày có sẵn nhưng thiếu cột scope, và từ khoá LIKE xuyên 4 bảng

`GET /accountant/treatment-course-doctor/report-treatment-doctor` (/check-scale 2026-09-15). KHÔNG dùng `revenue_staff`: handler
riêng, luôn `confirm_commission = 1`, lọc chi nhánh / loại liệu trình qua JOIN `treatment_course`, lọc nhóm dịch vụ qua
`order_service_used` → `service`, từ khoá LIKE trên khách + mã đơn + mã liệu trình + mã dịch vụ. Bench dựng từ migration, 600.000
dòng / 17 chi nhánh / 30 bác sĩ / 24 tháng + 600.000 liệu trình / dịch vụ / phiếu thu, 400.000 đơn. ⚠ Dev 45.509 dòng nhưng 45.461
dòng `doctor_id` NULL (1 bác sĩ) ⇒ phân bố bác sĩ là tổng hợp.

**Cơ chế:** bảng ĐÃ có `idx_tcd_confirm_doctor (confirm_time, doctor_id, revenue, payment)` (thêm cho `crm/report/doctor-advise`)
nhưng thiếu `company_id` / `branch_id` / `confirm_commission` ⇒ tháng này optimizer còn dùng nó (SUM 140 · COUNT 136 · trang 35ms),
dải rộng thì quay về bẫy `fk-treatment_course_doctor-company_id` đọc toàn bảng (1 năm: 937 · 912 · 627ms); 1 chi nhánh đi
`fk-…-branch_id` 141–148ms mỗi câu. Câu SUM bọc bảng dẫn xuất + COUNT của `Paginate` đều kéo `LEFT JOIN order_service_used`.

**Vá:** `idx_tcd_confirm_id_cover (confirm_time, id, company_id, branch_id, confirm_commission, payment, commissionDoctor,
doctor_id)` (migration `20260915185231`, 35,6MB — `commissionDoctor` thêm ở lượt 2 cho màn tổng hợp) + COUNT/SUM 1 câu chỉ JOIN bảng bộ lọc cần + `PaginateWithTotal` + `FORCE INDEX` có cổng
(`tcdAggregateTable`: tắt khi có từ khoá / lọc bác sĩ / 1 chi nhánh / lọc qua JOIN) + từ khoá qua ngram: tra id khách (`MATCH ∧ LIKE`),
id đơn (`code LIKE`) → id dịch vụ (`code LIKE` HOẶC thuộc đơn khớp), id liệu trình (`code LIKE`), lọc `IN` trên 3 cột của bảng hoa
hồng; vượt trần 10.000 / MATCH lỗi ⇒ đường JOIN cũ. Export tự bổ sung đúng JOIN còn thiếu (`tcdJoins`).

| `EXPLAIN ANALYZE` câu tổng, không hint → FORCE (tháng / 1 năm / không lọc) | Số đo | Kết luận |
|---|---|---|
| toàn chi nhánh | 4,6 → 4,4 · 523 → **101** · 513 → **160ms** | ép |
| 3 chi nhánh | 2,9 → 2,9 · 486 → **73** · 475 → **121ms** | ép |
| 1 chi nhánh | 2,7 → 2,7 · 34,6 → 68,5 · 36,9 → 115ms | TỆ hơn ⇒ không ép |
| lọc bác sĩ | 0,8 → 2,7 · 125 → 66 · 158 → 138ms | lẫn lộn, chênh vài chục ms ⇒ không ép (cổng đơn giản) |
| ⛔ LOẠI: FORCE `idx_tcd_confirm_doctor` có sẵn, không thêm chỉ mục | toàn CN 1 năm 536 → 490 · 1 CN 1 năm 145 → 467ms | thiếu cột scope ⇒ vẫn tra từng dòng |

| Cả request (handler thật) | Trước (load 3–7) | Sau (load 2–3) |
|---|---|---|
| Toàn CN: 1 ngày / tháng / 1 năm / không lọc | 10–16 / 285–294 / 2.489–2.521 / 2.882–2.890ms | **4–6 / 12 / 100–102 / 145ms** |
| Toàn CN `page=2000` | 3.178–3.205 | 1.091–1.102 |
| Toàn CN lọc chi nhánh qua JOIN: 1 CN tháng / 1 CN không lọc / 2 CN 1 năm | 665–917 / 1.723–1.824 / 2.826–2.828 | 158–159 / 252–254 / 836–839 |
| Toàn CN lọc bác sĩ: tháng / không lọc | 22–109 / 605–1.314 | **4,2 / 122** |
| 1 CN: tháng / 1 năm / không lọc / nhóm DV không lọc | 432 / 522–526 / 548–553 / 1.493–1.699 | **10 / 68 / 185 / 394–409** |
| Từ khoá toàn CN không lọc: tên / mã KH / SĐT / mã đơn / mã DV / không khớp | 9.548 / 24.021 / 24.045 / 23.863 / 23.934 / 27.437 | **369 / 456 / 381 / 374 / 250 / 233** |
| Từ khoá toàn CN: tên tháng / `Thu Hà` 1 năm / tên + 1 CN không lọc | 1.318–1.368 / 6.564 / 6.452 | 369–371 / 490 / 330 |
| ⚠ Từ khoá vượt trần `Nguyễn` không lọc: toàn CN / 1 CN | 9.971 / 1.392 | 4.262 / 909 (đường JOIN — còn lại) |
| 1 CN + từ khoá tên + lọc bác sĩ, không lọc ngày | 416 | 241 — lượt 1 đường nhanh ra 620 (TỆ hơn), lượt 2 cổng lọc bác sĩ ⇒ JOIN |

**Chi nhánh GIỮ đọc `treatment_course.branch_id` (không đổi sang cột của bảng hoa hồng):** `treatment_course_doctor.branch_id` chụp
chi nhánh liệu trình LÚC tính hoa hồng (`crm/service/payment_commission.go`), còn chi nhánh liệu trình sửa được sau đó
(`crm/service/treatment_course.go` `applyUpdateFields`) ⇒ 2 cột lệch được; dev hiện 0 dòng lệch. Đổi sang cột của bảng sẽ bỏ JOIN
(lọc 1 CN tháng còn ~158ms) nhưng đổi nghĩa bộ lọc ⇒ để user quyết.

**Đối chiếu:** sha1 nguyên body HEAD vs bản vá **60/60** (19 ca basic + 12 ca từ khoá nặng × 2 chế độ, 18 ca body rỗng ở cả 2 bản);
thêm 6 ca từ khoá CÓ khớp theo từng vế id (mã liệu trình 1 năm / đủ mã không lọc, mã đơn, mã DV + 1 CN, tên + nhóm DV, tên + loại liệu trình `page=2`) × 2 chế độ: **12/12**, 7 ca có dữ liệu — toàn CN mã liệu trình 21.941 / 30.623 → 253 / 247ms, mã đơn 1 năm 17.420 → 386ms, tên + nhóm DV 3.843 → 598ms. Test `handler_test.go` 8 test; mutation 5/5 — test đỏ: SUM sai cột · cổng FORCE bỏ vế chi nhánh active · export thiếu
JOIN service · lọc chi nhánh đọc `treatment_course_doctor.branch_id`; đối chiếu body đỏ 2/2 ca mã liệu trình có dữ liệu (4 ca không đi qua vế đó vẫn khớp — đúng): bỏ vế id liệu trình ở đường nhanh.

**Lượt 2 (cùng ngày) — `/report-commission`, chỉ mục V2, cổng từ khoá khi lọc bác sĩ.** Bench dựng lại cùng seed.

`GET /accountant/treatment-course-doctor/report-commission` (tổng hợp theo bác sĩ): trước là 4 lượt quét — SUM hoa hồng + SUM
doanh thu bọc bảng dẫn xuất + COUNT bọc GROUP BY + câu trang (toàn CN 1 năm 522 · 530 · 545 · 631ms; 1 CN tháng ~140ms mỗi câu, đi
`fk-…-branch_id`). Nay GROUP BY bác sĩ MỘT lần, dòng tổng + phân trang trong Go (khuôn `revenue_staff.SummaryRows`), `FORCE INDEX`
có cổng (`commissionAggregateTable`: ép toàn CN / nhiều CN; tắt khi 1 CN hay lọc bác sĩ). Chi nhánh ở màn này vốn đọc cột của chính
bảng hoa hồng — giữ nguyên.

| GROUP BY bác sĩ (`EXPLAIN ANALYZE`), tháng / 1 năm / không lọc | V1 (không `commissionDoctor`, 29,6MB) | V2 (ship, 35,6MB) |
|---|---|---|
| 1 chi nhánh, auto | 139 / 150 / 149ms — không phủ ⇒ optimizer về `fk-…-branch_id` | **3,7 / 39 / 42ms** |
| toàn chi nhánh, FORCE | 22 / 579 / 316ms | **7,8 / 175** / 323ms |
| 3 chi nhánh, FORCE | 4,7 / 125 / 180ms | 4,0 / **99** / 169ms |
| 1 CN ép / lọc bác sĩ ép (1 năm) | — | 39 → 87 · 35 → 100ms (TỆ hơn ⇒ không ép) |

| `/report-commission` cả request | Trước (load 5–8) | Sau (V2, load 5) |
|---|---|---|
| Toàn CN: tháng / 1 năm / không lọc | 96 / 2.168–2.210 / 2.195–2.217ms | **7,6–7,8 / 167–173 / 304–332ms** |
| Toàn CN: lọc 2 CN không lọc / lọc 1 CN 1 năm / 2 bác sĩ 1 năm / `page=2` 1 năm | 1.983–2.082 / 521–534 / 148–165 / 2.170–2.180 | **191 / 46–53 / 40 / 184–185** |
| 1 CN: tháng / 1 năm / không lọc / 2 bác sĩ 1 năm | 568–589 / 526–536 / 518–531 / 511–515 | **3,9–4,1 / 40 / 44 / 38** |

**Cổng từ khoá khi lọc bác sĩ** (`tcdKeywordGate`): JOIN đi `idx-tcd-doctor_confirm` rất hẹp nên rẻ hơn đường nhanh (chi phí cố định
~230ms tra 4 danh sách id). Đường nhanh → JOIN: 1 CN tên không lọc 629 → 246 · 1 năm 631 → 234 · mã KH 468 → 245 · 2 bác sĩ 381 → 268ms
(không khớp 233 → 272, chấp nhận); toàn CN tháng này 437 → 13ms; toàn CN dải rộng LẪN LỘN (tên 570 → 432 nhưng mã KH 440 → 1.124 ·
không khớp 245 → 1.075 · 2 bác sĩ 376 → 1.128ms) ⇒ cổng: có lọc bác sĩ thì JOIN khi 1 chi nhánh HOẶC dải ≤ 31 ngày; toàn CN dải rộng
giữ đường nhanh để chặn ca xấu. Sau: 1 CN tên + bác sĩ không lọc 241 · 1 năm 205 · mã KH 238ms; toàn CN tháng này 14ms.

**Đối chiếu lượt 2:** `/report-commission` sha1 HEAD vs bản vá **16/16** (8 bộ lọc × 2 chế độ, 12 có dữ liệu), chạy lại trên V2 16/16;
đường nhanh vs đường JOIN ở ca từ khoá + bác sĩ **12/12** (8 có dữ liệu), bản cuối 12/12. Package handler 10 test (thêm
`commission_test.go`); mutation lượt 2 4/4 — test đỏ: cổng tổng hợp bỏ vế lọc bác sĩ · bỏ `confirm_commission = 1` · cổng từ khoá bỏ
nhánh bác sĩ; đối chiếu body đỏ 14/16 (2 ca body rỗng): dòng tổng tiền thu cộng nhầm hoa hồng.

**Ngoài phạm vi, chưa đo:** màn workspace gọi endpoint riêng `GET /workspace/treatment-doctor`.

#### Hoa hồng phụ tá — scope qua bảng ĐƯỢC JOIN, bảng không có chỉ mục ngày nào

`GET /accountant/treatment-course-assistant/report-treatment-assistant` + `/report-commission` (/check-scale 2026-09-15). Bench
dựng từ migration, 600.000 dòng phụ tá / 17 chi nhánh / 40 phụ tá / 24 tháng + 600.000 liệu trình / dịch vụ, 400.000 đơn; ~5% liệu
trình khác chi nhánh dịch vụ (điều trị chéo) để đối chiếu bắt được nếu bản vá đổi nghĩa scope. ⚠ Dev 0 dòng ⇒ phân bố tổng hợp.
⚠ **Schema drift**: dev có thêm `role` / `source_id` / `service_id` và `branch_id` / `confirm_time` nullable; bản dựng từ
migration không có 3 cột đó, 2 cột kia NOT NULL.

**Cơ chế:** (1) bảng KHÔNG có chỉ mục nào dẫn đầu `confirm_time` (chỉ `(assistant_id, confirm_time)` + chỉ mục 1 cột) ⇒ list lọc
1 NGÀY cũng `Table scan` 597.584 dòng (COUNT của `Paginate` 163 + trang 147ms); (2) list scope bằng
`ScopedDBNullBranch(c, db, "order_service_used")` — scope + lọc company/chi nhánh đọc bảng ĐƯỢC JOIN ⇒ mọi câu JOIN
`order_service_used` và tra 1 dòng dịch vụ cho mỗi dòng phụ tá. **GIỮ nghĩa đó**: bản ghi phụ tá chép chi nhánh từ liệu trình
(`crm/service/treatment_course.go`), liệu trình có thể khác chi nhánh dịch vụ ⇒ đổi sang cột của bảng phụ tá là đổi nghĩa scope;
(3) tổng hợp scope bảng chính nhưng vẫn `LEFT JOIN order_service_used` không đọc cột nào, rơi bẫy `fk-…-company_id`.

**Vá:** chỉ mục `idx_tca_confirm_id_cover (confirm_time, id, company_id, branch_id, service_used_id, assistant_id, point,
commission_assistant)` (migration `20260915221516`, 31,6MB); list: đếm thẳng + `PaginateWithTotal`, query nền dựng 1 chỗ
(`listBase`) cho câu đếm / trang / export, câu ĐẾM bỏ chỉ mục khi dải `confirm_time` > 92 ngày (`tcaCountTable` — `IGNORE INDEX`; câu trang giữ chỉ mục để dừng sớm). Điểm lật đo `EXPLAIN ANALYZE` (auto → IGNORE, toàn CN): 1 tháng 88 → 157 · 3 tháng 232 → 247 (hoà) · 6 tháng 459 → 356 · 1 năm 878 → 531ms; không lọc ngày optimizer vốn không dùng chỉ mục ngày (833 → 835); tổng hợp: GROUP BY phụ tá 1 lần, tổng + phân trang trong Go, chỉ JOIN
`order_service_used` khi lọc company/chi nhánh, `FORCE INDEX` có cổng (`commissionAggregateTable`); từ khoá qua package dùng chung
**`accountant/treatmentkw`** (tách từ màn bác sĩ — cùng 4 vế, khác cột + bộ lọc nhân viên).

**Sửa kèm lỗi có sẵn:** `ExportTreatmentAssistant` tự JOIN `treatment_course` + `customer` (không alias) mà handler cũ đã JOIN sẵn khi
có từ khoá / lọc loại liệu trình ⇒ export kèm 1 trong 2 bộ lọc đó **luôn Error 1066** (xác nhận trên bench). Nhánh export nay không
tự JOIN 2 bảng này.

| `EXPLAIN ANALYZE`, tháng / 1 năm / không lọc | không chỉ mục | chỉ mục, auto | chỉ mục, FORCE |
|---|---|---|---|
| list trang, toàn CN | 147 / 260 / 398ms | 6,0 / 18 / 0,4ms | 15 / 17 / 0,4ms |
| list COUNT (JOIN scope), toàn CN | 139 / 540 / 833ms | 36 / 789 / 811ms | 31 / 736 / 1.442ms (TỆ) |
| tổng hợp GROUP BY bỏ JOIN, toàn CN | 523 / 592 / 622ms | 5,9 / 581 / 615ms | 5,7 / **149** / **258ms** |
| tổng hợp GROUP BY bỏ JOIN, 1 CN | 33 / 46 / 48ms | 2,7 / 46 / 47ms | 2,8 / 70 / 126ms (TỆ) |
| ⛔ LOẠI: chỉ mục hẹp `order_service_used (id, company_id, branch_id)` cho JOIN scope | COUNT 1 năm 628 vs 634 · không lọc 779 vs 780ms | — | optimizer vẫn tra PRIMARY |

| `/report-commission` cả request | Trước (load 2–3) | Sau |
|---|---|---|
| Toàn CN: tháng / 1 năm / không lọc | 1.104–1.121 / 1.858–1.864 / 2.455–2.460ms | **6,3 / 134 / 224–231ms** |
| Toàn CN: 2 phụ tá 1 năm / lọc 1 CN 1 năm / lọc 2 CN không lọc | 224 / 140 / 281–287 | 24 / 73 / 151 |
| 1 CN: tháng / 1 năm / không lọc | 70 / 234–235 / 272–280 | **3,2 / 38–42 / 38** |

| `/report-treatment-assistant` cả request | Trước | Sau |
|---|---|---|
| 1 CN: 1 ngày / tháng / 1 năm / không lọc | 220–222 / 283–288 / 742–757 / 1.108–1.121ms | **5,6–7,1 / 70–71 / 493–495 / 740–746ms** |
| 1 CN: `page=2000` / nhóm DV tháng / loại liệu trình tháng | 3.904–3.970 / 302 / 342–343 | 2.518–2.542 / 191–194 / 189 |
| Toàn CN: 1 ngày / tháng / không lọc | 202–204 / 235–272 / 1.136–1.150 | **3,3–5,0 / 78 / 797–977** |
| Toàn CN xen kẽ H/N 3 lượt (load 3–6): 1 năm / `page=2000` / lọc 2 CN 1 năm | 954–1.909 / 2.173–3.147 / 1.433–1.836 | **508–561 / 1.220–1.532 / 935–1.232** |
| Toàn CN xen kẽ: nhóm DV không lọc / loại liệu trình tháng / phụ tá không lọc | 586–711 / 187–397 / 141–183 | 481–670 / 183–228 / 132–170 (hoà) |
| ⚠ Lượt ĐẦU (chưa có cổng câu đếm): 1 CN 1 năm / toàn CN lọc 2 CN 1 năm | 742–757 / 928–930 | **1.306–1.371 / 1.481–1.502 — TỆ hơn** ⇒ thêm cổng câu đếm |
| Từ khoá toàn CN không lọc: tên / mã KH / SĐT / mã đơn / mã DV / không khớp | 5.065 / 30.729 / 23.540 / 23.567 / 27.072 / 23.528 | **371 / 480 / 379 / 398 / 248 / 233** |
| Từ khoá toàn CN: mã liệu trình 1 năm / mã đơn 1 năm / tên tháng | 12.079 / 12.263 / 835 | 266 / 409 / 377 |
| ⚠ Từ khoá vượt trần `Nguyễn`, toàn CN không lọc | 4.149 | 4.530 (đường JOIN — còn lại) |

**Đối chiếu:** sha1 nguyên body HEAD vs bản vá **84/84** (2 màn × 4 nhóm ca × 2 chế độ, 24 ca body rỗng ở cả 2 bản), chạy lại trên
bản cuối 84/84; bản cuối (có cổng câu đếm) 84/84, ca xen kẽ 6/6. Test `handler_test.go` 7 test; mutation 6/6 — test đỏ: export tự JOIN liệu trình (1066) ·
cổng tổng hợp bỏ vế chi nhánh active · tổng hợp luôn JOIN dịch vụ · scope `listBase` đọc bảng phụ tá (test ban đầu XANH GIẢ vì chỉ
soi câu đếm — nay soi cả câu trang); đối chiếu body đỏ: tổng điểm cộng nhầm hoa hồng (8/8) · vế liệu trình lọc nhầm cột khách (đúng
ca mã liệu trình có dữ liệu).

**Ngoài phạm vi, chưa đo:** màn workspace gọi endpoint riêng `GET /workspace/treatment-assistant`.

#### Hoa hồng tư vấn viên — lần thứ 5 cùng bệnh

`GET /accountant/commission-telesale` + `/report-commission` (/check-scale 2026-09-15). Bảng `order_commission_telesale` cùng bộ chỉ
mục 1 cột ⇒ toàn chi nhánh mọi câu tra `idx_oct_company_id` 600.000/600.000 dòng (list SUM 711 · COUNT 559 · trang 537ms; tổng hợp
4 câu 485–502ms mỗi câu); 1 chi nhánh đi `idx_oct_branch_id` 140–165ms mỗi câu. Bench dựng từ migration, 600.000 dòng / 17 chi nhánh
/ 88 tư vấn viên / 24 tháng. ⚠ Lượt seed đầu `revenue_id = (n*11)%88` chỉ ra **8** tư vấn viên ⇒ ca lọc tư vấn viên trả body rỗng
ở cả 2 bản build, đối chiếu "khớp" vô nghĩa — soát số giá trị phân biệt của cột nhóm trước khi tin sha1.

**Vá:** `idx_oct_confirm_id_cover (confirm_time, id, company_id, branch_id, commissionTelesale, amount, revenue_id)` (migration
`20260915173316`, 32,6MB) + handler dùng `revenue_staff.CommissionTelesale`. `SummaryPage` thêm `Totals` (tổng theo TỪNG alias) vì màn
tổng hợp trả cả `totalAmount`. Cổng hint đo lại ra đúng hình dạng cũ (`EXPLAIN ANALYZE`, không hint → FORCE): câu tổng toàn CN 1 năm
501 → 89ms · không lọc 498 → 149ms; GROUP BY 578 → 157 · 624 → 277ms; 3 chi nhánh 1 năm 484 → 62ms — tệ hơn khi ép: 1 CN GROUP BY 1 năm
37,6 → 70,4ms · lọc tư vấn viên 1 năm 6,0 → 55,9ms.

| Cả request (handler thật) | Trước (không chỉ mục, load 5–6) | Sau (load 5–6) |
|---|---|---|
| List toàn CN: 1 ngày / tháng / 1 năm / không lọc | 1.463–1.478 / 1.508–1.510 / 2.165–2.191 / 2.721–2.724ms | **2,5–3,2 / 9,3–9,5 / 86 / 118–119ms** |
| List toàn CN `page=2000` | 2.989–2.997 | 1.000–1.179 |
| List 1 CN: 1 ngày / tháng / 1 năm / không lọc | 376–380 / 375–376 / 472 / 512–524 | **1,8–2,1 / 8,0–8,9 / 45–46 / 160–163** |
| Tổng hợp toàn CN: tháng / 1 năm / không lọc | 1.889–1.896 / 2.041–2.057 / 2.011–2.060 | **7,1–7,5 / 145–148 / 248–250** |
| Tổng hợp 1 CN: tháng / 1 năm | 494–495 / 502–504 | **3,5–3,8 / 38–39** |
| List toàn CN từ khoá tên / không khớp / `0912`, không lọc ngày | 13.843 / 13.142 / 6.055 | **219 / 90 / 269** |
| ⚠ Từ khoá vượt trần (`Nguyễn`), toàn CN không lọc | 6.496 | 2.888 (đường JOIN — còn lại) |
| ⚠ 1 CN active + từ khoá + bộ lọc khiến kết quả rỗng (lọc CN khác / tư vấn viên) | 1,3 / 80 | 194 / 205 — đường nhanh trả ~200ms tra id ngram TRƯỚC khi bộ lọc khác loại hết |

**Đối chiếu:** sha1 nguyên body HEAD vs bản vá **66/66** (list 14 + tổng hợp 7 bộ lọc × 2 chế độ + 12 ca từ khoá nặng × 2). 24/66 ca từ
khoá ra body rỗng ở cả 2 bản (tên `Nguyễn Văn An` không có trong bench) ⇒ chạy thêm 10 ca từ khoá CÓ khớp (tên có/không dấu, mã KH,
SĐT đủ/tiền tố, `Thu Hà` 5.264 khách, `Hải` 15.790 khách = vượt trần, kèm lọc CN / tư vấn viên / `page=3`) × 2 chế độ: **sha1 **20/20** (17 body có dữ liệu; tên có dấu và không dấu ra CÙNG body ở cả 2 bản) — HEAD 0,5–17,5s → bản vá 170–420ms, `Hải` vượt trần 5,3 → 3,0s toàn CN**.
Test `revenue_staff_test.go` 11 test; mutation 5/5 — test đỏ: `Totals` chỉ khởi tạo cột chính · sai tên chỉ mục · sai cột nhân viên;
đối chiếu body đỏ (test giả-driver 0 dòng không bắt được): handler trả `totalAmount` = `Sum` · cộng dồn cột chính vào mọi alias.

## Bảng được JOIN không bị scope `company_id` ⇒ optimizer chọn sai bảng dẫn

> ⚠ Lịch sử (D15): schema mới không còn `company_id` — mục này chỉ giải thích số đo cũ, không áp dụng cho CRM standalone.

Anh em của bẫy trên, nhưng ở phía **bảng được JOIN vào** chứ không phải bảng chính — và lần này
nguyên nhân không nằm ở index thiếu cột, mà ở chỗ **query không hề nhắc tới `company_id` của bảng
kia**. Không có vế đó thì `idx_..._company_date` (index dẫn đầu `company_id`) KHÔNG dùng được,
bảng join mất đường range scan, optimizer buộc phải lấy bảng chính làm bảng dẫn rồi tra PK của
bảng join cho TỪNG dòng.

Đo trên `GET /crm/treatment-services/service-used-with-appointment`, lọc theo NGÀY HẸN 30 ngày,
DB clone 524.288 `order_service_used` / 300.000 `appointment` (2026-09-07):

```go
// TRƯỚC — appointment không bị scope gì, chỉ dựa gián tiếp vào osu.company_id
q.Joins("LEFT JOIN appointment ON appointment.id = order_service_used.appointment_id")
// -> Index lookup on order_service_used using code (company_id = 1)   rows=258079   ← bảng dẫn
//     -> Single-row index lookup on appointment using PRIMARY          ← 129k lần tra PK

// SAU — vế company gắn vào chính điều kiện JOIN (không phải WHERE)
q.Joins(apptJoin+" AND (appointment.company_id = ? OR appointment.company_id IS NULL)", companyID)
// -> Index range scan on appointment using idx_appointment_company_date   rows=8146  ← bảng dẫn
//     optimizer TỰ đảo thứ tự join, không cần hint
```

| | 524k dòng |
|---|---|
| `SUM` tổng, lọc ngày hẹn 30 ngày | 0.62–1.33s → **0.05–0.06s** |
| `COUNT(*)`, lọc ngày hẹn 30 ngày | 0.95–1.40s → **0.04–0.07s** |
| `SELECT` trang 1 | — → **0.06–0.07s** |

Cùng ra 2.511 dòng / 3.254.042.565đ.

Vì sao gắn vào **điều kiện JOIN** chứ không phải `WHERE`: với `LEFT JOIN`, đặt ở `WHERE` sẽ biến
nó thành INNER JOIN (loại mất dòng không khớp) — đổi kết quả. Đặt trong `ON` thì giữ nguyên ngữ
nghĩa LEFT mà vẫn cho optimizer cái range nó cần. Giữ nhánh `IS NULL` theo luật NULL-tenant.
Lấy phạm vi từ `tenant.BoundsFromContext(c)` (session), KHÔNG từ JWT. Phạm vi rỗng ⇒ để nguyên
join cũ, KHÔNG tự chế fallback.

⚠ Đây đồng thời là lỗ **tenant scope**, không chỉ chuyện tốc độ: bảng được join trước đó không bị
lọc company lần nào. Ở DB-per-customer thì vô hại, nhưng mẫu code này lan ra chỗ khác thì không.

⚠ Hint `/*+ JOIN_ORDER(...) */` **chưa được đo tách bạch** ở đây — phép thử chạy kèm luôn vế
`company_id` nên không tách được công của ai. Vế `company_id` một mình đã đủ, nên đừng đi đường
hint; ai muốn dùng hint phải đo lại có đối chứng.

Mẫu này lặp lại được: repo còn nhiều `LEFT JOIN customer / appointment / service` không có vế
company nào. Đụng handler nào thấy nút thắt "tra PK của bảng join cho từng dòng" thì kiểm vế này
TRƯỚC khi nghĩ tới thêm index mới.

Áp dụng: `crm/treatment_service/handler/treatment_service.go`
(`buildServiceUsedWithAppointmentQuery`).

## Xác nhận lần 2 khuôn "N subquery độc lập" — `report/revenue`, và giới hạn của nó

Cùng lớp với `report/appointment-statistics` (mục trên): 6 bảng dẫn xuất `F,G,H,J,K,L` LEFT JOIN
vào `user`, trong đó **F/G/H đọc CÙNG một dải ngày trên `appointment`**, khác nhau đúng điều kiện
`status` / `is_accepted`. Gộp bằng `MAX(<điều kiện>)` ở tầng trong (mỗi dòng đã là 1 khách của 1
`revenue_id` sau `GROUP BY revenue_id, customer_id`) rồi `COUNT(CASE WHEN …)` ở tầng ngoài.

Bench 872.284 `appointment` / 615.992 `customer` / 596.680 `order_treatment_plan` /
449.360 `order_service_used` / 372.504 `payment`, **toàn bộ chi nhánh** (2026-09-09):

| Kịch bản | Trước | Sau |
|---|---|---|
| Riêng cụm 3 chỉ số F+G+H, không lọc ngày | 140.920 dòng · 0,93–9,96s | **57.384 dòng · 0,34–0,58s** |
| TOÀN câu, lọc **tháng mặc định** (đường FE đi hằng ngày) | 199.450 dòng · 0,41–0,44s (lạnh 6,64s) | **38.775 dòng · 0,15–0,22s** |
| TOÀN câu, KHÔNG lọc ngày | 1.218.228 dòng + 743.786 dòng bảng tạm · 2,5–4,0s (lạnh 32,2s) | 1.134.692 + 613.824 · 2,1–2,6s |

⚠ **Đọc kỹ dòng cuối: gộp F/G/H gần như KHÔNG cứu được ca không-lọc-ngày.** Ở đó chi phí nằm ở
hai nhánh khác, mỗi nhánh quét cạn bảng của mình vì `BETWEEN 0 AND 9999999999` không lọc gì:
`J` (`order_treatment_plan` × `JSON_TABLE` × `order_service_used`) **701.068 dòng / 1,20–1,39s** và
`K` (`payment` × `osu` × `JSON_TABLE`) **372.504 dòng — TRỌN bảng payment / 0,97–1,01s**.
Bài học: gộp subquery ăn đậm ở ca CÓ lọc ngày (5,1× số dòng), còn ca không lọc thì thứ duy nhất
cứu được là **trần dải ngày**.

⚠ `LEFT JOIN customer … WHERE App.revenue_id IS NOT NULL` của bản cũ CHÍNH LÀ inner join — vế
`WHERE` trên bảng phải huỷ tính LEFT. Bản gộp viết thẳng `INNER JOIN customer` để giữ nguyên ngữ
nghĩa "chỉ đếm lịch hẹn mà khách còn tồn tại"; bỏ join này đi là đếm cả `customer_id` mồ côi.

⚠ Chỗ dễ vỡ nhất khi gộp là **khách có NHIỀU lịch**: `MAX()` phải cho khách đó vào mỗi chỉ số
đúng MỘT lần. Đã neo bằng test có khách 4 lịch cùng thoả — `MAX`→`MIN` và dùng nhầm điều kiện
đều làm test đỏ.

Đối chiếu: 6 bộ lọc (không lọc · tháng · tháng-1-chi-nhánh · 1 ngày · năm 2025 · dải rỗng),
chuỗi ghép toàn bộ dòng × 4 cột KHỚP tuyệt đối với bản 3 nhánh.

Kèm **trần dải ngày** (`define.ReportMaxRangeSec`, 366 ngày) — `getDateRange` dùng chung nhiều
handler tự đặt `from=0 / to=9999999999` khi thiếu ngày, và nút × trên ô chọn ngày của FE đi thẳng
vào đó. Guard chỉ đặt trong `Revenue`, KHÔNG sửa `getDateRange` (handler khác đang dựa vào nó).

## Xác nhận lần 3 khuôn "N subquery độc lập" — `report/telesales-online`, bản 12 nhánh

Cùng lớp với `appointment-statistics` và `revenue`, nhưng là bản nặng nhất tìm được: **12 bảng
dẫn xuất A–L** LEFT JOIN vào `user`, trong đó **5 lượt quét `appointment`** (D/E cùng `created_at`,
F/G/H cùng `appointment_date`) và một nhánh dựng bảng dẫn xuất trên **TOÀN BỘ `note` không có
WHERE**. Báo cáo này CỐ Ý không scope company/branch (xuyên chi nhánh, giữ hành vi Yii2 cũ) —
điều đó khiến mọi chỉ mục dẫn đầu `company_id` trở nên vô dụng ở đây.

Bench 3.370.004 `note` / 872.284 `appointment` / 615.992 `customer` / 596.680 `otp` /
449.360 `osu` / 372.504 `payment`, **lọc tháng mặc định** (2026-09-09):

| | Trước | Sau |
|---|---|---|
| Nhánh C "chăm lại" (`note`) | **3.374.702 dòng** · 6,4–6,8s | **2.528 dòng · 0,015s** |
| Cụm F+G+H (`appointment_date`) | 2.104.718 dòng · 1,58–3,92s | **114.768 dòng · 0,36–0,55s** |
| **TOÀN câu** | **~7,33 triệu dòng** (3.904.324 next + 3.424.396 tạm) · 6,25–6,97s | **~605.000 dòng** (569.591 + 35.677) · **1,41–1,62s** |

Ba cơ chế, mỗi cái một bài học:

**1. Điều kiện ngày phải nằm TRONG bảng dẫn xuất.** Bản cũ:
`LEFT JOIN (SELECT DISTINCT customer_id, ROUND(UNIX_TIMESTAMP(FROM_UNIXTIME(created_at,…))) …
FROM note) noTe ON … WHERE noTe.dayCreated BETWEEN ? AND ?` — khử trùng lặp trên 3,37 triệu dòng
và gọi chuỗi 4 hàm lồng nhau cho từng dòng, rồi mới lọc. Thêm vào trong một vế `created_at`
**NỚI RỘNG** (±1 ngày quanh dải, vế `dayCreated BETWEEN` ở ngoài giữ nguyên nên KHÔNG đổi kết quả):
`EXPLAIN ANALYZE` cho thấy phép so sánh số nguyên loại ~99,9% dòng **trước** khi phải gọi hàm ⇒
2.548ms → 729ms **ngay cả khi vẫn quét trọn bảng**.

**2. Chỉ mục phải có cột dẫn đầu ĐÚNG với truy vấn, không phải với bảng.** `note` vừa được thêm
`idx_note_company_created (company_id, created_at)` cùng ngày cho `note/get-all-note` — vô dụng ở
đây vì câu này không có vế `company_id`. Thêm `idx_note_created_at (created_at)`
(migration `20260909132102`): (a) không chỉ mục 3.374.702 dòng/0,600–0,611s · (b) auto **2.528
dòng/0,015–0,043s** · (c) FORCE 2.528/0,015s ⇒ **gate ĐẠT**, 1.335× ít dòng. Hai đường đi khác
nhau trên cùng một bảng cần hai chỉ mục khác nhau — đừng cố nới một cái cho cả hai.

**3. Gộp N nhánh cùng dải ngày.** D+E → 1 (`COUNT(CASE WHEN customer.created_day < from …)`);
F+G+H → 1 (`MAX(<điều kiện>)` tầng trong + `COUNT(CASE WHEN …)` tầng ngoài).

⚠ **Bẫy đối chiếu ở đây khác `revenue`**: bản cũ dùng `LEFT JOIN customer` **KHÔNG kèm**
`WHERE App.revenue_id IS NOT NULL`, nên sinh thêm một nhóm `revenue_id = NULL`. Đo ở mức NHÁNH thì
cũ ra 4 dòng, gộp ra 3 — nhìn như sai. Nhóm NULL đó không bao giờ khớp `u.id = F.revenue_id` nên
không vào kết quả: **đối chiếu phải làm ở mức TOÀN CÂU**, và ở đó 9 dòng khớp byte-for-byte.

⚠ **Mutation `MIN` thay `MAX` KHÔNG bị bắt ở lần chạy đầu** — vì bộ dữ liệu seed cho mỗi khách
đúng 1 lịch trong dải, lúc đó `MIN` và `MAX` cho cùng kết quả. Test gộp kiểu này BẮT BUỘC có một
khách **nhiều lịch** mà chỉ một lịch thoả điều kiện; thêm dòng đó xong mutation mới đỏ. Cùng bài
học với `revenue` nhưng ở đó may mà seed đã có sẵn ca này.

Kèm **trần dải ngày** (`define.ReportMaxRangeSec`, 366 ngày) — `getDateRange` lại tự đặt
`0 / 9999999999` khi thiếu ngày. Guard đặt riêng trong `TelesalesOnline`, KHÔNG sửa `getDateRange`.

## Xác nhận lần 4 khuôn "N subquery độc lập" — `report/doctor-advise`, và khi KHÔNG nên áp trần ngày

Bản nhẹ nhất của họ: 4 bảng dẫn xuất, gộp còn 2. A/B đọc cùng dải `appointment_date` khác đúng
`is_accepted = 1`; **C/D đọc CÙNG bảng, CÙNG WHERE, CÙNG GROUP BY — khác đúng cột được SUM**
(`SUM(revenue)` vs `SUM(payment)`). Cái sau là dạng hiển nhiên nhất của khuôn này, đáng grep tìm.

⚠ **Dữ liệu thật cho báo cáo này gần như RỖNG** — `appointment.doctor_id` **0 dòng có giá trị**,
`treatment_course_doctor.doctor_id` chỉ 384/364.072 dòng và đúng 1 bác sĩ. Đo nguyên trạng sẽ ra
"nhanh" một cách vô nghĩa: đây là biến thể của Luật số 0 mà bảng KHÔNG nhỏ — **cột khoá rỗng**.
Bench dưới đây gán tổng hợp 30 bác sĩ (~70% lịch hẹn), nên **không có phân bố thật để neo**.

Bench 872.284 `appointment` / 364.072 `treatment_course_doctor`, lọc tháng mặc định (2026-09-09):

| | Dòng InnoDB | Thời gian |
|---|---|---|
| Nguyên trạng (4 nhánh, chưa có chỉ mục) | **811.901** | 1,03–1,14s |
| Chỉ thêm chỉ mục, chưa gộp | 101.485 | 0,32–0,33s |
| **Gộp + chỉ mục** | **66.606** | **0,20–0,22s** |

Thủ phạm chính KHÔNG phải `appointment` (nhánh đó đọc 57.384 dòng qua
`fk-appointment_appointment_type_id`) mà là **`treatment_course_doctor` bị quét HAI LẦN** —
mỗi lần 364.072 dòng, chỉ để lấy hai cột SUM khác nhau.

### Cột dẫn đầu quyết định tất cả — hai chỉ mục, một cái vô dụng

Nhánh C/D lọc `confirm_time BETWEEN ? AND ?` và `doctor_id IS NOT NULL` (không chọn lọc).

| Chỉ mục | Dòng đọc | Thời gian |
|---|---|---|
| không có (chỉ có sẵn `idx-tcd-doctor_confirm (doctor_id, confirm_commission, confirm_time)`) | 364.072 | 0,474–0,479s |
| `(doctor_id, confirm_time, revenue, payment)` — **ĐÃ LOẠI** | **364.072** (không giảm) | 0,079–0,084s |
| **`(confirm_time, doctor_id, revenue, payment)`** | **8.864** | **0,004–0,006s** |

Bản bị loại vẫn nhanh hơn không-chỉ-mục 6× nhờ covering, nhưng **không giảm được dòng nào** vì
cột dẫn đầu `doctor_id` chỉ có `IS NOT NULL`. Phải để **cột NGÀY đứng đầu** thì dải mới thành
range — 100× so với nguyên trạng. Gate FORCE INDEX ĐẠT ((b) 0,004–0,006s ≈ (c) 0,004–0,005s).
Migration `20260909135020`.

### Và đây là endpoint đầu tiên trong nhóm mà trần dải ngày KHÔNG đáng áp

Bốn report anh em đều được áp trần 366 ngày. Ở đây thì **không**, vì có số đo: sau bản vá, ca
"xoá bộ lọc ngày" tốn **421.814 dòng / 0,39s** — so với `telesales-online` ~7,3 triệu dòng hay
`report-service-used` 14–18s. Chặn ở đây là hạn chế người dùng mà gần như không mua được gì.
Bài học: đừng áp mẫu vì nó là mẫu — áp vì con số ở CHÍNH endpoint đó.

Đối chiếu 6 bộ lọc (tháng · không lọc · 1 ngày · năm 2025 · dải rỗng · 3 tháng): khớp tuyệt đối
từng dòng × 4 cột.

### `direct-sale-advise` — cùng khuôn A/B, và là ca "gộp vẫn đúng nhưng KHÔNG cứu gì"

`DirectSaleAdvise` ngay dưới trong cùng file có đúng khuôn A/B (2 lượt quét cùng dải, khác
`is_accepted`). Đã đo và gộp — nhưng **endpoint này KHÔNG có nút thắt**, ghi lại để khỏi ai đi
tìm lại. Bench 872.284 lịch hẹn / 32 nhân viên sale (2026-09-09):

| | Dòng InnoDB | Thời gian |
|---|---|---|
| lọc tháng mặc định, trước | 83.719–84.377 | 0,285–0,352s |
| lọc tháng mặc định, sau | **57.723** | **0,176–0,200s** |
| KHÔNG lọc ngày, trước | 83.733 | 0,300–0,317s |
| KHÔNG lọc ngày, sau | 57.733 | 0,179–0,180s |

Hai ca gần bằng nhau vì plan đi từ `appointment_type` (3 loại tính doanh thu) rồi tra
`fk-appointment_appointment_type_id` — **dải ngày không giảm được dòng đọc nào**. Đây cũng là lý do
KHÔNG áp trần dải ngày (ca xấu nhất mới 0,3s). Cùng phân bố rỗng như `doctor-advise`:
`appointment.direct_sales_id` thật chỉ có **67/218.071 dòng**, bench gán tổng hợp 30 nhân viên ⇒
không có phân bố thật để neo. Đối chiếu 6 bộ lọc khớp tuyệt đối.

⇒ Bài học chung của hai cái cuối nhóm: **khuôn "N subquery độc lập" đáng gộp ở mọi nơi vì nó rẻ và
sạch, nhưng phần thắng chỉ LỚN khi nhánh trùng lặp thật sự nặng** (`treatment_course_doctor` 364k
dòng ×2 ở `doctor-advise`, `note` 3,37 triệu dòng ở `telesales-online`). Ở đây mỗi nhánh chỉ 57k
dòng nên gộp chỉ ăn 1,45×.

## `effective-advise` — nút thắt CHỈ lộ ở cache lạnh, và khi chỉ mục đúng vẫn nên LOẠI

Endpoint duy nhất trong nhóm `/crm/report/*` **không có gì để gộp** (1 bảng dẫn xuất) và **CÓ
tenant scope** (`buildScopeFilter`) — nhờ vế `company_id` mà `idx_appointment_company_date` dùng
được. Đo @872.284 lịch hẹn, toàn bộ chi nhánh (2026-09-09):

| Kịch bản | Dòng InnoDB | Warm | Cache LẠNH |
|---|---|---|---|
| lọc 1 ngày | 755 | 0,004–0,009s | — |
| lọc tháng mặc định | 38.096 | 0,014–0,017s | — |
| **KHÔNG lọc ngày** | 57.384 | 0,180–0,187s | **6,8–14,4s** |

⚠ **Bài học chính: warm và lạnh lệch 40×, và chỉ số dòng đọc KHÔNG cho thấy điều đó.** Bỏ dải ngày
thì optimizer rời `idx_appointment_company_date` và quay về
`Index lookup on fk-appointment_appointment_type_id` — chỉ mục đó chỉ có `appointment_type_id`
nên KHÔNG covering ⇒ 57.384 lượt **fetch row NGẪU NHIÊN**. Warm thì buffer pool giấu hết; lạnh thì
lộ. **Báo cáo là loại màn mở thưa, tức cache lạnh mới là trạng thái BÌNH THƯỜNG của nó** — đừng
kết luận "0,18s, không sao" từ số warm.

### Chỉ mục covering thắng đường ĐỌC nhưng vẫn LOẠI — vì đường GHI

`(appointment_type_id, company_id, status, is_accepted, appointment_date, branch_id)`:

| | Dòng đọc | Thời gian |
|---|---|---|
| (a) không có chỉ mục, KHÔNG lọc ngày | 57.384 | 0,180–0,187s |
| (b) có chỉ mục, auto, KHÔNG lọc ngày | 57.384 (hết fetch row) | **0,019–0,020s** |
| (c) FORCE INDEX | 57.384 | 0,017–0,020s |
| (b) có chỉ mục, lọc tháng | **18.460** (từ 38.096) | 0,006–0,007s |

Gate FORCE INDEX ĐẠT. Nhưng đo tiếp **đường GHI**: `UPDATE` 2.000 dòng `appointment`
**0,068–0,130s → 0,159–0,173s**, tức **+30–60% chi phí UPDATE mỗi dòng**. `appointment` là bảng
ghi liên tục (đặt lịch, check-in, check-out) còn báo cáo này mở thưa ⇒ **đánh thuế đường ghi nóng
để cứu một màn hiếm là lỗ**. Trần dải ngày đạt cùng mục tiêu với chi phí ghi bằng 0 ⇒ chọn trần.

⇒ Quy tắc rút ra: **gate FORCE INDEX chỉ chứng minh chỉ mục có tác dụng, KHÔNG chứng minh nên
ship nó.** Với bảng ghi nóng, phải đo thêm `UPDATE`/`INSERT` trước khi quyết. (Đối chứng chiều
ngược lại: `idx_customer_branch_revenue` cùng phiên đo ra **không** phạt ghi được ⇒ ship.)

### Bảng tổng kết trần dải ngày cho cả nhóm `/crm/report/*` (2026-09-09)

Áp hay không áp đều phải có số đo của CHÍNH endpoint đó, không áp vì nó là mẫu:

| Endpoint | Ca không-lọc-ngày | Quyết định |
|---|---|---|
| `appointment-statistics` | 9,6 triệu dòng / 22,9s | áp trần |
| `report-service-used` | ~14–18s (sau khi đã vá 3 chỗ) | áp trần |
| `telesales-online` | ~7,3 triệu dòng / 6,3–7,0s | áp trần |
| `revenue` | 1,2 triệu dòng / 2,5–4,0s (lạnh 32,2s) | áp trần |
| **`effective-advise`** | 57.384 dòng / 0,18s warm — **nhưng 6,8–14,4s lạnh** | **áp trần** |
| `doctor-advise` | 421.814 dòng / 0,39s | **KHÔNG áp** |
| `direct-sale-advise` | 57.733 dòng / 0,18s (không có nhánh fetch-row) | **KHÔNG áp** |

## Xác nhận lần 5 khuôn "N subquery độc lập" — `report/branch-statistics`, 11 nhánh gộp còn 6

Bản 11 bảng dẫn xuất (A, E, B, C, D, P, Q, R, S, T, U). Gộp được **7 lượt quét thành 2**:

- **B/C/D** đọc CÙNG dải `appointment_date`, khác đúng `status` / `is_accepted` → 1
- **R/S/T/U** đọc CÙNG bảng `payment`, CÙNG dải `payment_time`, khác đúng
  `payment_for` / `deposit_type` / cột được SUM → 1

Nhánh `E` (`created_at`), `A` (`customer`), `P` (`osu`), `Q` (`payment × osu`, hai vế ngày khác
nhau) giữ nguyên vì đọc dải/bảng khác.

Bench 872.284 `appointment` / 615.992 `customer` / 449.360 `osu` / 372.504 `payment`,
toàn bộ chi nhánh (2026-09-09):

| Kịch bản | Trước | Sau |
|---|---|---|
| lọc tháng mặc định (đường FE đi) | **2.670.044 dòng** · 1,26–1,31s | **701.639 dòng · 0,36–0,45s** |
| KHÔNG lọc ngày | 4.659.669 dòng · 4,72–6,40s | 2.598.623 dòng · 3,80–5,21s |

⇒ 3,8× dòng / 3,2× thời gian ở ca thường. Ca không-lọc vẫn 3,8–5,2s **sau khi gộp** ⇒ áp thêm
trần dải ngày. Đối chiếu 5 bộ lọc: khớp tuyệt đối.

### ⚠ Khi mutation `MIN` thay `MAX` KHÔNG đỏ mà test vẫn ĐÚNG

Ở `telesales-online` mutation này bắt được lỗ hổng seed thật. Ở đây nó **xanh — và xanh là đúng**:
tầng trong gom theo `GROUP BY customer_id, appointment.id, branch_id`, mà `appointment.id` là
KHOÁ CHÍNH ⇒ mỗi nhóm đúng **1 dòng** ⇒ `MIN ≡ MAX` bất kể dữ liệu. `MAX()` ở đây chỉ để thoả
`ONLY_FULL_GROUP_BY`, không phải lựa chọn ngữ nghĩa.

Cách phân biệt "test yếu" với "mutation là no-op" — **đo, đừng lập luận**:

```sql
-- 1. nhóm lớn nhất của tầng trong có bao nhiêu dòng?
SELECT MAX(c) FROM (SELECT COUNT(*) c FROM appointment WHERE branch_id IS NOT NULL
                    GROUP BY customer_id, id, branch_id) x;   -- ra 1
-- 2. chạy cả hai biến thể trên bench rồi diff output
```
Kết quả: 15 dòng, khớp byte-for-byte trên 872.284 lịch hẹn ⇒ no-op. Nếu bước 1 ra >1 thì mới là
lỗ hổng seed và phải thêm dữ liệu như đã làm ở `telesales-online`.

### ⚠ Bẫy sửa code: nhiều handler trong cùng file dùng CHUNG tên alias

`BranchStatistics` và `dimensionStatistics` (8 endpoint `*-statistics` khác) đều đặt tên bảng dẫn
xuất là `A/B/C/D/P/Q/R/S/T/U`. Một lệnh `replace` toàn file cho `COALESCE(R.totalPayment, 0)` sẽ
sửa nhầm sang hàm kia. Phải **cắt lấy thân hàm rồi mới thay** — và `assert count == 1` trong
phạm vi đó, không phải phạm vi file.

## Query GROUP BY không phân trang KHÔNG miễn nhiễm

`appointment-schedule-bydoctor` không dùng `pagination.Paginate` (không có `COUNT(*)` derived),
nhưng vẫn tốn ~1s ở 524k dòng — toàn bộ nằm ở **query threshold quét 56 ngày lịch sử**, không
phải query hiển thị. Mẫu chung của họ endpoint lịch (`appointment-schedule*`): mỗi lần load chạy
**2 query trên cùng bảng** — một quét quá khứ để tính baseline "đông/vắng", một quét tương lai để
hiển thị. Query quá khứ luôn quét nhiều dòng hơn hẳn (41.703 vs 22.570 dòng ở bench) nên đo phải
tách riêng hai câu; đo gộp sẽ đổ lỗi nhầm cho câu hiển thị.

⚠ Bẫy correctness đi kèm mẫu này: **câu baseline PHẢI có cùng phạm vi lọc với câu hiển thị**
(company + branch + loại). Câu hiển thị đi qua `tenant.ScopedDBNullBranch`, câu baseline là raw
SQL nên phải tự dựng lại fragment — rất dễ thiếu. Đã đo trên bench 524k dòng, user chỉ thấy
3/17 chi nhánh: baseline của một bác sĩ = **16.47 lịch/ngày** (quét toàn bộ chi nhánh) trong khi
sum hiển thị chỉ cộng 3 chi nhánh (≈3.75) ⇒ ngưỡng "đông" 30.2 không bao giờ chạm tới ⇒ cột
trạng thái **luôn báo "Bình thường"**. Lệch tương tự với loại lịch hẹn đã tắt (14.24 → 12.94) và
với record `company_id IS NULL`. Cách chặn tái diễn: lấy danh sách loại/phạm vi MỘT lần ở đầu
handler rồi dùng chung cho cả 3 chỗ (baseline · hiển thị · cột pivot) —
`activeAppointmentTypes` + `buildAppointmentBranchScopeSQL` trong `crm/appointment/handler/schedule.go`.
Và đừng nuốt lỗi câu baseline: `Raw(...).Scan(...)` không check error ⇒ baseline rỗng ⇒ cũng ra
"Bình thường" cho mọi ô, nhìn y hệt một ngày vắng khách.

## Query có pre-filter chọn lọc ⇒ không dính 3 nút thắt trên

`customer_id IN (SELECT ... FROM appointment WHERE date BETWEEN <1 ngày>)` được materialize trước
và làm bảng dẫn ⇒ khối lượng chặn theo **số khách trong ngày**, không theo kích thước bảng.

Đo `get-appointment-debts` @600k `order_service_used` / 530k `appointment`: mọi ca 73–166ms net;
tăng 718 → 6.000 lịch/ngày **không** chậm thêm; keyword còn nhanh hơn không-keyword (lọc bớt nữa),
`LIKE '%kw%'` chỉ chạy trên ~1.700 dòng đã lọc. `LEFT JOIN customer` vô điều kiện ở đây chỉ ~718
single-row PK lookup — **ĐỪNG bỏ join**, rủi ro vỡ export lớn hơn phần thắng được.

## Chỉ mục 1 cột `company_id` vừa là thủ phạm vừa là thứ PHẢI GIỮ

> ⚠ Lịch sử (D15): schema mới không còn `company_id` — mục này chỉ giải thích số đo cũ, không áp dụng cho CRM standalone.

Đo trên `customer_warranty_card` (615.992 thẻ / 76.999 khách thật, 2026-09-07). Bảng chỉ có PK +
10 chỉ mục MỘT cột kiểu `fk-...` — đúng hình dạng của **phần lớn bảng trong repo**, nên kết luận
dưới đây áp được rộng.

Thêm 3 composite `(company_id, branch_id, created_at)` · `(company_id, customer_id)` ·
`(company_id, card_code)`:

| Kịch bản | Trước | Sau |
|---|---|---|
| `COUNT` lọc theo ngày (bộ lọc mặc định của màn) | 2.884ms | **43ms** |
| `COUNT` không lọc | 5.455ms | **181ms** |
| `SELECT` trang 1 lọc theo ngày | 228ms | **44ms** |

⚠ **Đừng bỏ chỉ mục một cột `(company_id)` dù nó là tiền tố trái của composite.** Nghe thì thừa,
nhưng InnoDB nối PK vào đuôi mọi chỉ mục phụ ⇒ `(company_id)` thực chất là `(company_id, id)`, tức
phục vụ `ORDER BY id DESC` bằng `Index range scan … (reverse)` MIỄN PHÍ. Đo được cả hai chiều:

| | giữ chỉ mục 1 cột | bỏ chỉ mục 1 cột |
|---|---|---|
| `COUNT` tìm theo khách (`customer_id IN (6622 id)`) | 1.216ms | 360ms |
| `SELECT` phân trang sâu `OFFSET 300.000` | **869ms** | **4.793ms** |

⇒ Giữ. `COUNT` chạy mỗi lần load nhưng composite đã kéo nó xuống đủ; đổi lấy regression 5,5 lần ở
phân trang sâu là lỗ. `FORCE INDEX` cũng đã thử (460ms) và loại: hint cứng vỡ IM LẶNG khi chỉ mục
đổi tên.

⚠ **Ngoại lệ — scope `ScopedDBNullBranch` (`company_id = ? OR company_id IS NULL`)**: luật trên chỉ đúng
cho `company_id = ?` chặt. Với `OR IS NULL` + `ORDER BY id DESC LIMIT`, optimizer chọn `ref_or_null` trên
`(company_id)` rồi **sort cả tập** (log_appointment 153k: trang 1 0.27s); không có index thì quét ngược PK,
dừng ở dòng 20 (0.001s). `ANALYZE TABLE` không đổi plan. Bảng chỉ đọc qua NullBranch và đa số dòng cùng 1
company (DB-per-customer) ⇒ **không** tạo `(company_id)`; nhiều company chênh lệch lớn ⇒ đo trước khi chọn.

**Xác nhận lần 2 trên bảng khác + một sắc thái bắt buộc nhớ** (`order_service_used`, 524.288 dòng,
2026-09-07): cùng cơ chế, nhưng ở đây chỉ mục hẹp `(company_id, isLock)` là **tạo MỚI có chủ ý**,
và nó chỉ đúng khi đi CẶP với composite phủ — **một mình thì có HẠI**:

| | `COUNT` lọc 1 ngày | `SELECT` trang 1000 (`OFFSET 14985`) |
|---|---|---|
| Nguyên trạng (chưa index nào) | 1.12–1.35s | 4.31–19.04s |
| CHỈ chỉ mục hẹp `(company_id, isLock)` | **4.04–4.84s** ← tệ hơn baseline | 0.18–0.51s |
| CHỈ composite phủ | 0.02–0.03s | 12.81–15.57s |
| **Cả hai** | **0.02–0.03s** | **0.22–0.31s** |

⇒ Đừng suy ra "index hẹp luôn tốt" hay "index hẹp luôn thừa". Nó chỉ có giá trị ở đúng đường
`ORDER BY id DESC`; đường tổng/`COUNT` phải do composite phủ gánh. Ship một mình cái nào cũng là
đánh đổi lỗ. Và vì nó là tiền tố trái của composite, **phải ghi lý do ngay trong header migration**
— không thì đợt dọn index sau sẽ xoá đúng nó. Migration:
`20260907072400_osu_service_used_with_appointment_scale_indexes.up.sql`.

**Chiều ngược lại — index hẹp KHÔNG dẫn đầu bằng cột lọc equality thì DỤ optimizer bỏ quét PK ngược**
(`tooth_bio`, 540.000 dòng, 2026-09-07). `ADD FOREIGN KEY (branch_id)` tự sinh `KEY (branch_id)`;
scope là `branch_id = ? OR branch_id IS NULL` nên `OR` phá mất equality, index chỉ còn dùng được để
lọc rồi phải sort. Cùng câu `ORDER BY tooth_bio.id DESC LIMIT 20`:

| | 540k dòng |
|---|---|
| CÓ `KEY (branch_id)` | Index lookup → `Sort` 107.170 dòng — **356ms** |
| KHÔNG có | `Index scan on PRIMARY (reverse)`, 23 dòng — **1,35ms** (260×) |

- Cột scope `company_id`/`branch_id` **không bắt buộc FK** (`writing-mysql-migrations/SKILL.md`
  §FOREIGN KEY) — thêm FK là tự rước index hẹp đó về.
- Giữ FK an toàn khi cột là **tiền tố trái của composite đã có**: `company_id` dẫn đầu
  `(company_id, branch_id, id)` nên MySQL không sinh index mới (kiểm bằng `SHOW CREATE TABLE`).
- Thêm cột scope vào bảng có query `ORDER BY <PK> DESC LIMIT n` ⇒ `EXPLAIN ANALYZE` TRƯỚC và SAU khi
  thêm FK. Migration: `20260907074900_tooth_bio_tenant_scope_columns.up.sql`.

## Đã đo và KHÔNG có vấn đề — đừng bench lại

Ghi lại cả kết quả âm tính, không chỉ nút thắt: session sau sẽ tốn cả tiếng dựng bench để ra đúng
kết luận "không sao".

| Endpoint | Quy mô đo | Kết quả |
|---|---|---|
| `GET /crm/customer-warranty-card/get-service` | 529.360 `order_service_used` (2026-09-07) | **41ms** cả ca khách nặng nhất (180 dịch vụ) lẫn ca khách không có dịch vụ — trừ 35ms overhead còn ~5ms |
| `GET /crm/customer/:id` | 615.992 `customer` (2026-09-07) | **~0,3ms tổng SQL/request** — cả 5 câu đều là tra khoá chính/bảng nhỏ, `EXPLAIN` ra `Rows fetched before execution` (const table). Xem mục dưới. |
| `GET /crm/note/list-advise` | 6.740.008 `note` (2026-09-07) | **~13ms tổng SQL/request** ở khách nặng nhất (2.096 ghi chú): COUNT 5,5ms + SELECT 7,2ms, `OFFSET 2000` 11,9ms. Ở 842.501 dòng (dữ liệu dev thật) mọi ca **chạm sàn overhead 32ms** của client. Xem mục dưới. |
| `GET /crm/reminder/reminder-of-customer` | 650.000 `reminder` / 100.000 khách (2026-09-07) | **1–2ms SELECT trang 1 · 44–72ms COUNT** ở ca cực đoan 50.006 lời nhắc MỘT khách; khách thường (6 lời nhắc) và khách chưa có lời nhắc nào đều **≤1ms**. Xác nhận lần 2 khuôn "list con của một cha" ngay dưới. |
| `GET /crm/check-up-general/check-up-general-of-customer` | 652.000 `check_up_general` / 100.000 khách (2026-09-07) | **0,4ms SELECT trang 1 · 7,3ms COUNT** ở ca cực đoan 50.006 phiếu khám MỘT khách; ca chậm nhất là COUNT kèm lọc `department_type` = **54,8ms**. Xác nhận lần **3** khuôn "list con của một cha". ⚠ Endpoint ANH EM `GET /crm/check-up-general` (list ngoài) thì **CÓ nút thắt 0,9s** — mục dưới. |

Vì sao an toàn dù `order_service_used` có `idx_osu_company_islock_order (company_id, isLock)` khớp
**86% bảng** (457.777/529.360 dòng `isLock = 1`): điều kiện `customer_id = ?` là đẳng thức MỘT giá
trị nên optimizer ước lượng 185 dòng và chọn `fk-order_service_used-customer_id`, không rơi vào bẫy
ref-lookup giả rẻ ở mục trên. **Bẫy đó chỉ bật khi vế còn lại là RANGE hoặc IN nhiều giá trị** —
đẳng thức đơn trên cột chọn lọc cao thì miễn nhiễm.

⚠ Bẫy khi tự dựng ca đo: `define.ServiceUsedNotLock = 1`, **không phải 0**. Lấy nhầm 0 thì biến
`customer_id` ra RỖNG, câu SQL thành cú pháp sai, và harness vẫn in ra một con số ms trông hợp lệ
(40ms) — đúng loại "kết luận từ lệnh không hề chạy". Luôn in `[ -z "$VAR" ] && exit 1` + để stderr
của `mysql` hiện ra.

### Vì sao endpoint CHI TIẾT theo id miễn nhiễm — kiểm 3 điểm rồi dừng

Đo `GET /crm/customer/:id` trên 615.992 khách + 5.000 broker (2026-09-07). Toàn bộ request là 5 câu,
không câu nào phụ thuộc kích thước bảng:

| Kịch bản (200 lượt/lần đo, đã trừ baseline 200×`SELECT 1` = 20ms) | 615.992 dòng |
|---|---|
| Chỉ câu chính, khách KHÔNG có CTV (`LEFT JOIN` trượt) | 40–50ms / 200 lượt ⇒ **~0,15ms/lượt** |
| Chỉ câu chính, khách CÓ CTV (`LEFT JOIN` trúng) | 40–50ms / 200 lượt |
| Khách có `tags` dài nhất bảng | 40–50ms / 200 lượt (không khác) |
| id không tồn tại (đường 404) | `Zero rows (no matching row in const table)` |
| Thêm vế `leadscope` (`payment_user_id = ?`) | 30–60ms / 200 lượt |
| **Trọn 5 câu của 1 request** (option 5 · `user_profile` · câu chính · `location_ward` · option 14) | **80ms / 200 lượt ⇒ ~0,4ms/request** |

`EXPLAIN ANALYZE` câu chính ra `Rows fetched before execution … actual time=0,0005ms` — MySQL giải
xong ở giai đoạn tối ưu (const table) vì cả `customer.id` lẫn `broker.id` đều là khoá chính. Vế
`(company_id = ? OR company_id IS NULL)` của tenant scope KHÔNG kéo được nó ra khỏi đường const.

⇒ Khuôn để nhận diện nhóm này mà **không cần dựng bench**: handler chỉ có (a) đẳng thức trên khoá
chính của bảng chính, (b) join theo khoá chính bảng khác, (c) vài tra bảng cấu hình nhỏ
(`user_option`, `user_profile`) — thì kích thước bảng không vào phương trình. Ba thứ PHẢI kiểm
trước khi kết luận vậy, vì chúng là đường duy nhất biến "chi tiết" thành "quét bảng":

1. **Vòng lặp ẩn sau khi lấy record** — attach quan hệ 1-n, đếm lịch sử, tổng tiền. Ở đây chỉ có
   `location_ward` tra theo PK, và chỉ chạy khi `ward_id > 0`.
2. **Helper đọc metadata mỗi request.** `helpers.ParseJSONFieldForTable` /
   `ParseDecimalFieldsForTables` nhìn như quét `information_schema` mỗi lần gọi; thật ra map cột
   được dựng MỘT lần qua `sync.Once` (`pkg/helpers/json_schema.go`) ⇒ chi phí O(số cột), hằng số
   (customer có 143 cột).
3. **`SELECT customer.*`** — không phải nút thắt hiệu năng ở đây (dòng trung bình ~19 byte phần
   text, dòng nặng nhất 195 byte). Đã ĐO trước khi định thay bằng danh sách cột tường minh, và
   **kết luận là ĐỪNG**: đếm tham chiếu từng cột trong `frontend/src` (đối chứng: `fullname` khớp
   461 file) cho **137/143 cột CÓ người đọc**; 6 cột còn lại là `salesStaff_id`, `agentEmail`,
   `idCareSoft`, `ma_dantoc`, `ma_nghe_nghiep`, `search_ngram` — cộng lại ~50 byte/record
   (`AVG(LENGTH(search_ngram))` = 47). Liệt kê tay 137 cột đổi ~50 byte lấy một cái bẫy mất-dữ-liệu
   im lặng: migration sau thêm cột mà quên thêm vào `Select` thì FE mất field, build vẫn xanh. Cột
   nhạy cảm thì bảng `customer` không có (không token/không hash) — che số điện thoại đã do
   `phoneutil.MaskPhoneInItems` làm ở tầng trên. ⇒ Ngoại lệ có chủ đích của Hard Rule "KHÔNG
   `SELECT *`", ghi lại ở đây để lần sau khỏi mở lại.

⚠ Sắc thái không liên quan tốc độ, phát hiện lúc đo — **ĐÃ VÁ 2026-09-07**: `Where("customer.id = ?",
c.Param("id"))` bind tham số dạng CHUỖI, nên `/crm/customer/79738abc` **trả về đúng khách 79738**
(MySQL cắt chuỗi thành số, warning 1292 bị nuốt). Xác nhận bằng mutation test qua đúng đường
GORM + driver, không chỉ bằng `mysql` client: `Where("id = ?", "1abc")` trả về record id=1.
Vẫn nằm trong tenant scope nên không phải lỗ bảo mật cross-tenant, nhưng các route GHI cùng khuôn
(`add-tag`, `remove-tag`, `update-macro`, `create-avatar`, `PUT /:id`) thì URL rác **sửa trúng
record thật**. Bản vá: parse id ra số ở handler (`parseCustomerID` / `helpers.ParseID`) và đổi
`CustomerService.Update` sang nhận `id int` — để **compiler** chặn đường cũ thay vì dựa vào việc
nhớ. Test neo: `TestGetCustomer_NonNumericIDIsRejected`.

⚠ Bẫy khi tự viết mutation test cho lớp bug này: `parseCustomerID` TỰ ghi `response.NotFound(c)`.
Neutralize kiểu `if false && !ok` để lại lời gọi parse ⇒ 404 vẫn được ghi ⇒ test **vẫn xanh** và
báo sai rằng bản vá không load-bearing. Phải bỏ HẲN lời gọi parse thì lưới mới đỏ đúng (đã dính
một lần 2026-09-07).

### List CON của một cha (`WHERE <fk> = ?`) cũng miễn nhiễm — chi phí là O(số con), không O(bảng)

Đo `GET /crm/note/list-advise?id=<customer_id>` (2026-09-07). Handler chỉ có MỘT bảng, không join,
không keyword: `note` lọc `customer_id = ?` + vế tenant `(company_id = ? OR company_id IS NULL)`,
`ORDER BY pinned DESC, pinned_at DESC, id DESC`, rồi `pagination.Paginate`.

Bench: clone schema thật bằng `mysqldump --no-data` + nạp NGUYÊN dữ liệu `note` của DB dev
(842.501 dòng / 71.493 khách, phân bố thật: trung bình 11,8 ghi chú/khách, **nhiều nhất 262**),
rồi nhân đôi 3 lần lên **6.740.008 dòng** (khách nặng nhất thành 2.096 ghi chú).

| Ca | 842.501 dòng | 6.740.008 dòng (`EXPLAIN ANALYZE`) |
|---|---|---|
| SELECT khách thường (12 ghi chú) | 32–36ms = sàn overhead | — |
| SELECT khách nặng nhất | 33–35ms = sàn | **7,2ms** (2.096 dòng) |
| COUNT khách nặng nhất | 32–33ms = sàn | **5,5ms** |
| Phân trang sâu `OFFSET 240` / `OFFSET 2000` | 32–39ms = sàn | **11,9ms** |
| `customer_id` không tồn tại | 32ms = sàn | ~0ms |

Đối chứng: `SELECT 1` đo cùng lúc = 32–44ms ⇒ mọi ca ở 842k dòng nằm TRONG sai số của overhead
kết nối `mysql` client, không đo được bằng đồng hồ — phải đọc `EXPLAIN ANALYZE` mới thấy con số thật.

Cơ chế: `customer_id = ?` là đẳng thức đơn trên `fk_note-customer_id` ⇒ plan là
`Index lookup on note using fk_note-customer_id` ở CẢ hai câu; MySQL còn **loại bỏ `ORDER BY`** trong
derived table của `COUNT` (dòng `Sort:` biến mất khỏi plan COUNT). Chi phí ≈ **3,4µs × số ghi chú của
CHÍNH khách đó**, hằng số theo kích thước bảng — nhân lên 8× số dòng mà thời gian không đổi.

⇒ Khuôn nhận diện nhóm này: list con của một cha, `WHERE <fk> = ?` với fk CÓ chỉ mục, không join,
không `LIKE '%kw%'`. Ba nút thắt lớn ở đầu file (LEFT JOIN vô điều kiện · `COUNT` derived · `LIKE`
xuyên join) đều KHÔNG chạm tới nó. Thứ duy nhất phá được khuôn là **phân bố lệch**: một cha gom
hàng chục nghìn con. Ở đây trần thật đo được là 262 (dev) / 2.096 (bench ×8); ngay cả 10.000 ghi chú
một khách vẫn chỉ ~35ms. Trước khi tuyên bố miễn nhiễm, LUÔN chạy
`SELECT MAX(c) FROM (SELECT COUNT(*) c FROM <bảng> GROUP BY <fk>) t` — trần đó là toàn bộ câu chuyện.

⚠ Hai Hard Rule từng bị vi phạm ở `ListAdvise`, **ĐÃ VÁ 2026-09-07** (cùng lượt vá cả module `note`):
`SELECT *` (thiếu `.Select(...)`) và `c.Query("id")` truyền thẳng dạng CHUỖI vào
`Where("customer_id = ?", ...)` ⇒ `?id=12976abc` trả về đúng ghi chú của khách 12976 (xác nhận warning
1292 trên bench). Bản vá: `strconv.ParseUint` ở đầu handler (rác/0 → cùng lỗi `id_required` như trước,
KHÔNG đổi hợp đồng API) + hằng `noteColumns` liệt kê **đủ 9 cột** — trùng khít `GetAllNote` nên không
cột nào rụng khỏi response. `helpers.ParseID` KHÔNG dùng được ở đây: nó chỉ đọc **path param**.

Cùng lượt: `GetNote` (`c.Param` thô + `SELECT *`), `DeleteNote` (`strconv.ParseUint` nuốt lỗi bằng `_`)
và `PinNote` (path param không parse được thì **rơi về `id` trong body/query** ⇒ URL rác + body tự đặt
id = ghim nhầm note) đều đã vá; test neo `note_id_integration_test.go` (25 case) đã chứng minh bằng
mutation `-overlay`: hoàn nguyên từng bản vá thì lưới đỏ đúng chỗ. **Ngoại lệ trung thực** — case
"chỉ trả đúng 9 cột" KHÔNG đỏ khi hoàn nguyên `SELECT *`, vì danh sách tường minh trùng khít toàn bộ
cột của bảng; nó neo cho **column drift sau migration**, không neo cho việc bỏ `SELECT *`.

### Xác nhận lần 2 khuôn trên — `reminder-of-customer`, và mẹo đọc đồng hồ không bị sàn overhead

`GET /crm/reminder/reminder-of-customer?idCustomer=<id>` là ĐÚNG khuôn ở mục trên (một bảng, không
join, không keyword, `WHERE customer_id = ?` + vế tenant, `ORDER BY reminder.id DESC`, `Paginate`)
và đo ra cùng kết luận — nhưng lần này **đo được bằng đồng hồ**, vì dùng `mysql -e` (server time)
thay vì phiên `-vvv` nhiều câu: sàn overhead tụt từ 32ms xuống 0ms, `SELECT 1` = 0.000s.

Bench: schema thật (`mysqldump --no-data`) + **650.000 `reminder` / 100.000 khách**, phân bố lệch
cố ý (khách thường 6 · khách nặng 2.006 · khách cực đoan 50.006 · 0,1% `company_id IS NULL`).

| Ca | 2.006 lời nhắc/khách | 50.006 lời nhắc/khách |
|---|---|---|
| SELECT trang 1 | 1ms | **1–2ms** |
| COUNT của `Paginate` | 10–14ms | **44–72ms** |
| Phân trang sâu (`OFFSET` cuối) | 12–14ms | **54–55ms** |
| COUNT + lọc `start_time` 1 ngày | 4–9ms | 46–75ms |
| COUNT với scope `company_id IN (…)` | — | 43–44ms |
| Khách chưa có lời nhắc nào | \<1ms (`rows=0`, thoát ở 0,016ms) | — |

`EXPLAIN ANALYZE` cả ba ca: `Index lookup on reminder using fk-reminder-customer_id (customer_id = ?)`,
và câu SELECT thêm **`(reverse)`** — InnoDB nối PK vào đuôi chỉ mục phụ nên `fk-...-customer_id`
thực chất là `(customer_id, id)` ⇒ `ORDER BY id DESC` **miễn phí**, `LIMIT 20` early-exit thật.
Vế `(company_id = ? OR company_id IS NULL)` chỉ là `Filter` chạy trên đúng số dòng đã lấy ra, KHÔNG
kéo được câu query ra khỏi đường index lookup. ⇒ Bảng `reminder` **không cần thêm chỉ mục nào**;
`idx_reminder_company_start_time` hiện có là của endpoint danh sách anh em, không phải của đường này.

⚠ Đối chứng bắt buộc kèm theo (Iron Law): cùng harness đó chạy `COUNT(*) … note LIKE '%không-khớp%'`
trên chính bench này ra **317–382ms** ⇒ các số mili-giây ở trên là phép đo thật, không phải đồng hồ hỏng.

⚠ Cùng hai vi phạm Hard Rule như `ListAdvise`: `c.Query("idCustomer")` đi thẳng vào
`Where("reminder.customer_id = ?", ...)` dạng CHUỖI ⇒ `?idCustomer=777abc` trả về đúng lời nhắc của
khách 777 (GORM bind `customer_id = '777abc'`, MySQL cắt còn 777, warning 1292 bị nuốt). Đường ĐỌC
nên chỉ sai kết quả, không sửa nhầm record. **ĐÃ VÁ 2026-09-07**: `strconv.ParseUint` ở đầu handler,
không parse được / bằng 0 ⇒ 422 (dùng lại key `missing_id_customer_param`, không đẻ key i18n mới).
Test neo `TestReminderOfCustomer_NonNumericIDIsRejected` chạy với `DB: nil` — đường đúng chặn TRƯỚC
khi chạm DB, nên bỏ khâu parse là panic ⇒ đỏ (đã kiểm bằng mutation bỏ HẲN lời gọi parse, không phải
`if false &&`). ⚠ Ca `" 777"` viết trần trong test làm `httptest.NewRequest` panic khi dựng URL —
phải encode `%20777`, nếu không sẽ tưởng handler hỏng.

### Xác nhận lần 3 khuôn list-con — `check-up-general-of-customer`, và cái giá của bảng KHÔNG có `company_id`

`GET /crm/check-up-general/check-up-general-of-customer?idCustomer=<id>` là ĐÚNG khuôn hai mục trên
(một bảng, không join, không keyword, `WHERE customer_id = ?`, `ORDER BY id DESC`, `Paginate`) — và
lần này khuôn được xác nhận trên một bảng **không có `company_id`**: tenant scope đi qua
`customerInTenant()` (một lần đếm khoá chính trên `customer`) chứ KHÔNG nhét vào câu list.

Bench: schema thật (`mysqldump --no-data`) + **652.000 `check_up_general` / 100.000 khách**, lệch cố ý
(khách thường 6 phiếu · khách nặng 2.006 · khách cực đoan 50.006). ⚠ Bảng này **rỗng trên DB dev**
(feature mới) ⇒ không có phân bố thật để soi; trần 50.006 là ca dựng, thực tế một người không thể có
ngần ấy phiếu khám tổng quát.

| Ca (khách cực đoan 50.006 phiếu) | `EXPLAIN ANALYZE` | Đồng hồ (sàn ~15–22ms) |
|---|---|---|
| SELECT trang 1 | **0,40ms** | 15–16ms = sàn |
| COUNT của `Paginate` | **7,3ms** | 40–54ms |
| SELECT `OFFSET 49980` (trang cuối) | — | 81–154ms |
| COUNT + lọc `created_day` 1 ngày | — | 94–109ms |
| COUNT + lọc `department_type` (**chậm nhất**) | **54,8ms** | 90–126ms |
| Khách thường (6 phiếu) / khách chưa có phiếu nào | — | 14–18ms = sàn |

Đối chứng bắt buộc (Iron Law): cùng harness chạy `COUNT(*) … diagnosis LIKE '%khong-khop-gi-ca%'`
trên chính bench này ra **304–349ms**, `SELECT 1` ra 15–22ms ⇒ các số trên là phép đo thật.

Cơ chế, giống hệt `reminder`: `Index lookup … using idx_check_up_general_customer_id (customer_id = 1)`
**`(reverse)`** ở câu SELECT — InnoDB nối PK vào đuôi chỉ mục phụ nên nó thực chất là
`(customer_id, id)`, `ORDER BY id DESC` miễn phí và `LIMIT 20` early-exit thật (0,4ms bất kể bảng to
cỡ nào). COUNT đi `Covering index lookup` nên 50.006 dòng chỉ mất 7,3ms. Ca `department_type` đắt gấp
7 lần vì cột đó KHÔNG nằm trong chỉ mục ⇒ phải chạm dòng thật; vẫn chỉ 54,8ms, và tỷ lệ thuận với số
con của CHÍNH khách đó, không với kích thước bảng. ⇒ **Không cần thêm chỉ mục nào.**

⚠ **Endpoint ANH EM `GET /crm/check-up-general` (list ngoài) thì CÓ nút thắt — ĐÃ VÁ 2026-09-07**
(migration `20260907195602_check_up_general_tenant_columns`). Cùng module, cùng bảng, đo cùng lượt.

Đo bằng **số dòng InnoDB đọc** (bất biến theo tải máy — hôm đó máy có phiên bench khác, `SELECT 1`
nhảy 15→260ms nên đồng hồ không dùng được; hiệu chuẩn: `SELECT 1` = 18 dòng, `COUNT(*)` toàn bảng
= 652.004 ≈ 652.000):

| Ca | Trước | Sau |
|---|---|---|
| SELECT trang 1 (không filter) | **113.337 dòng đọc** cho 20 dòng trả về | **82 dòng** |
| COUNT của `Paginate` | 113.337 | 200.001 (**covering index**, không fetch dòng) |
| Đồng hồ lúc máy rảnh, SELECT trang 1 | 727–918ms | 0,12ms (`EXPLAIN ANALYZE`) |

Nguyên nhân **không phải** `LIKE`, **không phải** `COUNT` derived: `check_up_general` KHÔNG có cột
tenant, nên scope phải đi `customer_id IN (SELECT id FROM customer WHERE …)`. MySQL viết lại thành
semijoin **dẫn từ `customer`**, kéo toàn bộ dòng trong tenant rồi `Sort: id DESC limit 20` ⇒ `LIMIT`
mất sạch early-exit, chi phí O(cả tenant). Cộng thêm `LEFT JOIN customer` vô điều kiện chồng một
lượt tra khoá chính nữa cho MỌI dòng — trong khi khi không có từ khoá nó không đóng góp cột nào
(customer lồng được nạp bằng query riêng ở dưới).

**Bản vá gồm 3 phần**: (1) thêm `company_id NOT NULL` + `branch_id` snapshot từ customer;
(2) index `idx_check_up_general_scope (company_id, id, branch_id)`; (3) `checkUpGeneralTenantScope`
đổi sang `tenant.ScopedDBNullBranch(c, q, "check_up_general")`, và `LEFT JOIN customer` chỉ gắn khi
`_keyword` khác rỗng.

| Biến thể | Số dòng đọc | Vì sao |
|---|---|---|
| Bỏ `LEFT JOIN` thừa, scope vào `INNER JOIN … ON` | ~60.000 | **LOẠI** — optimizer VẪN dẫn từ `customer`, sort còn nguyên. Chữa triệu chứng. |
| Cột riêng + `(company_id, branch_id, id)` | SELECT 60.004 (sort) / COUNT 60.004 | **LOẠI cho SELECT** — `branch_id = ? OR IS NULL` = HAI range ⇒ không có luồng đã-sắp-xếp duy nhất. COUNT thì tốt nhất. |
| Cột riêng + `(company_id, id)` | SELECT 50.064 / COUNT 250.007 | **LOẠI** — COUNT phải fetch dòng để lọc `branch_id`, TỆ HƠN nguyên trạng. |
| **Cột riêng + `(company_id, id, branch_id)`** | **SELECT 82 / COUNT 200.001** | **CHỌN** |

⚠ Ba điều rút ra, tái dùng được cho mọi bảng con thiếu cột tenant:

1. **`company_id` PHẢI `NOT NULL`** — đó chính là thứ làm nên phần thắng. Cột NULL-able thì
   `(company_id = ? OR company_id IS NULL)` thành hai range và SELECT tụt về 399ms (đã đo). Cột
   `NOT NULL` ⇒ optimizer rút gọn vế `IS NULL` thành always-false, còn một range ⇒ đi
   `Index lookup … (reverse)`, `ORDER BY id DESC` miễn phí, `LIMIT 20` early-exit thật.
   An toàn vì `customer.company_id` cũng `NOT NULL` (0 dòng NULL trên DB dev).
2. **Vế `OR IS NULL` không tránh được thì đẩy cột đó xuống ĐUÔI index.** `branch_id` bắt buộc
   nullable (7.522/76.999 khách có `branch_id IS NULL`), nên để nó ở đuôi `(company_id, id, branch_id)`:
   lọc ngay trong index, không fetch dòng, và không phá thứ tự `id`.
3. **KHÔNG tạo thêm `(company_id, branch_id, id)` "cho COUNT".** Đo được: khi hai index tồn tại song
   song, optimizer chọn NÓ cho SELECT (60.004 dòng) thay vì index đúng (82 dòng). Một index thôi.
   Và KHÔNG chữa bằng `FORCE INDEX` — hint cứng vỡ im lặng khi index đổi tên (mục cost-model ở trên).

⚠ **Đổi ngữ nghĩa, có chủ đích**: scope nay đọc snapshot lúc tạo phiếu, không phải company/branch
HIỆN TẠI của khách. Khách chuyển chi nhánh ⇒ phiếu khám cũ ở lại chi nhánh đã khám. Đúng cho chứng
từ y tế và đồng bộ với `note`/`reminder`/`order_service_used` (đều mang cột tenant riêng). Đối chiếu
9/9 tổ hợp company×branch trên bench: số dòng cũ = mới.

⚠ **Đo lại trên hình dạng DB-per-customer THẬT (600.000 dòng, TẤT CẢ cùng `company_id`) — và trần
còn lại là `COUNT`.** Bench 3-company ở trên làm `company_id` trông chọn lọc hơn thực tế; dựng lại
với 1 company duy nhất (đúng mô hình 1 DB = 1 khách), 4 chi nhánh + 10% `branch_id IS NULL`:

| Cấu hình index | SELECT trang 1 | COUNT |
|---|---|---|
| **`(company_id, id, branch_id)`** ⟵ đang dùng | **63 dòng** | 600.002 (cả bảng) |
| `(company_id, branch_id, id)` | 180.003 + sort | 180.017 |
| thêm `(company_id, branch_id)` cạnh index đang dùng | 63 | 600.002 — optimizer **không thèm** dùng |
| thêm `(branch_id, company_id)` cạnh index đang dùng | 63 | 600.002 — cũng không dùng |

Mỗi request trả giá CẢ HAI câu, nên cấu hình đang dùng vẫn thắng tổng (63 + 600.002 so với
180.003 + 180.017 nhưng vế SELECT phải sort). Ép `FORCE INDEX (company_id, branch_id)` cho COUNT
đo được 180.062 dòng (216ms → 58ms) — **nhưng KHÔNG dùng được**: `pagination.Paginate` chạy
`COUNT(*) FROM (?)` trên **CHÍNH query object** của SELECT, nên hint sẽ dính cả hai câu và giết
SELECT. Muốn ăn phần này phải sửa `Paginate` — file dùng chung của mọi list, không đáng cho một
endpoint.

⇒ **Trần còn lại: `COUNT` đọc toàn bộ dòng trong scope**, đúng nút thắt hệ thống "`COUNT(*)` derived
của `Paginate`" đã ghi ở đầu file. Lý do nó không cứu được bằng index ở mô hình DB-per-customer:
`company_id = ?` khớp **cả bảng** nên không có vế nào thu hẹp được, và cost model chấm ref-lookup
`company_id` rẻ giả (cost 6281 cho 600.000 dòng, so với 38211 cho 180.000 dòng) nên optimizer luôn
quay về nó — thêm index phụ chỉ tốn chỗ ghi, không được dùng.

⚠ Đo lúc máy có phiên bench khác: đồng hồ vô dụng (`COUNT(*) … LIKE '%không khớp%'` toàn bảng ra
**37 giây**, đầu phiên là 0,3s). Mọi kết luận ở bảng trên dựa trên **số dòng InnoDB đọc**; hiệu chuẩn
`SELECT 1` = 1 dòng, `COUNT(*)` toàn bảng = 600.017 ≈ 600.000.

⚠ Bảng cũng thiếu **FOREIGN KEY thật** (chỉ có `KEY`) ⇒ `models.DeleteWithCheck` không bảo vệ được
gì. Migration thêm 4 FK: `customer_id`/`company_id` RESTRICT, `doctor_id`/`branch_id` SET NULL. Đã
kiểm bằng cách xoá thật: xoá khách có phiếu khám ⇒ **ERROR 1451** (đúng thứ `DeleteWithCheck` bắt);
xoá bác sĩ ⇒ `doctor_id` về NULL, phiếu khám còn nguyên. `created_by`/`updated_by` CỐ Ý không FK.


### Xác nhận lần 4 khuôn list-con — `appointment/get-by-customer`, `ORDER BY` KHÔNG phải `id`

`GET /crm/appointment/get-by-customer?idCustomer=<id>`
(`crm/customer_detail/handler/customer_detail.go` — **không** nằm trong module `appointment`, route
wire riêng ở `customer_detail/route.go`) là đúng khuôn: một bảng, KHÔNG join, không keyword,
`WHERE appointment.customer_id = ?` + vế tenant company/branch, `Paginate`. Khác hai lần trước ở một
điểm đáng đo: `ORDER BY appointment.appointment_date DESC` — **không** phải `id DESC`, nên nếu chỉ có
chỉ mục 1 cột `customer_id` thì mỗi lần mở tab lịch hẹn của khách là một filesort trên toàn bộ số
lịch hẹn của khách đó.

Không phải lo: `idx_appointment_customer_date (customer_id, appointment_date)` đã có sẵn trong
migration squash `20260814091602` ⇒ có trên MỌI DB khách, không riêng dev. Plan là
`Index lookup on appointment using idx_appointment_customer_date (customer_id = ?) (reverse)` —
`WHERE` và `ORDER BY` được phục vụ bằng **cùng một** chỉ mục, không có dòng `Sort:` nào.

Bench: schema thật (`mysqldump --no-data --set-gtid-purged=OFF`) + nạp NGUYÊN 218.071 lịch hẹn của DB
dev rồi nhân đôi 2 lần → **872.284 dòng / 39.159 khách** (trần thật 532 lịch hẹn/khách, trung bình
22,3), cộng một khách lệch cố ý **50.000 lịch hẹn** ⇒ tổng **922.284 dòng**. Số dưới đây là
`EXPLAIN ANALYZE` (thời gian server, không dính sàn overhead ~10ms của `mysql` client; đối chứng
`SELECT 1` = 0,01s = sàn).

| Ca | Khách 532 lịch hẹn | Khách 50.000 lịch hẹn |
|---|---|---|
| SELECT trang 1 (`LIMIT 20`) | **0,08ms** | **0,06–0,21ms** |
| COUNT của `Paginate` | **0,91ms** | **52ms** |
| Phân trang sâu (`OFFSET 49.900`) | — | 122ms |
| Vế branch KHÔNG khớp dòng nào | — | 117ms |

Cơ chế và hệ quả:

- **SELECT thoát sớm thật.** `LIMIT 20` dừng ngay sau 20 dòng khớp ⇒ 0,06ms ở khách 50k y như khách
  532. Bảng nở từ 218k lên 922k dòng mà con số không đổi: chi phí là **O(số con)**, không O(bảng) —
  đúng khuôn hai lần đo trước (`note/list-advise`, `reminder-of-customer`).
- **COUNT thì không thoát sớm được** ⇒ nó là trần thật của endpoint: 52ms ở 50.000 con. MySQL bỏ
  `ORDER BY` trong derived table của COUNT nên optimizer đổi sang `idx_appointment_customer_status`
  — vẫn `Index lookup`, vẫn O(số con).
- ⚠ **Bẫy riêng của lần này: `LIMIT` mất early-exit khi vế tenant lọc sạch.** Ca "branch không khớp"
  quét đủ 50.000 dòng để trả về 0 dòng (117ms) — bằng đúng ca `OFFSET` sâu. Vì `branch_id` không nằm
  trong chỉ mục, nó là `Filter:` **sau** index lookup, nên early-exit chỉ có khi thật sự có dòng
  khớp. Khách nhiều lịch hẹn ở chi nhánh khác + user bị giới hạn chi nhánh = ca xấu nhất, và nó là ca
  người dùng thật gặp được (khác `OFFSET 49.900` vốn không bấm tới).
- Ở phân bố THẬT (trần 133 lịch hẹn/khách trên dev, 532 sau khi nhân đôi) mọi ca đều ≤1ms ⇒ **không
  có nút thắt, không cần vá gì**. Chỉ khi một khách gom hàng chục nghìn lịch hẹn thì COUNT mới đáng
  bàn, và cách chữa lúc đó là `PaginateWithTotal`/đếm xấp xỉ, KHÔNG phải thêm chỉ mục.

### ⚠ Khuôn list-con KHÔNG tự động miễn nhiễm — `list-service-used-customer` là ca ĐỐI CHỨNG

Bốn mục trên dễ đọc thành "list con của một cha thì khỏi lo". SAI. Điều kiện thật là **`ORDER BY`
phải được phục vụ bởi CHÍNH chỉ mục đã dùng để lọc fk**. Thiếu điều kiện đó, `LIMIT` mất early-exit
và mỗi lần mở tab là một filesort trên TOÀN BỘ số con của khách.

`GET /crm/treatment-services/list-service-used-customer?id=<customer_id>`
(cùng file `crm/customer_detail/handler/customer_detail.go` với `get-by-customer`, cùng khuôn: một
bảng, không join, không keyword) đo ra khác hẳn — vì `order_service_used` **không có** chỉ mục nào
dạng `(customer_id, created_day)`, chỉ có `fk-order_service_used-customer_id` (1 cột) và
`idx_osu_customer_totalend (customer_id, totalEnd)`, trong khi `ORDER BY` là
`created_day DESC, id ASC`.

Bench: schema thật 4 bảng + dữ liệu thật nhân 6 lần ⇒ **724.040 `order_service_used` / 447.510
`order_treatment_plan`**, cộng một khách lệch cố ý 50.000 dòng. Đối chứng `SELECT 1` = 0,00008ms.

| Ca | Khách 234 dòng | Khách 50.000 dòng |
|---|---|---|
| SELECT trang 1 (`LIMIT 20`) | 1,3–7,6ms | **353–395ms** |
| COUNT của `Paginate` | 0,82ms | 95–114ms |
| Phân trang sâu (`OFFSET 49.900`) | — | 116ms |
| Lọc thêm `appointment_id` (kèm subquery `order_treatment_plan`) | 0,5–1,3ms | 116–120ms |
| Khách không tồn tại | — | 0,012ms |

So với `get-by-customer` ở cùng số con (50.000): **0,06ms → 353ms, chậm gấp ~6.000 lần**. Khác biệt
DUY NHẤT là chỉ mục — plan của `get-by-customer` không có node `Sort:`, plan ở đây có:

```
-> Sort: created_day DESC, id, limit input to 20 row(s) per chunk  (actual time=353..353 rows=20)
    -> Filter: (company_id = 1) and (branch_id = 14 or branch_id is null)  (actual … rows=50000)
        -> Index lookup on order_service_used using idx_osu_customer_totalend (customer_id = …)
```

⇒ **Cách đọc nhanh để biết một list-con có thật sự miễn nhiễm hay không: tìm chữ `Sort:` trong
`EXPLAIN ANALYZE`.** Có `Sort:` ⇒ chi phí là O(số con · log số con) và `LIMIT` vô dụng; không có ⇒
early-exit thật, O(20).

**KHÔNG ship chỉ mục — vì phân bố THẬT không cần.** Trần thật trên DB dev là **39 dòng dịch vụ/khách**
(234 ở bench chỉ là do nhân 6 lần). Ở 39 dòng, filesort là dưới 1ms. `order_service_used` đã mang
**11 chỉ mục** và bị ghi ở mọi lần chốt đơn ⇒ thêm chỉ mục thứ 12 để chữa một ca chưa tồn tại là lỗ.
Ngưỡng đáng quay lại: một khách vượt **vài nghìn** dòng dịch vụ.

Biến thể đã đo (giữ số để khỏi đo lại khi tới ngưỡng đó):

| Cách | SELECT trang 1 @50.000 con | Kết luận |
|---|---|---|
| Nguyên trạng (không chỉ mục composite) | 353–395ms | mốc |
| `KEY (customer_id, created_day)` — **tăng dần** | 86–114ms | **LOẠI.** `Sort:` VẪN CÒN: `ORDER BY` có `id ASC` ngược chiều nên index tăng dần không cho được thứ tự sẵn. Phần thắng chỉ do quét chỉ mục hẹp hơn, không phải bỏ được sort. |
| `KEY (customer_id, created_day DESC, id ASC)` — **chỉ mục giảm dần** (MySQL 8.0+) | **0,05ms** | ĐỦ ĐIỀU KIỆN ship khi tới ngưỡng: `Sort:` biến mất, early-exit thật. Qua gate `FORCE INDEX`: ép dùng chính nó = 0,057–0,079ms, KHÔNG chậm hơn để-optimizer-tự-chọn ⇒ phần thắng đúng là của chỉ mục. |

`COUNT` không đổi (~52ms @50.000) ở mọi biến thể — nó phải đếm hết số con, không chỉ mục nào cứu được.

Phần còn lại của endpoint đã đo và KHÔNG có vấn đề: 2 truy vấn attach sau `Paginate`
(`order_treatment_plan` theo `id IN (…)` = 0,05–0,64ms; `service LEFT JOIN service_type` theo
`service_id IN (…)` = 0,0004ms) — cả hai bị chặn trên bởi kích thước trang (≤20 id), không phải N+1.
Nhánh lọc `appointment_id` dùng subquery `order_treatment_plan` cũng an toàn:
`fk-order_treatment_plan-appointment_id` phục vụ nó.

## Chuỗi con trên cột MÃ không cứu được bằng chỉ mục — lần thứ hai đo ra cùng kết luận

`customer_warranty_card.card_code`, cùng bảng 615.992 dòng, chỉ mục `(company_id, card_code)`:

| | |
|---|---|
| tra id `card_code LIKE '%kw%'` | 2.419–11.928ms (TỆ HƠN cả đường LIKE cũ) |
| tra id `card_code LIKE 'kw%'` (tiền tố) | **82ms** |
| giữ `OR card_code LIKE '%kw%'` cạnh `customer_id IN (…)` | 6.637ms (`OR` phá range scan) |

Trùng khít kết luận đã đo cho `order_service_used.code` (2026-09-06) — và ngram cũng không cứu
được cột mã vì tiền tố gần như đồng nhất. **Cột mã chỉ có hai lựa chọn: tiền tố, hoặc chấp nhận
quét toàn bảng.** Đây là đổi hành vi người dùng cảm nhận được ⇒ PHẢI hỏi, đừng tự quyết. User đã
chốt tiền tố cho cả `order_service_used.code` (2026-09-06) và `card_code` (2026-09-07).

## ⚠ `mysqldump` KHÔNG bảo toàn trạng thái stopword của FULLTEXT ⇒ bench báo SÓT GIẢ

Sự cố thật 2026-09-07, suýt loại nhầm một bản vá đúng. Gate đối chiếu số dòng cũ/mới báo đường
nhanh **sót 1.088–2.744 dòng** trên "Nguyễn"/"Thị"/"Hải"/"0912" — đúng chiều nguy hiểm nhất
(nhân viên tìm không ra khách có thật).

Nguyên nhân nằm ở **bench**, không ở bản vá: migration dựng `ft_customer_ngram` với
`SET SESSION innodb_ft_enable_stopword = OFF`, nhưng `mysqldump` chỉ chép lại *định nghĩa* chỉ mục.
Nạp schema vào DB bench ⇒ index được dựng lại dưới `@@GLOBAL.innodb_ft_enable_stopword = 1`
(mặc định) ⇒ token phổ biến bị loại ⇒ `MATCH` trả thiếu.

⚠ **Cách dựng lại đúng KHÔNG phải là drop mỗi cái index đó** — cấu hình stopword gắn theo **BẢNG**,
cố định bởi chỉ mục FULLTEXT ĐẦU TIÊN. Còn bất kỳ FULLTEXT nào khác trên bảng thì index mới vẫn KẾ
THỪA stopword BẬT dù `SET SESSION` đã có hiệu lực. Quy trình đầy đủ + số đo:
`patterns-gotchas.md` §"Chỉ mục ngram dựng sai stopword → tìm kiếm SAI ÂM THẦM". Snippet dưới đây
chỉ đúng vì `customer` có ĐÚNG MỘT fulltext.

Đối chứng phân biệt bench-hỏng với bản-vá-hỏng, chạy trước khi kết luận bất cứ điều gì:

```sql
-- trên DB THẬT (chỉ đọc) và trên DB bench, cùng từ khoá — hai số phải bằng nhau ở CẢ HAI
SELECT (SELECT COUNT(*) FROM customer WHERE fullname LIKE '%Nguyễn%' OR code LIKE '%Nguyễn%' OR phone LIKE '%Nguyễn%'),
       (SELECT COUNT(*) FROM customer WHERE MATCH(search_ngram) AGAINST ('"Nguyễn"' IN BOOLEAN MODE)
          AND (fullname LIKE '%Nguyễn%' OR code LIKE '%Nguyễn%' OR phone LIKE '%Nguyễn%'));
```

Thật: `crm_dev_db` cho 6195/6195 · 6993/6993 · 998/998 (khớp tuyệt đối) trong khi bench cho
6195/**6059** · 6993/**6650** · 998/**963**. Dựng lại index trong bench mới đúng:

```sql
-- ⚠ Bảng có >1 FULLTEXT thì phải DROP HẾT trước, không chỉ cái này — xem patterns-gotchas.md.
SET SESSION innodb_ft_enable_stopword = OFF;   -- phải CÙNG kết nối với ALTER; SET GLOBAL KHÔNG ăn
ALTER TABLE customer DROP INDEX ft_customer_ngram;
ALTER TABLE customer ADD FULLTEXT INDEX ft_customer_ngram (search_ngram) WITH PARSER ngram;
```

Quy tắc rút ra: **bench có FULLTEXT thì bước clone chưa xong khi mới `mysqldump` + nạp** — phải
dựng lại mọi FT index dưới đúng session var, rồi đối chứng `MATCH` = `LIKE` trên DB thật TRƯỚC khi
tin bất kỳ số đối chiếu nào.

### ⚠ Cùng bẫy đó đánh vào DB THẬT, không riêng bench

Ở DB-per-customer, mọi đường sinh DB mới đều đi qua dump: onboard khách mới, phục hồi backup,
clone sang staging. Chỉ mục ngram của DB đó **đang hỏng trong khi `schema_migrations` sạch hoàn
toàn** — không có gì báo. Đo lại 2026-09-07 trên clone `crm_dev_db` (76.999 khách) ngay sau khi
nạp dump, so với chính `crm_dev_db` (khớp tuyệt đối):

| từ khoá | `LIKE` | `MATCH` trên DB từ dump |
|---|---|---|
| `Thị` | 7.566 | 7.122 |
| `Văn` | 1.905 | 1.751 |
| **`Nam`** | **125** | **0** ← khách tên Nam biến mất hoàn toàn |

⛔ **Chạy lại migration KHÔNG sửa được.** Guard `@done` của các migration ngram chỉ soi CẤU TRÚC
(có cột sinh + có FT index 1 cột trên nó); DB từ dump thoả cả hai nên file thành no-op. Giữ vậy là
CỐ Ý — nới `@done` ra sẽ dựng lại bảng ở MỌI lần migrate. Sửa bằng 3 câu chạy tay ở snippet trên
(cùng MỘT kết nối), đo 3,8s trên 76.999 khách.

Kiểm sau MỌI lần restore/onboard: so `MATCH('"Nam"')` với `LIKE '%Nam%'` — lệch là hỏng.

## `ORDER BY` bảng gốc + JOIN làm mất early-exit của `LIMIT`

Scope tenant hoặc bộ lọc nằm ở **bảng JOIN** (bảng gốc không có `company_id`) ⇒ optimizer drive từ
bảng đó ⇒ `ORDER BY <bảng gốc>.id` buộc materialize TRỌN kết quả join rồi mới sort. Không `ORDER BY`
thì nested loop dừng sau đủ `LIMIT` dòng; thêm vào là mất hẳn lối thoát đó.

```
KHÔNG ORDER BY: -> Limit: 20 row(s)
CÓ ORDER BY:    -> Sort: stock_entry_logs.id DESC, limit input to 20 row(s) per chunk
                    -> Stream results (rows=266948)   <- dựng trọn 266k dòng
```

Đo trên 524k `stock_entry_logs` + 524k `stock_entry` (`EXPLAIN ANALYZE`, `actual time`):

| | không lọc | lọc `warehouse_id` | keyword hiếm (0 hit) |
|---|---|---|---|
| JOIN, KHÔNG `ORDER BY` (thứ tự không xác định) | 107ms | – | – |
| JOIN + `ORDER BY` | 5453ms | 5390ms | 1925ms |
| `EXISTS` thay JOIN + `ORDER BY` — **LOẠI** | **5576ms** | – | – |
| `STRAIGHT_JOIN` + `ORDER BY` | 150ms | 52ms | **7826ms** |
| Thêm `company_id` vào bảng log ⇒ scope bảng gốc | **0.04ms** | **8387ms** | 1307ms |

Không phương án nào thắng mọi ca ⇒ nêu bảng này cho user quyết, đừng tự chọn. `EXISTS` là cách duy
nhất không đụng schema và nó KHÔNG cứu được. `STRAIGHT_JOIN` thắng ca thường nhưng sập ở từ khoá
hiếm (quét cạn bảng gốc tìm 20 dòng không bao giờ đủ).

⚠ Thêm `company_id` vào bảng log phải sửa MỌI đường INSERT — sót một chỗ ⇒ dòng `company_id` NULL ⇒
theo Hard Rule NULL-tenant scope, dòng đó hiện ra cho MỌI công ty.

⚠ Bẫy này kích hoạt ngay sau khi áp cách chữa ở §"Bảng được JOIN không bị scope `company_id`" —
cách đó cố ý biến bảng JOIN thành bảng dẫn, nên thêm `ORDER BY <bảng gốc>.id` vào sau là mất
early-exit. Đổi thứ tự bảng dẫn và thêm tie-breaker phải đo CÙNG nhau, không tách rời.

## Đo trên máy đang có phiên khác chạy bench

2026-09-07: 5 phiên Claude Code cùng bench song song (`crm_bench_scale2_tmp`,
`cluvix_logs_bench_tmp`, `cluvix_checkup2_tmp`…), load average vọt 17–21, cùng một câu cho
1.324ms lần này và 5.455ms lần sau. `SHOW PROCESSLIST` trước khi đo để biết mình đang đo cái gì;
lấy **min-of-5** thay vì trung bình; và mọi kết luận phải có `EXPLAIN` đỡ lưng — cơ chế
(`Covering index range scan` vs `Index lookup` khớp toàn bảng) không đổi theo nhiễu, con số thì có.

Phiên khác không chỉ gây nhiễu — nó **xoá DB bench của mình**. Đặt tên DB theo endpoint
(`cluvix_<endpoint>_scale_tmp`); `*_scale_tmp` trần bị phiên khác dùng trùng rồi DROP sạch bảng
giữa chừng. Hỏng theo kiểu im lặng: bảng biến mất nhưng phép đo vẫn ra số đẹp vì query trả 0 dòng.
⇒ Mọi ca "0 dòng" (từ khoá KHÔNG khớp, filter rỗng) phải có dòng đối chứng chứng minh data CÓ
khớp trước khi tin số đo — dữ liệu sinh máy móc cũng tạo ca này: id tương quan số học làm 718 khách
bench chỉ rơi vào 7/14 họ, nên từ khoá tên "có thật" vẫn khớp 0 dòng.

## Quy trình đo bắt buộc

1. Đọc `run.log` — tách phần SQL khỏi phần Go trước khi đoán.
2. Clone schema sang DB dùng-một-lần (`mysqldump --no-data --set-gtid-purged=OFF`, tên kết thúc
   `_test`/`_tmp`), sinh
   **≥500k dòng** bằng bảng số nhân đôi (`INSERT INTO nums SELECT n + @m FROM nums`).
   ⚠ `nums` phải là bảng **THƯỜNG**, KHÔNG `TEMPORARY`: MySQL không cho mở lại một
   `TEMPORARY TABLE` hai lần trong cùng một câu (`Error 1137 Can't reopen table`), mà
   `INSERT ... SELECT` từ chính nó thì cần đúng vậy.
   ⚠ Thiếu `--set-gtid-purged=OFF` thì nạp dump **chết ngay** ở dòng `SET @@GLOBAL.GTID_PURGED`:
   `ERROR 3546 (HY000): @@GLOBAL.GTID_PURGED cannot be changed` — lỗi không liên quan gì tới dữ
   liệu, rất dễ tưởng hỏng schema.
3. Đo **5 ca**, không chỉ ca đẹp: filter hẹp · không filter · phân trang sâu (`OFFSET` lớn) ·
   từ khoá CÓ khớp · từ khoá KHÔNG khớp.
3b. Endpoint có scope chi nhánh ⇒ đo CẢ HAI: `branchActiveId` = 1 chi nhánh **và** = 0 (thấy toàn
   bộ). Ca toàn-bộ làm bộ lọc gần như no-op ⇒ optimizer đổi hẳn kế hoạch ⇒ nhanh gấp ~50 lần và che
   sạch nút thắt (`tooth_bio` 540k: 1 chi nhánh 4155ms · toàn bộ 82ms). Người test thường là admin
   toàn quyền, nên đo đúng ca đó rồi kết luận "đã nhanh" là bỏ lọt.
4. Luôn có dòng đối chứng `SELECT 1` để trừ overhead kết nối.
5. Chạy mỗi biến thể ≥2 lần — lần đầu là cache lạnh, đừng kết luận trên nó.
5b. Lấy thời gian bằng `EXPLAIN ANALYZE` (`actual time=`), KHÔNG bằng wall-clock quanh `mysql`:
   client trả **0ms** trong khi plan có `Sort` qua 266k dòng — số thật là 5453ms.
5c. Seed phải THOẢ predicate của query: để cột bị lọc ở mặc định (0/NULL) ⇒ `LIMIT` không bao giờ
   đủ ⇒ quét cạn bảng ⇒ báo chậm giả (`stock_material` "1400ms" biến mất khi seed đúng: 30ms).
5d. KHÔNG nhồi bảng vượt quan hệ cardinality thật — bảng config 1:1 với danh mục thì trần là số
   dòng danh mục; nhồi lên 524k đẻ ra nút thắt không tồn tại (`med_medicine_config` "620ms" → 0ms).
5e. Máy có phiên/agent khác cùng bench (`uptime` load > ~4) ⇒ số tuyệt đối trước/sau VÔ NGHĨA
   (cùng một COUNT đo được 1,9s · 27,8s · 175s trong một buổi). Đo **cặp A/B xen kẽ trong cùng
   vòng lặp**: `IGNORE INDEX (<tên index>)` = đường cũ, không hint = đường mới, lấy median 3.
5f. Đo bản vá CODE cũng vậy: chạy đường cũ và đường mới **trong cùng một tiến trình**, xen kẽ
   từng từ khoá — không chạy 2 lượt cách nhau vài phút rồi trừ.
6. Trước khi ship: đối chiếu **số dòng trả về** của cách cũ và cách mới trên cùng bộ từ khoá.
7. Drop DB bench.
8. **Endpoint sau cache Redis / cần auth**: đo bằng spike `//go:build spike` gọi **thẳng handler** với gin
   context dựng sẵn (`gin.CreateTestContext` + set `branchActiveId`/`branchIds`)
   và `Deps{Cache: nil}` — `Cache: nil` ép mọi lượt đi đường **cache MISS**, khỏi dựng server hay đăng nhập.
   Lấy SQL **thật** từ chính spike đó (`logger.Info`), KHÔNG tự chép lại từ handler: bản chép tay sót một
   predicate là đo nhầm câu và ship nhầm chỉ mục.

Số tham chiếu khác đo cùng lúc, để biết ngưỡng nào đáng lo:

| | 524k dòng |
|---|---|
| Trang 1 CÓ lọc theo ngày (index range scan) | ~6ms |
| Trang 1 KHÔNG lọc | ~0ms (index phục vụ `ORDER BY`) |
| Trang 1000 (`OFFSET 14985`) | 989–1775ms |
| `appointment`: index 231MB trên data 48MB (32 index) | gấp 4.8 lần |

---

## `GROUP BY <bảng>.id` vô điều kiện — no-op ngữ nghĩa nhưng trả tiền thật trong `COUNT`

Khác mục [Query GROUP BY không phân trang](#query-group-by-không-phân-trang-không-miễn-nhiễm) ở
trên (mục đó nói về endpoint KHÔNG dùng `Paginate`). Đây là mẫu ngược lại: handler list gắn
`q.Group("<bảng>.id")` vô điều kiện rồi đi qua `pagination.Paginate`.

GROUP BY trên chính KHOÁ CHÍNH không gộp được dòng nào — nó chỉ ép MySQL **vật chất hoá** toàn bộ
tập kết quả vào bảng tạm rồi mới đếm, và tập đó gồm MỌI cột đã select, kể cả `TEXT`/`JSON`.

Đo trên `supplier_labo_order` 524.288 dòng / scope 30.841 dòng (2026-09-07), đơn vị = **số dòng
InnoDB thực đọc** (xem mục kế tiếp):

| | có `GROUP BY` | bỏ |
|---|---|---|
| `COUNT` không lọc | 61.684 + 1 bảng tạm | **30.842**, 0 bảng tạm |
| `COUNT` lọc 1 ngày | 30.886 + 1 bảng tạm | **30.842**, 0 bảng tạm |
| `COUNT` có từ khoá | 63.260 + 1 bảng tạm | **61.683**, 0 bảng tạm |

**Điều kiện bỏ được** (phải tự kiểm, KHÔNG mặc định đúng): mọi join của **cả list lẫn export** bám
KHOÁ CHÍNH/UNIQUE của bảng được join ⇒ không join nào nhân được dòng. Cạm bẫy hay gặp:
`user_profile` join bằng `user_id` — trông như cột thường nhưng `user_id` CHÍNH LÀ `PRIMARY KEY`
của bảng đó, nên vẫn an toàn. Kiểm bằng SQL thay vì đọc lướt:

```sql
SELECT COUNT(*) FROM (SELECT user_id FROM user_profile GROUP BY user_id HAVING COUNT(*)>1) x;  -- 0 = unique
```

**Đối chiếu phải tự chứng minh có hiệu lực.** So số dòng có/không `GROUP BY` trên ≥6 bộ lọc là
chưa đủ — nếu phép đo hỏng thì mọi cặp đều "khớp". Chạy thêm MỘT mẫu cố ý nhân dòng; không thấy
lệch nghĩa là phép đo vô giá trị:

```sql
-- join cố ý nhân dòng: có GROUP BY = 30.841 · bỏ = 185.046 ⇒ phép đo CÓ hiệu lực
LEFT JOIN supplier_labo_order dup ON dup.branch_id = supplier_labo_order.branch_id AND dup.id < 100
```

⚠ Bỏ `GROUP BY` là đặt một **giả định vào tương lai**. Người thêm join mới bám cột KHÔNG unique
sẽ làm list trả trùng IM LẶNG. Phải để lại cảnh báo ngay tại chỗ đã bỏ.

Còn **7 handler dính đúng mẫu này** (quét 2026-09-07: `.Group("<bảng>.id")` + `pagination.Paginate`),
chưa ai kiểm điều kiện join: `sale/opportunity/handler/re_exploitation.go` ·
`loyalty/broker/handler/handler.go` · `customer-care/review/handler/handler.go` ·
`loyalty/coupon_used/handler/handler.go` · `accountant/treatment_debts/handler/handler.go` ·
`hr/user/handler/handler.go` · `marketing/convert/handler/handler.go`.

## Đo bằng SỐ DÒNG InnoDB đọc khi máy quá nhiễu để tin thời gian

Bổ sung cho bước 5b (`EXPLAIN ANALYZE`) ở [Quy trình đo](#quy-trình-đo-bắt-buộc): `actual time=`
loại được overhead client nhưng VẪN là wall-clock bên trong server, nên vẫn nhảy theo tải máy.
Khi nhiều phiên cùng bench (đã gặp: load average 23, cùng một câu ra `min=726ms max=31.839ms`),
dùng đại lượng **bất biến cấu trúc**: số dòng InnoDB thực sự đọc.

```sql
FLUSH STATUS;
<câu cần đo>
SELECT SUM(VARIABLE_VALUE) FROM performance_schema.session_status
 WHERE VARIABLE_NAME IN ('Handler_read_next','Handler_read_rnd_next','Handler_read_key',
                         'Handler_read_first','Handler_read_prev');
-- kèm Created_tmp_tables / Created_tmp_disk_tables để thấy bảng tạm
```

**Bắt buộc hiệu chuẩn trước khi tin**: `SELECT COUNT(*)` toàn bảng phải ra ≈ đúng số dòng của bảng
(đo được 524.290 trên bảng 524.288), và `SELECT 1` ra ≈0. Không có hai mốc này thì con số vô nghĩa.

Đại lượng này đọc thẳng ra cơ chế mà ms không nói được: `COUNT` không lọc đọc **616.807 dòng trên
bảng 524.288 dòng** ⇒ biết ngay optimizer đang index-merge intersect chứ không phải quét thường.
Nó cũng bất biến giữa các lần chạy nên A/B một chỉ mục chỉ cần chạy 1 lần mỗi bên.

⚠ KHÔNG dùng để đo `MATCH ... AGAINST`: truy vấn FULLTEXT không đi qua `Handler_read_*` nên trả
về 0 và trông như miễn phí. Ca đó phải quay lại `EXPLAIN ANALYZE`.

## ⛔ Export `?_export=1` — vòng batch OFFSET (ĐÃ VÁ 2026-09-08) + trần RAM của nhánh in-memory

> **Trạng thái: ĐÃ VÁ.** `pkg/export.Export` nay đọc bằng MỘT con trỏ (`Rows()` + `ScanRows`), bỏ hẳn
> vòng `LIMIT/OFFSET`. Mục này giữ số ĐO TRƯỚC vá vì nó giải thích *vì sao* phải sửa và là mốc đối
> chiếu; số SAU vá ở mục con "Sau bản vá" bên dưới.

Đo 2026-09-07 trên `order_service_used` 524.288 dòng (scope company=1 ⇒ **262.144 dòng ra file**),
query thật của `ExportOrderServiceUsed` (15 `LEFT JOIN`). Đơn vị = **số dòng InnoDB đọc**
(hiệu chuẩn đạt: `SELECT 1`=16 · `COUNT(*)` toàn bảng=524.292 trên bảng 524.288).

`pkg/export.Export` KHÔNG stream. Nó chạy `COUNT` rồi lặp `fetchBatch` (`pkg/export/export.go:268`):

```go
Raw("SELECT * FROM (?) AS batch_q LIMIT ? OFFSET ?", query, batchSize, offset)  // batchSize=5000
```

Query gốc bị **bọc lại và chạy lại từ đầu cho MỖI batch**. Chi phí 1 batch tuyến tính theo OFFSET
(~15,4 dòng đọc thêm cho mỗi dòng bị bỏ qua) ⇒ tổng là **bậc hai theo số dòng**:

| OFFSET của batch | dòng InnoDB đọc |
|---|---|
| 0 | 80.006 |
| 25.000 | 475.006 |
| 50.000 | 850.006 |
| 130.000 | 2.075.006 |
| 200.000 | 3.150.006 |
| 255.000 (batch cuối) | 4.000.006 |

**Batch CUỐI một mình đắt ngang việc đọc TRỌN kết quả một lần** (`EXPLAIN ANALYZE`: 3.607/3.615ms
so với 3.676/3.650ms cho cả 262.124 dòng). Mà có 53 batch.

| Cả lượt export 262.144 dòng | dòng InnoDB đọc | wall |
|---|---|---|
| **Đường hiện tại** (1 COUNT + 53 batch) | **114.164.534** | 94,5s |
| 1 lần quét duy nhất (streaming) | 4.032.165 | 6,7s |
| riêng câu `COUNT` của `Export()` | 4.032.163 | — |

⇒ **gấp 28,3 lần**. Công thức đo được, `B = ⌈N/5000⌉`:
`tổng ≈ 80.006·B + 77.000·B(B−1)/2`. Nên tỷ lệ còn xấu đi theo N: 10k dòng ⇒ 237k đọc (không đau);
50k ⇒ 4,3M; 262k ⇒ 110M; **500k ⇒ 389M, tức ~50 lần** một lần quét.

### Nguy hiểm hơn tốc độ: export BỎ SÓT dòng, im lặng

Không hàm export khối lượng lớn nào đặt `ORDER BY`, và export bỏ qua `pagination.Paginate`
(`buildTreatmentServiceQuery` — `crm/treatment_service/handler/treatment_service.go:45` — không có
`Order(`). `LIMIT/OFFSET` không thứ tự xác định ⇒ cửa sổ trượt khi tập kết quả đổi giữa chừng.

- Bảng **tĩnh**: 262.144 dòng, 0 trùng, 0 thiếu. Đường này KHÔNG tự lộ ra khi test tay.
- Có **ghi đồng thời** (20 dòng bị `isLock=0` rải rác trong 94s export — đúng nhịp một phòng khám
  đang chạy): file ra 262.124 dòng, không trùng, nhưng **18 dòng ĐANG hợp lệ vắng mặt hoàn toàn**.

Đã chứng minh 18 dòng đó không phải dòng bị sửa: tập id thiếu
`20006, 80008, 120010, 150012, …, 390040` **giao = 0** với tập writer đụng vào
`422, 844, …, 8440`. Mỗi dòng rời scope làm trượt thêm đúng 1 dòng — đuôi id tăng đều 06→08→10→12,
mỗi lần sót rơi đúng ở biên batch. Không có lỗi, không có cảnh báo; kế toán đối soát mới phát hiện.

### ⚠ Keyset KHÔNG phải bản vá — đã đo và LOẠI

| Cách | Số đo | Vì sao loại |
|---|---|---|
| `WHERE id > X ORDER BY osu.id LIMIT 5000` | batch đầu **4.031.847 dòng đọc**, 4.361ms | `EXPLAIN`: `Sort: order_service_used.id, limit input to 5000` — stream trọn 262.124 dòng qua join rồi mới sort. Mất early-exit, đúng mục [`ORDER BY` bảng gốc + JOIN](#order-by-bảng-gốc--join-làm-mất-early-exit-của-limit). Chi phí giảm dần theo X (4,03M→114k) = quét hết phần còn lại mỗi lần. |
| Giữ OFFSET, chỉ tăng `BatchSize` | không đo | Giảm số batch nhưng không đổi bậc hai; đồng thời phá mục đích chặn bộ nhớ của batch. |

Hướng còn lại **chưa đo**: một lần quét duy nhất bằng con trỏ (`Rows()`/`FindInBatches` trên CÙNG
một câu), vừa xoá bậc hai vừa cho thứ tự nhất quán trong một ảnh chụp. Mốc so sánh đã có sẵn ở
bảng trên: 4.032.165 dòng đọc / 6,7s.

### Sau bản vá — số đo 2026-09-08 (bench dựng lại từ schema thật, đã `ANALYZE TABLE`)

⚠ **`ANALYZE TABLE` là bước BẮT BUỘC của công thức bench, không phải tuỳ chọn.** `Handler_read_*`
tất định theo **KẾ HOẠCH THỰC THI**, mà kế hoạch phụ thuộc thống kê bảng: cùng dữ liệu, thiếu
`ANALYZE` đo ra 7.681.849 — chạy `ANALYZE` xong ra **8.064.326**, khớp tuyệt đối lần đo trước.

**Cơ chế** (query hình dạng đại diện, `order_service_used` 524.288 dòng):

| | Dòng InnoDB đọc | |
|---|---|---|
| Trước vá (1 COUNT + 53 batch) | 114.164.534 | |
| **Sau vá** (1 COUNT + 1 con trỏ) | **8.064.326** | **−92,9%** |
| Dòng hợp lệ bị bỏ sót khi có ghi đồng thời | 18 → **0** | |

**Query THẬT của 3 export nặng nhất** — bench ở đúng số dòng của khách (`note` 842.501 ·
`treatment_course` 393.848 · `treatment_course_technical` 661.947):

| Export | Dòng ra file | Trước vá (suy từ đo) | **Sau vá (đo)** | Lần |
|---|---|---|---|---|
| `ExportNote` — 3 join, `ORDER BY note.id DESC` | 421.250 | ~108.683.000 | **2.527.927** · 4,5s | **43×** |
| `ExportTreatmentCourse` — 14 join, `ORDER BY created_time DESC` | 196.924 | ~105.008.000 | **5.514.493** · 6,5s | **19×** |
| `ExportTreatmentTechnical` — 8 join, `ORDER BY confirm_time DESC` | 330.973 | ~180.054.000 | **5.295.576** · 5,7s | **34×** |

⇒ Cả 3 nay ở mức **4,5–6,5 giây, 0 bảng tạm**. Không endpoint nào trong nhóm này còn là nút thắt.

#### ⚠ `ORDER BY` làm cơ chế CŨ tệ theo kiểu KHÁC — cần biết để đọc đúng số cũ

Query **không** `ORDER BY` (vd `ExportOrderServiceUsed`): chi phí batch **tăng tuyến tính theo
OFFSET** (80.006 → 4.000.006). Query **có** `ORDER BY`: derived table phải vật chất hoá + sắp TRỌN
tập kết quả cho **mỗi** batch ⇒ chi phí **bằng phẳng ở mức một-lần-quét-đầy-đủ**:

| | OFFSET 0 | OFFSET giữa |
|---|---|---|
| `ExportNote` | 1.263.770 | 1.263.756 |
| `ExportTreatmentCourse` | 2.562.286 | 2.560.038 |
| `ExportTreatmentTechnical` | 2.647.907 | 2.647.790 |

Hai hình dạng, cùng kết cục: tổng ≈ **số batch × một lần quét**. Đừng dùng "chi phí batch không tăng
theo OFFSET" làm bằng chứng rằng query đó không sao.

#### ⚠ `ORDER BY` cột KHÔNG phải khoá chính vẫn có filesort — chỉ là chưa tràn đĩa

`treatment_course` **không có** chỉ mục trên `created_time` ⇒ `EXPLAIN ANALYZE` cho
`Sort: treatment_course.created_time DESC (rows=196924, actual 434..440ms)`. Nhưng
`Sort_merge_passes = 0` ⇒ sắp gọn trong sort buffer, **không** tràn đĩa ở quy mô hiện tại.
⚠ `Created_tmp_tables = 0` **KHÔNG** chứng minh không có filesort — filesort dùng sort buffer chứ
không dùng bảng tạm. Có sort hay không thì đọc `EXPLAIN`; có tràn đĩa không thì đọc
`Sort_merge_passes`.

### ⛔ Nhánh dựng-file-TRONG-BỘ-NHỚ: ~7,5 KB RAM mỗi dòng, KHÔNG có trần

20 hàm `Export*` **không** đi qua `pkg/export.Export` mà tự `excelize.NewFile()` + `SetCellValue`
từng ô + `SaveAs` ở cuối ⇒ toàn bộ workbook nằm trong heap tới lúc ghi đĩa. Đo trực tiếp
(`pkg/export/excelize_mem_test.go`, chạy với `EXPORT_MEM_PROBE=1`), 12 cột:

| Số dòng | Heap đang giữ | Tổng đã cấp phát (áp lực GC) |
|---|---|---|
| 10.000 | 72 MB | 145 MB |
| 50.000 | 426 MB | 712 MB |
| 100.000 | **737 MB** | 1.416 MB |
| 200.000 | **1.473 MB** | 2.815 MB |

Tuyến tính, **~7,5 KB/dòng**.

**Vì sao đường `pkg/export.Export` an toàn mà đường này thì không:** `CSVThreshold` = 10.000 không
chỉ là lựa chọn định dạng — nó chính là **trần RAM**. Từ 10.000 dòng trở lên `Export` chuyển sang
CSV ghi thẳng ra file (RAM không đáng kể); Excel chỉ dùng dưới ngưỡng ⇒ trần ~72 MB. **20 hàm
in-memory KHÔNG có ngưỡng tương đương.**

**4 hàm CÒN SỐNG không có trần số dòng** (đã kiểm từng thân hàm):
`ExportStockEntry` · `ExportMedPurchaseOrder` · `ExportMedStockImport` · `ExportFundCount`.
⚠ `ExportStockEntry` đọc theo trang 2.000 nên phần ĐỌC có trần, nhưng nó ghi **mọi** dòng vào cùng
một workbook ⇒ RAM vẫn tỉ lệ tổng số dòng. Ở khách hiện tại 4 bảng này đều **0 dòng** (chưa bật tính
năng) — nhưng DB-per-customer: khách dùng kho/dược với 100k phiếu ⇒ **một request export ăn ~750 MB
heap**.

**2 hàm CÓ trần**: `ExportAppointmentByhourList` / `ExportAppointmentBydoctorList` —
`Limit(apptExportLimit)` = **2.000** ⇒ ~15 MB, an toàn.

⚠ **Trần này là CỐ Ý, đừng "sửa" nó.** Nhìn thoáng qua nó giống lỗi bỏ-sót-dòng (chạm trần → chỉ
`log.Printf(… may be truncated)` → vẫn giao file), nhưng bối cảnh khác hẳn: đây là **màn IN**, không
phải export dữ liệu. Có endpoint anh em `/api/print/appointment-byhour-list` →
`PrintAppointmentByhourList` với hằng `apptPrintLimit = 2000`
(`print/appointment/handler/handler.go:27`), **giống nhau từng chi tiết**: cùng lọc MỘT ngày, cùng
`ORDER BY branch_id, appointment_date`, cùng trần, cùng khuôn log. Bản Excel cố ý soi gương bản in
để bấm "In" và "Xuất Excel" ra cùng nội dung. 2.000 dòng in ra ≈ 40–60 trang A4 cho MỘT ngày.

Đo trên DB thật (399 ngày có dữ liệu): **đỉnh 1.064 lịch/ngày**, trung bình 501, **0 ngày ≥2.000**;
đỉnh theo tháng đi ngang 900–1.050 suốt 10 tháng ⇒ chưa từng chạm trần, còn ~2× dư địa.

⛔ Nếu có ngày thật sự cần nâng: nâng **CẢ HAI** hằng cùng lúc (`apptPrintLimit` + `apptExportLimit`),
nâng một bên làm Excel và bản in lệch nhau. Cũng KHÔNG chuyển 2 hàm này sang `StreamWriter` như 4
hàm trên: `buildApptExcel` dùng `MergeCell`+`SetCellStyle` mà StreamWriter không hỗ trợ, và trần ở
đây không phải lá chắn RAM nên chẳng có gì để đánh đổi.

📌 **Bài học chung**: cùng một khuôn code (`chạm trần → log → vẫn trả kết quả`) mang ý nghĩa NGƯỢC
nhau tuỳ nơi nó đứng — ở đường export dữ liệu là mất dữ liệu im lặng, ở đường bản in là guard hợp
lý. Trước khi xếp một khuôn là bug, phải trả lời "đầu ra này phục vụ ai". `git log` không trả lời
được (hằng số có từ commit đầu tiên, không comment) — **endpoint anh em và template FE thì trả lời
được**.

**3 hàm nhận `rows` dựng sẵn từ caller** (`ExportFundCashbook`, `ExportProfitLoss`,
`ExportVatReport`): rủi ro RAM nằm ở caller; cả 3 là báo cáo tổng hợp nên số dòng nhỏ.

### Phạm vi ảnh hưởng — triage tĩnh 100 hàm `Export*` (2026-09-07)

**Ngưỡng quyết định: `BatchSize` = 5000.** `for offset < total` ⇒ tập kết quả **≤ 5000 dòng chạy
đúng 1 batch, OFFSET 0** ⇒ miễn nhiễm CẢ HAI lỗi (không có batch thứ 2 để trượt, không có OFFSET để
nở). Vẫn trả 2 lần quét (COUNT + 1 batch) nhưng đó là hằng số, không phải bậc hai.

| Nhóm | Số hàm | Rủi ro |
|---|---|---|
| Đi qua **vòng batch** `pkg/export.Export` | 80 (75 sống + 5 chết) | N² **và** bỏ sót dòng — nếu >5000 dòng |
| Dựng thẳng trong **bộ nhớ** (`excelize.NewFile()` + `.Find(&all)`) | 20 | KHÔNG dính 2 lỗi trên (1 câu = ảnh chụp nhất quán). Đổi lại: **nạp trọn kết quả vào RAM**, không có trần |

Số dòng thật đo trên `crm_dev_db` (76.999 khách) để chấm khối lượng:

| bảng gốc | dòng | bảng gốc | dòng |
|---|---|---|---|
| `note` | 842.501 | `order_treatment_plan` | 74.585 |
| `treatment_course_technical` | 661.947 | `payment` | 46.563 |
| `treatment_course` | 393.848 | `treatment_course_doctor` | 45.509 |
| `appointment` | 218.071 | `customer_care` | 16.617 |
| `order_service_used` | 112.340 | `order_commission_directSale` | 13.398 |
| `customer` | 76.999 | `order_revenue_telesale` | 5.396 |

**Đau ngay ở khách hiện tại** (bảng gốc ≫ 5000, không gộp nhỏ) — ~40 hàm, gồm:
`ExportNote` · `ExportTreatmentTechnical` · `ExportWorkspaceTechnical` · `ExportTreatmentCourse` ·
`ExportTreatmentDoctor` · `ExportAppointment` (3 endpoint) · `ExportOrderServiceUsed(WithAppt)` ·
`ExportTreatmentDebts` / `SessionDebts` / `SessionPeriod` / `ProductSaleDebts` ·
`ExportCustomer` / `Lead` / `Convert` / `Debts` / `Birthday` / `ReferredCustomersList` ·
`ExportAppointmentDebts` (gộp theo `customer_id` ⇒ vẫn ~77k) · `ExportOrderPlan(Debts)` ·
**10 hàm `ExportPayment*`** · `ExportCustomerCare` · `ExportOrderCommission`.

**Miễn nhiễm THẬT — gộp về số nhân viên/dịch vụ, luôn ≤ vài trăm dòng** (đã đọc khoá `Group`):
`ExportDoctorCommission`/`AssistantCommission`/`TechnicalCommission`/`DirectSaleCommission`/
`DoctorConsultantCommission`/`TelesaleCommission` (gộp theo `*_id` nhân viên) ·
`ExportRevenue*Summary` (3 hàm) · `ExportTreatmentServices` (gộp `service_id`, 646 dịch vụ) ·
`ExportBroker` (gộp `broker.id`).
⚠ Gộp KHÔNG tự động là miễn nhiễm: khoá gộp phải nhỏ. `ExportAppointmentDebts` gộp theo
`customer_id` nên vẫn ~77k dòng.

**Miễn nhiễm vì bảng DANH MỤC có trần** (không phải vì đang ít dòng):
`ExportService` (646) · `ExportUsers` (323 nhân viên) · `ExportServiceType` (27) · `ExportUnit` (6) ·
`ExportTreatmentCourseType` (6) · `ExportServiceTreatmentStep` · `ExportMaterial` ·
`ExportMedMedicine` · `ExportServiceMaterial`.

⛔ **"0 dòng ở `crm_dev_db`" KHÔNG phải kết luận** — DB-per-customer, khách này chưa bật tính năng đó
thôi. Các bảng sau bản chất là **giao dịch, sẽ nở**, phải xếp vào nhóm rủi ro khi khách bật:
`supplier_labo_order` (`ExportLaboOrder` — chính bảng mà mục [`GROUP BY <bảng>.id`](#group-by-bảngid-vô-điều-kiện--no-op-ngữ-nghĩa-nhưng-trả-tiền-thật-trong-count) đã bench ở **524.288 dòng** trên khách khác) ·
`coupon_used_by_order_plan` (`ExportCouponUsed`) · `broker_commission` (`ExportBrokerCommission`) ·
`order_installment` (`ExportInstallmentDebts`) · `stock_usages` (`ExportStockUsages`) ·
`order_commission_doctor_consultant` (`ExportDoctorConsultantList`) ·
`treatment_course_assistant` (`ExportTreatmentAssistant`, `ExportWorkspaceTreatmentAssistant`).

⚠ **16 hàm `Export*` KHÔNG có call site nào** — 15 trong nhóm `med_*_report.go` dựng-trong-bộ-nhớ
(`ExportMedStockReport`, `ExportMedRevenueReport`, `ExportMedProfitReport`, `ExportMedTopSelling`,
`ExportMedLowStock`, `ExportMedExpiry`, `ExportMedControlledDrug`, `ExportMedBatchTracking`,
`ExportMedInventoryAudit`, `ExportMedSupplierReport`, `ExportMedByStaffReport`) + 5 đi qua vòng batch
(`ExportMedBookImport`, `ExportMedBookDispensed`, `ExportMedBookControlled`, `ExportMedReturnDestroy`,
`ExportStockMaterial`). Hỏi còn dùng không TRƯỚC khi tối ưu — hoặc trước khi coi màn báo cáo dược là
"đã có export".

### Dựng lại bench này: 3 cái bẫy đã tốn 1 buổi

1. `customer.search_ngram` là **STORED GENERATED + FULLTEXT ngram**; `order_treatment_plan.note`
   cũng có FT. Nạp mà không bỏ 2 chỉ mục đó ⇒ **~100 dòng/s** (524k dòng = 2,7 giờ). Bỏ đi:
   customer 100k + otp 150k trong **7 giây**, osu 524.288 trong **5,7 giây**.
   Query export không có `MATCH` nên bỏ FT không đổi kế hoạch đường đang đo.
2. `order_service_used` có 16 chỉ mục phụ + 8 FK. Bỏ trước khi nạp, `ADD INDEX` lại sau (8,8s) —
   schema lúc ĐO vẫn đúng 17 chỉ mục như bản thật.
3. **KHÔNG để script bench trong `backend/tmp/`** — đó là thư mục build của air, air dọn sạch nó
   giữa chừng (đã mất toàn bộ script + `run.log` một lần). Dùng scratchpad của phiên.

## Helper ngram dùng chung — đừng đẻ bản sao thứ N

`NgramMinLen` · `NgramPhrase` · `NgramPhraseSafe` nằm ở **`pkg/query/ngram.go`** (gộp 2026-09-07).
Trước đó mỗi màn tự copy một bản; trong đúng một buổi đã sinh ra **4 bản giống hệt nhau**
(`crm/appointment`, `crm/treatment_service`, `crm/labo_order`, `crm/customer_warranty_card`) vì hai
phiên làm song song không biết nhau. Màn mới ⇒ import, KHÔNG copy.

Ranh giới cố ý, đừng "dọn" nốt: **`keywordIDCap` GIỮ RIÊNG trong từng package**. Mỗi màn tra id
trên bộ bảng phụ khác nhau nên trần phải chỉnh lệch được. Chỉ những thứ là thuộc tính của MySQL
server mới lên `pkg/query`.

Test của helper (`pkg/query/ngram_test.go`) là hợp nhất 2 bộ ca đo thật trước đây — mọi ca đều kèm
số LIKE vs MATCH đo được, đừng thay bằng ca tự nghĩ.

## ⚠ Hai plan, mỗi plan một nhánh thảm hoạ — `get-payment-detail-for-customer` và bẫy "khách cũ đã ngừng giao dịch"

`GET /api/crm/payment/get-payment-detail-for-customer?customer_id=<id>`
(`crm/customer_detail/handler/customer_detail.go`) — cùng khuôn list-con với 4 mục xác nhận ở trên:
một bảng, không join, `WHERE payment.customer_id = ?`, `ORDER BY payment.id DESC`, 2 truy vấn
attach `IN (…)` chặn trên bởi kích thước trang.

**Ở phân bố THẬT không có nút thắt** (trần 29 payment/khách trên DB dev; trung bình 10,5 ở bench).
Nhưng khuôn này KHÔNG miễn nhiễm, và nó hỏng theo một cách khác hẳn ca đối chứng
`list-service-used-customer`: ở đây optimizer **luân phiên giữa hai plan, mỗi plan có một nhánh
thảm hoạ riêng**, và chỉ mục mới KHÔNG kéo được nó ra khỏi nhánh xấu.

Bench: schema thật (`mysqldump --no-data`) + dữ liệu thật nhân 11 lần, `customer_id` remap ngẫu
nhiên để giữ hình dạng phân bố ⇒ **607.046 `payment` / 74.585 `order_treatment_plan` /
112.340 `order_service_used`**, cộng 2 khách lệch cố ý. Đối chứng `SELECT 1` = 0,000167ms.

`payment` **KHÔNG có chỉ mục 1 cột `customer_id`** — chỉ có `idx_payment_customer_for
(customer_id, payment_for)`. Hậu tố PK ngầm của InnoDB làm nó thành `(customer_id, payment_for, id)`
⇒ **không phục vụ được `ORDER BY id DESC`**, luôn kèm `Sort:`.

| Khách | Số dòng | Plan optimizer tự chọn | SELECT trang 1 | Dòng InnoDB đọc |
|---|---|---|---|---|
| 35093 (thật) | 28 | `idx_payment_customer_for` + `Sort:` | 0,15ms | 38 |
| 777100 (id cũ) | 50 | `idx_payment_customer_for` + `Sort:` | 0,08ms | 50 |
| 777200 (id cũ) | 200 | `idx_payment_customer_for` + `Sort:` | 0,23ms | 200 |
| 777300 (id cũ) | 1.000 | `idx_payment_customer_for` + `Sort:` | 1,0ms | 1.000 |
| **777001 (id cũ)** | **5.000** | **`PRIMARY (reverse)`** | **217–418ms** | **602.066** |
| 51696 (id mới, dày) | 48.319 | `PRIMARY (reverse)` | 0,04ms | 20 |

**Điểm lật nằm giữa 1.000 và 5.000 dòng/khách.** Trên ngưỡng đó optimizer bỏ chỉ mục để quét ngược
PK — đúng khi số con của khách **dày ở dải id MỚI** (khách 51696: gặp đủ 20 dòng sau 20 lần đọc,
0,04ms), **sai thảm hoạ khi số con nằm ở dải id CŨ** (khách 777001: quét ngược **602.066/607.046
dòng**, tức gần trọn bảng, để lấy 20 dòng). Hình dạng dữ liệu gây ra nó rất đời thường: *khách gắn
bó lâu năm rồi ngừng giao dịch* — nhiều payment, tất cả đã cũ. Chi phí tăng **tuyến tính theo kích
thước bảng**, không theo số con của khách.

⇒ Bổ sung cho mẹo đọc `Sort:` ở mục `list-service-used-customer`: **`Sort:` biến mất chưa chắc là
tin mừng.** Phải đọc thêm `rows=` của node quét: `Index scan … PRIMARY (reverse) … rows=602066`
nghĩa là early-exit đã hỏng theo chiều ngược lại. Đọc cả hai: có `Sort:` ⇒ O(số con); không `Sort:`
nhưng `rows=` xấp xỉ kích thước bảng ⇒ O(bảng).

### Biến thể đã đo (giữ số để khỏi đo lại khi tới ngưỡng)

| Cách | SELECT @khách 5.000 dòng id cũ | Kết luận |
|---|---|---|
| Nguyên trạng | 217–418ms · 602.066 dòng | mốc |
| Thêm `KEY (customer_id, id DESC)`, để optimizer tự chọn | **418ms · 602.066 dòng** | **LOẠI.** Optimizer **vẫn chọn `PRIMARY (reverse)`**, chỉ mục mới nằm chơi. Đây là biến thể LOẠI vì lý do khác mọi mục trên: không phải "thắng giả", mà là **không thắng gì cả** — ship xong tốn thêm 1 chỉ mục trên bảng ghi ở mọi lần thu tiền, mà đường chậm không đổi một mili giây. |
| `KEY (customer_id, id DESC)` + `FORCE INDEX` | **0,080ms · 20 dòng** | Qua gate `FORCE INDEX`: ép dùng chính nó NHANH HƠN để-optimizer-tự-chọn (0,080ms vs 418ms) ⇒ phần thắng đúng là của chỉ mục. **Nhưng đòi hint trong code Go** — chưa ship, xem dưới. |
| `IGNORE INDEX (PRIMARY, …)` | 145ms · 93.750 dòng | LOẠI. Optimizer rơi sang `fk-payment-branch_id` quét ngược — vẫn O(bảng), chỉ đỡ hơn chút. |

Đối chiếu kết quả: 3 plan (tự chọn · `FORCE` chỉ mục mới · `FORCE idx_payment_customer_for`) trả
**cùng 20 `id`, cùng thứ tự** trên khách 777001 — chỉ mục không đổi ngữ nghĩa.

### Phần còn lại của endpoint — đã đo, KHÔNG có vấn đề

| Ca | Số đo |
|---|---|
| `COUNT` của `Paginate` — 38 / 1.000 / 5.000 / 48.319 dòng | 0,08ms / 1,0ms / 5,1ms / **51ms** |
| Phân trang sâu `OFFSET 48.000` | 66ms |
| Attach `order_treatment_plan` theo `id IN (≤20)` | 0,5–1,1ms |
| Attach `order_service_used` theo `id IN (≤20)` | 0,3–1,0ms |
| Khách không tồn tại | 0,01ms |

`COUNT` tuyến tính theo số con và **không chỉ mục nào cứu được** (phải đếm hết) — giống kết luận đã
có ở `reminder-of-customer`. Hai truy vấn attach chặn trên bởi kích thước trang, không phải N+1.

### ĐÃ SHIP 2026-09-09 — chỉ mục 1 cột + `FORCE INDEX` có cổng

Bench dựng lại: 605.319 payment / 76.999 khách; khách đối kháng = 5.000 payment id CŨ nhất
(id 68–5.172). Bộ đếm SESSION `Handler_read_*` (đối chứng `SELECT 1`=0, `COUNT(*)`=605.524):

| Ca | trước | chỉ mục (auto) | chỉ mục + FORCE |
|---|---|---|---|
| 1 chi nhánh, khách đối kháng | 600.677 | 5.938 | **358** |
| **toàn chi nhánh**, khách đối kháng | 600.339 | **600.339** | **20** |
| khách THƯỜNG (trần thật 29) | 30 | 30 | 20 |

**Chỉ mục một mình KHÔNG đủ** — đây là điểm phiên trước chưa đo tới: ở ca "toàn chi nhánh" (đúng
người xem là quản lý) optimizer vẫn bỏ chỉ mục mới và quay lại quét ngược `PRIMARY`. Thời gian:
582–4.921ms → **0,04–0,58ms**. Giá: +11 MB/605k dòng.

Giả thuyết đã đúng và nên nhớ: chỉ mục cần là **1 CỘT** `(customer_id)`, không phải
`(customer_id, id DESC)` mà phiên trước thử. Chỉ mục 1 cột nhận hậu tố PK ngầm ⇒ `(customer_id, id)`
⇒ đúng hình dạng đã cứu `warranty-card/of-customer` và `prescription-of-customer`. Chỉ mục 2 cột
sẵn có (`idx_payment_customer_for (customer_id, payment_for)`) hậu tố thành
`(customer_id, payment_for, id)` — vô dụng cho `ORDER BY id DESC`, và đó chính là lý do endpoint
này hỏng còn hai endpoint kia thì không.

⚠ **FORCE bị TẮT khi request có `appointment_id`**: nhánh đó `idx-payment-appointment_id` chọn lọc
hơn hẳn (20 dòng), FORCE ép bỏ nó ⇒ **4.072 dòng**. Bản thân việc THÊM chỉ mục cũng đưa nhánh này
20 → ~1.277 dòng (optimizer chuyển sang index-merge intersect; 0,05ms → ~0,9ms) — chấp nhận được,
và cột `payment.appointment_id` hiện NULL ở **100%** dòng nên nhánh này thực tế chưa ai chạm.

⚠ **Vì sao FORCE an toàn ở đây mà KHÔNG an toàn ở `note/get-all-note`**: predicate là ĐẲNG THỨC
trên `customer_id` nên phạm vi chỉ mục bị chặn trên bởi "số payment của MỘT khách" — đúng bằng ngữ
nghĩa endpoint, không tham số nào làm nó phình ra. Ở `note` thì phạm vi là ĐỘ RỘNG CỬA SỔ NGÀY,
người dùng kéo rộng được tuỳ ý ⇒ phải có cổng. Luật chung: chỉ FORCE khi phạm vi chỉ mục bị chặn
trên bởi CHÍNH ngữ nghĩa của endpoint.

Migration `20260909131146_payment_customer_index.up.sql` (idempotent, đã chạy thử 3 lượt).
Đối chiếu 5 ca khớp byte-for-byte, mutation-verified. 2 test cổng + tên chỉ mục khớp migration.

### `get-treatment-course-of-customer` — ĐO LẠI 2026-09-09, KHÔNG ship (kết luận âm tính có số)

Cùng bench (393.848 liệu trình, trần thật **185**/khách), dựng khách đối kháng 5.000 liệu trình
id cũ nhất:

| Ca | dòng InnoDB |
|---|---|
| khách thường (trần thật 185) | **90** |
| khách đối kháng 5.000 con | 5.021 |
| toàn chi nhánh, đối kháng | 5.020 |

**KHÔNG có bẫy quét-cạn-bảng** — khác hẳn `payment`. Lý do: endpoint sắp xếp theo
`created_time DESC, id DESC`, KHÔNG phải `id` đơn thuần, nên optimizer không có ván cược
early-exit trên `PRIMARY` để mà thua; nó dùng thẳng `fk-treatment_course-customer_id` (1 cột) rồi
`Sort`. Chi phí = **O(số con)**, đúng ngữ nghĩa endpoint. Endpoint đo được **4ms** end-to-end ở
phân bố thật.

⇒ Chỉ mục `(customer_id, created_time DESC)` mà phiên trước đo (54,7ms→0,45ms ở ca cực đoan)
**vẫn KHÔNG đáng ship**: nó chỉ bỏ được `Sort` của tối đa 185 dòng ở thực tế. Quyết định hoãn của
phiên trước nay có số đo riêng đỡ lưng — đóng mục này, đừng đo lại.

⇒ Cùng lập luận cho `list-service-used-customer` (trần thật **39** dòng/khách): chi phí O(số con),
chỉ mục đã đo 0,05ms ở ca cực đoan 50.000 dòng vẫn KHÔNG tới ngưỡng. Không dựng bench lại.

### CHƯA ship gì — và ngưỡng đáng quay lại

Trần thật hôm nay là **29 payment/khách**, cách điểm lật (1.000–5.000) hai bậc độ lớn; dưới ngưỡng
đó mọi ca đều **≤1ms**. `payment` đã mang 10 chỉ mục và bị ghi ở mọi lần thu tiền. Ship chỉ mục
thứ 11 để chữa ca chưa tồn tại đã là lỗ — mà ở đây còn tệ hơn: **chỉ mục KHÔNG tự được dùng**, nên
muốn có tác dụng phải nhét index hint vào code Go, tức trả giá bảo trì thật cho một ca giả định.

Ngưỡng đáng quay lại: **một khách vượt ~2.000 payment**. Lúc đó bản vá đã đo sẵn là
`KEY (customer_id, id DESC)` **kèm** hint ép dùng nó (0,080ms) — chỉ mục trần không đủ.

## Xác nhận lần 5 khuôn list-con — `get-treatment-course-of-customer` có `Sort:` nhưng VẪN không đau

Đo 2026-09-08. Endpoint này là ca **thứ hai** rơi đúng vào cảnh báo của mục
`list-service-used-customer` ở trên (`ORDER BY` KHÔNG được phục vụ bởi chỉ mục đã lọc fk ⇒ có
`Sort:` ⇒ `LIMIT` mất early-exit), nhưng kết luận lại là **không cần vá** — vì phân bố thật quá xa
ngưỡng. Ghi lại để lần sau thấy chữ `Sort:` thì đừng vá phản xạ: `Sort:` cho biết chi phí là
O(số con), CHƯA nói nó có đau hay không — số con thật mới nói.

`treatment_course` chỉ có `fk-treatment_course-customer_id` (1 cột), `ORDER BY` là
`treatment_course.created_time DESC` ⇒ mọi lần mở tab là một filesort trên TOÀN BỘ liệu trình của
khách đó.

**Phân bố THẬT (DB dev, 2026-09-08): 393.848 liệu trình / 39.435 khách — trung bình 9,99,
nhiều nhất 185 liệu trình/khách.**

Bench: `mysqldump --no-data` schema thật 6 bảng + nạp NGUYÊN dữ liệu dev rồi nhân đôi 2 lần
(`code` phải sinh lại vì UNIQUE `(company_id, code)`) ⇒ **1.625.712 dòng / 39.436 khách**, cộng một
khách lệch cố ý **50.320** liệu trình. Số dưới là `EXPLAIN ANALYZE` lượt 2 (warm); đối chứng
`SELECT 1` = 0,000125ms.

Endpoint chạy **BA lượt quét** trên cùng tập liệu trình của khách mỗi lần mở tab, không phải hai:

| Lượt | Khách 208 (≈trần thật) | Khách 740 (4× trần thật) | Khách 50.320 (lệch cố ý) |
|---|---|---|---|
| SELECT trang 1 (`LIMIT 20`) | 0,43ms | 1,15ms | **54,7ms** |
| COUNT của `Paginate` | 0,46ms | 1,61ms | **109ms** |
| `labelFilter` (raw, GROUP BY, **KHÔNG có LIMIT**) | 0,60ms | 1,99ms | **132ms** |
| **Tổng SQL chính/request** | **~1,5ms** | ~4,8ms | **~296ms** |
| Phân trang sâu `OFFSET 49.900` | — | — | 127ms |
| Khách không tồn tại | — | — | 0,005ms |

⇒ **Không có nút thắt ở phân bố thật, KHÔNG ship gì.** Ở trần 185 liệu trình/khách, cả ba lượt
cộng lại dưới 1,5ms.

Hai chi tiết cấu trúc đáng nhớ (đo được, không suy diễn):

- **`LEFT JOIN order_service_used osu_src` là vô điều kiện và nằm trong CẢ `COUNT`.** Đúng mục
  "MySQL KHÔNG loại-bỏ-left-join" ở đầu file: join theo khoá chính, không thể đổi số dòng, nhưng
  vẫn chạy đủ 50.320 lượt trong derived table của COUNT. Đo: COUNT **109ms có join → 52ms bỏ join**
  (~2×). Cùng chuyện với `labelFilter`: **132ms → 55,9ms**. Ở trần thật thì cả hai đều dưới 1ms nên
  chưa đáng đụng vào.
- **`labelFilter` là lượt quét thứ BA và là lượt đắt nhất** — nó `GROUP BY` toàn bộ liệu trình của
  khách, KHÔNG có `LIMIT`, nên không có đường thoát sớm nào kể cả khi thêm chỉ mục. Chỉ đo COUNT
  sẽ báo thiếu ~45% chi phí thật của endpoint này.

Biến thể đã đo, **giữ số để khỏi đo lại khi tới ngưỡng** (ngưỡng đáng quay lại: một khách vượt
**vài nghìn** liệu trình — hiện trần là 185):

| Cách | SELECT trang 1 @50.320 con | Kết luận |
|---|---|---|
| Nguyên trạng | 54,7ms (có `Sort:`) | mốc |
| `KEY (customer_id, created_time DESC)` — optimizer tự chọn | **0,449ms** | ĐỦ ĐIỀU KIỆN ship khi tới ngưỡng: `Sort:` biến mất, early-exit thật (`rows=20` ở Index lookup). |
| — cùng chỉ mục, **`FORCE INDEX` gate** | **0,326ms** | Qua gate: ép dùng chính nó KHÔNG chậm hơn để-optimizer-tự-chọn ⇒ phần thắng đúng là của chỉ mục, không phải cost model đổi plan. |
| — cùng chỉ mục, đo COUNT | 102ms (so với 109ms) | Chỉ mục **không cứu được COUNT** — nó phải đếm hết số con. Cũng không cứu `labelFilter`. |

⇒ Nếu tới ngưỡng phải vá: chỉ mục chữa được **1 trong 3 lượt** (54,7ms/296ms). Muốn chữa 2 lượt còn
lại thì phải bỏ join khỏi COUNT + `PaginateWithTotal`, hoặc đếm xấp xỉ — KHÔNG phải thêm chỉ mục
thứ 13 vào `treatment_course` (bảng đã mang 12 chỉ mục).

⚠ **Bẫy đo dính HAI lần trong phiên này** — cùng lớp "lệnh chạy nhưng đo nhầm ca": chọn
`branch_id` không khớp chi nhánh của khách test ⇒ `Index lookup` vẫn trả đủ 740 dòng nhưng
`Filter:` cho `rows=0`, và `EXPLAIN ANALYZE` vẫn in ra một con số ms trông hợp lệ (32ms). Đó là ca
"branch không khớp", KHÔNG phải ca thường. Luôn đọc `rows=` ở node **Filter** (không chỉ node
Index lookup) trước khi tin số; chọn khách test bằng
`GROUP BY customer_id WHERE branch_id=<b> AND company_id=<c>` chứ đừng chọn theo `customer_id` rồi
đoán branch.

⚠ Sắc thái riêng của endpoint này, khác `appointment/get-by-customer`: ca **branch lọc sạch** ở đây
lại NHANH (0,01ms) vì optimizer đổi sang `fk-treatment_course-branch_id`. Nhưng khi ĐÃ có chỉ mục
composite thì ca đó quay lại chậm (**51,6ms** — quét đủ 50.320 dòng để trả 0 dòng), đúng cảnh báo
"`LIMIT` mất early-exit khi vế tenant lọc sạch". Thêm chỉ mục ⇒ phải đo lại ca này, không chỉ ca đẹp.

Phần còn lại đã đo và KHÔNG có vấn đề: 6 truy vấn attach sau `Paginate` đều chặn trên bởi kích
thước trang (≤20 id) và đều có chỉ mục `treatment_course_id` — `treatment_course_technical`
(661.947 dòng) ra `Index range scan` = **0,019ms**. `treatment_course_assistant` và
`supplier_labo_order` hiện **0 dòng** trên DB dev nên optimizer chọn `Table scan` (miễn phí khi
bảng rỗng); cả hai đã có chỉ mục `treatment_course_id` sẵn nên khi có data sẽ tự chuyển sang range
scan — không phải nợ.

## Endpoint không-SQL: `list-folder` — trục scale là S3, và cái đáng lo là CẮT IM LẶNG

Đo 2026-09-08. `POST /api/crm/treatment-course/list-folder?idCustomer=<id>` là ca đầu tiên trong sổ
mà **MySQL gần như không tham gia**: toàn bộ chi phí nằm ở S3. Ghi lại vì khuôn "đo SELECT + COUNT"
của mọi mục trên KHÔNG áp được ở đây, và nếu đo theo thói quen sẽ kết luận "0,3ms, quá nhanh" rồi
bỏ sót thứ thật sự đáng sửa.

⚠ Là **POST** dù tên là `list-` và FE gọi `apiService.post(...)` — đừng tìm bằng `crm.GET(`.

Handler (`crm/customer_detail/handler/customer_detail.go`) làm đúng 3 việc:
1. Tra `customer` theo khoá chính lấy `code` → const table, cùng khuôn `customer/:id` (~0,1ms).
2. Gọi `h.S3.ListObjects("customer/<id>/")`.
3. Nếu `code != id` (gần như LUÔN đúng — code dạng `KH37140`), gọi **lần thứ hai**
   `ListObjects("customer/<code>/")` cho đường legacy, rồi merge khử trùng.

### Số đo (spike Go dùng ĐÚNG `storage.S3Client` của repo, 3 lượt)

| Ca | Lượt 1 (lạnh) | Lượt 2–3 (ấm) |
|---|---|---|
| Prefix chắc chắn RỖNG (đối chứng) | 159,4ms | 29,1–32,8ms |
| Khách 38574, đường id (0 mục) | 51,7ms | 32,1–32,2ms |
| Khách 38574, đường code legacy (0 mục) | 35,5ms | 31,1–31,3ms |
| Khách NẶNG NHẤT thật (5 folder) | 32,1ms | 31,8–32,9ms |

⇒ **~32ms mỗi lời gọi, HẰNG SỐ** — trả 0 mục hay 5 mục đều bằng nhau. Chi phí là round-trip mạng
tới `s3.example.com`, không phải khối lượng dữ liệu. Lượt 1 đắt gấp 5 vì TLS handshake.

⇒ **Mỗi lần mở tab ảnh tốn ~64ms** (2 lời gọi TUẦN TỰ) + ~0,1ms SQL. S3 chiếm **99,5%**, và con số
đó KHÔNG đổi khi dữ liệu lớn lên. Về mặt "có scale không": **có**, endpoint này miễn nhiễm với
kích thước dữ liệu.

### Phân bố thật (bucket `cluvix`, folder `develop`, 2026-09-08)

**206 object** trên TOÀN BỘ cây `develop/customer/`, 84 khách có dữ liệu, và thứ endpoint thật sự
trả về (mục MỨC 1, vì `Delimiter="/"`) nhiều nhất là **5 folder/khách**.

### ⛔ Cái đáng sửa không phải tốc độ — là CẮT IM LẶNG ở 1.000 mục

`storage.ListObjects` gọi `ListObjectsV2` **một lần**, KHÔNG set `MaxKeys` (⇒ mặc định 1.000) và
**KHÔNG bao giờ đọc `IsTruncated`**; `ListFolder` cũng không phân trang. Vượt 1.000 mục mức 1 ⇒
folder từ thứ 1.001 trở đi **biến mất khỏi UI, không lỗi, không dấu hiệu gì**.

Chứng minh read-only trên chính prefix có 5 folder (không ghi gì lên bucket):

| Cùng prefix, đổi `--max-keys` | Folder trả về | `IsTruncated` | `NextContinuationToken` |
|---|---|---|---|
| `--max-keys 100` | 5 | `False` | không |
| `--max-keys 2` | **2** | **`True`** | **có** |

⇒ Khi số mục vượt cap, S3 trả đủ cap rồi báo `IsTruncated=true` + token. Code bỏ qua cả hai.

Đối chứng nội bộ chứng minh đây là thiếu sót chứ không phải chủ đích: **`DeleteFolder` trong CÙNG
file `pkg/storage/s3.go` CÓ vòng lặp `IsTruncated`** (`deleteFolderPage` trả `bool` truncated).
Repo biết khuôn này; riêng `ListObjects` không làm.

**ĐÃ VÁ 2026-09-08** (trần thật lúc vá mới là 5/1.000 — vá sớm vì đây là lỗi ĐÚNG/SAI, không phải
lỗi chậm: khi chạm trần thì triệu chứng là ảnh biến mất chứ không phải màn hình lag, nên sẽ không ai
báo bug kịp). `ListObjects` và `ListContents` nay đều đi qua `mergeListPages` — vòng
`ContinuationToken` theo đúng khuôn `deleteFolderPage` — và trả về `*s3.ListObjectsV2Output` gộp,
nên **cả 5 điểm gọi được vá cùng lúc mà không phải sửa dòng nào ở handler**:
`customer_detail.ListFolder`, `treatment_course.CreateFolderTreatment`, `.ListFileTreatment`,
`.getImageList` (ListContents), và nhánh `source/`.

Nâng `MaxKeys` KHÔNG phải cách sửa — chỉ dời chỗ cắt.

Hai nhánh dừng bắt buộc phải có, vì đây là vòng lặp tự-lặp theo dữ liệu server trả về:
`IsTruncated=true` mà token nil/rỗng, và token lặp lại chính nó ⇒ dừng. Thiếu chúng thì một
implementation S3 lỗi làm request treo và giữ goroutine tới khi client bỏ cuộc.

Test neo: `pkg/storage/s3_list_paging_test.go` — `mergeListPages`/`parallelList` được tách thành hàm
thuần chính vì `S3Client.client` là `*s3.Client` (kiểu cụ thể, không mock được). Đã kiểm bằng
mutation: cấy `break` ngay đầu vòng (hành vi CŨ) ⇒ 3 test đỏ, trong đó
`TestMergeListPages_GomDuMoiTrang` chỉ thẳng "gọi 1 lần — vòng phân trang không chạy".

### Biến thể đã đo, CHƯA ship

| Cách | Số đo | Kết quả |
|---|---|---|
| Gọi 2 prefix SONG SONG (goroutine) thay vì tuần tự | tuần tự **70,6–117,6ms** (median ~77) → song song **35,4–41,6ms** (median ~37) qua 10 lượt ⇒ **2,1×** | **ĐÃ SHIP 2026-09-08** qua `S3Client.ListObjectsParallel`, dùng ở cả `ListFolder` và `ListFileTreatment`. |
| Bỏ hẳn lời gọi đường code legacy | 64ms → 32ms | **LOẠI** — không biết trước khách có dùng đường legacy hay không; bỏ đi là mất ảnh cũ im lặng, đúng thứ mục này đang cảnh báo. |
| Nâng `MaxKeys` lên 10.000 | — | **LOẠI** — chỉ dời chỗ cắt, vẫn im lặng. Phải phân trang thật. |

### ✅ KHÔNG phải lỗ cross-tenant — `S3_FOLDER` chính LÀ đoạn tenant (user xác nhận 2026-09-08)

Ghi lại vì đây là kết luận sai suýt đưa ra trong lúc đo, và người sau nhìn bucket sẽ nghi đúng chỗ đó.

Nhìn bucket thấy prefix dạng số trần (`370/`, `383/`, `523/`…) nằm cạnh prefix có tiền tố công ty
(`AAA.…`, `BB.…`, `CC.…`) rất dễ kết luận "đường id thiếu đoạn tenant ⇒ hai khách SaaS cùng
`customer.id = 370` sẽ dùng chung folder ảnh". **SAI.** `S3_FOLDER` là biến môi trường **riêng cho
từng SaaS** — `customer-a`, `customer-b`, … và `develop` chỉ là giá trị của môi trường dev. Mỗi thương hiệu
có cây prefix riêng, nên `customer.id` trùng nhau giữa các SaaS không gặp nhau bao giờ.

⇒ Bài học đo: `.env` của máy dev chỉ cho biết giá trị của MỘT deployment. Thấy một hằng số trông
"dùng chung cho mọi khách" thì phải hỏi nó có phải per-deployment không, TRƯỚC khi báo lỗ bảo mật.

### ⚠ Còn lại, cần người quyết

**Có prefix `<S3_FOLDER>/customer/null/` với 5 object** — chuỗi `"null"` lọt vào đường tạo folder ở
đâu đó. `list-folder` nay đã parse id ra số nên không tạo thêm được, nhưng dữ liệu cũ vẫn nằm đó.
Xoá là thao tác KHÔNG hoàn tác được trên bucket thật ⇒ để user quyết, đừng tự dọn.

## Endpoint đọc S3 vẫn có thể chậm vì SQL — `get-image-from-path` và cái giá của `WHERE id = ? OR code = ?`

`GET /api/crm/treatment-course/get-image-from-path?path=<s3 path>`
(`crm/treatment_course/handler/treatment_course.go`) nhìn như endpoint thuần I/O: không
`Paginate`, không join, thân hàm chỉ gọi `h.S3.ListObjects`. **Nút thắt lại nằm ở SQL** — đúng một
câu, trong hàm kiểm quyền sở hữu `verifyCustPathSegment`:

```go
q := h.DB.Table("customer").Where("customer.id = ? OR customer.code = ?", segment, segment)
```

`customer` **KHÔNG có chỉ mục trên `code`** (có `old_code` trên cột `old_code` — dễ nhìn nhầm là
đã có). Vế `OR` trên cột không chỉ mục làm PK **vô dụng**: optimizer bỏ luôn đường khoá chính và
rơi xuống quét nhánh `company_id` của `idx_customer_scope_status`.

Đây là endpoint đầu tiên trong sổ **đã chậm ở quy mô HÔM NAY**, không phải chậm ở ca giả định.

| Quy mô | segment = code thật | segment = id | segment không tồn tại |
|---|---|---|---|
| **76.999 khách (DB thật, TRƯỚC vá)** | **83–117ms** | 80–86ms | 84–114ms |
| **76.999 khách (DB thật, SAU vá — apply 2026-09-08)** | **0,036ms** | 0,070–0,117ms | 0,033ms |
| 615.992 khách (bench) | 1.429–1.468ms | **1.446–1.515ms** | 1.393–1.499ms |

Đối chứng `SELECT 1`: 0,000209ms (DB thật) · 0,000416ms (bench). Bench = schema thật
(`mysqldump --no-data`) + dữ liệu thật nhân 8 lần.

```
-> Aggregate: count(0)  (actual time=1404..1404 rows=1)
    -> Filter: ((customer.id = 536) or (customer.`code` = '536'))  (actual time=1404..1404 rows=0)
        -> Index lookup on customer using idx_customer_scope_status (company_id = 1)
           (cost=36472 rows=290183) (actual time=0.31..1372 rows=615992)
```

**615.992 dòng đọc để trả lời "khách này có tồn tại không".** Chi phí tuyến tính theo số khách của
công ty, KHÔNG theo số ảnh — nên nó không hiện ra khi nhìn endpoint như một đường S3.

`verifyCustPathSegment` có **3 call site**, đều trong `treatment_course/handler`: mở tab ảnh
(`GetImageFromPath`), xoá file (`DeleteFileTreatment`), xoá folder (`DeleteFolderTreatment`).
Vá một chỗ là vá cả ba.

> **ĐÃ APPLY lên DB dev 2026-09-08** (`migrations/20260908155444_customer_code_index.up.sql`).
> Đo lại trên chính DB thật sau apply: **83–117ms → 0,033–0,117ms**, đối chứng `SELECT 1` =
> 0,000083ms, plan đúng `index_merge union(PRIMARY, idx-customer-code)` như bench đã dự đoán.
>
> ⚠ Lúc apply gặp `schema_migrations` kẹt `dirty = 1` — thủ tục xử lý (và vì sao KHÔNG được
> `force` về previous theo phản xạ):
> `skills/writing-mysql-migrations/reference/migration-idempotent.md` §Auto-recovery dirty state.

### Biến thể đã đo

| Cách | @615.992 khách | Kết luận |
|---|---|---|
| Nguyên trạng | 1.393–1.515ms · 615.992 dòng | mốc |
| **`KEY (code)`, optimizer tự chọn** | **0,042–0,097ms** | **Thắng thật, ~33.000×.** Plan thành `index_merge union(PRIMARY, idx_customer_code)` — `Deduplicate rows sorted by row ID` + 2 `Index range scan`. Ổn định trên cả 4 segment (id thật · code thật · id không có · code không có) |
| Tách `OR` trong Go (segment toàn số → `id`, ngược lại → `code`), KHÔNG thêm chỉ mục | nhánh id **0,0008ms**; nhánh code **vẫn 1.495ms** | **LOẠI một mình.** Folder S3 THẬT đặt theo `code` (`DA.TDT.KH.2509.012`, `ID.ST.KH198` — khảo sát bucket thật), nên nhánh chạy nhiều nhất chính là nhánh không được cứu |
| `FORCE INDEX (idx_customer_code)` | 645–684ms | Xem mục dưới — **KHÔNG phải lý do để loại** |

### ⚠ Ngoại lệ của gate `FORCE INDEX`: index_merge

Gate ở `/check-scale` nói *(c) ép dùng chính chỉ mục mà chậm hơn (b) ⇒ phần thắng không đến từ
chỉ mục ⇒ không ship*. Ca này **thủng gate mà vẫn đúng để ship**, và cần hiểu vì sao để không
loại nhầm lần sau:

- (a) không chỉ mục = 1.404ms · (b) có chỉ mục, tự chọn = **0,042ms** · (c) `FORCE` = 684ms.
- `FORCE INDEX (idx_customer_code)` **giới hạn optimizer vào ĐÚNG MỘT chỉ mục ⇒ tắt luôn
  `index_merge`**. Mà plan thắng ở (b) là `index_merge union(PRIMARY, idx_customer_code)` — nó cần
  **hai** chỉ mục cùng lúc. Ép một cái là ép bỏ nhánh `id = ?` về quét.
- Bằng chứng phần thắng đúng là của chỉ mục mới: **không có nó thì `index_merge` không tồn tại**,
  và (a) = 1.404ms.

⇒ Luật bổ sung khi chạy gate: đọc `EXPLAIN` của (b) TRƯỚC. Thấy `Deduplicate rows sorted by row ID`
(hoặc `Using union(...)` ở EXPLAIN cổ điển) ⇒ plan là index_merge, `FORCE INDEX` **không phải phép
thử hợp lệ** cho nó. Gate chỉ áp cho plan dùng một chỉ mục.

### Đối chiếu kết quả — 8 segment, trước/sau khớp tuyệt đối

`536` · `KH0001` · `999999999` · `KHONG-TON-TAI-XYZ` · `1` · `0` · `536abc` · `` (rỗng):
`COUNT` giống hệt trước và sau khi thêm chỉ mục. Chỉ mục không đổi ngữ nghĩa.

Vì sao KHÔNG chọn cách tách `OR` dù nó rẻ hơn (không cần migration): nó đổi ngữ nghĩa ở ca **khách
có `code` toàn số trùng `id` của khách KHÁC**. DB thật hôm nay: **14 khách có code toàn số, 0 ca
trùng** — chưa vỡ, nhưng là hành vi đổi im lặng, không phải tối ưu thuần.

### Vế S3 — nghiệm thu bản vá phân trang ở NGƯỠNG THẬT 1.000

Cơ chế, phạm vi 5 điểm gọi, bản vá `mergeListPages` và test neo: xem mục
"⛔ Cái đáng sửa không phải tốc độ — là CẮT IM LẶNG ở 1.000 mục" ở trên (cùng ngày, phiên khác).
Mục này chỉ bổ sung đúng một thứ mục kia CHƯA có: **phép đo tại chính ngưỡng 1.000**, thay vì suy ra
từ `--max-keys 2` trên prefix 5 mục.

Ghi 1.100 object vào prefix dùng-một-lần `develop/_perf_probe_20260908_claude/source/` (user cho
phép ghi lên bucket production), đo, rồi xoá sạch:

| Cách gọi | KeyCount | IsTruncated | Key CUỐI nhìn thấy |
|---|---|---|---|
| MỘT lời gọi `ListObjectsV2` trần (hành vi TRƯỚC vá) | **1000** | **True** | `img-1000.txt` |
| Đọc hết mọi trang (hành vi SAU vá) | **1100** | False | `img-1100.txt` |

Chạy 2 lần trùng khít ⇒ **100 object biến mất, không lỗi, không dấu hiệu**, và mất đúng phần đuôi
theo thứ tự lexicographic — không phải "1000 ảnh mới nhất" như trực giác. Đối chứng lệnh có chạy
thật: `develop/customer/` đếm 206 object trước và sau probe, không đổi; prefix probe sau dọn còn
**0**.

Giá trị lâu dài của phép đo này: nhà cung cấp ở đây là **một dịch vụ S3-compatible (không phải AWS)** — trần mặc
định 1.000 key vẫn đúng y hệt. Đừng giả định trần này là đặc sản của AWS rồi bỏ qua khi đổi nhà
cung cấp.

## Vì sao `prescription-of-customer` KHOẺ còn `get-payment-detail-for-customer` HỎNG — khác đúng một chỗ

Đo 2026-09-08. `GET /api/med/med-customer-prescription/prescription-of-customer?customer_id=<id>`
(`med/customer_prescription/handler/handler.go`) cùng khuôn list-con với mọi mục trên: một bảng,
không join, `WHERE customer_id = ?`, `ORDER BY id DESC`, `Paginate`, 1 truy vấn attach `IN (…)`.

Mục này đáng đọc CẶP với mục `get-payment-detail-for-customer` ở trên: hai endpoint **gần như giống
hệt nhau về code**, nhưng một cái miễn nhiễm còn một cái có nhánh thảm hoạ. Khác biệt DUY NHẤT là
hình dạng chỉ mục trên cột fk:

| | `payment` (HỎNG) | `med_customer_prescriptions` (KHOẺ) |
|---|---|---|
| Chỉ mục phục vụ fk | `idx_payment_customer_for (customer_id, payment_for)` | `fk-…-customer_id-customer-id (customer_id)` — **1 cột** |
| Hậu tố PK ngầm của InnoDB | `(customer_id, payment_for, id)` | **`(customer_id, id)`** |
| Phục vụ được `ORDER BY id DESC`? | **KHÔNG** — `id` bị `payment_for` chắn ⇒ luôn kèm `Sort:` | **CÓ** — plan ra `Index lookup … (reverse)`, không có `Sort:` |
| Hệ quả ở khách nhiều dòng | optimizer bỏ chỉ mục sang `PRIMARY (reverse)` ⇒ O(bảng), 602.066 dòng InnoDB | giữ nguyên chỉ mục, đọc đúng 20 dòng |

⇒ **Bài học tái dùng được: chỉ mục 1 cột trên fk TỐT HƠN chỉ mục composite `(fk, cột_khác)` khi
list con sắp xếp theo `id DESC`.** Composite nghe "mạnh hơn" nhưng chính cột giữa lại chắn mất hậu
tố PK ngầm — thứ duy nhất cho phép quét ngược. Thêm một cột vào giữa một chỉ mục fk đang phục vụ
list con là đủ để biến early-exit thành quét toàn bảng, mà không câu query nào đổi.

### Số đo — 611.250 đơn / 60.005 khách / 1.833.750 dòng chi tiết

⚠ **KHÔNG có phân bố thật để neo: bảng `med_customer_prescriptions` trên DB dev có 0 dòng** (khách
này chưa dùng phân hệ Dược). Nên dữ liệu bench là tổng hợp, cố ý gồm cả 2 hình dạng lệch mà mục
`payment` đã chứng minh là nguy hiểm. Đối chứng `SELECT 1` = 0,000666ms; số là `EXPLAIN ANALYZE`
lượt 2 (warm).

| Ca | SELECT trang 1 | COUNT | Dòng InnoDB đọc (SELECT) |
|---|---|---|---|
| Khách 780 đơn, chỉ 50 khớp branch | 1,18ms | 2,95ms | 275 |
| Khách 2.451 đơn, id trải đều | 0,49ms | 2,64ms | 361 |
| **Khách 5.000 đơn, toàn id CŨ** (ca giết `payment`) | **0,098ms** | 10,6ms | **20** |
| Khách 5.000 đơn, id MỚI dày | 0,078ms | 9,19ms | 20 |
| Từ khoá CÓ khớp | 0,035ms | 4,99ms | 20 |
| Từ khoá KHÔNG khớp | 6,7ms | 6,24ms | 5.000 |
| Phân trang sâu `OFFSET 4.900` | 5,05ms | 4,29ms | 4.920 |
| Branch lọc sạch (không dòng nào khớp) | 5,19ms | — | 5.000 |
| Khách không tồn tại | 0,004ms | 0,003ms | 0 |
| Attach chi tiết `IN (20 id)` trên 1,83M dòng | **0,28–0,32ms** (warm) | — | 60 |

⇒ **Không có nút thắt.** Ca xấu nhất đo được (5.000 đơn cho MỘT bệnh nhân — đã là phi thực tế với
phòng khám) tốn ~15ms tổng SQL/request. Chi phí là **O(số con), không bao giờ O(bảng)** — đây mới
là điều `payment` không có.

⚠ Bẫy đọc số: attach chi tiết đo lượt 1–2 ra **4,03ms**, lượt 3–4 ra **0,28ms**. Chênh 14× hoàn
toàn do cache lạnh. Kết luận trên lượt đầu ở đây sẽ báo sai thành "attach là phần đắt nhất".

### Ba nhánh mất early-exit — đều bị chặn trên bởi số con, không phải kích thước bảng

`LIMIT 20` chỉ thoát sớm khi có đủ 20 dòng THẬT SỰ khớp. Ba vế được lọc SAU index lookup nên khi
chúng loại nhiều dòng thì phải quét hết số con của khách: **branch không khớp** (5,19ms), **từ khoá
không khớp** (6,7ms), **`OFFSET` sâu** (5,05ms). Cùng khuôn đã ghi ở `appointment/get-by-customer`.
Ở đây tất cả đều ≤7ms vì trần là 5.000 dòng của MỘT khách — không có đường nào chạm tới 611k dòng
của cả bảng.

### Đã APPLY lên DB dev (2026-09-09 15:15)

`go run cmd/migrate/main.go -cmd=up` đưa dev `20260909115816 → 20260909135142`, `dirty=0`, 4 file:
`payment_customer_index` · `note_created_at_index` · `tcd_confirm_time_index` ·
`tct_technical_confirm_index`. Kết quả kiểm lại bằng `information_schema`:

- `treatment_course_technical.idx-tct-technical_confirm`: `(confirm_time, technical_id)` →
  **`(technical_id, confirm_time)`** — drift đã sửa.
- `index-confirm_time` (bản trùng chỉ có ở dev) — **đã bỏ**.
- `payment.idx_payment_customer`, `note.idx_note_created_at`, `treatment_course_doctor.idx_tcd_confirm_doctor`
  — đã có mặt.
- Chạy lại gate: mục **"chỉ mục cùng tên, KHÁC cột" nay RỖNG**; drift chỉ mục 30/39 → **28/35**.
  Drift bảng/cột/FK không đổi (là drift lịch sử, không thuộc phạm vi 4 migration này).

⚠ **Không nghiệm thu được PLAN trên chính DB dev**: hook `guard-bash.sh` chặn cả `EXPLAIN ANALYZE`
lẫn `EXPLAIN FORMAT=TREE` khi câu lệnh trỏ `DB_NAME` (khớp từ khoá `ANALYZE`/hình dạng câu). Bằng
chứng hành vi vì thế đến từ bench clone-đúng-hình-dạng, còn trên dev chỉ xác nhận tới tầng SCHEMA.
Ai cần đo plan trên dev: clone sang `*_tmp` rồi `EXPLAIN` ở đó.

### Đã kiểm và KHÔNG phải lỗi

- **Vế `_keyword` CÓ dấu ngoặc.** `query.Where("name LIKE ? OR content LIKE ? OR instruction LIKE ?")`
  là fragment chứa `OR` cạnh vế tenant scope — thiếu ngoặc là thủng scope. Đã xác minh bằng GORM
  `DryRun` (không phải suy đoán): SQL sinh ra là
  `… AND customer_id = 777001 AND (name LIKE … OR content LIKE … OR instruction LIKE …)`.
  GORM tự bọc ngoặc mọi điều kiện chứa `OR`.
- **Attach chi tiết không phải N+1** — một truy vấn `IN (≤20 id)`, có
  `fk-med_cust_pres_detail-prescription_id` phục vụ, ra `Index range scan`.
- `med_customer_prescription_detail` **không có cột `company_id`**, và handler cũng không scope nó —
  đúng, vì `customer_pres_id` đã lấy từ danh sách cha ĐÃ scope.

## Xác nhận lần 3 cơ chế "1 cột fk cứu early-exit" — `customer-warranty-card/of-customer`

`GET /api/crm/customer-warranty-card/of-customer`
(`crm/customer_warranty_card/handler/find.go`, `WarrantyOfCustomer`). Bench: schema thật
(`mysqldump`) + sinh dữ liệu ⇒ **545.547 thẻ**, khách thường tối đa **12 thẻ** (trung bình 6,29),
cộng 2 khách lệch cố ý. Đối chứng `SELECT 1` = 0,00046ms.

⚠ **Không có phân bố thật để neo: bảng trên DB dev chỉ có 1 dòng.** Mọi kết luận dưới đây là về
HÌNH DẠNG query, không phải về tải thật.

| Ca | SELECT trang 1 | COUNT của `Paginate` |
|---|---|---|
| Khách thường (9 thẻ) | 0,22–0,98ms | 0,034ms |
| Khách **50.000 thẻ, id MỚI** | **0,081–0,158ms** | 19–27ms |
| Khách **5.000 thẻ, id CŨ** | **0,038–0,071ms** | 1,5–2,2ms |
| Khách không tồn tại | — | 0,005ms |
| Lọc `warranty_card_type_id` không khớp | — | 0,007ms |
| `OFFSET 49.900` | 51–102ms | — |
| **KHÔNG filter `customer_id`** | 0,026–0,064ms | **150–273ms · 218.506 dòng đọc** |

### Vì sao ca "khách CŨ 5.000 dòng" KHÔNG vỡ ở đây mà vỡ ở `payment`

Đây là ca đối chứng trực tiếp với mục "Hai plan, mỗi plan một nhánh thảm hoạ". Cùng hình dạng dữ
liệu (nhiều con, toàn id cũ) — `payment` tốn 217–418ms đọc 602.066 dòng, bảng này tốn **0,07ms đọc
20 dòng**. Khác biệt DUY NHẤT vẫn là hình dạng chỉ mục fk:

```
-> Index lookup on customer_warranty_card
   using fk-customer_warranty_card-customer_id-customer-id (customer_id = 888002) (reverse)
   (actual time=0.44..0.442 rows=20 loops=1)      ← KHÔNG có node Sort:
```

`fk-…-customer_id` là chỉ mục **MỘT cột** ⇒ hậu tố PK ngầm của InnoDB biến nó thành
`(customer_id, id)` ⇒ `ORDER BY id DESC` được phục vụ bằng quét ngược, early-exit thật.
`payment` thì chỉ mục là `(customer_id, payment_for)` ⇒ thứ tự trong khách là `(payment_for, id)`
⇒ không phục vụ được `id DESC` ⇒ optimizer bỏ chỉ mục sang quét ngược PRIMARY.

⇒ **Quy tắc rút ra, nay đã 3 lần đo ra cùng kết luận** (`prescription-of-customer`,
`appointment/get-by-customer`, và ca này): list-con `WHERE fk = ? ORDER BY id DESC` an toàn khi
chỉ mục fk là **1 cột**; thêm cột thứ hai vào GIỮA fk và PK là thứ phá early-exit. Khi thấy một
bảng có chỉ mục `(fk, <cột khác>)` mà KHÔNG có `(fk)` trần, đó là cờ đỏ.

Ngoài lề đáng ghi: 3 composite index thêm ngày 2026-09-07 cho endpoint list NGOÀI
(`idx_wcard_company_customer` …) **không phải** thứ cứu endpoint này — plan chọn chỉ mục fk 1 cột.

### Nút thắt DUY NHẤT: `COUNT` khi thiếu `customer_id` — nhưng FE không đi đường đó

`customer_id` ở đây đi qua `query.ApplyFilter` nên là **tuỳ chọn**, khác các endpoint `*-of-customer`
khác vốn bắt buộc và trả lỗi khi thiếu. Gọi trần `/of-customer` không tham số ⇒ trả thẻ của TOÀN BỘ
công ty, và `COUNT` của `Paginate` đọc **218.506 dòng** (`Covering index lookup on
fk-…-company_id`) = 150–273ms.

**Không vá**: FE luôn gửi `filter[customer_id]`
(`warranty-of-customer.ts:161`), nên đây không phải đường của màn hình. Và `COUNT` thì không chỉ
mục nào cứu được — nó phải đếm hết, cùng kết luận đã có ở `reminder-of-customer`. Ngưỡng đáng quay
lại: nếu có màn hình mới gọi endpoint này KHÔNG kèm `customer_id`.

### ✅ Nợ correctness phát hiện kèm — `filter[customer_id]` không parse ra số (ĐÃ VÁ)

Không phải vấn đề hiệu năng nhưng phát hiện trong cùng lượt đo: `ApplyFilter` đẩy chuỗi thô vào
query nên `filter[customer_id]=888002abc` trả về đúng 5.000 thẻ của khách 888002.

**Đã vá 2026-09-08** ở hai tầng: `pkg/query/numeric_id_filter.go` (allowlist cột ID thuần số, phủ
toàn bộ 636 call site `ApplyFilter`) + `customer_id` thành BẮT BUỘC trong `WarrantyOfCustomer`.
Số đo, bảng hành vi từng operator và thủ tục thêm cột vào allowlist:
`skills/backend-hard-rules/reference/security-patterns.md` §Ép kiểu im lặng ở `filter[<cột>_id]`.

## Endpoint 1-dòng-mỗi-khách: `get-warranty-code` — cả 3 câu đều là const/index lookup

Đo 2026-09-08. `GET /api/crm/customer-warranty-code/get-warranty-code?customer_id=<id>`
(`crm/customer_warranty_code/handler/handler.go`). Đây là nhóm NHẸ NHẤT trong sổ — nhẹ hơn cả
`customer/:id` — nhưng vẫn phải đo vì bảng `customer_warranty_code` hôm nay mới có **7 dòng**
(tính năng gần như chưa dùng), tức trạng thái hiện tại KHÔNG nói gì về trạng thái bão hoà.

⚠ Đừng nhầm với `customer-warranty-**card**` đã có trong sổ (2026-09-07, đã vá bằng 3 composite
index). Khác resource, khác bảng, khác đường đi.

Handler chạy tối đa 3 câu: (1) `customerInTenant` — `COUNT(*) FROM customer WHERE id = ? + scope`;
(2) `First` trên `customer_warranty_code WHERE customer_id = ?`; (3) chỉ khi (2) trượt — đọc
`phone`+`branch_id` của khách theo PK để sinh mã gợi ý.

Bench: schema thật (`mysqldump --no-data`) + dữ liệu `customer` thật nhân 3 lần ⇒ **615.992 khách**,
rồi sinh mã ở trạng thái **bão hoà: mỗi khách 1 mã = 615.992 dòng** (gấp 88.000× thực tế).
Đối chứng `SELECT 1` = 0,00025ms. Số là `EXPLAIN ANALYZE` lượt 3 (warm).

| Câu | Ca | Số đo | Plan |
|---|---|---|---|
| Q1 | khách có thật, đúng scope | **0,0026ms** | `Rows fetched before execution` (const table) |
| Q1b | khách KHÔNG tồn tại | 0,00046ms | `Zero input rows (no matching row in const table)` |
| Q1c | khách có thật nhưng SAI branch | 0,00017ms | `Impossible WHERE noticed after reading const tables` |
| Q2 | khách ĐÃ có mã, bảng 615.992 dòng | **0,028ms** | `Index lookup … fk-customer_warranty_code-customer_id`, 1 dòng |
| Q2b | khách CHƯA có mã (miss) | 0,007ms | cùng chỉ mục, 0 dòng |
| Q3 | đọc `phone`+`branch_id` theo PK | 0,00037ms | const table |

⇒ **Không có nút thắt.** Đường `hasExisting` = Q1+Q2 ≈ **0,031ms**; đường sinh mã = Q1+Q2b+Q3 ≈
0,010ms; đường 404 ≈ 0,0005ms. Cách ngưỡng đáng bàn nhiều bậc độ lớn, ở quy mô gấp 88.000× hiện tại.

### Ca đối kháng đã đo: `customer_id` KHÔNG có UNIQUE

`fk-customer_warranty_code-customer_id` là `NON_UNIQUE=1`, và bất biến "1 mã / 1 khách" chỉ được
giữ ở tầng app (`CreateWarrantyCodeForCustomer` đọc-rồi-ghi, không có khoá). Nên phải đo ca một
khách có nhiều mã:

| Ca | Số đo |
|---|---|
| Khách có **50.001** mã trùng, bảng 665.991 dòng — `First` (đúng thứ handler chạy) | **1,99ms, đọc 1 dòng** |
| Đối chứng: đếm HẾT 50.001 mã đó (KHÔNG phải thứ handler chạy) | 16,4ms, đọc 50.001 dòng |

Early-exit thật, vì chỉ mục 1 cột `(customer_id)` + hậu tố PK ngầm ⇒ `(customer_id, id)`, khớp đúng
`ORDER BY id LIMIT 1` mà GORM `First` sinh ra. Cùng cơ chế đã ghi ở mục
`prescription-of-customer`. Dòng đối chứng 16,4ms là bằng chứng phép đo có chạy thật — không phải
"nhanh vì chẳng có dữ liệu".

### ⚠ Không phải hiệu năng, nhưng phát hiện lúc đo — cần người quyết

1. **Không có UNIQUE trên `customer_warranty_code.customer_id`.** Hai request tạo mã đồng thời cho
   cùng một khách đều thấy "chưa có" rồi cùng ghi ⇒ 2 mã cho 1 khách, **im lặng**. Sau đó `First`
   luôn trả mã có `id` NHỎ NHẤT, nên người dùng thấy một mã ổn định và sẽ không ai phát hiện. Hôm
   nay bảng có 7 dòng / 7 khách ⇒ **chưa có bản sao nào**, tức thêm `UNIQUE KEY (customer_id)` lúc
   này không phải xử lý dữ liệu cũ. Để user quyết vì nó đổi hành vi ghi (create thứ 2 sẽ lỗi thay
   vì âm thầm thành công).
2. **`customer_id=536` trong URL được hỏi KHÔNG tồn tại trên DB dev** ⇒ request đó trả **404** ở
   `customerInTenant`, không phải lỗi hiệu năng. Nếu bro thấy màn hình trống thì nguyên nhân là đây.
3. Câu Q2 chạy trên `h.DB` **không có tenant scope** — an toàn vì `customerInTenant` đã chặn trước
   đó, nhưng nó là an toàn *nhờ thứ tự lời gọi*, không nhờ chính câu query. Đổi thứ tự hoặc tái dùng
   hàm này ở chỗ khác là mất lưới.

### Bẫy dựng bench gặp ở lượt này (cả hai đều "chạy nhưng sai âm thầm")

- **`GROUP_CONCAT` cắt ở 1.024 byte.** Lấy danh sách cột của `customer` (140 cột = 2.404 byte) bằng
  `GROUP_CONCAT` trả về chuỗi CỤT, và `INSERT` sinh ra chỉ báo `syntax error` ở giữa tên cột chứ
  không hề nói "bị cắt". Phải `SET SESSION group_concat_max_len = 1000000` (SESSION, KHÔNG phải
  GLOBAL) và đối chiếu số cột đếm được với `COUNT(*)` từ `information_schema`.
- **Cột GENERATED không được nằm trong danh sách INSERT.** `customer` có `birthday_md` và
  `search_ngram` là generated ⇒ `ERROR 3105 … is not allowed`. Lọc `EXTRA NOT LIKE '%GENERATED%'`.

## Chỉ mục composite ĐÚNG cột thì list-con miễn nhiễm cả với `ORDER BY` không phải `id` — `care-of-customer`

Đo 2026-09-08. `GET /api/customer-care/care-of-customer?filter[customer_id]=<id>`
(`customer-care/care/handler/handler.go`). Mục này bổ sung mảnh còn thiếu cho cặp
`prescription-of-customer` ⇄ `get-payment-detail-for-customer` ở trên.

Ba mục trước rút ra "chỉ mục fk **1 cột** cứu early-exit nhờ hậu tố PK ngầm `(fk, id)`". Nhưng mẹo
đó chỉ chạy khi `ORDER BY` **là** `id`. Ở đây `ORDER BY customer_care.day_need_care ASC,
customer_care.id ASC` — không phải `id` — mà vẫn miễn nhiễm, vì bảng có
`idx_customer_care_customer_day (customer_id, day_need_care)`; cộng hậu tố PK ngầm thành
**`(customer_id, day_need_care, id)`**, khớp TRỌN cả `WHERE` lẫn `ORDER BY`.

⇒ Quy tắc tổng quát hơn (thay cho cách phát biểu "1 cột thì tốt"): chỉ mục phải là
**`(cột lọc, …đúng các cột ORDER BY theo đúng thứ tự)`**, và hậu tố PK ngầm được tính là cột cuối
MIỄN LÀ `id` đứng cuối trong `ORDER BY`. `payment` hỏng không phải vì composite — mà vì composite
của nó `(customer_id, payment_for)` có cột GIỮA không nằm trong `ORDER BY`, chắn mất `id`.

### Số đo — 696.744 dòng / 11.911 khách. Đối chứng `SELECT 1` = const.

Phân bố THẬT trên DB dev: 16.617 dòng / 11.910 khách, **nhiều nhất 10 chăm sóc/khách**. Bench nhân
32 lần (trần thật thành 320) + 2 khách lệch cố ý.

| Ca | SELECT trang 1 | COUNT | Dòng đọc (SELECT) |
|---|---|---|---|
| Khách 320 dòng (≈32× trần thật) | 0,069ms | 0,585ms | 20 |
| **Khách 160.000 dòng** | **0,052ms** | 155ms | **20** |
| Khách 5.000 dòng id MỚI | 0,034ms | 4,43ms | 20 |
| Khách 160.000 + lọc `status_care` KHỚP | 0,192ms | 210ms | 20 |
| Khách 160.000 + lọc `status_care` KHÔNG khớp | **198ms** | — | 160.000 |
| Phân trang sâu `OFFSET 4.900` | 5,62ms | 4,48ms | 4.920 |
| Branch không khớp / khách không tồn tại | 0,008ms / 0,004ms | 0,004ms | 0 |
| **KHÔNG có `customer_id`** (filter tuỳ chọn) | **234ms** | **180ms** | **192.360** |

Khách 160.000 dòng trả trang 1 trong 0,052ms vì đọc đúng 20 dòng — không có `Sort:` ở bất kỳ ca nào
có `customer_id`. Chỉ 2 nhánh mất early-exit và cả hai đều **O(số con)**: lọc phụ không khớp dòng
nào, và `OFFSET` sâu.

### ⛔ Nút thắt thật: `customer_id` là filter TUỲ CHỌN — ĐÃ VÁ

Thiếu `customer_id` thì optimizer bỏ `idx_customer_care_customer_day` (không còn vế `customer_id`
để tra), rơi sang `fk-customer_care-branch_id`, rồi phải **`Sort:` 192.360 dòng của cả chi nhánh**
để lấy 20 ⇒ **234ms SELECT + 180ms COUNT ≈ 414ms/request**. Chi phí tăng theo **kích thước bảng**,
không theo số con của khách.

Bản vá: `customer_id` nay **bắt buộc** ở handler. Không mất tính năng — danh sách chăm sóc toàn
công ty đã có endpoint riêng `GET /customer-care/care`; endpoint tên "care-of-customer" mà gọi
không kèm khách vừa chậm vừa trùng chức năng. Rà consumer: **đúng 1 nơi gọi** (FE
`care-for-customer.ts`), và nó LUÔN gửi `filter[customer_id]` (`initRequiredFilter`).

⚠ Guard phải dùng `query.GetFilterValues` (số nhiều) + đòi đúng 1 phần tử — rule + lý do ở
`skills/backend-hard-rules/SKILL.md` §"Lớp thứ ba, gate KHÔNG phủ".

Endpoint anh em `customer_warranty_card.WarrantyOfCustomer` dính đúng lỗ này — **đã vá cùng ngày**
(2026-09-08), kèm 4 test `warranty_of_customer_integration_test.go`. Quét toàn repo: chỉ có **2**
điểm dùng `GetFilterValue` để lấy GIÁ TRỊ id bắt buộc rồi ném thẳng vào `Where(= ?)` — hai cái này.
Mọi chỗ khác chỉ dùng nó làm phép kiểm CÓ/KHÔNG (`!= ""`) rồi vẫn giao giá trị cho `ApplyFilter`
(bản này xử lý mảng đúng) ⇒ không thuộc lớp lỗi này.

**Lớp lỗi này nay ĐÓNG: cả 3/3 điểm đã vá 2026-09-08**, mỗi điểm kèm 4 integration test và
mutation cô lập (đỏ đúng test đa-giá-trị, các test còn lại xanh):
`customer-care/care-of-customer`, `customer_warranty_card.WarrantyOfCustomer`,
`customer_detail.buildTreatmentCourseOfCustomerQuery`. Gate kiểm lại về sau:
`grep -rn 'ParseUint(query.GetFilterValue(' --include='*.go' internal/` phải = **0**.

Thu hẹp API có chủ đích, nói rõ để khỏi tưởng là regression: `ApplyFilter` trước đây nhận cả toán tử
(`filter[customer_id][gt]=5`, `[neq]`, `[like]`) và mảng. Sau bản vá chỉ nhận **đúng một giá trị số**.
Không consumer nào dùng các dạng kia.

`filter[customer_id]=4006abc` **đã** được `pkg/query/numeric_id_filter.go` (phiên khác, cùng ngày)
chặn không cho ép kiểu — nó trả `1 = 0` ⇒ danh sách rỗng, KHÔNG rò rỉ. Guard mới chỉ đổi
"200 rỗng im lặng" thành lỗi tường minh; **đừng ghi công nó là bịt rò rỉ**.

Test neo: `care_of_customer_integration_test.go` — 4 test, mutation-verified bằng cách khôi phục
đúng `ApplyFilter` cũ: 3 test guard ĐỎ, còn test parity đường-FE vẫn XANH (chứng minh màn hình
không đổi hành vi).

---

## Dashboard: 21 endpoint aggregate bắn SONG SONG — khác hẳn khuôn endpoint list

Đo 2026-09-08, epic `crm-dashboard-scale`, toàn bộ 21 endpoint `GET /api/crm/crm-dashboard/*`.
Bench: `customer` 615.992 · `appointment` 1.090.355 · `payment` 512.193 · `order_service_used`
561.700 · `order_treatment_plan` 522.095 · `reminder`/`customer_lead`/`customer_care` ~300k mỗi
bảng · `coupon_used_by_order_plan` 200.000.

### Vì sao màn dashboard KHÔNG đo được như một endpoint list

| Đặc thù | Hệ quả |
|---|---|
| 21 request bắn **song song** mỗi lần vào module | chỉ số phải đo là **cả trang**, không phải p95 một endpoint |
| 100% aggregate (`COUNT`/`SUM`/`GROUP BY`) | **không bao giờ early-exit** — `LIMIT` không cứu, khác hẳn khuôn list-con đã xác nhận 5 lần |
| Tất cả cache Redis 60–300s | ca phải đo là **cache MISS**; test tay luôn trúng HIT nên che sạch vấn đề |
| Không có singleflight | 1→3→5 user cùng lúc = 10,0M → 30,1M → 50,2M dòng InnoDB (tuyến tính đúng bội số user) |
| Ca **"toàn bộ chi nhánh"** (branchActive=0) | ở đây CHẬM GẤP ~1,5 lần ca 1 chi nhánh — **ngược** với khuôn `tooth_bio` ở mục trên. Và đó đúng là ca của người xem dashboard BOD |

### Kết quả (cùng bench, cùng code, chỉ khác bộ chỉ mục)

| Ca | Trước | Sau | |
|---|---|---|---|
| Cả trang @1 chi nhánh | 5.045ms · 10.047.268 dòng | **1.003ms · 2.166.900** | −80% |
| Cả trang @toàn bộ chi nhánh | 7.173ms · 15.665.603 | **1.499ms · 3.273.552** | −79% |
| 5 user đồng thời (105 request) | 16.196ms · 50.224.577 | **2.372ms · 10.831.375** | −85% |
| Tuần tự 21 endpoint @BOD | 18.814ms · 15.665.618 | **2.336ms · 2.700.318** | **8,1×** |

Top: `payment-today` 465× · `status-customer-overview` 313× · `quick-starts` 206× ·
`payment-in-month` 43× · `overview-chart-source` 44× · `additional-stats` 30×.

Nguyên nhân gốc gần như đồng nhất: **cột ngày của các bảng nghiệp vụ không có chỉ mục nào** —
`payment.payment_time`, `payment.payment_of_date`, `customer.created_day`,
`order_treatment_plan.created_time`, `reminder.start_time`, `broker.created_at`,
`coupon_used_by_order_plan.created_at`. Migration: `20260908204557`, `20260908214512`,
`20260908233018`, `20260908222015`.

### 6 bài học chuyển được sang việc khác

1. **Chỉ mục hẹp trên cột ngày có thể bị optimizer BỎ ở cửa sổ rộng hơn.** `payment
   (company_id, payment_time)` chạy tốt cho "tháng này" (5.995 dòng) nhưng "tháng trước"
   (44.704 dòng = **8,7%** bảng) thì optimizer quay lại quét cạn 512.193 dòng/479ms, vì phải tra
   44k dòng cho `amount`/`payment_for`/`deposit_type`/`branch_id`. Phủ đủ 6 cột ⇒ 44.704 dòng/20,6ms.
   Ngưỡng bỏ chỉ mục ở đây **thấp hơn nhiều** so với quy tắc "20-30%" hay được nhắc.
2. **Thêm chỉ mục có thể LÀM CHẬM endpoint khác.** `osu (company_id, created_day)` trần khiến
   `service-used-statistics` từ 1.744 → 13.588 dòng (4ms → 11,8ms): câu đó lọc `isLock = 1` và
   đang dùng `idx_osu_appt_day_cover`, chỉ mục hẹp mới làm cost model đổi sang plan tệ hơn. Cách
   thoát: làm chỉ mục mới **phủ hẳn** cho câu cần nó (`…, branch_id, totalEnd`) để nó không cướp
   câu kia.
3. **MỞ RỘNG chỉ mục sẵn có an toàn hơn đẻ chỉ mục mới**, khi giữ nguyên tiền tố: consumer cũ
   không đổi kế hoạch. `idx_appointment_company_date` 6 → 9 → 10 cột phục vụ 4 câu dashboard;
   bản chỉ mục MỚI tương đương lại làm `quick-starts` đọc 353.550 thay vì 235.733 dòng.
   ⚠ Idempotency của migration khi đó phải so **danh sách cột**, không so tên chỉ mục.
4. **Điều kiện ngày nằm trong `CASE WHEN` mà `WHERE` không có = quét cạn bảng.** `quick-starts`
   đếm số của MỘT ngày nhưng quét toàn bộ lịch hẹn của công ty. Thêm
   `AND (created_at BETWEEN … OR appointment_date BETWEEN …)` vào `WHERE` **không cứu được**
   (đo 2 lần, 235.732 dòng trước và sau — `OR` trên hai cột khác nhau không dùng được một dải
   chỉ mục). Phải **tách thành 2 câu**, mỗi câu tự chặn dải ngày trên cột của chính nó:
   235.732 → 7.261 dòng, 509ms → 2,6ms, giá trị khớp tuyệt đối.
5. **`FORCE INDEX` nhanh mà auto không đổi = KHÔNG ship được.** `customer_care`
   `(company_id, day_need_care, branch_id, staff_id)`: auto 19,1ms y như không có chỉ mục,
   FORCE 3,7ms. Đảo thứ tự cột vẫn không được chọn. Ship chỉ làm phình bảng.
6. **Plan xấu không phải lúc nào cũng do thiếu chỉ mục.** `report-statistics` join 2 bảng nhỏ
   (`appointment_is_accepted` 11 dòng × `appointment_type` 7 dòng) ⇒ MySQL hash-join **không điều
   kiện** hai bảng đó rồi nested-loop vào `appointment`, quét lại dải ngày ~6 lần: 707.193 dòng /
   2.108ms. Đưa `is_accepted` vào chỉ mục để câu thành index-only ⇒ 45.196 dòng / 13,6ms (155×).

### Bẫy khi DỰNG bench cho dashboard (khác bench endpoint list)

- **Nhân bản dữ liệu thật thì ngày cũng bị nhân theo** ⇒ cửa sổ "hôm nay/tháng này" gần rỗng trong
  khi bảng đã lớn gấp 5–11 lần ⇒ **6 endpoint trả `[]` và đo ra "nhanh" giả**. `report-statistics`
  đo **7ms**; sau khi dời ngày đúng mật độ ⇒ **1.539ms**. Sai **220×**. Bắt được vì kiểm response
  body, không chỉ nhìn đồng hồ.
- **`RAND(N)` với seed nguyên liên tiếp cho dải hẹp, không đều.** `1+FLOOR(RAND(n+2)*17)` chỉ ra
  chi nhánh 5..13, chi nhánh 1 trống ⇒ đo trên cửa sổ rỗng. Dùng `RAND()` không seed.
- **Kiểm cardinality thật của bảng danh mục trước khi seed FK.** DB này chỉ có **1 company**;
  seed `company_id = 1 hoặc 2` lọt qua nhờ `FOREIGN_KEY_CHECKS=0` rồi bị scope loại ⇒ đo nhẹ hơn
  thực tế ~một nửa. Tương tự, id danh mục **không liên tục 1..N** nên `1+RAND()*300` đâm FK 1452.
- **`mysql -N -e "SHOW CREATE TABLE"` escape xuống dòng thành `\n` literal** ⇒ parse DDL theo dòng
  ra RỖNG, script báo "đã bỏ 0 index" rồi chết ở `Duplicate entry`. Đọc DDL từ file `mysqldump`.
- `mysqldump` mặc định `LOCK TABLES` ⇒ `error 1449: definer … does not exist` nếu có view definer
  mồ côi. Thêm `--single-transaction --skip-lock-tables`.
- **Từ khi chỉ mục đổi độ rộng, tổng dòng InnoDB không so trực tiếp được giữa các lần đo**: chỉ mục
  rộng quét nhiều *mục chỉ mục* hơn nhưng bỏ hẳn bước tra dòng, nên dòng có thể TĂNG trong khi thời
  gian GIẢM. So trước/sau trên **cùng một câu** thì vẫn hợp lệ.

### Còn nợ (chưa sửa)

- **Singleflight — ĐÃ VÁ 2026-09-09.** Trước: mỗi endpoint tự cache Redis nhưng cache LẠNH thì
  mọi request cùng miss và cùng tính; các endpoint dùng CHUNG một truy vấn thì chi phí nhân theo
  số endpoint rồi nhân tiếp theo số user (đo: 1→3→5 user = 10,0M → 30,1M → 50,2M dòng InnoDB,
  tuyến tính đúng bội số user). Nay 3 truy vấn dùng-chung đi qua `sfDo`
  (`crm/dashboard/handler/singleflight.go`): `queryCrmMarketingFunnelStats` (2 endpoint),
  `getOnlineSourceIDs` (4 endpoint), `getIsAcceptedIDs`. Hợp đồng được neo bằng 3 test chạy
  `-race -p 1`: 32 goroutine trùng khoá ⇒ hàm tính chạy ĐÚNG 1 lần; 8 khoá khác nhau ⇒ chạy đủ 8
  (chống gộp nhầm tenant); lỗi tới TẤT CẢ người chờ. Mutation-verified hai chiều (khoá duy nhất
  mỗi lần gọi ⇒ test gộp đỏ; khoá hằng số ⇒ test khác-khoá đỏ).

  ⚠ Ba điều bắt buộc nhớ khi singleflight một hàm có cache:
  1. **Khoá singleflight phải LÀ khoá cache.** Khoá cache dashboard đi qua `tenantCacheKey(c)`
     (companyActive + branchActive + toàn bộ companyIds/branchIds đã sort) nên hai người khác phạm
     vi không bao giờ chung khoá. Tự chế khoá riêng = mở đường rò dữ liệu chéo chi nhánh.
  2. **Hàm bên trong KHÔNG được ghi thẳng response.** Bản cũ gọi `pagination.FailQuery(c, …)` bên
     trong; sau khi gộp thì chỉ MỘT request chạy nó ⇒ lỗi rơi vào response của người dẫn, còn
     người chờ nhận giá trị rỗng với HTTP 200 — hỏng im lặng. Phải trả `error` ra để từng caller
     tự `FailQuery` trên context của chính mình. (Bản vá tách `computeCrmMarketingFunnelStats`
     đúng vì lý do này, không phải cho đẹp.)
  3. Singleflight **không thay** cache: cache lo lần sau, singleflight lo cửa sổ cache lạnh.

- **Còn nợ**: `overview-online` 378ms vẫn là endpoint chậm nhất, tự quyết định thời gian cả trang
  (không dùng chung truy vấn nào nên singleflight không đụng tới).
- **`customer_lead` không bị lọc tenant** trong `additional-stats` (bảng không có cột
  `company_id`/`branch_id`) ⇒ các chi nhánh cùng một chủ thấy chung số Lead. Câu hỏi nghiệp vụ,
  chờ user quyết.

## Aggregate nhiều chỉ số viết thành N subquery độc lập — mỗi chỉ số một lượt quét

Đo trên `GET /api/crm/report/appointment-statistics` (2026-09-09), bench 1.090.355 lịch hẹn /
615.992 khách dựng từ schema thật `mysqldump --no-data`.

Màn này trả 6 chỉ số theo ngày. Bản cũ viết **6 subquery độc lập**, mỗi cái tự quét dải ngày trên
`appointment` rồi `GROUP BY customer_id` vào bảng tạm riêng — trong khi cả 6 chỉ là 6 lát của đúng
**hai** tập: lịch hẹn TẠO trong kỳ (`created_at`) và lịch hẹn CÓ NGÀY HẸN trong kỳ
(`appointment_date`). Cộng thêm `LEFT JOIN customer` vô điều kiện ở cả 6 (chỉ 1 câu đọc
`customer.created_day`), mà [MySQL không loại-bỏ-left-join](#mysql-không-loại-bỏ-left-join).

⚠ **ĐO LẠI TRÊN HÌNH DẠNG KHÁCH (2026-09-09).** Bench gốc clone từ dev — mà dev THIẾU
`idx-appointment-appointment_date` và `idx-appointment-created_at` (hai chỉ mục MỘT CỘT khách có).
Dựng lại bench bằng `cmd/migrate`, cùng khối lượng 1.090.355 lịch hẹn / 615.992 khách, ca 1 năm
toàn chi nhánh, bộ đếm SESSION (đối chứng `SELECT 1`=14, `COUNT(*)`=1.090.357):

| | dev-shape (số đã báo) | **migration-shape (số THẬT của khách)** |
|---|---|---|
| CŨ — 6 subquery | 34.880.597 dòng · 27,5s | **11.827.933 dòng · ~90s** |
| MỚI — 2 CTE | 2.718.239 · 4,9–8,7s | **2.925.783 · 16,6s** |
| tỉ lệ | 12,8× | **4,0×** |

⇒ Vế "sau" gần như y hệt (2,7M ⟷ 2,9M) vì bản vá dùng chỉ mục composite có ở CẢ HAI hình dạng.
Vế "trước" bị dev phóng đại **3×**. **Kết luận vẫn đứng**: endpoint này chậm thật ở khách
(11,8 triệu dòng cho một lượt xem 1 năm), bản vá gộp CTE vẫn đúng — chỉ là lợi ích 4× chứ không
phải 12,8×.

Bảng gốc (dev-shape, giữ để đối chiếu):

| Ca (toàn bộ chi nhánh) — **dev-shape** | Trước | Sau |
|---|---|---|
| 1 năm | 34.880.597 dòng InnoDB · 27,5s | **2.718.239 · 4,9–8,7s** |
| KHÔNG lọc ngày (nút xoá trên FE) | 9.647.780 · 22,9s | **2.729.935 · 3,3s** |
| tháng trước | 1.177.215 · 3,32s | **315.190 · 842ms** |
| tháng này (mặc định FE) | 209.880 · 544ms | **60.805 · 212ms** |

@1 chi nhánh: 1 năm 745.770/3,19s → **177.611/842ms**; không-lọc 839.360/5,21s → **178.288/943ms**.
⚠ Máy lúc đo có phiên khác cùng bench (load 5–14) nên tin **số dòng InnoDB**, không tin ms tuyệt đối.

Bỏ join là phần dễ nhưng KHÔNG phải phần lớn nhất — đo tách bạch từng subquery (1 năm, toàn chi nhánh):

| | có join | bỏ join |
|---|---|---|
| `AppNew` (`created_at`) | 8.001ms | **615ms** |
| `AppDate` (`appointment_date`) | 33.502ms | **1.693ms** |

Nhưng bỏ join xong cả endpoint vẫn 16s, vì 6 lượt quét vẫn còn. Gom về 2 CTE mới xuống 3–5s.

**Khuôn gom (áp được cho mọi report "N chỉ số cùng một dải ngày"):**

- 4 chỉ số cùng lọc theo `appointment_date`, chỉ khác `status`/`is_accepted` ⇒ MỘT lần
  `GROUP BY customer_id` với `MIN(CASE WHEN <điều kiện> THEN appointment_date END)` cho từng chỉ số,
  rồi mỗi nhánh `UNION ALL` đọc lại CTE đó và bỏ `NULL`. Giữ nguyên ngữ nghĩa "ngày sớm nhất của
  KHÁCH trong tập con đó" — thứ mà một `COUNT(DISTINCT)` phẳng KHÔNG diễn đạt được.
- Chỉ số "khách cũ" (`numAppNewPlus`) hoá ra chỉ là chỉ số "khách mới" lọc thêm
  `customer.created_day < from`. Cột đó là thuộc tính của KHÁCH ⇒ hằng số trong nhóm ⇒ `MIN` không
  đổi ⇒ join `customer` **sau khi đã gom nhóm** (≈195k khách) thay vì trước (1,09M lịch hẹn).
  Đây là cách rẻ nhất để giữ một filter xuyên bảng mà không trả giá theo số dòng bảng chính.
- CTE bị đọc lại 4 lần ⇒ MySQL vật chất hoá **một lần** (đo được `Created_tmp_disk_tables=1`,
  chấp nhận được). Đừng gắn join vào CTE — làm thế là quay lại đúng bệnh cũ.

**Vì sao ca "toàn bộ chi nhánh" mới là ca thật.** Bộ lọc chi nhánh sinh
`(branch_id IN (17 giá trị) OR branch_id IS NULL)` — vế `OR … IS NULL` (luật NULL-tenant) làm
`branch_id` không chặn được dải chỉ mục, nên toàn-chi-nhánh **chậm hơn** 1-chi-nhánh 5–8 lần chứ
không nhanh hơn. Cùng chiều với [dashboard](#dashboard-21-endpoint-aggregate-bắn-song-song), ngược
với `tooth_bio`. Người xem báo cáo này là BOD ⇒ đo thiếu ca đó là bỏ lọt toàn bộ vấn đề.

**Ca "KHÔNG lọc ngày" phải đo**: handler mặc định `from=0`, `to=9999999999` khi query rỗng, và FE
có nút xoá ngày (`clearStartTime()`) ⇒ quét cạn bảng là một cú bấm chuột, không phải giả định.

**Đã chặn ở handler (user chốt 2026-09-09)**: dải ngày nay BẮT BUỘC và trần `define.ReportMaxRangeSec`
= 366 ngày; thiếu/rác/đảo đầu-cuối hoặc vượt trần ⇒ `response.Validate` 422 với
`report_date_range_required` / `report_date_range_max_one_year`. **KHÔNG tự cắt dải ngày** — cắt
im lặng là trả thiếu số liệu mà người xem báo cáo không có cách nào biết. Guard đặt TRƯỚC mọi truy
vấn (kể cả `tz.LocFromContext`) nên test neo nó chạy với `Deps{DB: nil}`, không cần DB:
`appointment_statistics_guard_test.go` — 6 ca 422 + 1 ca đối chứng "đúng trần thì đi tiếp"
(biểu hiện bằng panic vì DB nil), cả hai vế đã mutation-verified riêng lẻ.

⚠ Chặn ở BE thì phải sửa luôn nút đã đi vào ca đó, không thì chỉ đổi màn chậm lấy toast 422:
`clearStartTime()` ở FE nay reset về mặc định (đầu tháng → hôm nay) thay vì `null`.

⚠ Bẫy khi mutation-test chính guard này (dính 2026-09-09): cấy `if false && A || B` KHÔNG tắt được
guard — `&&` ưu tiên hơn `||` nên vế `B` vẫn chạy và vẫn chặn 5/6 ca, đọc kết quả sẽ tưởng test
yếu. Muốn tắt một điều kiện ghép thì thay TRỌN mệnh đề bằng `false`.

Đối chiếu: 12 ca (6 bộ lọc × 2 chế độ chi nhánh) khớp **byte-for-byte** với bản cũ; phép đối chiếu
tự chứng minh có hiệu lực bằng mutation (`status IN (2,3)` → `IN (2)` ⇒ lệch ngay).

⚠ Nợ để lại: endpoint này **chưa có test integration** neo 6 chỉ số (package đang có uncommitted
work của phiên khác nên không thêm file test cùng package lúc đó).

## Alias truyền làm TÊN BẢNG cho `ScopedDB*` ⇒ mất scope IM LẶNG, và mất luôn chỉ mục

Đo trên `GET /api/crm/report/service-type-statistics` (2026-09-09), bench 674.040
`order_service_used` (586.524 `isLock=1`) / 238.236 khách / 646 dịch vụ / 27 loại, dựng từ schema
thật.

```go
// SAI — "osu" là ALIAS, không phải tên bảng
tenant.ScopedDBNullBranch(c, h.DB, "osu").Table("order_service_used osu")
// ĐÚNG — tham số 3 là tên bảng THẬT, tham số 4 mới là alias
tenant.ScopedDBNullBranch(c, h.DB, "order_service_used", "osu")
```

`applyNullBranchScope` chỉ thêm vế khi `tenantTables[table]` / `branchTables[table]` = true. Tên
không có trong bảng đăng ký ⇒ **trả nguyên `db`, không thêm vế nào, không lỗi, không cảnh báo**.
Hệ quả kép:

1. **Lỗ phân quyền chi nhánh.** Báo cáo trả dữ liệu MỌI chi nhánh bất kể quyền người xem. Đo cụ
   thể (user chỉ có chi nhánh 1, cửa sổ tháng): **2.688 khách / 7,80 tỷ** → đúng ra là
   **258 khách / 1,54 tỷ**. Không ai phát hiện được từ màn hình vì số vẫn ra, vẫn cộng đúng.
2. **Mất chỉ mục.** Chỉ mục nghiệp vụ của repo hầu hết dẫn đầu `company_id`
   (`idx_osu_appt_day_cover (company_id, isLock, created_day, …)`). Query không còn vế
   `company_id` ⇒ không range-scan được ⇒ `Table scan` toàn bảng, kể cả khi bộ lọc ngày chỉ khớp
   972 dòng.

⚠⚠ **ĐÍNH CHÍNH SỐ ĐO (2026-09-09, sau khi user chỉ ra bench sai đối tượng).** Bảng dưới đây đo
trên bench clone từ **DB dev** — mà dev THIẾU `idx-order_service_used-created_day`, chỉ mục MỘT CỘT
mà DB khách (dựng từ migration) CÓ. Đo lại trên bench **dựng từ migration**, cùng khối lượng
674.040 osu / 586.524 `isLock=1`, cùng cửa sổ tháng, bộ đếm SESSION (đối chứng `SELECT 1`=14,
`COUNT(*)`=674.042):

| | dev-shape (số đã báo) | **migration-shape (số THẬT của khách)** |
|---|---|---|
| TRƯỚC vá (không scope) | 1.368.838 dòng | **15.403 dòng** |
| SAU vá @1 chi nhánh | 2.868 | **9.283** |
| SAU vá @toàn chi nhánh | 30.660 | **15.403** |

`EXPLAIN` trên hình dạng khách: câu KHÔNG scope vẫn chạy
`Index range scan (MRR) on osu using idx-order_service_used-created_day` — **không hề có table
scan**. Tức phần "chậm" tôi báo là **hiện vật của DB dev**, phóng đại ~89 lần ở vế trước.

⇒ **Phần CÒN ĐÚNG**: lỗi scope là bug PHÂN QUYỀN thật (user 1 chi nhánh thấy 2.688 khách/7,80 tỷ
thay vì 258/1,54 tỷ) — bản vá vẫn cần và vẫn đúng.
⇒ **Phần SAI**: lợi ích hiệu năng ở khách chỉ là 15.403 → 9.283 dòng (1,66×), KHÔNG phải 477×.
Endpoint này chưa bao giờ chậm ở khách vì lý do đó.

Bảng gốc (giữ lại để đối chiếu, KHÔNG dùng làm số của khách):

| Ca (dòng InnoDB · median) — **dev-shape** | Trước | Sau @1 chi nhánh | Sau @toàn chi nhánh |
|---|---|---|---|
| 1 ngày | 1.452.169 · 791ms | **614 · 2ms** | **2.650 · 9ms** |
| tháng này (mặc định FE) | 1.368.838 · 844ms | **2.868 · 8ms** | **30.660 · 43ms** |
| tháng trước | 1.472.805 · 1,10s | **127.836 · 310ms** | **252.180 · 565ms** |
| 1 năm | 3.174.719 · 10,58s | **284.342 · 821ms** | **1.634.138** |
| KHÔNG lọc ngày | 3.708.161 · 15,68s | **290.448 · 792ms** | **1.651.579** |

`EXPLAIN` 1 ngày: `Table scan on osu (rows=674040, 400ms)` →
`Index range scan (Multi-Range Read) on osu using idx_osu_appt_day_cover over (company_id = 1 AND
isLock = 1 AND 1788627600 <= created_day <= 1788713999)` — 12,3ms.

⚠ **Ô "1 năm @toàn chi nhánh" bỏ trống cột ms là cố ý**: máy lúc đo có phiên khác chạy bench, cùng
một câu đo được 15s · 18s · 50s. Phải quay về đại lượng bất biến — A/B cách ly bằng
`Handler_read_*` (hiệu chuẩn `SELECT 1`=0, `COUNT(*)`=674.042): cũ **1.547.991** dòng → mới
**816.390** (−47%), và `IGNORE INDEX (idx_osu_appt_day_cover)` cho **đúng 816.390** ⇒ chênh lệch
thời gian đọc được lúc đó là nhiễu máy, KHÔNG phải regression của plan. Đừng kết luận "vá xong
chậm hơn" từ ms trên máy load cao.

**Cách đối chiếu kết quả cho một bản vá SCOPE** (bản vá cố ý đổi số người dùng thấy, nên không so
thẳng được): chọn cấu hình mà scope là **no-op ngữ nghĩa** — ở đây mọi dòng đều `company_id=1`,
`branch_id` ∈ 1..17, và user "toàn chi nhánh" có đủ 17 quyền ⇒ 5/5 ca khớp **byte-for-byte**. Rồi
chạy lại ở cấu hình 1 chi nhánh để chứng minh phép so CÓ răng: 5/5 ca **phải lệch**.

**Quét toàn repo (2026-09-09) — ĐÃ RÀ HẾT VÀ VÁ.** 1.004 điểm gọi `ScopedDB*` có tên bảng dạng
literal (+28 điểm truyền BIẾN, không kiểm tĩnh được — xem dưới). 19 tên không nằm trong registry,
đối chiếu `information_schema` chia làm 3 nhóm:

| Nhóm | Kết luận |
|---|---|
| 11 tên KHÔNG có cột scope (`branch`, `company`, `source`, `user`, `user_profile`, `material`, `campain_event`, `coupon_catagory`, `customer_care_type`, `customer_category`, `group_source`) | no-op ĐÚNG, giữ nguyên |
| 2 tên không phải bảng (`canary_scope` = fixture test, `medical_record`) | xem nợ dưới |
| **6 tên là bảng THẬT CÓ cột scope ⇒ mất scope im lặng** | **đã vá** |

Sáu chỗ vá: `order_service_used` (bị truyền alias `"osu"`) + 5 bảng chưa từng đăng ký —
`bank_accounts` (company_id, 5 chỗ) · `gcal_account` (company_id, 2) · `gcal_subscription`
(company_id, 2) · `med_route` (company_id, 3) · `warehouse` (branch_id, 1).

Hai chỗ có hậu quả thật, không chỉ "phòng thủ theo chiều sâu":

- **`med_route` — guard super-user bị bỏ qua** (đã bỏ, D15 — sự cố xảy ra khi hệ thống còn trục
  `company_id`, cột này đã xoá khỏi schema story-62). `loadForMutate` tìm dòng company-level bằng
  helper company-only TRƯỚC, không thấy mới fallback sang seed quốc gia (`company_id IS NULL`)
  và chỉ cho super-user. Helper là no-op ⇒ câu đầu khớp luôn cả seed NULL ⇒ **user thường sửa/xoá
  được danh mục đường dùng thuốc quốc gia** (10/10 dòng trên DB dev là seed NULL). Bài học chung vẫn
  áp dụng cho trục `branch_id` hiện tại: bảng chưa đăng ký `branchTables` → mọi helper scope no-op.
- **`bank_accounts`** (đã bỏ, D15 — cùng bối cảnh company_id cũ) — 5 điểm gọi đều là list/get/update/delete theo id, tức đúng khuôn mà Hard
  Rule bắt buộc scope; no-op ở đây là IDOR xuyên tenant.

⚠ **Có cột `branch_id` KHÔNG tự động nghĩa là phải vào `branchTables`.** `gcal_subscription` cố ý
CHỈ đăng ký company: `branch_id` ở đó là THAM SỐ của đăng ký ("đồng bộ lịch hẹn của chi nhánh
nào"), không phải chi nhánh sở hữu — thêm vào `branchTables` sẽ làm chính chủ mất đăng ký của mình
khỏi danh sách ngay khi đổi chi nhánh đang hoạt động. Quyết định này được neo bằng test và đã
mutation-verify theo cả HAI chiều (thêm nhầm ⇒ đỏ; bỏ sót `warehouse` ⇒ cũng đỏ).

⚠ **Blast radius của `warehouse` rộng hơn 1 handler**: `config/indexeddb/handler/sync.go` truyền
`cfg.Table` (BIẾN) vào cùng helper, và bảng `warehouses` nằm trong danh sách master-data đồng bộ
xuống Dexie ⇒ FE nay chỉ cache kho của chi nhánh đang hoạt động (+ kho `branch_id IS NULL`). Kiểm
lại luồng chuyển kho liên chi nhánh trước khi deploy. Bảng `warehouse` trên DB dev đang 0 dòng nên
KHÔNG đo được ảnh hưởng thật.

**Lưới chặn cả lớp lỗi**: `pkg/tenant/registry_coverage_test.go` quét toàn bộ `internal/`+`pkg/`,
bắt mọi tên literal truyền cho `ScopedDB*` phải hoặc nằm trong registry, hoặc nằm trong
`khongCoCotScope` kèm LÝ DO. Thêm điểm gọi mới với tên lạ ⇒ đỏ ngay kèm đường dẫn file. Test có
sẵn 2 mốc đối chứng (thấy ≥50 tên, và phải thấy `"customer"`) để regex hỏng không thành "xanh giả".
28 điểm gọi truyền BIẾN nằm ngoài tầm test này — nếu cần chặn tiếp thì phải chuyển sang kiểm lúc
chạy (helper panic khi tên không có trong cả 2 registry lẫn allowlist), chưa làm.

**Nợ phát hiện kèm, KHÔNG phải lỗi scope**: `crm/attachment/handler/upload.go` nhánh
`ref_type = "medical_record"` truy vấn `.Table("medical_record")` — bảng đó KHÔNG tồn tại (tên
thật là `emr_medical_record`) ⇒ nhánh này luôn lỗi runtime. FE hiện không gửi `ref_type` đó nên
chưa ai chạm; sửa tên bảng sẽ biến nhánh chết thành nhánh sống, cần người quyết.

**Cách neo bằng test** (`service_type_stats_scope_test.go`): GORM DryRun, không đụng DB nào; test
gọi CHÍNH `buildServiceTypeStatsQuery` của handler — trước đó hai câu (gom nhóm + đếm khách) chép
tay giống nhau nên lỗi nằm ở cả hai, đã gom thành một builder. Test dựng lại query trong test sẽ
xanh kể cả khi handler mất scope (bài học đã ghi ở `crm/reminder/handler/list_query_test.go`).
Kèm 1 test đối chứng khẳng định dạng SAI (`"osu"` làm tên bảng) sinh SQL KHÔNG có vế scope nào —
vừa chứng minh phép kiểm có hiệu lực, vừa ghi lại cơ chế. Mutation-verified: trả lại dạng SAI ⇒
2 test đỏ.

## Cột ngày KHÔNG có chỉ mục ⇒ chỉ vế `COUNT` đau, vế `SELECT` vẫn nhanh — `note/get-all-note`

Đo 2026-09-09, bench 2.527.503 ghi chú / 76.999 khách dựng từ schema thật. Bảng `note` chỉ có PK +
4 chỉ mục MỘT cột kiểu `fk-*`; `created_at` không có chỉ mục nào.

Điều đáng học ở ca này: **hai vế của `pagination.Paginate` phản ứng NGƯỢC nhau**, và đo gộp ở tầng
endpoint sẽ đổ lỗi nhầm chỗ. Bộ đếm SESSION `Handler_read_*` (đối chứng `SELECT 1`=0,
`COUNT(*)` toàn bảng = 2.527.505):

| Câu | Trước | Sau |
|---|---|---|
| `SELECT` trang 1, lọc tháng | **20 dòng** | 20 — vốn ĐÃ tốt |
| `COUNT` derived, lọc tháng | **2.527.504 dòng** | **1.897** |
| `SELECT` trang 1, KHÔNG lọc | 20 | 20 |
| `SELECT` lọc `status` (JOIN customer) | 100 | 100 |

`ORDER BY note.id DESC LIMIT 20` cho vế SELECT early-exit trên PK ⇒ nó *luôn* rẻ, kể cả không có
chỉ mục ngày nào. Vế `COUNT` derived thì không bao giờ early-exit được ⇒ **một mình nó gánh toàn bộ
chi phí**. Ở tầng endpoint: lọc tháng 2,81s → **2ms**, tháng trước 2,70s → **5ms**, tháng + lọc
`status` 2,45s → **7ms**. Chỉ mục: `note (company_id, created_at)`, migration
`20260909115816_note_company_created_index.up.sql`. Giá **+63 MB / 2,53M dòng** (193→256 MB).

Gate `FORCE INDEX` ĐẠT sạch: (a) không index 2.527.570 · (b) auto **1.897** · (c) FORCE **1.897** —
(b) = (c) nên phần thắng đến từ chính chỉ mục, không phải cost model đổi plan.

### ⚠ Ca CÒN LẠI: `_keyword` khớp ít ⇒ vế SELECT quay lại quét cạn bảng

Chỉ mục ngày KHÔNG chữa được ca này, và nó là ca người dùng gõ hằng ngày (FE luôn kèm dải ngày):

```
-> Index range scan on note using fk_note-company_id over (company_id = 1) (REVERSE)  rows=1.2e+6
   Filter: created_at BETWEEN … AND note LIKE '%zzqq9%'      actual time=2667ms  rows=0
```

Optimizer BỎ chỉ mục ngày để cược vào early-exit của `LIMIT 20` trên `fk_note-company_id` quét
ngược. Từ khoá khớp nhiều ⇒ cược thắng; khớp ít/không khớp ⇒ đi hết 1,2 triệu dòng. Vế `COUNT` của
chính ca đó thì ĐÃ ăn chỉ mục (25,9ms).

**ĐÃ VÁ (2026-09-09) bằng `USE INDEX` có CỔNG theo độ rộng cửa sổ.** Điều đáng học: hint không
phải "bật là tốt" — nó biến chi phí BIMODAL thành HẰNG SỐ theo số dòng trong cửa sổ ngày, nên chỉ
lãi khi cửa sổ hẹp. Đo median 3 lượt, `EXPLAIN ANALYZE`:

| cửa sổ | dòng trong cửa sổ | từ khoá HIẾM | từ khoá PHỔ BIẾN |
|---|---|---|---|
| 1 tháng (mặc định FE) | 1.896 | 3.204ms → **9,4ms** (340×) | 1,3ms → 8,8ms (phạt 7,5ms) |
| 92 ngày | 39.786 | 2.925ms → 190ms (15×) | 1,2ms → 185ms (phạt 185ms) |
| 1 năm | 296.755 | 3.560ms → 690ms (5,2×) | 1,3ms → 680ms (**phạt 680ms**) |

⇒ cổng ở **31 ngày** (đúng cửa sổ mặc định FE): đổi 7,5ms lấy 3,2 GIÂY. Rộng hơn thì phần phạt cho
ca may vượt phần cứu cho ca xui ⇒ để optimizer tự quyết, KHÔNG cắt kết quả. Ở tầng endpoint:
"tháng này + từ khoá hiếm" **5,42s / 2.529.399 dòng → 17ms / 3.792 dòng**. Đối chiếu 11 ca
khớp byte-for-byte. `maybeHintDateIndex` trong `crm/note/handler/find.go`.

⚠ **`FORCE INDEX` bị LOẠI, `USE INDEX` được chọn** — khác nhau ở ĐUÔI phân bố: bằng nhau ở cửa sổ
hẹp, nhưng ở cửa sổ ~toàn bảng (5 năm) `FORCE` đọc **4.996.165** dòng, TỆ HƠN cả không hint
(2.527.504), vì bị ép quét chỉ mục rồi tra dòng; `USE` cho optimizer quyền bỏ hint nên thoái lui êm
(2.527.506). Luật: hint nào cũng phải đo ở cửa sổ RỘNG NHẤT người dùng chạm được, không chỉ ở cửa
sổ mặc định.

⚠ Hint chứa TÊN chỉ mục trong SQL. Đổi tên ⇒ MySQL lỗi 1176 (lớn tiếng, không hỏng im lặng), và
`TestNoteDateIndexHintKhopVoiMigration` quét thư mục `migrations/` bắt ngay trong repo. Đọc giá trị
dải lọc phải qua `query.GetOperatorValue` (export 2026-09-09) — tự ghép `filter[k][op]` bằng tay sẽ
trượt format `k[op]` và im lặng trả rỗng ⇒ hint không bao giờ bật.

### ⚠ Biến thể đã LOẠI: đổi `ORDER BY id` sang `ORDER BY created_at`

Đổi thứ tự sắp xếp sẽ cho chỉ mục `(company_id, created_at)` phục vụ TRỌN cả lọc lẫn sắp xếp ⇒
early-exit mọi ca kể cả có từ khoá. **LOẠI vì đổi kết quả người dùng thấy**: đếm trên 842.500 dòng
THẬT của DB dev có **52.249 cặp liền kề (6,2%)** mà `id` tăng nhưng `created_at` giảm — hai thứ tự
KHÔNG tương đương. Luật rút ra: trước khi coi `id` là proxy của thời gian, ĐẾM số cặp nghịch, đừng
suy luận từ "id tự tăng".

### Hai cái bẫy ĐO gặp ở lượt này

1. **`pagination.Paginate` chạy câu `COUNT` qua biến GLOBAL `db.DB`**, không qua `*gorm.DB` được
   truyền vào (`pkg/pagination/pagination.go:39`). Spike gọi thẳng handler phải gán
   `appdb.DB = <db bench>`, không thì nil-pointer panic ngay câu COUNT đầu tiên — và nếu vô tình
   để nó trỏ DB dev thì spike đếm trên DB THẬT mà không ai biết.
2. **`SHOW GLOBAL STATUS LIKE 'Innodb_rows_read'` là bộ đếm TOÀN SERVER.** Spike mẫu
   (`spike/dashboard_scale`) dùng nó; máy có phiên khác chạy MySQL thì delta lẫn dòng của họ. Ở
   lượt này nó tạo ra một "regression" MA: ca `status` không-date đọc 1.502.817 → 3.005.406 dòng,
   nhưng A/B cách ly bằng `FLUSH STATUS` + `performance_schema.session_status` cho **1.537.622 dòng
   Y HỆT nhau** cả hai bên. Kết luận nào dựa trên delta của bộ đếm GLOBAL đều phải xác nhận lại
   bằng bộ đếm SESSION trước khi báo.

## Chỉ mục `company_id` một cột kéo optimizer vào index_merge VÔ DỤNG — `treatment-course-technical`

> ⚠ Lịch sử (D15): schema mới không còn `company_id` — mục này chỉ giải thích số đo cũ, không áp dụng cho CRM standalone.

Đo 2026-09-09 trên `GET /api/accountant/treatment-course-technical` (đường LIST — khác nhánh
`?_export=1` đã có trong sổ), bench 1.323.894 dòng dựng từ schema thật.

Hai nút thắt độc lập, một cấu trúc một chỉ mục.

### 1. Bốn lượt quét mỗi lần mở màn

Handler chạy **ba** bảng dẫn xuất riêng trên CÙNG bộ lọc — `SUM(commissionDoctor)`,
`SUM(point)`, và `COUNT(*)` của `pagination.Paginate` — rồi mới tới câu SELECT trang. Đo ca
"tháng này + technical_id": **413 + 494 + 520 + 118ms**.

Gộp ba câu aggregate thành MỘT (`SUM … , SUM … , COUNT(*)` trên cùng bảng dẫn xuất) rồi
`PaginateWithTotal`. Điều kiện gộp được: **không có `GROUP BY` nào**, nên `COUNT(*)` trên bảng
dẫn xuất đúng bằng con số `Paginate` tự đếm. Đây là lần thứ 2 trong ngày khuôn này xuất hiện
(lần 1: `report-service-used`).

### 2. `technical_id` + dải ngày ⇒ `Intersect rows sorted by row ID` với chỉ mục `company_id`

```
-> Filter: (company_id = 1 and branch_id = 1 and confirm_time <= …)   actual time=308ms rows=0
    -> Intersect rows sorted by row ID   rows=19660
        -> Index range scan using fk-…-technical_id over (technical_id = 154)   rows=345
        -> Index range scan using fk-…-company_id  over (company_id = 1)   rows=659292   ← vô dụng
```

Bảng có `fk-…-technical_id` (1 cột) và `idx-tct-technical_confirm (confirm_time, technical_id)` —
cái sau **dẫn đầu bằng cột RANGE** nên `technical_id` không làm tiền tố lọc được. Optimizer bù bằng
index_merge, và nhánh thứ hai nó chọn là `company_id` — ở DB-per-customer thì **mọi dòng cùng
company_id**, quét 659.292 mục chỉ mục để không loại được gì. Câu trả về **0 dòng** vẫn tốn 308ms.

⚠⚠ **ĐÍNH CHÍNH (cùng ngày, sau khi đọc migration): đây KHÔNG phải "thiếu chỉ mục" mà là
SCHEMA DRIFT.** Bản đầu của mục này kết luận "thêm chỉ mục `(technical_id, confirm_time)`" — sai.
Chạy TOÀN BỘ migration vào DB rỗng rồi soi `information_schema` cho thấy:

| | `idx-tct-technical_confirm` | có `index-confirm_time`? |
|---|---|---|
| DB migrate-từ-đầu | **`(technical_id, confirm_time)`** — ĐÚNG sẵn | không (không có trong baseline) |
| DB dev | **`(confirm_time, technical_id)`** — ĐẢO NGƯỢC | có |

Baseline khai `(technical_id, confirm_commission, confirm_time)`; squash `20260814091602` DROP cột
`confirm_commission` ⇒ MySQL tự co chỉ mục còn `(technical_id, confirm_time)`. Dev đã lệch khỏi
đường đó (và mọc thêm `index-confirm_time` không có trong migration nào). Chỉ mục mới tôi định
thêm vì thế là **bản TRÙNG** trên mọi DB build đúng.

**Bài học chuyển được**: bench clone từ `mysqldump` của DEV thì mang theo **cả drift của dev**.
Trước khi kết luận "thiếu chỉ mục X", đối chiếu với DB dựng từ migration — nếu hai bên khác nhau
thì việc phải làm là **sửa drift**, không phải thêm chỉ mục. Cách kiểm rẻ nhất: chạy một test
integration bất kỳ với `TEST_MYSQL_DSN` trỏ DB rỗng (testhelpers apply toàn bộ migration) rồi
`SELECT … FROM information_schema.STATISTICS`.

Cách sửa đã ship: migration so **DANH SÁCH CỘT** (không so tên) rồi drop+recreate cho đúng thứ tự;
no-op trên DB đã đúng. Gate:

| | dòng InnoDB (bộ đếm SESSION) |
|---|---|
| (a) không có chỉ mục | **1.363.029** (nhiều hơn cả số dòng của bảng) |
| (b) có, optimizer tự chọn | **181** |
| (c) có + `FORCE INDEX` | **181** |

(b) = (c) ⇒ phần thắng đến từ chính THỨ TỰ CỘT. Đối chứng: `SELECT 1` ≈ 0,
`COUNT(*)` = 1.323.896. Migration `20260909135142` — sửa thứ tự cột, KHÔNG thêm chỉ mục mới
(nên không tốn thêm dung lượng), và gỡ luôn `idx_tct_technical_confirm2` nếu DB nào đã lỡ chạy
bản đầu của file đó.

### ⚠ ĐO LẠI TRÊN HÌNH DẠNG KHÁCH — phần chỉ mục KHÔNG có giá trị ở khách

Bench gốc clone từ dev, mang theo drift `(confirm_time, technical_id)`. Dựng lại bằng
`cmd/migrate` (nơi chỉ mục vốn ĐÃ đúng), cùng 1.323.894 dòng, toàn chi nhánh:

| Ca | Trước | Sau | Ghi chú |
|---|---|---|---|
| tháng này (mặc định FE) | 80ms | **33ms** | |
| tháng trước | 4.266ms | **1.519ms** | |
| KHÔNG lọc ngày | 7.436ms | **422** | guard chặn |
| **tháng này + technical_id** | **6ms** | 3ms | **dev-shape đo 632ms — chênh 105×** |
| + category_id | 103ms | **40ms** | |
| + từ khoá CÓ khớp | 193ms | **69ms** | |
| + từ khoá KHÔNG khớp | 235ms | **129ms** | |
| phân trang sâu (trang 500) | 5.618ms | **3.652ms** | |

⇒ Ở khách, **toàn bộ lợi ích đến từ việc gộp 4 lượt quét còn 2** (đều đặn 2,4–2,8×) và từ trần
ngày. Phần chỉ mục **không đóng góp gì** vì khách vốn đã có đúng thứ tự cột — nó chỉ sửa drift cho
DB đời cũ. Con số "878ms → 1ms" trong bảng dev-shape dưới đây là **hiện vật của dev**, đừng trích.

### Kết quả hai việc cộng lại — **dev-shape, KHÔNG phải số của khách**

| Ca | @1 chi nhánh | @toàn chi nhánh |
|---|---|---|
| tháng này (mặc định FE) | 44 → **18ms** | 98 → **33ms** |
| tháng trước | 78 → **34ms** | 3.638 → **1.231ms** |
| KHÔNG lọc ngày | 554 → **177ms** | 6.169 → **2.356ms** |
| **tháng này + technical_id** | 878 → **1ms** | 632 → **5ms** |
| + category_id (join `treatment_course`) | 75 → **19ms** | 115 → **49ms** |
| + từ khoá CÓ khớp | 79 → **18ms** | 165 → **66ms** |
| + từ khoá KHÔNG khớp | 77 → **36ms** | 207 → **106ms** |
| phân trang sâu (trang 500) | 106 → **47ms** | 4.868 → **2.489ms** |

Đối chiếu **16 ca** (8 bộ lọc × 2 chế độ chi nhánh) khớp byte-for-byte; 2 test integration.

### ⚠ Bẫy khi mutation-test một bản vá GỘP `COUNT`

Mutation `COUNT(*)` → `COUNT(DISTINCT sub.point)` **KHÔNG bị bắt** ở bản test đầu, vì seed vô tình
cho mỗi dòng một `point` khác nhau ⇒ hai hàm ra cùng số. Phải seed **≥2 dòng trùng giá trị** của
cột đem ra DISTINCT thì phép kiểm mới có răng. Cùng họ với bài học `MIN`/`MAX` ở
`report/telesales-online` (không bắt được cho tới khi seed khách có NHIỀU lịch): mutation trên hàm
tổng hợp chỉ lộ khi dữ liệu seed **phân biệt được** hai hàm.

### Hai việc user chốt sau đó (2026-09-09)

- **Trần dải ngày**: `confirm_time` gte/lte nay BẮT BUỘC + trần 366 ngày ⇒ 422 tường minh. Guard
  đặt TRƯỚC nhánh `_export=1` nên export cũng bị chặn (xuất >1 năm ⇒ chia nhiều lần). FE
  `clearPaymentTime()` reset về mặc định thay vì `null`. 8 unit test `Deps{DB: nil}`,
  mutation-verified 3 chiều — trong đó có một mutation riêng cho **vị trí** guard (đẩy xuống sau
  nhánh export ⇒ đỏ).
- **Chỉ mục `(confirm_time)` trùng**: giữ 1, bỏ 1. Tên khác nhau tuỳ DB
  (`index-confirm_time` trên dev · `idx-tct-confirm_time` trên bản migrate-từ-đầu — **cả hai DB đều
  có đúng 2 cái**) nên migration phải **tra động theo danh sách cột**, không hardcode tên; và mỗi
  lệnh DROP chỉ chạy khi vẫn còn một chỉ mục `(confirm_time)` khác, để không bao giờ bỏ mất cái
  cuối. Đã thử trên CẢ HAI hình dạng DB, mỗi bên 3 lượt.

### Số đo cuối, đo lại trên bench dựng lại từ schema dev (mang drift)

| Ca | @1 chi nhánh | @toàn chi nhánh |
|---|---|---|
| tháng này (mặc định FE) | → **17ms** | 88 → **37ms** |
| tháng trước | → **26ms** | 4.502 → **1.638ms** |
| KHÔNG lọc ngày | — | 7.556ms → **422** (đã chặn) |
| tháng này + technical_id | → **1ms** | 736 → **3ms** |
| + category_id | → **17ms** | 517 → **45ms** |
| + từ khoá CÓ khớp | → **18ms** | 564 → **71ms** |
| + từ khoá KHÔNG khớp | → **37ms** | 509 → **118ms** |
| phân trang sâu (trang 500) | → **49ms** | 7.514 → **2.846ms** |

Đối chiếu **14 phép so** (7 bộ lọc × 2 chế độ chi nhánh, trừ ca bị guard chặn) khớp byte-for-byte.

### Còn lại

- ~~Phân trang sâu @toàn chi nhánh vẫn 2,85s — chi phí `OFFSET` tuyến tính, chữa được thì phải đổi
  sang keyset (đổi API).~~ **Sai — đo lại 2026-09-16, xem mục ngay dưới.**

### Đo lại phân trang sâu (2026-09-16) — 2,85s KHÔNG phải do `OFFSET`, và keyset không cần

Bench dựng TỪ MIGRATION, 1.320.000 dòng (17 chi nhánh, 24 tháng, tháng trước 56.837 dòng — 13.781 dòng trùng
giây `confirm_time` ⇒ có tie thật). Toàn chi nhánh, `per-page=20`, đo xen kẽ 2 vòng lúc load 2–3, đối chiếu băm
kết quả **44/44 KHỚP** (đảo chiều thì đảo lại trong ứng dụng rồi mới băm). `ApplySort` đã nối `id DESC` sau
`confirm_time DESC` ⇒ thứ tự là TOÀN PHẦN, điều kiện bắt buộc của mọi mẹo dưới đây.

**1. Endpoint trang 1 đã tốn 1,27s** (tháng trước) — trang 500 chỉ 1,36s. Thủ phạm là **câu tổng**
(`SUM commissionDoctor, SUM point, COUNT`): đọc **1.376.838 dòng** = quét trọn bảng, vì 2 cột SUM không nằm
trong chỉ mục nào. Bỏ `LEFT JOIN order_service_used` + `ORDER BY` thừa trong bảng dẫn xuất: 1 tháng 1.292 →
1.149ms, **1 năm 2.020 → 1.180ms** (1.990.080 → 1.320.001 dòng) — vẫn quét trọn bảng. Chữa thật là khuôn
`idx_*_confirm_id_cover` kèm cột SUM đã áp cho 6 bảng hoa hồng/doanh số (CHƯA đo trên bảng này).

**2. `OFFSET` chỉ đau từ trang ~2000**, và đau vì LẬT PLAN chứ không phải tuyến tính: OFFSET 9.980 = `Index
range scan` đọc 20.000 dòng (108–152ms); OFFSET 39.980 = `Sort … limit input to 40000` trên cả bảng, 1.360.001
dòng (1,5–1,7s). Endpoint: trang 2000 2,7s · trang cuối tháng trước 2,75s · 1 năm trang giữa 4,9s · trang cuối
**5,0–6,7s**.

| Câu trang (SQL), toàn CN | V0 hiện tại | V1 tra id trước, câu trong bỏ JOIN | V1f = V1 + `FORCE INDEX` | V2 đảo chiều | V2f = đảo chiều + V1f |
|---|---|---|---|---|---|
| tháng trước trang 2000 | 1.500ms / 1.360.001 | **11ms** / 40.061 | 11ms | 1.360ms (vô dụng) | **4ms** / 16.918 |
| tháng trước giữa | 1.423ms | **8ms** / 28.481 | 8ms | — | — |
| tháng trước trang cuối | 1.549ms | 15ms / 56.890 | 15ms | **3,6ms** / 34 | **0,5ms** |
| 1 năm trang 2000 | 1.643ms | 1.300ms (**optimizer bỏ chỉ mục**) | **11ms** / 40.061 | — | — |
| 1 năm giữa (OFFSET 335.020) | 3.756ms | 1.340ms | **87ms** / 335.101 | — | — |
| 1 năm trang cuối | 6.016ms | 1.357ms | 172ms | **3,5ms** / 38 | **0,5ms** / 77 |

Cả V1/V1f đều CẦN chỉ mục bench `(confirm_time, id, company_id, branch_id)`. Không có nó, V1 đọc y hệt V0 (bộ lọc
chi nhánh phải tra dòng). **Biến thể LOẠI có số đo:**

- **Tra id trước nhưng GIỮ JOIN** (dạng duy nhất `pkg/pagination` làm tổng quát được vì không biết JOIN nào bỏ
  được): 1.332–1.417ms / cùng số dòng V0, kể cả khi có chỉ mục phủ. ⇒ **KHÔNG làm được ở tầng dùng chung.**
- **Đảo chiều không cổng**: trang 500 của 1 năm 139ms → **5.484ms**. Kể cả có cổng "nửa sau", trang 2000 của
  tháng trước (offset đảo 16.837) vẫn 1.360ms. ⇒ đảo chiều chỉ thắng SÁT CUỐI.
- **Keyset**: không đo lại, vì V1f đã đưa mọi vị trí về ≤172ms mà giữ nguyên API số trang của FE (374 bảng).

⇒ Chữa phân trang sâu = **theo từng endpoint**: (1) chỉ mục phủ cột bộ lọc + sắp xếp, (2) câu trong không JOIN,
(3) `FORCE INDEX` có cổng dải rộng, (4) tuỳ chọn đảo chiều khi trang nằm ở phần cuối. KHÔNG phải một thay đổi ở
`pkg/pagination`.

### Bản vá đã ship (2026-09-16) — số đo xen kẽ HEAD/bản vá

Migration `20260916071451` (chỉ mục phủ, **59,7MB**/1,32 triệu dòng) + handler: câu tổng bỏ `LEFT JOIN
order_service_used` và chạy trên chỉ mục phủ (hint có cổng) · câu trang tra id trước + đọc ngược khi trang ở nửa
sau · từ khoá qua `treatmentkw` (ngram) với `Options{SkipPlanCode: true}`.

| Ca (endpoint, gọi thẳng handler) | HEAD | Bản vá |
|---|---|---|
| 1 ngày | 28ms | **4ms** |
| tháng này (mặc định FE) | 190–302ms | **12–17ms** |
| tháng trước | 1.301ms | **23ms** |
| 1 năm | 1.948ms | **226ms** |
| trang 500 / 2000 / cuối (tháng trước) | 1.380 / 2.594 / 2.631ms | **24 / 26 / 21ms** |
| 1 năm: trang 500 / 2000 / giữa / cuối | 2.032 / 3.501 / 4.083–5.869 / 4.673–7.454ms | **203 / 211 / 290 / 199ms** |
| 1 năm trang cuối, `per-page=100` | 4.673ms | **205ms** |
| từ khoá hiếm 1 năm (tên / mã KH / mã LT / mã DV / không khớp) | 23.602 / 22.061 / 22.700 / 22.090 / 21.329ms | **308 / 360 / 207 / 259 / 182ms** |
| 1 chi nhánh: tháng trước / 1 năm / trang sâu / 1 năm trang cuối | 58 / 153 / 90 / 424ms | **19–31 / 126 / 10 / 118ms** |

**Đối chiếu**: băm sha1 nguyên body, **192 phép so khớp, 0 lệch** (132 lượt đầu + 60 lượt sau bản sửa JOIN).
8 test (fake driver) + **mutation 8/8 đỏ**.

⚠ **Lỗi mà đối chiếu body bắt được, test không bắt**: bỏ `LEFT JOIN order_service_used` khỏi câu tổng làm
đường LIKE cũ vỡ (`order_service_used.code` là một vế tìm kiếm) ⇒ **12/144 ca trả HTTP 500**. Toàn bộ 12 ca đều
là từ khoá PHỔ BIẾN ("Nguyễn") — vượt trần 10.000 id nên rơi về đường cũ; từ khoá hiếm đi đường nhanh nên không
lộ. Bài học chuyển được: khi thêm đường nhanh có đường lui, ca kiểm phải gồm **từ khoá vượt trần**, nếu không
chỉ đo đúng nửa đường đi.

**Còn lại (có số đo, chưa vá)**:
- Từ khoá vượt trần id bị PHẠT vì tra ngram xong mới rơi về đường cũ: "Nguyễn" tháng này 471 → **711ms**, trang
  sau 1 năm 5.183 → **6.077ms**, + nhóm liệu trình 5.564 → **6.111ms**. Cùng khuôn đã chấp nhận ở 6 bảng hoa
  hồng/doanh số.
- Người dùng TỰ chọn sắp xếp (`sort=`) thì trang sâu vẫn đi đường cũ: 1 năm trang cuối + ASC 7.021–7.290 →
  4.905–5.726ms (nhanh hơn nhờ câu tổng, không nhờ phân trang).
- Lọc nhóm liệu trình (JOIN `treatment_course`) không dùng được cả hint lẫn tra-id-trước: tháng này 224 → 219ms.

### Lượt 3 (2026-09-16) — đường tra-id-trước thành của DÙNG CHUNG, phủ thêm 6 endpoint

Hai việc, cùng ngày:

**1. Người dùng tự chọn sắp xếp nay cũng dùng được đường nhanh.** Cổng cũ đẩy mọi request có `sort=` về đường
cũ, chỉ vì không muốn chép lại luật thứ tự. Cách chữa đúng là tách luật đó ra: `query.SortClause` trả về ĐÚNG
mệnh đề mà `ApplySort` áp (kể cả nhánh từ chối + ghi log), còn `query.ReverseOrderClause` đảo chiều từng vế cho
bản đọc ngược; `ApplySort` nay chỉ là `q.Order(SortClause(...))` nên 73 chỗ gọi cũ không đổi hành vi. Đo lại
@1.320.000 dòng toàn CN: **1 năm trang cuối + sắp xếp tăng dần 5.298–6.346 → 432–438ms**; trang cuối thường
9.437–11.131 → 202–236ms. Đối chiếu HEAD/bản vá **72/72 khớp**.

⚠ `ReverseOrderClause` trả `""` khi gặp vế không phải `<cột> [ASC|DESC]` (biểu thức, hàm) — caller phải hiểu là
"không đảo được" và đi chiều xuôi. Đoán bừa ở đây = trả nhầm dòng, không có lỗi nào bật ra.

**2. Gói `accountant/revenue_staff` có `ListPage`** — làm trọn phần phân trang cho 6 màn dùng chung nó
(`revenue-{doctor-consultant,telesale,direct-sale}` + `commission-{doctor-consultant,telesale}` +
`commission_direct-sale`), nên mỗi handler chỉ đổi 2 dòng. Bench riêng `order_commission_telesale` 600.000 dòng
(17 chi nhánh, 24 tháng), đo xen kẽ HEAD/bản vá, toàn chi nhánh:

| Ca | HEAD | Bản vá |
|---|---|---|
| Không lọc ngày, `page=2000` | 1.140ms | **131ms** |
| Không lọc ngày, trang giữa | 3.114ms | **188ms** |
| Không lọc ngày, trang cuối | 5.813–6.255ms | **121ms** |
| Trang cuối, `per-page=100` | 5.540ms | **127ms** |
| Trang cuối + sắp xếp ASC | 2.663ms | **305ms** |
| Trang vượt tổng (không chạm DB) | 2.557ms | **118ms** |
| 1 năm `page=500` / `page=2000` | 746 / 1.021ms | **84–88 / 92ms** |
| Tháng `page=500` / trang cuối | 96 / 85ms | **5,5 / 3,3ms** |
| 1 chi nhánh, trang sâu | 230ms | **76ms** |

Đối chiếu body **76/76 khớp**. Trang 1 và các ca nông không đổi (đường cũ, sai số đo).

⇒ 5 endpoint còn lại dùng ĐÚNG hàm đó nên ăn theo, nhưng **chưa đo riêng từng cái** — bảng trên là của
`commission-telesale`.

**3. Từ khoá VƯỢT TRẦN id: `IN (SELECT …)` thay cho `LIKE` xuyên 3 JOIN.** Trần 10.000 id là để chặn ca xấu,
nhưng từ khoá phổ biến ("Nguyễn" = 21.428 khách) luôn vượt trần ⇒ luôn rơi về LIKE. Viết lại 3 vế đó thành
truy vấn con thì không còn khái niệm trần mà vẫn đúng tập kết quả:

| Câu tổng, "Nguyễn" | LIKE (cũ) | `IN (SELECT …)` | `EXISTS` |
|---|---|---|---|
| Toàn CN, 1 năm | 4.532ms / 3.330.238 dòng | **1.788ms** / 3.190.084 | 6.031ms (LOẠI) |
| Toàn CN, không lọc ngày | 7.264ms / 5.280.001 | **1.879ms** / 3.840.005 | 10.195ms (LOẠI) |
| 1 CN, 1 năm | **385ms** / 134.901 | 503ms / 1.271.239 (TỆ) | 647ms (LOẠI) |
| 1 CN, không lọc ngày | **435ms** | 542ms (TỆ) | 897ms (LOẠI) |

⇒ cổng BA lớp, mỗi lớp có số đo: (a) chỉ khi thất bại VÌ VƯỢT TRẦN — thất bại vì chỉ mục ngram hỏng thì subquery
(cũng dùng MATCH) chết y hệt; (b) chỉ khi xem NHIỀU chi nhánh; (c) chỉ khi dải ngày > 31 ngày — dải tháng đo ra
466 (LIKE) so với **1.002ms** (subquery). Endpoint, toàn CN: từ khoá phổ biến + trang sau 1 năm 5.187 →
**3.130ms**, + nhóm liệu trình 5.030 → **1.798ms**. Đối chiếu body 16/16 khớp.

⚠ **Lỗi lớp (a) do test bắt được, không phải số đo**: bản đầu cho mọi ca "đường nhanh trả nil" đi subquery ⇒ DB
thiếu chỉ mục ngram thì cả request chết HTTP 500, trong khi trước đó nó vẫn chạy bằng LIKE. Bản sửa tách hai lý do (vượt trần / thiếu chỉ mục) thay vì gộp một nhánh.

**Còn lại sau cả 3 việc**: từ khoá phổ biến vẫn trả giá cho lượt tra ngram rồi mới rơi về đường lui — toàn CN
dải tháng 466 → 653ms, 1 CN trang sau 1 năm 2.064 → 2.385ms. Chữa được thì phải đoán TRƯỚC là từ khoá sẽ vượt
trần (thống kê tần suất), chưa đo.

## ⛔ Bench clone từ DEV mang theo DRIFT của dev — rà toàn schema 2026-09-09

Bench hiệu năng luôn dựng bằng `mysqldump --no-data` từ **DB dev**. Nếu dev đã lệch khỏi schema mà
MIGRATION sinh ra, thì bench đo trên một hình dạng **không tồn tại ở khách hàng nào**. Trong MỘT
phiên, việc này đẻ ra hai sai lầm thật:

1. Kết luận "`treatment_course_technical` thiếu chỉ mục `(technical_id, confirm_time)`" — DB
   dựng-từ-migration ĐÃ có, chỉ dev bị **đảo thứ tự cột** dưới cùng một cái tên.
2. Đăng ký `warehouse` vào `branchTables` vì dev có cột `branch_id` — DB dựng-từ-migration chỉ có
   **`branchId`** (camelCase). Suýt ship `warehouse.branch_id = ?` ⇒ **lỗi 1054 trên MỌI DB khách**,
   chết cả `ListWarehouse` lẫn đồng bộ master-data xuống IndexedDB.

⇒ **Luật: trước khi kết luận "thiếu chỉ mục/thiếu cột", đối chiếu với DB dựng TỪ MIGRATION.**
Gate chạy lại được: `skill/skills/_tooling/verify-schema-drift.sh <db>` — dựng DB tham chiếu bằng
`cmd/migrate`, so 4 chiều (bảng · cột · chỉ mục · FK), tự drop DB tham chiếu, có dòng đối chứng số
bảng đọc được để snapshot rỗng không ra "0 khác biệt".

### Kết quả rà `crm_dev_db` (dev) vs migration, 2026-09-09

| Chiều | dev thừa | dev thiếu |
|---|---|---|
| Bảng | 5 (`service_fail`, `service_material_treatment`, `supplier_category`, `tooth_bio_service`, `tooth_bio_status`) | 1 (`document`) |
| Cột (tên cột hẳn) | 21 trên 14 bảng | 1 (`supplier.supplier_category`) |
| Cột (cùng tên, KHÁC định nghĩa) | 30 | |
| Chỉ mục | 30 | 39 |
| FK | 6 | 9 |

**Lớp nguy hiểm nhất — chỉ mục CÙNG TÊN nhưng KHÁC cột: đúng 1 ca** (`idx-tct-technical_confirm`),
đã vá. Đây là lớp không thể phát hiện bằng cách nhìn tên chỉ mục, và là thứ gate trên soi riêng.

Vài khác biệt định nghĩa đáng nhớ (dev ⟵⟶ migration): `appointment.appointment_date`
**int ⟵⟶ bigint** (dev còn dính trần 2038, khách thì không) · `customer.status`
**tinyint(1) ⟵⟶ smallint** (giá trị >127 chạy được ở khách, vỡ ở dev) · `customer.zalo/viber`
varchar(25) ⟵⟶ (100) · `user_profile.email` (100) ⟵⟶ (255) · mặc định lệch:
`appointment_type.appointmentFromZalo` 0⟵⟶1, `source.isZaloSource` 0⟵⟶1.

### Đã kiểm và KHÔNG phải lỗi

- **Code có phụ thuộc cột chỉ-dev-có không?** Quét toàn repo theo dạng **qualified `bảng.cột`** cộng
  với entity GORM có `TableName()` khớp: sau khi gỡ `warehouse`, **0 điểm**. (Đối chứng scanner:
  `warehouse.branchId` và `payment.customer_id` đều tìm thấy entity ⇒ phép quét có chạy.)
  ⚠ Grep trần theo TÊN CỘT là vô nghĩa ở đây: `service_id`/`branch_id`/`owner_id` cho 318–808 hit
  giả vì trùng tên qua hàng chục bảng — bắt buộc phải ràng buộc bảng.
- **4 cột NOT NULL ở migration nhưng nullable ở dev** (`treatment_course_assistant.branch_id/
  confirm_time/customer_id/service_used_id`): entity khai `not null` + kiểu non-pointer và điểm
  INSERT truyền đủ ⇒ lành. Entity khớp MIGRATION, dev mới là bên dễ dãi.
- **Hai migration chỉ mục còn lại ship cùng ngày KHÔNG trùng**: `note (company_id, created_at)` và
  `payment (customer_id)` đều không có bản tương đương trên DB dựng-từ-migration.

## `dimensionStatistics` — MỘT hàm, 8 endpoint `*-statistics`, 7 lượt quét cùng dải ngày

`source · pathology · province · country · category · page-facebook · campain-event ·
interested`-statistics đều gọi chung `dimensionStatistics` (`crm/report/handler/crm.go`). Câu SQL
là một bảng dimension `m` LEFT JOIN **7 bảng dẫn xuất** — A (lead), E (lịch mới), B/C/D (3 chỉ số
lịch hẹn), và R/S/T/U (4 chỉ số tiền) — mỗi cái tự quét lại `appointment`/`payment` trên **cùng
một dải ngày**, chỉ khác điều kiện lọc. Đúng khuôn "N subquery độc lập" đã gặp 5 lần trước đó.

### Bản vá: gộp theo bảng nguồn, không gộp theo chỉ số

- **B/C/D → 1 bảng `BCD`**: `COUNT(...)` + `MAX(appointment.status IN (2,3)) AS den` +
  `MAX(... AND is_accepted = 1) AS dongY`. `LEFT JOIN customer` đổi thành `INNER JOIN` — nhóm
  dimension NULL sinh ra bởi LEFT JOIN không bao giờ khớp `m.id` nên bỏ đi không đổi kết quả.
- **R/S/T/U → 1 bảng `RSTU`** khi `useOsuFinancial=false` (7/8 endpoint): 4 `SUM(CASE WHEN …)`
  trên một lượt quét `payment`.
- **`useOsuFinancial=true`** (`source-statistics`) không gộp được thành 1: R+U đi qua
  `payment × order_service_used`, S+T đi qua `payment × customer` — hai cấp bảng khác nhau ⇒ gộp
  thành **2** (`RU`, `ST`), không phải 1.

Đo @615.992 khách / 872.284 lịch hẹn / 449.360 osu / 372.504 payment, lọc tháng mặc định, toàn bộ
chi nhánh (2026-09-09):

| Biến thể | Trước | Sau |
|---|---|---|
| `useOsuFinancial=false` (7 endpoint) | 2.683.539 dòng InnoDB / 0,94–1,22s | **717.130 / 0,33–0,61s** |
| `useOsuFinancial=true` (`source-statistics`) | 2.669.676 / 1,21–1,28s | **705.889 / 0,41–0,43s** |
| `false`, KHÔNG lọc ngày | 2.690.630 / 4,33–4,45s | 759.208 / 0,79–1,16s |
| `true`, KHÔNG lọc ngày | 4.636.274 / 8,43–8,53s | 2.963.026 / **6,1–14,4s** |

Đối chiếu kết quả cũ/mới: **8/8 khớp tuyệt đối** (2 biến thể × 4 bộ lọc).

Hai dòng cuối là lý do phải **áp trần dải ngày** ở đây: gộp không cứu nổi ca không-lọc-ngày. Guard
đặt trong chính `dimensionStatistics` nên phủ cả 8 endpoint bằng một chỗ; vượt 366 ngày ⇒ 422
(`report_date_range_max_one_year`), KHÔNG tự cắt dải.

### ⚠ `service-type-statistics` KHÔNG thuộc nhóm 8

Tên giống nhưng là handler riêng, không đi qua `dimensionStatistics`. Endpoint thứ 8 là
`interested-statistics` (bảng dimension của nó tình cờ tên `service_type`). Rà consumer theo
**hàm được gọi**, đừng theo tên route.

### ⚠ `fmt.Sprintf` với slice tham số động — `go vet` mù hoàn toàn

Sau khi gộp, hai nhánh `useOsuFinancial` có số verb `%d/%s` KHÁC nhau (13 vs 12). `financialSQLArgs`
là `[]interface{}` dựng lúc chạy nên `go vet` không đếm được; lệch thì câu SQL mang `%!d(MISSING)`,
MySQL từ chối, và vì `pagination.FailQuery` ghi log qua `logger.L` (nil trong test binary) nó hiện
ra thành **panic che mất thông điệp lỗi thật**. Hai biện pháp bắt buộc khi viết kiểu này:

1. Dựng danh sách tham số **riêng trong từng nhánh** `if`, không dùng chung một slice rồi
   append có điều kiện.
2. Test integration phải chạy **cả hai** biến thể, và gắn `zaptest/observer` vào `logger.L` để
   lỗi SQL hiện nguyên văn thay vì panic.

### ⚠ Mutation xanh: phải phân biệt "no-op thật" với "test không phủ"

Hai lần trong cùng một bản vá này:

- Hoán vị `define.PaymentForDeposit` ↔ `define.PaymentTypeDepositKG` **xanh và ĐÚNG** — hai hằng
  cùng giá trị `3`, câu SQL sinh ra giống hệt từng byte. No-op thật.
- Hoán vị `define.PaymentForRefundDeposit` (2) ↔ `define.PaymentForRefundService` (1) **cũng xanh,
  nhưng SAI**: test không seed một dòng `payment` nào ⇒ cả 4 cột tiền luôn bằng 0 ⇒ **mọi** hằng
  `payment_for` đều vô hình. Vá bằng cách seed 5 payment với 4 số tiền đôi một khác nhau
  (1000 / 200+500 / 70 / 9 / 5000-KG-bị-loại) và assert đủ 6 cột tiền; mutation này lập tức đỏ
  (`totalRefundDeposit = 9, muốn 70`).

Cách phân biệt rẻ nhất: **tra giá trị hằng trước khi kết luận**. Trùng giá trị ⇒ no-op; khác giá
trị mà vẫn xanh ⇒ lưới thủng, đi tìm dữ liệu seed còn thiếu.

## ⚠ "Gộp N lượt quét" KHÔNG phải luật — nó thắng hay thua tuỳ CHI PHÍ MỖI DÒNG

Sáu lần trước, khuôn "N subquery độc lập → gộp bằng `SUM/COUNT(CASE WHEN …)`" đều thắng, tới mức
dễ tưởng là luật. Lượt `group-source-statistics` (2026-09-09) đo được **cả hai chiều trong CÙNG
một câu SQL**, nên nay có tiêu chí phân biệt thay vì phải đoán.

Bench: 615.992 khách / 872.284 lịch hẹn / 372.504 payment / 449.360 osu, toàn chi nhánh,
**trung vị 5 lượt** (không phải lượt đầu — xem cảnh báo cache lạnh bên dưới).

| Cụm gộp | Mỗi dòng phải làm gì | Trước | Sau | Phán quyết |
|---|---|---|---|---|
| B/C/D (`appointment`, cùng `appointment_date`) | chỉ đọc chính `appointment` | 1 năm 0,811s · tháng 0,177s | **0,196s · 0,014s** | GIỮ (4,1× · 13×) |
| A/B/C/D của `branch-appointment` | chỉ đọc chính `appointment` | không lọc 4,13s | **1,29s** | GIỮ (3,2×) |
| R/S/T/U (`payment`, cùng `payment_time`) | mỗi dòng còn `JOIN order_service_used` | 1 năm 0,794s | **1,908s** | **LOẠI (chậm 2,4×)** |
| R/S/T/U của `dimensionStatistics` | mỗi dòng chỉ tra PK `customer` | 1 năm 0,055s | **0,020s** | GIỮ (2,8×) |

**Cơ chế**: mỗi khối con có một bộ lọc rất chọn lọc (`payment_for = …`). Gộp lại tức là **bỏ bộ
lọc đó khỏi `WHERE`** — lượt quét duy nhất phải xử lý **hợp** của cả N tập (đo được 375.681 →
745.008 dòng). Đổi N lượt rẻ lấy 1 lượt đắt chỉ lãi khi công việc mỗi dòng rẻ.

**Tiêu chí quyết định trước khi gõ phím**: nhìn phần thân của khối con.
- Chỉ đọc một bảng, hoặc tra khoá chính ⇒ gộp gần như luôn thắng.
- Mỗi dòng kéo theo một `JOIN` sang bảng khác ⇒ **phải đo**, đừng gộp theo phản xạ.
Hai khối `dimensionStatistics` và `group-source-statistics` nhìn giống hệt nhau trên màn hình,
chỉ khác đúng chỗ đó, mà kết quả ngược nhau.

### ⚠ Lượt đo ĐẦU TIÊN nói dối — suýt ghi sổ sai 14×

Lượt đầu của `group-source-statistics` cho "tháng: 1,50–3,13s → 0,11–0,18s ⇒ nhanh 14×". Chạy
xen kẽ 5 lượt thì sự thật là **0,0872s → 0,0569s (1,53×)** — con số 14× hoàn toàn là cache lạnh
của lần chạy đầu tiên trên bench vừa dựng. Cùng lượt đó, ca 1 năm lượt đầu bảo bản gộp *chậm hơn*
7%, trung vị 5 lượt lại bảo *nhanh hơn* 1,62%. Luật rút ra: **báo cáo bằng trung vị của ≥5 lượt
xen kẽ base/opt**, không bao giờ bằng lượt đầu, và không bao giờ bằng "2 lượt liên tiếp cùng biến
thể" (biến thể chạy sau luôn được hưởng cache của biến thể chạy trước).

Cùng lớp, một tầng nữa của phép đo tự nói dối: **bộ trích số từ output phải neo bằng ranh giới,
không dựa vào `.*`**. `sed -E 's/.*t=([0-9.]+).*/\1/'` trả `0` cho cả 20 phép đo, vì regex tham lam
khớp `t=` nằm BÊN TRONG `next=` và `sort=` chứ không phải `t=` thật. Dùng
`'^.*[[:space:]]t=([0-9.]+)[[:space:]].*$'` hoặc `awk -F'\t'` theo cột. Đối chứng bắt buộc: dòng
`SELECT 1` phải ra thời gian **khác 0** trước khi tin bất kỳ con số nào của lượt đo.

### Lớp bọc `FROM customer LEFT JOIN (…)` không đóng góp gì

Bốn khối E/B/C/D của `group-source-statistics` viết:

```sql
FROM customer
LEFT JOIN ( SELECT appointment.id, appointment.customer_id, appointment.source_id FROM appointment … ) App
  ON customer.id = App.customer_id
WHERE App.source_id IS NOT NULL
```

`customer` không cho cột nào ra ngoài, và `WHERE App.source_id IS NOT NULL` biến LEFT JOIN thành
INNER ⇒ toàn bộ tác dụng là kéo 615.992 dòng khách vào rồi vứt đi. Bỏ được **vì**
`appointment.customer_id` có FOREIGN KEY thật tới `customer` (RESTRICT) — id khác NULL thì chắc
chắn có khách. Nhưng cột **nullable**, nên phải thay bằng `AND appointment.customer_id IS NOT NULL`;
thiếu vế đó là đếm dư lịch hẹn không gắn khách (hôm nay 0 dòng — đó là *dữ liệu*, không phải ràng
buộc). `GROUP BY appointment.customer_id, appointment.id, …` bên trong cũng thừa: `id` là khoá chính.

### Còn lại sau bản vá

`group-source-statistics` ca 1 năm vẫn **4,03s** và không-lọc **4,13s**. Phần lớn nằm ở khối A
(`customer WHERE created_day BETWEEN …`): dải hẹp thì dùng `idx_customer_company_created_day`
(tháng: 1.768 dòng), dải rộng thì optimizer bỏ chỉ mục và quét trọn 615.992 dòng — thêm chỉ mục
không cứu được vì optimizer sẽ không chọn. Vì vậy kèm **trần dải ngày 366 ngày**, cùng luật với 8
endpoint `*-statistics` anh em. `branch-appointment` thì **cố ý KHÔNG áp trần**: sau khi gộp, ca
xấu nhất (không lọc, toàn bộ lịch sử) chỉ 1,29s — chặn thì hạn chế người dùng mà không mua được gì.

## Bench CRM standalone 2026-10-19 (schema 1 thương hiệu/DB, KHÔNG còn `company_id`)

Số đo ERP ở các mục trên là trước khi bỏ `company_id` — dùng làm gợi ý, không làm số thật. Lượt này
dựng bench TỪ MIGRATION (`cluvix_crm_bench_tmp`), 2 cỡ cộng dồn: **S** 50k khách / 153k lịch hẹn /
114k osu / 76k payment / 202k note; **L** 500k / 1,2M / 983k / 536k / 2,5M. 53 FK, 0 dòng mồ côi.
⚠ Máy đo có `innodb_buffer_pool_size` 128MB (L ≈ 2,7GB) ⇒ số L là trần IO-bound và có lật plan; dữ
liệu dồn về gần đây ⇒ ca "tháng này" ở L bi quan ~3×. Đo qua HTTP (QA server 8089) + EXPLAIN ANALYZE.

| # | Endpoint / truy vấn | S | L | Cơ chế | Hướng sửa (CHƯA ship) |
|---|---|---|---|---|---|
| 1 | `GET /api/crm/appointment/appointment-overview?year=` | timeout 180s | timeout; 1 query 816s | `appointment` LEFT JOIN chính nó 4 lần theo `customer_id` ⇒ ~n⁵ dòng/khách (khách đuôi dài: 97 triệu dòng trung gian, 148s) + `CONVERT_TZ(FROM_UNIXTIME(col))` trong mọi điều kiện | Viết lại 1 lượt quét `COUNT(DISTINCT CASE …)`, lọc theo khoảng unix tính sẵn ở Go. Server không huỷ query khi client ngắt ⇒ reload chồng query 100% CPU (cân nhắc `WithContext(c.Request.Context())` / `MAX_EXECUTION_TIME`) |
| 2 | `report/debt/service` | 0,4s | 13s | plan lật theo cache: đi từ `service` rồi tra osu qua fk `service_id` (5,2s) | `FORCE INDEX (idx_osu_created_day_branch)` 252ms |
| 3 | `report/service-type-statistics`, `report-service-used` | — | 2,3–10,5s · 3,7–7,5s | `GROUP BY st.id, st.name` sort 858k dòng khoá VARCHAR | gom theo id trước, join tên sau |
| 4 | `GET /api/crm/customer` (nhân viên 1 chi nhánh) | — | 698ms (admin 5 CN: 85ms) | `branch_id = ? OR IS NULL` đọc 200k dòng qua `fk-customer_branch_id` + filesort | trang: `FORCE INDEX (PRIMARY)` 0,2ms; COUNT qua `idx_customer_scope_status` 33ms |
| 5 | `GET /api/crm/appointment` tháng này / không lọc | — | 0,86s / 1,2–1,7s | tie-breaker `id DESC` tự thêm chặn sắp theo index ⇒ quét 1,2M | index `(appointment_date, id DESC)`: trang 834ms → 0,24ms (chỉ thử trên bench) |
| 6 | `LIKE '%kw%'` xuyên join | — | payment không khớp + không lọc ngày 7,7s; note 1,5–1,9s; order-plan 1,4s; customer 1,15s; `load-by-keyword` 0,4–1,1s | | |
| 7 | `report/debt/order-plan` | — | 3,1s mọi bộ lọc | gộp toàn bộ order_plan × osu + LATERAL payment, ngày chỉ vào CASE | |
| 8 | `*-statistics`, revenue, appointment-statistics | — | 1,2–3,5s | 6–7 bảng dẫn xuất quét lại cùng dải ngày | |
| 9 | export customer / appointment | — | 6,8s / 4,5s | | |

Không có nút thắt: bộ request trang chi tiết khách đuôi dài (9 request, 6–13ms); dashboard lọc mặc định (lạnh ≤953ms).

**7 index "thừa" còn treo** (đo bằng `ALTER INDEX … INVISIBLE`, không xoá):
`index_status_customer` BỎ (đang làm COUNT status=12 chậm: 152 vs 19ms) · `fk-order_service_used-customer_id`
BỎ (lùi <1ms tuyệt đối, tiết kiệm 13,5MB) · `idx_osu_list_cover` BỎ (118MB ở L, chậm INSERT ~20%, chỉ còn
`code LIKE` 0,23→0,40ms) · `idx-payment-appointment_id` BỎ (không lùi) · `fk-customer_branch_id` GIỮ (ẩn đi
optimizer chọn `branch_revenue`, chậm 40–57% ở 1 CN — sửa gốc là mục 4) · `idx_osu_islock` GIỮ
(`service-used-with-appointment` 1,1ms → 446ms) · `idx_payment_customer` GIỮ (bị gọi đích danh
`FORCE INDEX` ở `customer_detail.go:277`; ẩn ⇒ lỗi 1176).

**Chi phí UPDATE dòng cha theo số FK con** (p50, 200 lượt): `user` (21 FK) 2,6–2,8ms vs bản sao không FK
0,10ms (0,58ms khi `table_open_cache=16000`); `branch` (12) 1,06ms; `customer` (10) 0,37–0,39ms vs 0,13–0,15ms,
sau `FOR UPDATE` 0,34–0,39ms. Cơ chế: FK tầng SQL (`innodb_native_foreign_keys=OFF`) mở mọi bảng nối qua
ON UPDATE CASCADE (~150 lần mở bảng/`UPDATE user`); cache bảng tràn vì instance dùng chung nhiều DB. FK thuần
~0,022ms/FK con — con số "~5ms/FK" trong CLAUDE.md đo khi `user` có 355 FK. Đăng nhập/refresh KHÔNG update
`user`; `customer` update trên đường tiền sau `FOR UPDATE` +~0,2ms/lần.

### Đã vá top 5 + bỏ 4 index (2026-10-19, đo lại HTTP trên bench L, median ấm 3 lượt)

| Endpoint · ca | Admin 5 CN trước → sau | NV 1 CN trước → sau | Cách sửa (file) |
|---|---|---|---|
| `appointment-overview` · năm | timeout → **901ms** | — | 1 lượt quét trên `idx_appointment_date_scope`, cờ theo khách bằng `MAX(...)` + mặt nạ tháng `BIT_OR`, mốc ngày tính ở Go; `MAX_EXECUTION_TIME(define.AppointmentOverviewMaxExecMs)` vì driver chỉ đóng socket khi huỷ context, KHÔNG kill query (`crm/appointment/handler/overview.go`). Sửa luôn lỗi cũ: `BETWEEN '…-12-31'` bỏ sót trong ngày cuối kỳ |
| `GET /api/crm/appointment` · tháng | 864 → **37ms** | 667 → **34ms** | index `idx_appointment_date_id (appointment_date, id DESC)` + hint `NO_INDEX(fk-appointment_branch_id)` chỉ cho câu trang (COUNT không hint: hint làm COUNT 1 CN 79 → 539ms) |
| ↳ không lọc ngày / trang 500 | 1.173 / 1.604 → **227 / 262ms** | 1.162 / 1.303 → **484 / 559ms** | (còn lại chủ yếu là COUNT) |
| `GET /api/crm/customer` · mặc định / trang 500 | 85 / 102 → 86 / 100ms | 698 / 762 → **39 / 58ms** | chỉ phạm vi 1 CN: tách `branch_id = ? OR IS NULL` thành UNION ALL 2 nhánh tra ngược PRIMARY (cổng `offset+perPage ≤ total/2`), COUNT hint `NO_INDEX(fk-customer_branch_id, idx_customer_branch_revenue)` (1 khối — 2 khối thì MySQL bỏ khối 2 im lặng, warning 3126) (`crm/customer/handler/list_branch_split.go`) |
| `report/debt/service` · tháng / năm / không ngày | 12.848 / 13.848 / 13.302 → **547 / 1.157 / 1.392ms** | — | `STRAIGHT_JOIN` giữ osu dẫn đầu (hết lật plan) + `FORCE INDEX (idx_osu_created_day_branch)` có cổng (đủ 2 mốc, ≤31 ngày, không lọc service, không scope telesale) (`crm/report/debt/service.go`) |
| `report/service-type-statistics` · tháng / năm / không ngày | 2.349 / 4.798 / 10.590 → **429 / 1.869 / 3.038ms** | — | gộp theo `service_type_id` `WITH ROLLUP` (bỏ câu COUNT DISTINCT riêng) + hint đường đọc osu (`osu_report_hint.go`) |
| `report-service-used` · tháng / năm | 3.687 / 7.750 → **661 / 2.287ms** | — | cùng hint + chỉ JOIN `customer` khi lọc nguồn/khu vực + gộp COUNT DISTINCT vào câu tổng |

Chẩn đoán đính chính: C-3 ở lọc tháng/năm KHÔNG do sort GROUP BY VARCHAR mà do optimizer chọn `ref isLock=1`
trên các index dẫn đầu `isLock` (đọc 858k dòng rồi mới lọc ngày) — hint `osuLockedReportTable`: ≤31 ngày ⇒
`FORCE INDEX (idx_osu_appt_day_cover)`, rộng hơn/không ngày ⇒ `IGNORE INDEX` 3 index `isLock` + fk `service_id`.
Bỏ 4 index (`index_status_customer`, `fk-order_service_used-customer_id`, `idx_osu_list_cover`,
`idx-payment-appointment_id`): không ca nào lùi. **Còn nợ** (vẫn > ngưỡng ở L): `service-type-statistics`
không ngày 3,0s & năm 1,9s; `report-service-used` năm 2,3s; `debt/service` năm/không ngày 1,2–1,4s; list lịch hẹn
1 CN không ngày 484ms (COUNT); `LIKE '%kw%'` (khách không khớp 1,16s, 1 CN 0,8s); export khách 6,9s / lịch hẹn 4,6s;
`search-global` + `get-number-customer-by-*` cùng bệnh 1 CN chưa vá; phạm vi 2+ CN (không phải tất cả) chưa vá.

### Vòng 2 (2026-10-19, cùng bench L, HTTP median ấm) — gốc → vòng 1 → vòng 2

| Endpoint · ca | Admin 5 CN | NV 1 CN | Cách sửa vòng 2 |
|---|---|---|---|
| list lịch hẹn · không ngày / trang 500 | 1.173 → 227 → **132** / 1.604 → 262 → **162** | 1.162 → 484 → **59** / 1.303 → 559 → **143** | COUNT bọc derived KHÔNG kèm `ORDER BY` + chỉ chọn `appointment.id` ⇒ `fk-appointment_branch_id` phủ (`appointmentListCountQuery`). ⚠ `pagination.Paginate` dùng chung cũng bọc COUNT kèm ORDER BY + mọi cột — chưa sửa ở tầng dùng chung |
| `search-global` · khớp / SĐT / không khớp | 20 / 499 / 488 → 15 / 492 / 481 | 396 / 390 / 384 → **22** / 353 / 348 | phạm vi 1 CN: tách UNION ALL như list khách (`customerSplitByBranch`). Phần còn lại là quét `LIKE '%kw%'` — chưa có đường ngram (thử 1 lượt: 0,03–0,05ms cho từ khoá có số, cần duyệt) |
| `debt/service` · tháng / năm / không ngày | 12.848 → 547 → **328** / 13.848 → 1.157 → **612** / 13.302 → 1.392 → **716** | 604 / 974 / 1.023 | gộp totals + totalDeposit 1 câu (gom theo khách trước rồi LEFT JOIN customer); dải rộng: `service STRAIGHT_JOIN osu IGNORE INDEX (fk service_id)` ⇒ hash join |
| `report-service-used` · tháng / năm | 3.687 → 661 → **351** / 7.750 → 2.287 → **1.185** | 151 / 743 | 1 câu `GROUP BY service_id WITH ROLLUP` trong derived, JOIN service/customer chỉ khi bộ lọc cần, phân trang ở Go. Năm + lọc nguồn vẫn 3,6s (tra PK customer từng dòng) |
| `service-type-statistics` · tháng / năm / không ngày | 2.349 → 429 → **430** / 4.798 → 1.869 → **1.307** / 10.590 → 3.038 → **1.970** | 186 / 816 / 1.070 | dải rộng: `service_type STRAIGHT_JOIN service STRAIGHT_JOIN osu IGNORE INDEX (…)` hash join. Phần còn ~1,5s là sort + `COUNT(DISTINCT)` + ROLLUP trên 858k dòng — KHÔNG gỡ được bằng hình SQL/index (đã thử `COUNT(DISTINCT CASE…)` 1 lượt 2,6s, derived DISTINCT 2,0s); cần đổi đường ghi (lưu `service_type_id` vào osu hoặc bảng tổng hợp) |

Bug bắt được vòng 2: MySQL 9.7 trả 1 dòng `(NULL, GROUPING()=0, COUNT 0)` cho `GROUP BY … WITH ROLLUP` trên tập RỖNG ⇒
nhận dòng tổng bằng `id IS NULL`, không bằng `GROUPING()`. Index ứng viên đã đo rồi LOẠI (không vào migration):
`osu ((totalEnd <> payment), created_day, …)` 61,7MB — deposit không nhanh hơn, từ khoá không ngày lùi 0,8 → 3,1s;
`osu (isLock, service_id, created_day, … 15 cột)` 124MB — năm 1.196 → 778–809ms, chưa đo chi phí ghi.
`get-number-customer-by-*` (kiểm trùng) 320–390ms ở cả 2 phạm vi: by-old-code cần index `old_code`; by-phone có thể đổi sang
`phone_unique_key = ?` nhưng khác ngữ nghĩa `LIKE '%x%'` — chờ duyệt.
