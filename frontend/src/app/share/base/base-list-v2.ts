import {
  DestroyRef,
  Directive,
  Injector,
  OnDestroy,
  Type,
  afterNextRender,
  computed,
  effect,
  inject,
  isDevMode,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormGroup } from '@angular/forms';
import { Location } from '@angular/common';
import { ActivatedRoute, NavigationEnd, NavigationSkipped } from '@angular/router';
import { FilterMetadata } from 'primeng/api';
import { FileSelectEvent } from 'primeng/fileupload';
import { Table } from 'primeng/table';
import { Observable, Subject, finalize, of } from 'rxjs';
import { catchError, debounceTime, distinctUntilChanged, filter, switchMap } from 'rxjs/operators';

import { Identifiable } from '@/core/interface/identifiable';
import { BaseApp } from '@/share/base/BaseApp';
import { PERMISSION_SELECT, perPageOptionsTable, rowsTable, STORAGE_KEY_SELECT } from '@/define';
import { ListApiResponse, PaginationInfo } from '@/share/models/list-api-response';
import { BaseApiService } from '@/share/services/base-api.service';
import {
  FilterFieldDef,
  MAX_TEXT_FILTER_LENGTH,
  clearedValueOf,
  defaultValueOf,
  isBlankFilterValue,
  matchModeOf,
  toDateOnlyString,
  toMonthDayString,
  urlKeyOf,
} from '@/share/base/list-filter.model';
import { UtilsService, dequal } from '@/share/services/utils.service';
import {
  ListUrlParams,
  URL_STATE_FLAG,
  URL_STATE_PAGE,
  URL_STATE_SIZE,
  URL_STATE_SORT,
  filterParamName,
  hasListUrlState,
  parseFilterValue,
  serializeFilterValue,
  stateParamName,
} from '@/share/utils/list-url-state.util';

/**
 * BaseList thế hệ 2 — kế thừa nguyên BaseList cũ (CRUD, cột hiển thị, cell edit... giữ nguyên),
 * thêm 2 thứ và KHÔNG đụng gì tới các component đang extend BaseList:
 *
 * 1. **Filter khai báo** — khai `filterFields()` một lần, base tự lo FormGroup, giá trị mặc định,
 *    subscription valueChanges → `dt.filter()`, object filters khởi tạo, và `clearFilter()`.
 *    Ở BaseList cũ mỗi list phải tự viết `initFilter()` (~90 dòng valueChanges lặp lại) +
 *    `getDefaultFilterValues()` (~30 dòng lặp lại lần 2) + `clearFilter()` (lần 3).
 *
 * 2. **State nằm trên URL** — filter/trang/sort đẩy ra query param đọc được, nên bấm vào 1 bản ghi
 *    → xem chi tiết → Back là còn nguyên filter; F5 còn nguyên; copy link gửi người khác ra đúng
 *    danh sách đang xem. Bật bằng cách đặt `URL_STATE_PREFIX`.
 *
 * ## Dùng
 * ```ts
 * export class XxxList extends BaseListV2<XxxModel> implements OnInit {
 *   @ViewChild('dt') dt!: Table<XxxModel>;
 *   protected override URL_STATE_PREFIX = 'xx';
 *
 *   protected filterFields(): FilterFieldDef[] {
 *     return [
 *       { name: '_keyword', kind: 'text', urlKey: 'keyword' },
 *       { name: 'status', kind: 'in' },
 *       { name: 'created_at', kind: 'dateRange',
 *         default: () => [this.timezoneService.todayStart(), this.timezoneService.todayEnd()] },
 *     ];
 *   }
 *
 *   ngOnInit() { this.initListV2(); }
 * }
 * ```
 *
 * Template bắt buộc 3 binding để khôi phục state chạy đúng:
 * ```html
 * <p-table #dt [lazy]="true" (onLazyLoad)="loadRecords($event)"
 *          [filters]="initialFilters()" [first]="first()" [lazyLoadOnInit]="false" ...>
 * ```
 * (`[lazyLoadOnInit]="false"` vì base tự bắn lần load đầu — xem `scheduleFirstLoad`.)
 *
 * ## Giới hạn đã biết
 * - Nguồn sự thật của filter là `rfFilterModel`. Filter đặt trực tiếp ở header cột
 *   (`p-columnFilter`) vẫn lọc bình thường nhưng KHÔNG lên URL — cần giữ qua URL thì khai
 *   thành `FilterFieldDef` trong panel.
 * - Chỉ mục sort trên header chưa khôi phục theo URL (sort vẫn đúng, chỉ là mũi tên trên
 *   header không sáng). Bind `[sortField]` trên bảng lazy dễ đẻ thêm 1 request lúc init nên
 *   chưa làm.
 */
@Directive()
export abstract class BaseListV2<T extends Identifiable> extends BaseApp implements OnDestroy {
  /** ViewChild của p-table ở component con */
  abstract dt: Table<any>;

  /** Prefix query param. Rỗng = tắt hẳn URL sync (filter vẫn chạy). */
  protected URL_STATE_PREFIX = '';

  // ── Thành viên từ BaseList V1 (gộp 2026-08-05, epic list-v2-final-cleanup story-03) ──
  // BaseListV2 không còn `extends BaseList` — CRUD/cột hiển thị/cell-edit chép thẳng vào đây.
  // `base-list.ts` (BaseList V1) VẪN GIỮ NGUYÊN, không xoá: 16 component (Schedule*, config-omnichannel
  // không lazy, UserRole...) tiếp tục `extends BaseList` vĩnh viễn có chủ đích (không có `<p-table>`
  // lazy hoặc BE không phân trang do dữ liệu quá nhỏ) — xem `list-v2-rollout.md` §"Lộ trình xoá hẳn
  // BaseList V1". Bỏ có chủ đích khi gộp (dead code xác nhận 0 usage toàn app): `initBaseList()`
  // (cửa vào V1, V2 tự có `initListV2()`), `saveFilter()`/`loadFilter()`/`STORAGE_KEY_FILTER` (bộ nhớ
  // filter qua localStorage, 0 component nào set key này). Giữ `loadSort()`/`STORAGE_KEY_SORT` —
  // `initListV2()` (dưới) vẫn gọi thật khi URL không mang sort.
  permission = PERMISSION_SELECT;
  abstract modelService: BaseApiService<T>;

  timer: any = null;
  fileToUpload: File | null = null;
  formComponent!: Type<any>;
  updatePermissionString = '';
  updateDialogTitleString = '';
  updateDialogTitleKey = '';
  createPermissionString = '';
  createDialogTitleString = '';
  createDialogTitleKey = '';
  perPageOptionsTable = perPageOptionsTable;
  readonly page = signal(1);
  readonly perPage = signal(rowsTable);
  readonly sort = signal('-id');
  readonly models = signal<T[]>([]);
  readonly paginationInfo = signal<PaginationInfo | null>(null);
  readonly selectedRecords = signal<T[]>([]);
  readonly cols = signal<any[]>([]);
  readonly search = signal<Partial<Record<string, any>>>({});
  readonly defaultSearch = signal<Partial<Record<string, any>>>({});
  readonly requiredSearch = signal<Partial<Record<string, any>>>({});
  selectedColumns = signal<string[]>([]);
  readonly searchParams = computed(() => ({
    page: this.page(),
    sort: this.sort(),
    'per-page': this.perPage(),
    ...this.defaultSearch(),
    ...this.search(),
    ...this.requiredSearch(),
  }));
  /**
   * Param cho request export (`?_export=1`) — PHẢI dùng cái này, không tự dựng lại từ
   * `{...defaultSearch(), ...search(), ...requiredSearch()}`.
   *
   * Khác `searchParams` ở chỗ bỏ `page`/`per-page` (export lấy trọn kết quả, không phân trang)
   * nhưng GIỮ `sort`: BE dùng CHUNG một query cho list và export (nhánh `_export=1` nằm ngay
   * trong list handler), nên thiếu `sort` là BE rơi về defaultOrder (thường `id DESC`) → file
   * export ra thứ tự khác hẳn bảng user đang nhìn.
   */
  readonly exportParams = computed(() => ({
    sort: this.sort(),
    ...this.defaultSearch(),
    ...this.search(),
    ...this.requiredSearch(),
  }));
  readonly skeletonRows = signal(this.perPage());
  readonly padCols = signal(0);
  protected utilsService = inject(UtilsService);
  protected destroyRef = inject(DestroyRef);
  protected LS_SELECTED_COLUMNS_KEY = '';
  protected STORAGE_KEY_SORT = '';
  protected rfFilterModel!: FormGroup;
  protected isOpeningForm = signal(false);
  protected prevCellValues = new Map<string, any>();

  constructor() {
    super();
    const saved = this.storageService.loadStorage<string>(STORAGE_KEY_SELECT.KEY_ROWS_TABLE, false);
    if (saved) this.perPage.set(parseInt(saved));
  }

  /** Kiểm tra cột có hiển thị không */
  isColVisible = (col: string) =>
    computed(() => Array.isArray(this.selectedColumns()) && this.selectedColumns().includes(col));

  /** Object filters khởi tạo cho p-table — bind `[filters]="initialFilters()"` */
  readonly initialFilters = signal<Record<string, FilterMetadata>>({});

  /** Bản ghi đầu của trang hiện tại — bind `[first]="first()"` để khôi phục đúng trang */
  readonly first = signal(0);

  /**
   * Chỉ lần tải ĐẦU TIÊN của trang mới hiện skeleton — bind `[loading]="showSkeleton()"`.
   *
   * Điều kiện phải là "đã từng tải xong lần nào chưa", KHÔNG phải "models có rỗng không":
   * lọc lúc kết quả hiện tại đang rỗng thì models rỗng, điều kiện theo models sẽ bật lại
   * skeleton → chớp nguyên khối 15 dòng giữa empty message và kết quả mới (đo được ~25ms,
   * chiều cao nhảy 1 → 15 → 2 dòng).
   */
  readonly showSkeleton = computed(() => this.loading() && !this.loadedOnce());

  /** Đang tải lại trong khi vẫn giữ nội dung cũ — dùng cho thanh báo mảnh trên caption */
  readonly refreshing = computed(() => this.loading() && this.loadedOnce());

  /** Đã tải xong ít nhất 1 lần (kể cả kết quả rỗng hoặc lỗi) */
  private readonly loadedOnce = signal(false);

  /** `loading` do searchHandler của từng list tự tắt → bám theo nó thay vì bắt list phải khai báo */
  private readonly trackLoaded = effect(() => {
    if (!this.loading()) this.loadedOnce.set(true);
  });

  private activatedRoute = inject(ActivatedRoute);
  private location = inject(Location);
  private injector = inject(Injector);
  private filterDefs: FilterFieldDef[] = [];
  private initialSort = '';
  private initialPerPage = 0;
  private urlSyncReady = false;
  /**
   * `writeUrlState` có lúc chạy trong `setTimeout` (đợi router commit URL). Nếu component bị huỷ
   * trong khoảng đó — bấm nhanh sang menu khác — thì nó sẽ ghi đè URL của TRANG MỚI.
   */
  private destroyed = false;

  /** Khai báo toàn bộ filter của list */
  protected abstract filterFields(): FilterFieldDef[];

  private readonly reload$ = new Subject<void>();

  /**
   * Lấy dữ liệu cho list. Component con override HÀM NÀY (không override `searchHandler`) khi cần
   * xử lý thêm response — vd gắn label từ IndexedDB, tính cờ hiển thị cho từng dòng.
   *
   * KHÔNG tự set `loading`/`models`/`paginationInfo` trong đây: base lo hết ở
   * `wireReloadPipeline()`, có vậy cơ chế huỷ request cũ mới đúng.
   */
  protected fetchList(): Observable<ListApiResponse<T>> {
    return this.modelService.list({ params: this.searchParams() });
  }

  /**
   * Kết quả rỗng hợp lệ (không phải lỗi) — dùng khi `fetchList()` override cần bỏ qua request
   * vì thiếu điều kiện bắt buộc (vd `customer.id` chưa sẵn sàng lúc tab con mới mount).
   */
  protected emptyListResult(): Observable<ListApiResponse<T>> {
    return of({ items: [], _meta: { totalCount: 0 } });
  }

  /**
   * Mọi lần tải đều đi qua một stream duy nhất có `switchMap` → request cũ bị HUỶ THẬT ở tầng
   * HTTP khi có request mới.
   *
   * Không có nó thì mỗi lần lọc là một request độc lập, không ai bảo đảm thứ tự về: gõ từ khoá
   * nhanh hoặc mạng chập chờn → response của filter CŨ về sau cùng và ghi đè kết quả mới, bảng
   * hiển thị một đằng còn panel filter một nẻo. Lỗi này có sẵn ở `BaseList` V1; ở V2 nó còn khó
   * thấy hơn vì đã bỏ skeleton (giữ nguyên rows cũ) nên không còn ranh giới trực quan nào.
   *
   * Chỉ áp cho list dùng V2 — `BaseList` cũ giữ nguyên để không đổi hành vi 300+ list chưa migrate.
   */
  private wireReloadPipeline(): void {
    this.reload$
      .pipe(
        switchMap(() =>
          this.fetchList().pipe(
            catchError((err: any) => {
              console.error(err);
              return of(null);
            }),
          ),
        ),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((res: ListApiResponse<T> | null) => {
        this.loading.set(false);
        if (!res) {
          this.models.set([]);
          this.paginationInfo.set(null);
          return;
        }
        this.models.set(res.items ?? []);
        this.paginationInfo.set(res._meta ?? null);
      });
  }

  /** Đẩy yêu cầu tải vào stream. Component con KHÔNG override hàm này — override `fetchList()`. */
  searchHandler(): void {
    this.reload$.next();
  }

  /**
   * Toggle 1 field boolean (0/1) qua p-toggleswitch dùng chung cho mọi list.
   * [(ngModel)] đã đổi giá trị field TRƯỚC khi (onChange) fire — gọi API lưu ngay, và
   * rollback về giá trị cũ nếu lỗi (vd version conflict) để UI không nói đã lưu trong khi
   * DB chưa đổi (silent mismatch nếu thiếu bước rollback này).
   */
  protected toggleField(id: number, field: string) {
    const row = this.models().find((m: any) => m.id === id) as any;
    if (!row) return;
    const newVal = +row[field] ? 1 : 0;
    const prevVal = newVal ? 0 : 1;
    // story-23: khoá optimistic lock đổi tên `version` → `updated_at` cho 9 bảng đã sang BE story-17;
    // bảng CHƯA đổi (còn dùng `toggleField` dùng chung này) vẫn giữ `version` — chọn theo field row
    // ĐANG CÓ, không hardcode 1 trong 2 tên để không phá các bảng khác.
    const lockField = row.updated_at !== undefined ? 'updated_at' : 'version';
    this.modelService.update(id, { [field]: newVal, [lockField]: row[lockField] } as any).subscribe({
      next: (res: any) => {
        const updated = res?.data ?? res;
        const existing = this.models().find((m: any) => m.id === updated.id);
        if (existing) {
          this.updateRow({ ...existing, [field]: updated[field], [lockField]: updated[lockField] } as T);
        }
        this.noti.successRes(res);
      },
      error: (err: any) => {
        const existing = this.models().find((m: any) => m.id === id);
        if (existing) {
          this.updateRow({ ...existing, [field]: prevVal } as T);
        }
        this.noti.errorMsg(err?.error?.message || this.translate.instant('common.message.error'));
      },
    });
  }

  /** Xử lý thay đổi trạng thái (ẩn/hiện) của bản ghi */
  handleStatusChange(id: number) {
    this.toggleField(id, 'status');
  }

  /** Xử lý lỗi hình ảnh (avatar) */
  errorHandler(event: any) {
    event.target.src = 'assets/overrides/images/no-avatar.svg';
  }

  /** Tìm label theo value trong mảng object */
  findValue(idValue: any, objectArray: { value: any; label: string }[]): string | undefined {
    return objectArray.find((x) => x.value === idValue)?.label;
  }

  /** Xử lý upload file import dữ liệu */
  onBasicUploadAuto(files: FileSelectEvent, form: any) {
    this.fileToUpload = files['files'][0];
    // Template chuẩn: import-button ngay trước <p-fileupload class="hidden">.
    // Anchor confirmpopup vào button (visible) — fileupload host hidden → popup render ở 0,0.
    const host: HTMLElement = form.el.nativeElement;
    const anchor = (host.previousElementSibling as HTMLElement) ?? host;
    this.confirmationService.confirm({
      target: anchor,
      message: this.translate.instant('common.message.import_confirm'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: { severity: 'danger', size: 'small' },
      accept: () => {
        this.modelService
          .importData(this.fileToUpload!)
          .pipe(finalize(() => form.clear()))
          .subscribe({
            next: () => {
              this.noti.successMsg(this.translate.instant('common.message.import_success'));
              this.getList();
            },
          });
      },
      reject: () => {
        this.getList();
        form.clear();
      },
    });
  }

  /** Mở dialog tạo mới bản ghi */
  createRecord(): void {
    if (
      this.createPermissionString &&
      !this.permissionsService.getPermission(this.createPermissionString)
    ) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const header = this.createDialogTitleKey
      ? this.translate.instant(this.createDialogTitleKey)
      : (this.createDialogTitleString || this.translate.instant('common.action.create'));
    const dialogRef = this.dialogWrapperService.open(this.formComponent, {
      width: '840px',
      header,
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: T) => {
      if (result) {
        this.getList();
      }
    });
  }

  /** Mở dialog cập nhật bản ghi */
  updateRecord(model: T) {
    if (
      this.updatePermissionString &&
      !this.permissionsService.getPermission(this.updatePermissionString)
    ) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const header = this.updateDialogTitleKey
      ? this.translate.instant(this.updateDialogTitleKey)
      : (this.updateDialogTitleString || this.translate.instant('common.action.update'));
    const dialogRef = this.dialogWrapperService.open(this.formComponent, {
      width: '840px',
      header,
      data: { model },
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: T) => {
      if (result) {
        this.updateRow(result);
      }
    });
  }

  /** Cập nhật lại dữ liệu trong models khi có bản ghi mới */
  updateRow(updated: T): void {
    const current = this.models();
    const index = current.findIndex((item) => item.id === updated.id);
    if (index !== -1) {
      const cloned = [...current];
      cloned[index] = updated;
      this.models.set(cloned);
    }
  }

  /** Xóa bản ghi */
  deleteRecord(event: Event, id: number): void {
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: event.currentTarget as EventTarget,
      message: this.translate.instant('common.label.do_you_want_to_delete'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: { severity: 'danger', size: 'small' },
      accept: () => {
        this.loadingService.showLoading('deleteRecord');
        this.modelService
          .delete(id)
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('deleteRecord');
            }),
          )
          .subscribe({
            next: (res: any) => {
              this.getList();
              // BE trả {message: "Xóa thành công"} — dùng successRes để đọc res.message
              this.noti.successRes(res);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  /** Lưu lại các cột đã chọn vào localStorage */
  saveSelectedColumns() {
    if (this.LS_SELECTED_COLUMNS_KEY) {
      this.storageService.saveStorage(this.LS_SELECTED_COLUMNS_KEY, this.selectedColumns());
    }
  }

  /** Load các cột đã chọn từ localStorage */
  loadSelectedColumns() {
    const loaded = this.storageService.loadStorage<string[]>(this.LS_SELECTED_COLUMNS_KEY, true);
    this.selectedColumns.set(Array.isArray(loaded) ? loaded : this.selectedColumns());
  }

  /** Lưu sort vào localStorage */
  saveSort() {
    if (this.STORAGE_KEY_SORT && this.sort()) {
      this.storageService.saveStorage(this.STORAGE_KEY_SORT, this.sort());
    }
  }

  /** Load sort từ localStorage — `initListV2()` chỉ gọi khi URL không mang sort. */
  loadSort() {
    if (this.STORAGE_KEY_SORT) {
      const sort = this.storageService.loadStorage<string>(this.STORAGE_KEY_SORT, false);
      if (sort) {
        this.sort.set(sort);
      }
    }
  }

  /** Xử lý thay đổi lựa chọn bản ghi */
  onSelectionChange(val: T[]) {
    this.selectedRecords.set(val);
  }

  /** Hàm hủy, đóng dialog khi destroy component */
  ngOnDestroy(): void {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    if (this.dialogRef) {
      this.dialogRef.close(false);
    }
  }

  /** Reset lại trang, gọi lại getList */
  resetPage() {
    this.getList();
  }

  /** Khởi tạo giá trị trước đó */
  onCellEditInit(e: any) {
    const rowId = e.data?.id ?? e.index?.id;
    const field = e.field ?? 'SortOrder';
    const key = `${rowId}:${field}`;
    this.prevCellValues.set(key, e.data?.[field]);
  }

  /** Xử lý sự kiện khi chỉnh sửa ô trong table */
  onCellEditComplete(e: any) {
    const id = e.data?.id ?? e.index?.id;
    const field = e.field ?? 'SortOrder';
    const key = `${id}:${field}`;
    const prev = this.prevCellValues.get(key);
    const curr = e.data?.[field];

    // Không đổi thì thôi
    if (String(prev) === String(curr)) {
      this.prevCellValues.delete(key);
      return;
    }

    // story-23: xem lý do chọn field theo row ở `toggleField()` (cùng file).
    const lockField = e.data?.updated_at !== undefined ? 'updated_at' : 'version';
    const patch: any = { [field]: curr, [lockField]: e.data?.[lockField] };

    this.modelService.update(id, patch).subscribe({
      next: (res: any) => {
        const updated = res?.data ?? res;
        // Merge thay vì replace nguyên row — updated là entity thô từ BE, không có các field
        // label FE tự gắn (addLabel/addLabels qua IndexedDB helper). Replace nguyên row sẽ làm
        // mất các label đó cho tới khi reload (vd companyInfo/unitLabel ở ServiceList).
        const existing = this.models().find((m: any) => m.id === updated?.id) as any;
        this.updateRow({ ...(existing ?? {}), ...updated } as T);
        const messageKey = field === 'SortOrder' ? 'common.message.saved_position' : 'common.message.saved_value';
        this.noti.success(this.translate.instant(messageKey, { prev, curr }));
        this.prevCellValues.delete(key);
      },
      error: (err) => {
        if (e.data) this.updateRow({ ...e.data, [field]: prev } as T);
        this.noti.error(this.translate.instant('common.message.save_error'), err?.message || '');
        this.prevCellValues.delete(key);
      },
    });
  }

  /** Hàm để override ở class con nếu cần — gọi trong `initListV2()`. */
  protected afterBaseListInit(): void {
    // Optional override in children
  }

  private makeRange = (n: number) => Array.from({ length: Math.max(0, n | 0) });

  /** Mảng số lượng dòng skeleton */
  readonly rowsIt = computed(() => this.makeRange(this.skeletonRows()));
  /** Mảng số lượng cột padding */
  readonly colsIt = computed(() => this.makeRange(this.padCols()));

  /**
   * Gọi trong ngOnInit của component con — thay cho `initFilter()` + `initBaseList()` cũ.
   */
  protected initListV2(): void {
    this.destroyRef.onDestroy(() => (this.destroyed = true));
    this.filterDefs = this.filterFields();
    this.assertFilterDefs();
    this.wireReloadPipeline();
    this.initialSort = this.sort();
    this.initialPerPage = this.perPage();
    this.rfFilterModel = this.buildFilterForm();

    // Khôi phục trước khi đấu dây valueChanges: patchValue lúc này không bắn filter thừa.
    const restoredFromUrl = this.restoreFromUrl();
    this.initialFilters.set(this.buildFiltersFromForm());
    this.wireFilterControls();

    // URL đã có sort thì không để localStorage ghi đè lên nó.
    if (!restoredFromUrl) this.loadSort();
    this.afterBaseListInit();
    this.loadSelectedColumns();
    this.urlSyncReady = true;
    this.watchUrlNavigation();
    this.scheduleFirstLoad();
  }

  /**
   * Đồng bộ lại state khi có ĐIỀU HƯỚNG THẬT tới chính route đang đứng.
   *
   * Ca điển hình: đang lọc ở `/crm/appointment?ap_f=1&ap_date=...` rồi bấm lại chính mục menu
   * "Danh sách lịch hẹn" (link không kèm query param). Route không đổi nên Angular TÁI SỬ DỤNG
   * component — `ngOnInit` không chạy lại — kết quả là URL trần trong khi UI vẫn giữ nguyên
   * filter. Lúc đó F5 sẽ ra kết quả khác hẳn cái đang nhìn.
   *
   * Xử theo đúng nguyên tắc "URL là nguồn sự thật":
   * - URL còn cờ state → khôi phục theo URL (vd người dùng dán link đã lọc trong khi đang ở trang).
   * - URL không còn cờ → coi như vào trang sạch từ menu → đặt filter về mặc định (như lần đầu mở
   *   màn — KHÁC nút "Xoá bộ lọc" là xoá về rỗng).
   *
   * PHẢI nghe `NavigationSkipped` chứ không chỉ `NavigationEnd`/`queryParams`: `replaceState` đổi
   * thanh địa chỉ nhưng KHÔNG báo cho router, nên sau lần ghi đầu router vẫn tưởng đang ở URL trần.
   * Bấm menu lần 2 (cũng trỏ URL trần) trùng đúng URL router đang giữ → router bỏ qua, phát
   * `NavigationSkipped`, `queryParams` không phát → nếu chỉ nghe `queryParams` thì lần 2 không
   * reset gì cả (đo được: lần 1 chạy, lần 2 im).
   *
   * Trạng thái đích đọc từ `e.url` của sự kiện, KHÔNG đọc `activatedRoute.snapshot` — với
   * `NavigationSkipped` thì snapshot không hề được cập nhật.
   *
   * `writeUrlState` dùng `replaceState` nên không sinh điều hướng → không có vòng lặp vô hạn.
   */
  private watchUrlNavigation(): void {
    if (!this.URL_STATE_PREFIX) return;
    const ownPath = this.route.url.split('?')[0];
    this.route.events
      .pipe(
        filter(
          (e): e is NavigationEnd | NavigationSkipped =>
            e instanceof NavigationEnd || e instanceof NavigationSkipped,
        ),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((e) => {
        const [path, query = ''] = e.url.split('?');
        // Điều hướng sang route khác (component sắp bị huỷ) → không đụng vào URL của trang mới.
        if (path !== ownPath) return;
        if (this.restoreFromUrl(new URLSearchParams(query))) {
          const filters = this.buildFiltersFromForm();
          this.initialFilters.set(filters);
          this.search.set(this.utilsService.buildFilter(filters));
          this.getList();
        } else {
          this.resetToDefaultFilters();
        }
        // Ghi URL phải ĐỢI router commit xong, không được gọi đồng bộ ở đây.
        // Angular mặc định `urlUpdateStrategy: 'deferred'` — router ghi URL lên thanh địa chỉ ở
        // CUỐI chu kỳ điều hướng, tức SAU khi `queryParams` phát. `replaceState` gọi ngay trong
        // handler sẽ bị router ghi đè lại bằng URL của điều hướng (vd `/crm/appointment` trần),
        // để lại URL không khớp state đang hiện. Đẩy sang macrotask để ghi sau cùng.
        setTimeout(() => this.writeUrlState());
      });
  }

  /**
   * Tự bắn lần load đầu tiên thay vì để p-table tự bắn.
   *
   * Lý do (PrimeNG v22): input `[filters]` được đồng bộ vào `dt.filters` bằng một `effect()`,
   * mà effect chạy SAU `ngOnInit` của bảng — trong khi chính `ngOnInit` mới là chỗ bảng bắn
   * `onLazyLoad` đầu tiên. Để bảng tự bắn thì request đầu đi với filter RỖNG, tức là state
   * khôi phục từ URL bị bỏ qua đúng ở lần load quan trọng nhất.
   *
   * `afterNextRender` chạy sau khi DOM đã render (ViewChild `dt` đã có) và nằm ngoài chu kỳ
   * change detection nên set signal ở đây không gây NG0100.
   *
   * → Template của list dùng BaseListV2 PHẢI có `[lazyLoadOnInit]="false"`, nếu không sẽ có
   *   thêm 1 request thừa lúc mở trang.
   */
  private scheduleFirstLoad(): void {
    afterNextRender(
      () => {
        if (!this.dt) return;
        this.dt.filters = { ...this.initialFilters() };
        this.loadRecords(this.dt.createLazyLoadMetadata());
      },
      { injector: this.injector },
    );
  }

  /**
   * Tải lại dữ liệu KHÔNG xoá rows đang hiển thị (stale-while-revalidate).
   *
   * `BaseList.getList()` cũ làm `models.set([])` trước mỗi lần gọi API: toàn bộ body bị huỷ,
   * thay bằng skeleton, xong lại dựng lại rows thật. Mỗi lần đổi filter là 2 lần đổi chiều cao
   * + mất vị trí scroll → giật/CLS. Nặng nhất là khi kết quả rỗng: skeleton → empty message →
   * rows, ba trạng thái nhấp nháy liên tiếp ngay dưới popover filter đang mở.
   *
   * Giữ nguyên rows cũ trong lúc chờ response thì chiều cao không đổi, không có gì nhảy.
   */
  getList(): void {
    // Số dòng skeleton phải khớp per-page hiện tại, nếu không lần đầu vẫn nhảy chiều cao
    // (chốt cứng giá trị này lúc khởi tạo thì per-page đổi sau đó sẽ lệch).
    this.skeletonRows.set(this.perPage());
    this.loading.set(true);
    this.searchHandler();
  }

  /**
   * p-table gọi mỗi lần đổi trang/sort/filter. Ghi lại state ra URL sau khi đã cập nhật.
   *
   * Thân hàm trước đây gọi `super.loadRecords(event)` (code V1 của `BaseList`) — gộp thẳng logic đó
   * vào đây (2026-08-05, story-03), BỎ lời gọi `this.saveFilter()` gốc (dead code đã xác nhận 0
   * component set `STORAGE_KEY_FILTER`), GIỮ `this.saveSort()` (còn dùng thật).
   */
  loadRecords(event: any): void {
    this.first.set(event.first ?? 0);
    const { first, rows, sortField, sortOrder, filters } = event;
    const page = Math.floor(first / rows) + 1;
    const filterObj = this.utilsService.buildFilter(filters);
    const sort = sortField ? (sortOrder === 1 ? sortField : `-${sortField}`) : this.sort();
    this.page.set(page);
    this.perPage.set(rows);
    if (typeof rows === 'number') {
      this.storageService.saveStorage(STORAGE_KEY_SELECT.KEY_ROWS_TABLE, rows, true);
    }
    this.sort.set(sort);
    this.saveSort();
    this.search.set(filterObj);
    this.getList();
    this.writeUrlState();
  }

  /**
   * Nút "Xoá bộ lọc" — dùng chung cho mọi list, component con không phải tự viết.
   *
   * Xoá về RỖNG, kể cả field có `default` (khoảng ngày "tháng này", ...): `default` chỉ là giá trị
   * lúc mới mở màn, người dùng bấm xoá là muốn bỏ hết điều kiện. Ngoại lệ duy nhất là field
   * `required` (BE trả 422 khi thiếu) → về mặc định. F5 sau đó không áp lại default: URL giữ cờ
   * `<prefix>_f` mà không có param filter ⇒ `restoreFromUrl()` hiểu là "đã xoá hết".
   */
  clearFilter(): void {
    this.resetFilters(clearedValueOf);
  }

  /** Về đúng trạng thái lúc mới mở màn (áp `default`) — khi bấm lại mục menu trỏ URL trần. */
  private resetToDefaultFilters(): void {
    this.resetFilters(defaultValueOf);
  }

  private resetFilters(valueOf: (def: FilterFieldDef) => any): void {
    const values: Record<string, any> = {};
    for (const def of this.filterDefs) values[def.name] = valueOf(def);
    this.rfFilterModel.reset(values, { emitEvent: false });

    const filters = this.buildFiltersFromForm();
    // Gán qua signal (input binding) để p-table nhận object mới, không tự tay sờ dt.filters.
    this.initialFilters.set(filters);
    this.first.set(0);
    this.page.set(1);
    this.perPage.set(this.initialPerPage);
    this.sort.set(this.initialSort);
    this.search.set(this.utilsService.buildFilter(filters));
    this.getList();
    this.writeUrlState();
  }

  /** Đặt giá trị 1 filter từ code (vd bấm vào ô trong bảng để lọc theo giá trị đó) */
  protected setFilterValue(name: string, value: any): void {
    this.rfFilterModel.controls[name]?.setValue(value);
  }

  /** Control → FilterMetadata. Override được nếu list cần luật riêng cho 1 field. */
  protected toFilterMeta(def: FilterFieldDef, value: any): FilterMetadata | null {
    if (def.toFilter) return def.toFilter(value);
    if (isBlankFilterValue(value)) return null;

    if (def.kind === 'dateRange') {
      const range = this.timezoneService.buildFilterTimestamp(value);
      return range ? { value: range, matchMode: 'between' } : null;
    }
    // Ngày đơn: BE nhận chuỗi `yyyy-MM-dd` (không phải unix, không phải khoảng).
    // matchMode mặc định 'custom' → với field bắt đầu bằng `_` thì `buildFilter` gửi thẳng
    // `<field>=yyyy-MM-dd`; field thường muốn `filter[x][eq]` thì khai `matchMode: 'equals'`.
    if (def.kind === 'dateSingle') {
      const str = value instanceof Date ? toDateOnlyString(value) : String(value);
      return { value: str, matchMode: def.matchMode ?? 'custom' };
    }
    // Khoảng ngày-tháng không năm: BE so cột dạng `MM-DD` (vd `birthday_md`), không phải unix.
    // `buildFilter` với matchMode 'between' sinh `filter[x][gte]=MM-DD&filter[x][lte]=MM-DD`.
    // Range chọn dở ([Date, null]) → null: chưa đủ 2 đầu thì không lọc.
    if (def.kind === 'monthDayRange') {
      const [a, b] = Array.isArray(value) ? value : [value, value];
      if (!(a instanceof Date) || !(b instanceof Date)) return null;
      return { value: [toMonthDayString(a), toMonthDayString(b)], matchMode: 'between' };
    }
    // Khoảng ngày mà BE đọc bằng 2 param TÊN RIÊNG (vd `_dueDateGte`/`_dueDateLte`):
    // một FilterMetadata chỉ sinh được MỘT tên param, nên base cố ý KHÔNG sinh gì ở đây —
    // component tự ghép trong `fetchList()`. Base vẫn giữ FormControl + đồng bộ URL cho field này.
    if (def.kind === 'dateRangeSplit') return null;
    // 'equals' = select 1 giá trị: control lỡ trả mảng (vd bị patch nhầm) thì vẫn phải gửi đi
    // 1 giá trị, nếu không BE nhận `in` và list hành xử như multiselect.
    if (def.kind === 'equals') {
      const scalar = Array.isArray(value) ? value[0] : value;
      return isBlankFilterValue(scalar) ? null : { value: scalar, matchMode: matchModeOf(def) };
    }
    return { value, matchMode: matchModeOf(def) };
  }

  /**
   * Chặn 4 lỗi khai báo mà nếu để lọt sẽ hỏng âm thầm — rất khó lần ra sau này:
   * trùng `name`, trùng param URL (vd `_status` và `status` cùng suy ra `status`), param
   * đụng tên các khoá state dành riêng (`_f`, `_page`, `_size`, `_sort`), và field `required`
   * thiếu `default`.
   *
   * Dev thì ném lỗi để thấy ngay lúc mở trang; production chỉ log để không làm chết cả màn hình.
   */
  private assertFilterDefs(): void {
    const problems: string[] = [];
    const reserved = new Set<string>([URL_STATE_FLAG, URL_STATE_PAGE, URL_STATE_SIZE, URL_STATE_SORT]);
    const seenNames = new Set<string>();
    const byUrlKey = new Map<string, string[]>();

    for (const def of this.filterDefs) {
      if (seenNames.has(def.name)) problems.push(`trùng name "${def.name}"`);
      seenNames.add(def.name);
      // `required` mà không có `default` ⇒ clearFilter() vẫn về rỗng ⇒ BE trả 422, không ai hay.
      if (def.required && def.default === undefined) {
        problems.push(`field "${def.name}" khai required nhưng thiếu default`);
      }
      if (def.urlSync === false) continue;
      const key = urlKeyOf(def);
      if (reserved.has(key)) {
        problems.push(`field "${def.name}" có urlKey "${key}" đụng khoá state dành riêng`);
      }
      byUrlKey.set(key, [...(byUrlKey.get(key) ?? []), def.name]);
    }
    for (const [key, names] of byUrlKey) {
      if (names.length > 1) {
        problems.push(`urlKey "${key}" bị dùng bởi ${names.join(', ')} — khai urlKey tường minh`);
      }
    }
    if (!problems.length) return;

    const msg = `[BaseListV2] filterFields() sai: ${problems.join(' | ')}`;
    if (isDevMode()) throw new Error(msg);
    console.error(msg);
  }

  private buildFilterForm(): FormGroup {
    const controls: Record<string, any> = {};
    for (const def of this.filterDefs) controls[def.name] = this.fb.control(defaultValueOf(def));
    return this.fb.group(controls);
  }

  private wireFilterControls(): void {
    for (const def of this.filterDefs) {
      const control = this.rfFilterModel.controls[def.name];
      if (!control) continue;
      // Chỉ báo 1 lần khi vừa vượt ngưỡng — gõ tiếp phía sau (raw vẫn dài dần) không toast dồn dập.
      let warnedTruncate = false;
      control.valueChanges
        .pipe(
          debounceTime(def.debounce ?? (def.kind === 'text' ? 500 : 300)),
          distinctUntilChanged(dequal),
          takeUntilDestroyed(this.destroyRef),
        )
        .subscribe((value: any) => {
          // Chặn trên độ dài filter text — value vượt ngưỡng đi thẳng vào query string GET,
          // có thể làm request vượt giới hạn độ dài dòng request của backend/proxy (414).
          if (def.kind === 'text' && typeof value === 'string' && value.length > MAX_TEXT_FILTER_LENGTH) {
            value = value.slice(0, MAX_TEXT_FILTER_LENGTH);
            control.setValue(value, { emitEvent: false });
            if (!warnedTruncate) {
              warnedTruncate = true;
              this.noti.warnMsg(
                this.translate.instant('common.validation.maxlength', { max: MAX_TEXT_FILTER_LENGTH }),
              );
            }
          } else {
            warnedTruncate = false;
          }
          const meta = this.toFilterMeta(def, value);
          // Value null vẫn phải gửi kèm matchMode để p-table xoá đúng entry filter cũ.
          this.dt?.filter(meta ? meta.value : null, def.name, meta?.matchMode ?? matchModeOf(def));
          // `initialFilters` phải là GƯƠNG TRUNG THỰC của form ở MỌI đường, không chỉ lúc khởi
          // tạo — component con dựng giá trị hiển-thị-lại từ nó (`computed()`) cho những ô lọc
          // không gắn được `formControlName` (vd `app-user-select`, `<input type="month">`,
          // hoặc signal mà nghiệp vụ đọc như `LeaveBalanceList.year` → nút "Tính lại").
          // Thiếu dòng này thì 3 đường kia (`:213` init · `watchUrlNavigation` · `clearFilter`)
          // có cập nhật còn đường người dùng TỰ đổi filter thì không → hiển thị/nghiệp vụ lệch
          // đúng ở ca phổ biến nhất (epic hr-list-v2, QA 2026-08-04).
          // KHÔNG đẻ thêm request: input `[filters]` của p-table chỉ có effect gán
          // `this.filters = val` (`primeng-table.mjs:1885`), không gọi `_filter()`.
          this.initialFilters.set(this.buildFiltersFromForm());
        });
    }
  }

  private buildFiltersFromForm(): Record<string, FilterMetadata> {
    const raw = this.rfFilterModel.getRawValue();
    const filters: Record<string, FilterMetadata> = {};
    for (const def of this.filterDefs) {
      const meta = this.toFilterMeta(def, raw[def.name]);
      if (meta) filters[def.name] = meta;
    }
    return filters;
  }

  /**
   * Đọc state từ URL. Có cờ `<prefix>_f` → URL là nguồn sự thật: filter nào vắng mặt
   * trên URL là do người dùng đã xoá, KHÔNG lấy lại giá trị mặc định (trừ field `required`).
   * @param source nguồn query param. Mặc định là snapshot của route; khi xử `NavigationSkipped`
   *   phải truyền `URLSearchParams` dựng từ URL đích vì snapshot lúc đó KHÔNG được cập nhật.
   * @returns true nếu URL có state và đã khôi phục
   */
  private restoreFromUrl(source?: ListUrlParams): boolean {
    const prefix = this.URL_STATE_PREFIX;
    if (!prefix) return false;
    const map = source ?? this.activatedRoute.snapshot.queryParamMap;
    if (!hasListUrlState(prefix, map)) return false;

    const patch: Record<string, any> = {};
    for (const def of this.filterDefs) {
      if (def.urlSync === false) continue;
      const key = filterParamName(prefix, def);
      patch[def.name] = map.has(key)
        ? parseFilterValue(def, map.get(key)!, this.timezoneService)
        : clearedValueOf(def);
    }
    this.rfFilterModel.patchValue(patch, { emitEvent: false });

    // LUÔN gán, không gán có điều kiện: hàm này còn chạy ở luồng đồng bộ lại (watchUrlNavigation),
    // param vắng mặt nghĩa là "về mặc định" chứ không phải "giữ giá trị đang có". Trước đây gán
    // theo `if (page > 1)` nên đang ở trang 3 mà mở link không có `ap_page` thì vẫn ở trang 3,
    // rồi còn ghi ngược `ap_page=3` vào chính cái link vừa mở.
    const size = Number(map.get(stateParamName(prefix, URL_STATE_SIZE)) ?? 0);
    this.perPage.set(size > 0 ? size : this.initialPerPage);

    const page = Number(map.get(stateParamName(prefix, URL_STATE_PAGE)) ?? 1);
    this.page.set(page > 1 ? page : 1);
    this.first.set((this.page() - 1) * this.perPage());

    this.sort.set(map.get(stateParamName(prefix, URL_STATE_SORT)) || this.initialSort);
    return true;
  }

  /**
   * Ghi state ra URL bằng `Location.replaceState`, KHÔNG dùng `router.navigate`.
   *
   * Lý do (đo được 2026-08-03): `router.navigate([], {queryParams})` chạy TRỌN một chu kỳ điều
   * hướng — NavigationStart → RoutesRecognized → GuardsCheck → ActivationEnd ×3 → NavigationEnd
   * → Scroll — chỉ để đổi vài query param. Component không bị tạo lại, nhưng chu kỳ đó (đặc biệt
   * là re-activate route + sự kiện Scroll của router) làm cả vùng `p-table` vẽ lại và đè lên
   * popover filter đang mở, popover chớp mất rồi hiện lại sau khi load xong. `BaseList` cũ không
   * điều hướng bao giờ nên không có lỗi này — đó là điểm khác biệt duy nhất giữa 2 base.
   *
   * `replaceState` chỉ ghi thanh địa chỉ + thay entry history hiện tại: Back/F5/copy link vẫn
   * đúng như cũ, mà không đụng gì tới router. Nó cũng thay luôn vai trò của `replaceUrl: true`
   * (mỗi lần gõ filter không đẻ thêm entry history, nếu không bấm Back phải lùi qua từng lần lọc).
   *
   * Đánh đổi: `ActivatedRoute.snapshot.queryParams` không được cập nhật theo. Không sao —
   * `restoreFromUrl()` chỉ đọc snapshot đúng một lần lúc khởi tạo component.
   *
   * Query dựng từ THANH ĐỊA CHỈ hiện tại (`location.path()`), KHÔNG dùng
   * `createUrlTree({queryParamsHandling:'merge'})`. Vì `replaceState` không báo cho router nên cây
   * URL của router vẫn là của lần điều hướng thật gần nhất — merge lên đó sẽ làm mất param do nơi
   * khác thêm sau đó, và làm sống lại param đã bị xoá. Chỉ đụng đúng các key mình sở hữu, phần
   * còn lại của query giữ nguyên.
   */
  private writeUrlState(): void {
    const prefix = this.URL_STATE_PREFIX;
    if (!prefix || !this.urlSyncReady || this.destroyed) return;

    const [path, currentQuery = ''] = this.location.path().split('?');
    const params = new URLSearchParams(currentQuery);
    const apply = (key: string, value: string | number | null) => {
      if (value === null || value === '') params.delete(key);
      else params.set(key, String(value));
    };

    apply(stateParamName(prefix, URL_STATE_FLAG), 1);
    const raw = this.rfFilterModel.getRawValue();
    for (const def of this.filterDefs) {
      if (def.urlSync === false) continue;
      apply(
        filterParamName(prefix, def),
        serializeFilterValue(def, raw[def.name], this.timezoneService),
      );
    }
    apply(stateParamName(prefix, URL_STATE_PAGE), this.page() > 1 ? this.page() : null);
    apply(
      stateParamName(prefix, URL_STATE_SIZE),
      this.perPage() !== this.initialPerPage ? this.perPage() : null,
    );
    apply(
      stateParamName(prefix, URL_STATE_SORT),
      this.sort() && this.sort() !== this.initialSort ? this.sort() : null,
    );

    // Trả dấu phẩy về nguyên bản: URLSearchParams encode thành %2C, mà mục tiêu của scheme này là
    // URL đọc được bằng mắt. Dấu phẩy là sub-delim hợp lệ trong query theo RFC 3986.
    this.location.replaceState(path, params.toString().replace(/%2C/g, ','));
  }
}
