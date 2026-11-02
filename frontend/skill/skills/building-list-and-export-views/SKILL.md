---
name: building-list-and-export-views
description: >-
  BaseListV2 (filterFields, state trên URL, fetchList), BaseDetail/BaseReport/BasePrint,
  và export/import Excel trong Cluvix FE. Dùng skill này KHI làm màn hình danh sách
  mới, migrate list V1→V2, thêm report có export, hoặc thêm chức năng in — trước khi
  đụng colDef/ExportOptionsDialog hoặc PrintLayout.
---

# Building list and export views

> Print flow chi tiết (PrintLayout, PrintContext, window.print() gotchas): `reference/fe-print.md`
> Import/Export Excel chi tiết: `reference/fe-patterns-import-export.md`
> Gotchas Search/AutoComplete/List display: `reference/fe-patterns-gotchas-search.md`
> Khuôn `.ts` + `.html` cho list config mới (toolbar lọc, bộ chọn cột, cột thao tác cố định): `reference/fe-list-config-template.md`



Dùng cho list mới, hoặc list cần giữ filter khi bấm vào bản ghi → xem chi tiết → Back.
`BaseList` (V1) giữ nguyên — KHÔNG migrate hàng loạt, chuyển từng list khi có nhu cầu.

## Bật V2

```ts
export class XxxList extends BaseListV2<XxxModel> implements OnInit {
  @ViewChild('dt') dt!: Table<XxxModel>;
  protected override URL_STATE_PREFIX = 'xx';   // 2–3 ký tự, khác nhau giữa 2 list cùng 1 route
  protected filterFields(): FilterFieldDef[] { return [ /* ... */ ]; }
  ngOnInit() { this.initListV2(); }
}
```

```html
<p-table #dt [lazy]="true" [lazyLoadOnInit]="false" (onLazyLoad)="loadRecords($event)"
         [filters]="initialFilters()" [first]="first()" [loading]="showSkeleton()" [filterDelay]="0">
  <ng-template #caption>
    <div class="h-[2px] mb-1">
      @if (refreshing()) { <p-progressbar [showValue]="false" [style]="{height:'2px'}" mode="indeterminate"/> }
    </div>
```

- 5 binding bắt buộc: `[lazyLoadOnInit]="false"` · `[filters]` · `[first]` · `[loading]="showSkeleton()"` · `[filterDelay]="0"`.
- Thanh báo tải lại đặt trong hộp cao cố định (`h-[2px]`) — tự nó không được gây CLS.
- KHÔNG override `initFilter()`, `getDefaultFilterValues()`, `clearFilter()`, `searchHandler()` — base lo hết.

## filterFields()

- 1 field = 1 dòng: `{name, kind, default?, urlKey?, debounce?, itemType?, toFilter?, urlSync?}`.
- `kind`: `text` (contains) · `in` (multiselect, LUÔN mảng) · `equals` (select 1, LUÔN scalar) · `dateRange` (between, cặp unix) · `dateSingle` (chọn 1 ngày, gửi BE chuỗi `yyyy-MM-dd`, matchMode mặc định `custom`) · `monthDayRange` (khoảng ngày-tháng KHÔNG năm, between, cặp chuỗi `MM-DD` — vd lọc sinh nhật) · `dateRangeSplit` (khoảng ngày mà BE đọc bằng **2 param tên riêng**, vd `_dueDateGte`/`_dueDateLte`) · `custom` (tự lo qua `toFilter`).
- BE nhận **khoảng ngày-tháng không năm** (`MM-DD`, vd `birthday_md`) → `kind: 'monthDayRange'`. KHÔNG dùng `custom`: `custom` không parse ngược URL→`Date` nên filter không sống sót qua Back/F5 (cùng bài học với `dateSingle`). Ngày chỉ tồn tại ở năm nhuận (`02-29`) được giữ bằng cách lùi về năm nhuận cố định — panel lịch sẽ hiện năm đó, ô input `dateFormat="dd/mm"` không lộ.
- BE nhận khoảng ngày qua **2 tên param khác nhau** (không phải 1 param `[gte]/[lte]`) → `kind: 'dateRangeSplit'` (accountant 2026-08-04, `InstallmentDebts._dueDateGte`/`_dueDateLte`). Một `FilterMetadata` chỉ sinh được 1 tên param nên base **không sinh gì** (`toFilterMeta` trả `null`) — chỉ lo FormControl + URL sync; component tự ghép 2 param trong `fetchList()`. Nhờ vậy filter này **sống sót qua Back/F5**, khác hẳn cách cũ (`custom` + `urlSync:false`) không làm được.
- Select chọn-một: template KHÔNG `[multiple]`, PHẢI `[showClear]="true"` (thiếu → không xoá được filter), def `kind: 'equals'`.
- **`p-datepicker` filter với `(onClear)="clearXxx()"` — PHẢI có `[showClear]="true"` trên CÙNG thẻ, nếu không handler CÂM LẶNG mãi mãi** (PrimeNG v22: `clearIconEnabled = showClear() && !disabled()`, mặc định `showClear=false` → icon xoá không bao giờ render → `clear()` không ai gọi được → `onClear.emit` không bao giờ chạy). Không phải lỗi cú pháp, `tsc`/`ng build` đều sạch — chỉ lộ ra khi bấm thật trên UI. Đo được ở accountant: 70/71 chỗ dính lỗi này từ trước khi có epic migrate.
- `default` phụ thuộc thời điểm → truyền factory: `default: () => [this.timezoneService.todayStart(), this.timezoneService.todayEnd()]`.
- Sentinel/luật riêng → `toFilter`, vd `revenue_id` id=0 nghĩa là chưa có → `{value: '1', matchMode: 'is'}`.
- `urlKey` bỏ trống = tự suy: bỏ `_` đầu + camelCase→snake_case (`_groupLocal`→`group_local`, `customerType`→`customer_type`). Chỉ khai khi muốn khác quy ước (`appointment_date`→`date`) hoặc khi bị trùng (`_status` vs `status`).
- Control tên có ký tự `[`/`]` (kind `'custom'`, BE đọc raw `c.Query("field[x]")`) → BẮT BUỘC khai `urlKey` tường minh (vd `urlKey:'status'`) — mặc định giữ nguyên ký tự lạ, `URLSearchParams` percent-encode thành `%5B%5D` trên address bar (round-trip đúng, chỉ xấu URL). `assertFilterDefs()` không chặn được ca này.
- `assertFilterDefs()` ném lỗi lúc mở trang (dev) khi trùng `name`, trùng urlKey, hoặc đụng khoá dành riêng `_f`/`_page`/`_size`/`_sort`.
- Lọc từ code: `this.setFilterValue(name, value)` — KHÔNG `patchValue()` + `resetPage()` (đẻ 2 request).
- Field BE chưa có `query.ApplyFilter` thì KHÔNG khai def và KHÔNG để nút bấm lọc — nút sẽ đổi URL mà kết quả không đổi (sai âm thầm).
- `kind: 'text'` value đi thẳng vào query string GET, KHÔNG giới hạn độ dài mặc định — gõ/paste vài nghìn ký tự làm request vượt giới hạn dòng request của backend/proxy → HTTP 414, list trắng im lặng. Base đã tự chặn ở `MAX_TEXT_FILTER_LENGTH = 200` (`list-filter.model.ts`) — mọi `kind:'text'` tự cắt còn 200 ký tự + toast cảnh báo (`base-list-v2.ts` `wireFilterControls()`), áp dụng mọi list dùng `BaseListV2`, không cần khai gì thêm.

## State trên URL

- Param phẳng, không mang matchMode: `?ap_f=1&ap_date=1785690000,1785776399&ap_status=1&ap_page=2`.
- `ap_f=1` = cờ "URL đang giữ state": có cờ → URL là nguồn sự thật, param vắng mặt = người dùng đã xoá filter đó; không cờ → dùng default.
- Ghi URL bằng `Location.replaceState`, **KHÔNG** `router.navigate` — navigate chạy trọn chu kỳ điều hướng (ActivationEnd ×N + Scroll) làm `p-table` vẽ lại đè lên popover filter đang mở.
- Query dựng từ `location.path()`, chỉ `set`/`delete` key mình sở hữu — KHÔNG `createUrlTree({queryParamsHandling: 'merge'})`; `replaceState` không báo cho router nên cây URL của router luôn cũ.
- Trả `%2C` → `,` sau `URLSearchParams.toString()`.
- Đồng bộ lại khi điều hướng: nghe `NavigationEnd` **và `NavigationSkipped`** — bấm lại chính menu đang đứng thì router skip, `queryParams` KHÔNG phát. Đọc state từ `e.url`, KHÔNG từ `activatedRoute.snapshot` (skip không cập nhật snapshot).
- Khi đồng bộ lại phải gán LUÔN `page`/`per-page`/`sort` (param vắng = về mặc định), KHÔNG gán theo `if (page > 1)`.
- `writeUrlState()` gọi trong `setTimeout` + guard cờ `destroyed` — router commit URL ở cuối chu kỳ, và component có thể đã huỷ trong khoảng đó.
- Chặn theo path (`e.url` vs path lúc init) trước khi xử — không ghi đè URL của trang khác.

## Tải dữ liệu

- Override `fetchList()` trả `Observable<ListApiResponse<T>>`; KHÔNG override `searchHandler`, KHÔNG tự set `loading`/`models`/`paginationInfo`.
- Base đẩy mọi lần tải qua `Subject + switchMap` → huỷ request cũ, hết cảnh response cũ về sau ghi đè kết quả mới.
- `getList()` KHÔNG xoá `models` (stale-while-revalidate). Skeleton chỉ ở lần tải đầu (`showSkeleton` = `loading && !loadedOnce`); các lần sau dùng `refreshing`.
- List có `#loadingbody` dùng `colsIt()` PHẢI gán `effect(() => this.padCols.set(this.selectedColumns().length + <số cột luôn hiện>))` trong constructor — `padCols` mặc định 0 ⇒ hàng skeleton không có `<td>` nào, `tsc`/`ngc` không bắt.
- Endpoint không theo convention `filter[field][matchMode]` (vd query phẳng riêng) → vẫn khai `filterFields()` bình thường (để có URL state + wiring), nhưng `fetchList()` đọc thẳng `rfFilterModel.getRawValue()` và tự dựng param như V1 — KHÔNG dùng `searchParams()`. Ghi comment tại chỗ giải thích lý do, kẻo người sau "dọn" nhầm về `searchParams()`.
- List có field tổng/side-effect ngoài `items`/`_meta` gán trong `fetchList()` (vd `total`/`discount` hiển thị footer) → PHẢI tự thêm sequence guard (`private xxFetchSeq = 0`, tăng ở đầu `fetchList()`, chỉ gán khi `seq === this.xxFetchSeq` lúc response về) nếu paginator/sort không debounce — `wireReloadPipeline` (base) chỉ huỷ được việc GIAO `models`/`paginationInfo` cho request cũ, KHÔNG huỷ side-effect tự viết chạy trong thân `switchMap(async...)` của chính `fetchList()`.
- `effect()` re-tính state "theo trang mới" (vd cơ chế chọn-dòng tự viết cần `currentPageItems` khớp trang hiện tại) phải watch `this.paginationInfo()`, KHÔNG watch `this.models()` — `models` còn bị mutate NGOÀI luồng fetch bởi `updateRow()` (base, dùng khi cập nhật 1 dòng tại chỗ vd toggle status), `paginationInfo` chỉ đổi khi có fetch thật.
- List nhúng nhận `@Input customer` từ parent có `model()` khởi tạo rỗng-nhưng-truthy (vd `customer-detail.ts` dùng `signal({} as T)`) → `fetchList()` có thể chạy với `customer.id === undefined` lúc parent chưa load xong HTTP, rõ nhất khi Back/Forward remount toàn trang (không phải chỉ đổi tab). Guard đầu `fetchList()`: `if (!this.customer?.id) return this.emptyListResult();` — helper có sẵn trên `BaseListV2`, trả `{items:[], _meta:{totalCount:0}}` hợp lệ (không phải lỗi); `ngOnChanges` sẵn có sẽ tự refetch khi customer load xong. Cùng lý do gây interceptor văng cả app nếu BE endpoint coi id rỗng là 404 — xem `frontend/skill/skills/working-on-fe-modules/reference/fe-patterns-gotchas.md`.

## Gotcha PrimeNG v22

- Input `[filters]` đồng bộ vào `dt.filters` qua `effect()` → chạy SAU `ngOnInit` của bảng, tức sau lần `onLazyLoad` đầu tiên. Bảng tự bắn thì request đầu đi với filter RỖNG → bắt buộc `[lazyLoadOnInit]="false"`, base tự bắn trong `afterNextRender`.
- `filterDelay` mặc định đã là 300 → gỡ binding vô ích, phải `[filterDelay]="0"` (base đã debounce 300/500ms).
- `p-select [multiple]="true"` trông y hệt dropdown chọn-một (không checkbox, không chip) — dễ tưởng single trong khi nó cộng dồn.
- Control inline trong `p-table` (`p-toggleswitch`, checkbox) bind `[ngModel]` một chiều **KHÔNG tự quay lại giá trị cũ khi API lỗi**: control ghi thẳng vào state nội bộ lúc bấm, Angular chỉ `writeValue` khi giá trị binding ĐỔI, và `rowTrackBy` mặc định track theo THAM CHIẾU dòng → tạo mảng mới mà giữ nguyên reference từng dòng thì view được tái dùng, không revert. Revert đúng = **thay reference dòng**: `models.update(rows => rows.map(r => r.id === id ? {...r, status: cũ} : r))`, kèm set lạc quan trước khi gọi API.

## Điều kiện đủ để migrate 1 list sang V2

Kiểm 3 thứ TRƯỚC khi bắt tay, thiếu bất kỳ cái nào thì **giữ nguyên V1**:
- Template có `<p-table>` **và** `[lazy]="true"` — mọi giá trị của V2 (URL state, SWR, huỷ request cũ) đi qua `onLazyLoad` → `loadRecords()`. Bảng không lazy thì V2 chỉ đổi base class, không được gì.
- Có `@ViewChild('dt')` (thiếu thì thêm — `BaseListV2` khai `abstract dt`).
- Component có nơi dùng thật (route hoặc được nhúng) — grep selector trước, đừng migrate component chết.

## Checklist migrate V1 → V2 (đúc từ 45 lần làm, epic `crm-list-v2`)

- `defaultSearch.set(...)` trong `ngOnInit` → **xoá**, chuyển sang `default: () => ...` (factory) của field tương ứng. `[monthStart, todayStart]` và `[monthStart, todayEnd]` cho ra CÙNG range vì `buildFilterTimestamp` chuẩn hoá end về cuối ngày.
- `sort.set(...)` nằm trong `searchHandler()` → chuyển lên **constructor**. Để nguyên chỗ cũ thì mỗi lần tải nó đá sort người dùng chọn về mặc định (bug thật ở `ContactList`), và `initialSort` của base cũng sai.
- Control khai trong `fb.group` nhưng **không có `formControlName` nào trong template** = control chết → không khai def. Grep template trước khi chép cả danh sách control sang `filterFields()`.
- Field mà BE **không có** `query.ApplyFilter` lẫn `c.Query` → không khai def, gỡ luôn control lọc chết đó (nó đổi URL mà không đổi kết quả — sai âm thầm).
- BE đọc raw `c.Query("<field>")` thay vì `ApplyFilter` → `kind: 'custom'` + `toFilter` trả `matchMode: 'custom'`; tên control phải TRÙNG tên param BE. `buildFilter()` gửi raw `field=value` cho MỌI field có matchMode `'custom'` — bất kể tên có `_` hay không (đã fix 2026-08-04, trước đó chỉ đúng với field bắt đầu bằng `_`).
- BE nhận **1 ngày** dạng chuỗi `yyyy-MM-dd` (không unix, không khoảng) → `kind: 'dateSingle'`. KHÔNG dùng `kind: 'custom'` cho ca này: `custom` không có đường parse ngược URL→`Date` nên filter ngày sẽ không sống sót qua Back/F5. `dateSingle` format theo **wall-clock cục bộ** của Date (giống `formatDate(d,'yyyy-MM-dd','en-US')` của V1), KHÔNG chuyển timezone lần nữa — `p-datePicker` và `BaseApp.today` đều đã là Date theo giờ cần dùng.
- 2 bảng khác nhau có thể lưu Unix ở 2 đơn vị khác nhau (vd `log_call.started_at`=ms, `zalo_zns_log.sent_time`=giây) — `kind: 'dateRange'` LUÔN sinh giây. Đọc `comment` của cột trong `dao/entity.go` TRƯỚC khi khai `dateRange`; cột ms → override `toFilterMeta()` nhân 1000 tại component (KHÔNG dùng `kind:'custom'` — mất khả năng sống qua Back/F5).
- Response shape khác chuẩn (vd `{sumPoint, object:{items,_meta}}`) → bóc trong `fetchList()`, đừng sửa base.
- Đổi tên form control thì phải sửa `formControlName` trong template — **`tsc` không bắt được lệch tên**, reactive form chỉ vỡ lúc chạy.
- Component tự `inject(ActivatedRoute)` vào field `private activatedRoute` → **TS2415** khi lên V2 ("Types have separate declarations of a private property 'activatedRoute'") vì `BaseListV2` đã khai field private cùng tên. Đổi tên field local (vd `printTemplateRoute`), **KHÔNG** sửa base. Gặp thật 2 lần: `config-print-template`, `hr/certificate`.
- Lọc cứng theo query param của route (vd `?type=...`, không nằm trong panel filter) → `this.requiredSearch.set({...})`, KHÔNG nhét vào `filterFields()`: `requiredSearch` luôn được gộp vào `searchParams()` và không cho người dùng chỉnh.
- **Quên `URL_STATE_PREFIX` = tắt URL sync CÂM LẶNG.** Prefix rỗng là giá trị mặc định hợp lệ (dành cho list nhúng) nên `watchUrlNavigation`/`writeUrlState`/`restoreFromUrl` đều return sớm, không một lời cảnh báo: `tsc` xanh, list chạy bình thường, code review "đạt" đủ AC — mà FR-1/FR-2 mất trắng. Gặp thật 1/6 list ở epic `care-list-v2` (2026-08-03), agent còn tự khai trong báo cáo là đã đặt. ✅ Cuối mỗi wave, Planner grep đối chiếu **toàn bộ** list vừa migrate: `grep -rn "URL_STATE_PREFIX = " src/app/modules/<module>/` — đếm phải khớp số list *routed*, và list nhúng phải KHÔNG có dòng này. Đừng tin báo cáo của Dev.
- **Ô lọc không gắn được `formControlName`** (`app-user-select`, `<input type="month">`, hay signal mà nghiệp vụ đọc) → dựng giá trị hiển-thị-lại bằng `computed(() => this.initialFilters()['<field>']?.value ?? <rỗng>)`, **KHÔNG** sync bằng `control.valueChanges`: base patch/reset form bằng `emitEvent:false` ở `restoreFromUrl()` và `clearFilter()` nên `valueChanges` bỏ lỡ 2 ca đó. Từ 2026-08-04 `wireFilterControls()` cũng cập nhật `initialFilters` nên nó là **gương trung thực của form ở cả 4 đường** (init · người dùng đổi · điều hướng · xoá lọc). Hệ quả kèm theo: field lọc qua `p-columnFilter` (không có trong `filterFields()`) sẽ bị **rơi khỏi `dt.filters`** mỗi lần user đổi filter panel — cần giữ thì phải khai nó thành `FilterFieldDef`.
- Component tự khai `private activatedRoute = inject(ActivatedRoute)` (vd để đọc deep-link `?user_id=`) sẽ **đụng field `private activatedRoute` của `BaseListV2`** → `TS2415/TS2515` "incorrectly extends base class... separate declarations of a private property". Đổi tên field của component (vd `ceActivatedRoute`), **KHÔNG** sửa base. Gặp thật ở `hr/certificate/list` (epic `hr-list-v2`, 2026-08-03); V1 không dính vì `BaseList` không inject `ActivatedRoute`.
- Control lọc có thể nằm **ngoài** `rfFilterModel`: gọi thẳng `dt.filter(v, '<field>', ...)` từ một `[ngModel]` standalone. Quét `dt.filter(` toàn module trước khi chốt danh sách field, đừng chỉ đọc `fb.group` — sót kiểu này làm khảo sát thiếu hẳn 1 filter (`LeaveBalanceList.year`, epic `hr-list-v2`).
- V1 gọi `this.defaultSearch.set({})` (không phải `.set({field: ...})`) trong 1 hàm `clearXxx()` → đang xoá NHIỀU field cùng lúc (side-effect chéo), không chỉ field trong tên hàm. Migrate 1-1 sang `setFilterValue(field, null)` cho đúng 1 field sẽ thu hẹp phạm vi xoá — đối chiếu kỹ hàm `clearXxx()` V1 xem `defaultSearch`/`search` bị reset bao nhiêu key trước khi migrate.
- Base trung gian (Component → `MiddleBase` → `BaseListV2`) tự khai method tên `fetchList`/`filterFields`/tên khác trùng với thành viên mới của `BaseListV2` → phải đổi tên method cũ (vd `fetchApi`) ở TẤT CẢ subclass TRƯỚC khi đổi `MiddleBase` sang `extends BaseListV2`. Không đổi base rồi để subclass "sửa dần sau": `ngOnInit` của base đổi hành vi cho MỌI subclass đồng thời — build vẫn xanh nhưng runtime vỡ ngay (bảng bắn request trước khi filter từ URL kịp áp) tới khi từng subclass migrate xong. Gặp thật: `WorkspaceRevenueListBase`/`WorkspaceCommissionListBase`, epic `workspace-list-v2`.
- `(input)="searchKeyword($event)"` cạnh `formControlName` cùng field = lọc 2 đường (gõ 1 ký tự bắn 2 request) — bug có sẵn nhiều epic, không phải riêng vài list. Gỡ `(input)=...` khi migrate, giữ `formControlName` (base tự debounce). Grep TOÀN module bằng lệnh riêng cho từng file, KHÔNG gộp 1 vòng lặp shell — dễ lỗi quote silent-fail khiến kết luận nhầm "không dính" trong khi thực tế 100% list dính (epic `workspace-list-v2`, 14/14).
- `default:` trong `filterFields()` CHỈ áp lúc mở màn và khi bấm lại mục menu (URL trần). Nút "Xoá bộ lọc" (`clearFilter()`) xoá MỌI field về RỖNG, kể cả field có `default` (user chốt 2026-11-01) — đúng hành vi `defaultSearch.set({})` của V1, migrate field "tháng này" V1 sang `default:` factory là đủ. Ngoại lệ: field mà BE trả 422 khi thiếu (báo cáo bắt buộc dải ngày + trần 1 năm) → khai `required: true` (bắt buộc kèm `default`, `assertFilterDefs()` chặn) — xoá bộ lọc và URL đã-lọc-thiếu-param trả field đó về `default`. Field nặng khi bỏ ngày nhưng BE không chặn → hỏi user, KHÔNG tự gắn `required`.
- List nhúng dùng `requiredSearch` (vd theo `customer.id`) — thứ tự gọi `initXxxFilter()` (set `requiredSearch`) trước hay sau `initListV2()` trong `ngOnInit` KHÔNG quyết định request đầu có đủ param: request đầu bắn trong `afterNextRender()` (macrotask, sau khi `ngOnInit` đã chạy xong hoàn toàn), và `requiredSearch` là signal đọc live trong `searchParams`, không bị chụp snapshot. Đặt trước chỉ vì đọc tự nhiên hơn, không phải vì đúng/sai kỹ thuật — đừng suy diễn thành "rò dữ liệu chéo" nếu đặt sau.
- List có field drill-down ngoài `filterFields()` (đọc/ghi qua route queryParam) mà tự `rfFilterModel.reset(..., {emitEvent:false})` cho field khác trong `filterFields()` → KHÔNG được tự `this.initialFilters.set({}); this.search.set({})` để "đồng bộ lại" — 2 dòng đó xoá SẠCH mọi filter kể cả default theo điều kiện (vd `payment_user_id` theo role), và chạy ngay ở lần emit ĐẦU TIÊN (đồng bộ, trong `ngOnInit()`, TRƯỚC `scheduleFirstLoad()` async) nên bug lộ ra NGAY LẦN TẢI TRANG ĐẦU, không cần thao tác gì (epic `sale-list-v2`, user telesale mất filter theo role, thấy khách toàn phòng). Rebuild đúng: chỉ khi field trong `filterFields()` THẬT SỰ đổi, dùng `this.toFilterMeta(def, value)` cho từng field + `this.search.set(this.utilsService.buildFilter(filters))` (cả 2 đều `protected`, gọi được từ subclass) — KHÔNG blank về `{}`.
- Pattern "cờ boolean chặn 1 lần fetch kế tiếp" đặt TRƯỚC `route.navigate()` (async) → phải reset cờ về `false` ngay trong `valueChanges` của MỌI control tự kích điều hướng mới (vd chọn drill-down khác) — Angular Router huỷ navigation đang chờ khi có navigation mới, nên navigation cũ (đã set cờ) có thể KHÔNG BAO GIỜ emit lại để tự tiêu thụ cờ, làm rò rỉ chặn nhầm request hợp lệ kế tiếp (epic `sale-list-v2`, phát hiện qua `/bmad-review`).
- Trước khi đổi 1 endpoint list sang phân trang server-side (điều kiện để migrate lên V2), grep HẾT consumer của service/endpoint đó — không chỉ component đang migrate. Consumer khác có thể cần TOÀN BỘ dữ liệu (vd popup autocomplete tự lọc client-side) và sẽ bị BE mặc định cắt còn `per-page` mặc định (thường 20), mất dữ liệu im lặng không lỗi. Gặp thật: `/omi/quick-reply` (`QuickReplyList` cần trang, `OmnichatQuickreplyPopup` cần hết) — fix bằng tham số `?all=1` bỏ qua Paginate (epic `list-v2-final-cleanup`).

## CRITICAL: bảng KHÔNG phân trang thì đừng dùng `searchParams()`

`searchParams()` của base luôn kèm `page`/`sort`/`per-page`. Bảng không có `[rows]`/paginator → `dt.rows()` là `undefined` → `per-page=undefined` → BE trả **404** (gặp thật ở `NoteStatistics`).

```ts
// Bảng thống kê không phân trang — giữ đúng bộ params V1:
const params = { ...this.defaultSearch(), ...this.search(), ...this.requiredSearch() };
return this.modelService.list({ params }).pipe(...);
```
Quét nhanh cả module: component `extends BaseListV2` mà template có `<p-table>` nhưng **không** có `[rows]` → soi lại `fetchList()`.

## p-table full-height (footer/paginator dính đáy) — `[appFillHeight]`, KHÔNG `TableLayoutService`

`TableLayoutService.tableNormal()`/`tableDetail()` (breakpoint cứng theo `window.innerWidth`) bind vào `[style]="{minHeight/maxHeight: calc(100vh - Npx)}"` là cách CŨ, sai khi p-table lồng trong layout riêng của trang (con số breakpoint không trừ đúng phần header/toolbar phía trên — bug thật "treo lơ lửng" ở `emr/examination`, epic `p-table-full-height`). Dùng directive `[appFillHeight]` (`share/directive/fill-height.directive.ts`) thay thế:

```html
<p-table appFillHeight [scrollHeight]="'flex'" ...>
```

- Directive chỉ tự thêm `flex-1 min-h-0` cho CHÍNH `<p-table>` — KHÔNG tự sửa ancestor. `[scrollHeight]="'flex'"` PHẢI tự khai tường minh (PrimeNG v22 `Table.scrollHeight` là `InputSignal` chỉ đọc, directive không gán được).
- **Ancestor PHẢI tự lo** — chain flex LIÊN TỤC từ khung bị chặn (viewport/tab bounded) xuống tận `<p-table>`, mỗi tầng `display:flex;flex-direction:column;min-height:0`. 2 case thật đã làm:
  - **Ancestor CÓ SẴN flex-bounded** (`emr/examination` — `.cx-workspace`/`.cx-sec-anchor--fill`): không cần `calc()` nào, `appFillHeight` là đủ.
  - **Ancestor CHỈ CÓ `min-height`, không phải khung chặn thật, VÀ dùng chung giữa nhiều tab** (`customer-detail.html` — `<p-tabpanels>` PrimeNG chỉ `[style.min-height]`, 11+ tab dùng CHUNG 1 `<p-tabpanels>`) — **KHÔNG style thẳng element `p-tabpanel`/`p-tabs`/`p-tablist` bằng CSS blanket cho TOÀN BỘ tabpanel** (đã thử, revert 2026-08-07 — xem lịch sử `bmad/p-table-full-height/stories/story-02-*.md`). 2 lỗi thật khi làm vậy: (1) `p-tabpanel { display:flex }` (author CSS) đè mất `[hidden]{display:none}` (UA stylesheet, author LUÔN thắng UA bất kể specificity) → mọi tabpanel (kể cả tab không active) cùng hiện, chia đều flex-space; (2) tab CHƯA migrate (list thường, cao tự nhiên, có paginator ở cuối) bị kẹp cứng theo `overflow-y:auto` của tabpanel dùng chung → paginator rớt xuống dưới vùng cuộn nội bộ thay vì hiện ngay, khác hẳn hành vi cũ (trang ngoài tự cuộn); (3) riêng tab có `display:grid` layout nhiều cột (vd tab "Thông tin" 2 cột) còn bị `p-tabpanel > * { display:flex }` đè mất `display:grid` do specificity `p-tabpanel > *` (0,1,1) cao hơn class `.grid` của Tailwind (0,1,0). **Đúng cách: SCOPE riêng 1 class (`.cx-tabpanel-fill`) cho ĐÚNG `<p-tabpanel>` đã migrate, KHÔNG đụng `<p-tabpanels>` gốc hay các `<p-tabpanel>` khác** — style thẳng element `p-tabpanel`/`p-tablist` CHỈ an toàn khi TOÀN BỘ các tab dùng chung `<p-tabpanels>` đó đều đã migrate (không còn tab "tự nhiên" nào trong nhóm).
```html
<div class="cx-tabpanel-fill" [style.--cx-table-detail]="tableDetail() + 'px'">
  <app-list-appointment-in-contact class="flex-1 min-h-0 flex flex-col" .../>
</div>
```
```scss
.cx-tabpanel-fill {
  height: calc(100vh - var(--cx-table-detail, 265px));
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
```
- **KHÔNG áp dụng được** khi 1 tab/section có ≥2 khối cạnh tranh chiều cao (vd tab "orders" của `examination`: `service-order` + `cls-results` xếp chồng) — ép `flex:1` đồng loạt sẽ vỡ layout (không tự chia được không gian). Cần thiết kế riêng, không dùng `appFillHeight` mù quáng.
- Ancestor có `overflow-y:auto` riêng (vd `.cx-workspace`) + p-table tự scroll nội bộ (`scrollHeight="flex"`) → 2 lớp scroll chồng nhau, sai số làm tròn dưới-pixel làm thanh cuộn NGOÀI chớp hiện rồi ẩn. Tắt hẳn `overflow` ở ancestor đó khi tab đang active là list full-height (xem `.cx-workspace--fill` trong `examination.scss`).
- **Cuộn ngoài trang dư vài px SAU KHI đã đo đúng KHÔNG chắc do bảng** — xác nhận bằng cách đo tab/trang KHÁC hoàn toàn không đụng gì (`document.scrollingElement.scrollHeight - window.innerHeight`), nếu vẫn dư y hệt thì đó là bug GLOBAL không liên quan p-table. Gặp thật: `src/assets/overrides/custom.scss` có 2 rule TRÙNG selector `.layout-content-wrapper .layout-content` (padding khác nhau, cả 2 `!important`, rule sau âm thầm thắng) — sửa ở `custom.scss`, KHÔNG cố bù bằng cách co bảng lại (sai chỗ, blast radius sai).

## `appFillHeight="page"` — trang list ROUTED top-level (thay `tableNormal()`)

Mode KHÁC với `[appFillHeight]` trần ở trên (đó là cụm `tableDetail()`, p-table NHÚNG trong tab/trang
khác). Mode `page` dành cho trang list ROUTED TOP-LEVEL trước đây dùng
`TableLayoutService.tableNormal()` bind `[style]="{minHeight/maxHeight: calc(100vh - tableNormal()px)}"`
— cùng lớp lỗi hằng số cứng theo `window.innerWidth`, xác nhận qua đo thật (epic
`p-table-full-height-list-pages`, 310 file, `bmad/p-table-full-height-list-pages/pilot-findings.md`):

```html
<p-table appFillHeight="page" [scrollHeight]="'flex'" [style]="{ width: '100%' }" ...>
```

- Vì sao hằng số sai: **`1rem = 12px` trong app này** (KHÔNG phải 16px mặc định trình duyệt — mọi
  tính rem phải dựa vào số đo thật, không suy từ 16px), breadcrumb (`flex-wrap: wrap`) có thể xuống
  2 dòng ở màn hẹp, và mode menu ngang (`_sidebar_horizontal.scss`) đổi `padding-top` của
  `.layout-content-wrapper` 4rem→7rem — không có 1 hằng số nào đúng cho mọi viewport/mode.
- Directive (`share/directive/fill-height.directive.ts`) tự đi ngược DOM từ `<p-table>` lên
  `.layout-content-wrapper` (shell dùng chung MỌI route, `AppLayout`), gắn `display:flex;
  flex-direction:column` cho MỌI tầng trung gian nó đi qua (class `.cx-fill-chain`) + chính wrapper
  (`.cx-fill-page`) ở `ngAfterViewInit` — **và BẮT BUỘC tự gỡ ở `ngOnDestroy`**. `.layout-content-
  wrapper` KHÔNG tái tạo khi đổi route (SPA) — không gỡ ⇒ trang KẾ TIẾP (form dài, dashboard) bị
  khoá `height:100vh` bởi tính năng của trang TRƯỚC, cắt cụt nội dung mà không có lỗi console nào.
- **Gate chống "quên import" — BẮT BUỘC chạy, KHÔNG có gate build nào khác bắt được lỗi này**:
  `appFillHeight="page"` là attribute TĨNH, không phải `[appFillHeight]="'page'"`. Thiếu
  `FillHeightDirective` trong `imports: [...]` của `@Component` ⇒ Angular bỏ qua attribute lạ CÂM
  LẶNG — đã đo thực nghiệm: `tsc --noEmit`/`ngc --noEmit`/`ng build` đều KHÔNG lệch 1 lỗi nào. Chạy
  `frontend/scripts/verify-fill-height-page.sh` sau MỌI lần thêm/sửa `appFillHeight="page"`.
- 12 file có ≥2 `<p-table>` trong cùng host — soi tay TRƯỚC khi áp mặc định (11/12 trường hợp thật
  ở epic này là bảng thứ 2 lồng trong `<ng-template #expandedrow>` — PrimeNG row-expansion, KHÔNG
  cạnh tranh chiều cao, áp `appFillHeight="page"` cho bảng NGOÀI bình thường; chỉ 1/12 là 3 bảng
  cạnh nhau thật trong `<p-tabs>`, xem 2 gotcha dưới).
- **Directive KHÔNG chỉ dùng cho `<p-table>`** — `[appFillHeight]` là generic (`Renderer2` thao tác
  `ElementRef` bất kỳ). Trang dùng `<div [style]="{height:calc(...)}}">` bọc cả report/form (KHÔNG
  có `<p-table>` nào) vẫn áp được y hệt, chỉ bỏ `[scrollHeight]="'flex'"` (thuộc tính riêng
  `p-table`). Đừng kết luận "không áp dụng được" chỉ vì file không có `<p-table>`.
- **Ancestor đã tự là flex-row hoặc CSS Grid** (PrimeFlex `.grid` — `display:flex;flex-wrap:wrap`,
  dùng cho layout 2+ CỘT CẠNH NHAU; hoặc Tailwind `.grid`/`grid-cols-*` — CSS Grid thật, dùng cho
  layout lưới nhiều cột như `emr/examination` cấp trang): directive TỰ đo `getComputedStyle(node)
  .display` trước khi quyết định — ancestor đã là `flex`/`inline-flex`/`grid`/`inline-grid` thì CHỈ
  thêm sizing (`cx-fill-chain`), KHÔNG ép `flex-direction:column` (`cx-fill-chain-stack`) đè lên,
  tránh phá layout ngang/lưới đang có. **Bẫy đặt tên trùng**: PrimeFlex và Tailwind CÙNG định nghĩa
  class tên `.grid` với Ý NGHĨA KHÁC NHAU (PrimeFlex: `display:flex`; Tailwind: `display:grid`) —
  đo `getComputedStyle` thật để biết cái nào đang thắng trong file cụ thể, ĐỪNG suy từ tên class.
- **Ancestor có thể mang attribute `hidden`** (PrimeNG `<p-tabpanel [hidden]="!active()">`, xem
  `node_modules/primeng/.../primeng-tabs.mjs`): CSS tác giả LUÔN thắng UA stylesheet với declaration
  thường (không `!important`) BẤT KỂ specificity — set `display:flex` không điều kiện sẽ làm tab đã
  ẩn vẫn hiện đè (bug thật đã phải revert 2026-08-07). 2 class CSS đều guard `:not([hidden])` trong
  `_content.scss` — ancestor `[hidden]` thì không rule author nào khớp, UA `[hidden]{display:none}`
  áp dụng bình thường. Áp dụng được cho MỌI `<p-tabpanel>` chứa `appFillHeight="page"`, không cần xử
  lý riêng từng file.

## Giới hạn đã biết

- Lối vào DUY NHẤT của một hành động (nút "Chi tiết", link mở panel) đặt trong `@if (isColVisible('x')())` sẽ biến mất hẳn khi `x` không nằm trong `selectedColumns` mặc định — build xanh, không lỗi. Trước khi neo nút vào cột nào: đối chiếu `selectedColumns` của CHÍNH list đó; cột có mặt ở mọi list là tên khách hàng.
- Filter đặt ở header cột (`p-columnFilter`) vẫn lọc nhưng KHÔNG lên URL — cần giữ qua URL thì khai thành `FilterFieldDef` trong panel.
- Mũi tên sort trên header không sáng lại sau khôi phục (sort vẫn đúng); bind `[sortField]` trên bảng lazy đẻ thêm request lúc init.
- URL nội bộ của router lệch thanh địa chỉ (hệ quả của `replaceState`) — chỗ nào so URL đầy đủ cần biết điều này.



---

## 1. `BaseDetail<T>` — single record view (routed)

**File:** `src/app/share/base/base-detail.ts`
**Pattern:** extend + inject modelService + override `afterModelLoaded` nếu cần

```typescript
export class CustomerDetail extends BaseDetail<CustomerModel> implements OnInit {
    modelService = inject(CustomerService);

    ngOnInit() {
        // BaseDetail tự đọc route param :id + gọi modelService.get(id)
        super.ngOnInit();
    }

    // Override hook
    override afterModelLoaded(model: CustomerModel): void {
        this.title.setTitle(model.fullname);
    }
}
```

Template: `{{ model().fullname }}`, `{{ model().phone }}`, etc.

---

## 2. `BaseReport<T>` — report với export Excel

**File:** `src/app/share/base/base-report.ts`
**Pattern:** extend + implement 3 abstract members

```typescript
export class RevenueReport extends BaseReport<RevenueItem> implements OnInit {
    modelService = inject(RevenueService);

    // BẮT BUỘC implement
    rfFilter = this.fb.group({
        dateFrom: [this.timezoneService.monthStartDate()],
        dateTo: [this.timezoneService.todayDate()],
    });

    fetchData(): Observable<any> {
        return this.modelService.getRevenue({ params: this.searchParams() });
    }

    fetchExport(): Observable<string> {
        return this.modelService.exportRevenue({ params: this.searchParams() });
    }
}
```

BaseReport tự handle: `loadReport()`, `exportExcel()`, pagination, quick date select, keyword search.

`rfFilter` phải là single source of truth: → Xem `frontend/skill/skills/frontend-hard-rules/SKILL.md` B.16.

---

## 3. `BasePrint` — print pages

**File:** `src/app/share/base/base-print.ts`
**@Injectable** — KHÔNG phải @Directive, KHÔNG extend BaseApp.

```typescript
export class PrintXxx extends BasePrint implements OnInit {
    // KHÔNG có sẵn BaseApp services — inject thủ công nếu cần
    modelService = inject(XxxService);
    private destroyRef = inject(DestroyRef);

    ngOnInit(): void {
        // Pattern chuẩn: tryLoadFromCache → API fallback
        this.activatedRoute.params
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe((params: any) => {
                const cacheKey = this.activatedRoute.snapshot.queryParams['cacheKey'];
                const cached = this.tryLoadFromCache(cacheKey);
                if (cached) { this.applyData(cached); return; }
                this.modelService.getXxx(params.id).subscribe((d) => this.applyData(d));
            });
    }

    applyData(data: any): void {
        this.model.set(data);
        this.printData.setModel({ company: ..., layoutType: ... });
        setTimeout(() => void this.handlePrintAfterInit());
    }
}
```

→ Xem `fe-print.md` cho toàn bộ print flow.


## ExportOptionsDialog (reusable)

**File:** `src/app/modules/utilities/export-options-dialog/export-options-dialog.ts`

Mở trước khi gọi API export, cho user chọn:
- `'selected'` — chỉ export cột đang bật trong `selectedColumns`
- `'all'` — export toàn bộ cột

```typescript
// Trong component có export:
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';

exportExcel() {
  const ref = this.dialogWrapperService.open(ExportOptionsDialog, {
    header: this.translate.instant('utilities.export_options_dialog.header'),
    width: '480px',
    data: {
      selectedCount: this.selectedColumns().length,
      totalCount: this.displayCols.length,   // tổng cột trong availableColumns/displayCols
    },
  });

  ref.onClose.subscribe((mode: ExportMode | null) => {
    if (!mode) return;
    const params: any = { ...this.searchParams() };
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    // gọi API export với params
  });
}
```

**i18n keys** (đã có đủ 4 locale): `utilities.export_options_dialog.header/selected_label/selected_desc/all_label/all_desc`

---

## BE — colDef pattern cho export có column filter

Khi BE cần support `?columns=...` param, dùng `colDef` slice thay vì hardcode headers/mapper.

**Xem pattern đầy đủ:** `(repo cha) ../skill/skills/working-on-ops/reference/export.md § colDef pattern`

---

## Quy tắc áp dụng

- `displayCols` (array tất cả cột) và `selectedColumns` (signal) phải có cùng `value` key với BE `colDef[i].key`
- `mode === 'all'` → KHÔNG gửi `columns` param → BE fallback export all
- `mode === 'selected'` → gửi `columns=val1,val2,...` → BE filter theo thứ tự `colDef`
- Default mode trong dialog = `'selected'` (UX: user thường muốn export những gì đang thấy)
- **Param export LUÔN lấy từ `this.exportParams()` của `BaseListV2` — KHÔNG tự dựng lại** (bug 2026-08-13, dính 58 màn):

  ```ts
  // ĐÚNG
  const params = {...this.exportParams()} as any;
  // SAI — thiếu `sort`, file export ra thứ tự khác bảng user đang nhìn
  const params = {...this.defaultSearch(), ...this.search(), ...this.requiredSearch()} as any;
  ```

  Lý do: BE dùng CHUNG một query cho list và export (nhánh `_export=1` nằm trong list handler), thiếu `sort` là BE rơi về defaultOrder (thường `id DESC`). `exportParams()` = `searchParams()` bỏ `page`/`per-page` (export lấy trọn kết quả) nhưng giữ `sort`.
  Kiểm tra kèm: cột cho `pSortableColumn` phải là cột DB thật có trong whitelist `ApplySort` của handler BE — sort cột hiển thị/tính toán (vd `appointment_time` chỉ là format của `appointment_date`) thì BE reject âm thầm, list và export đều không sort được như user tưởng.
