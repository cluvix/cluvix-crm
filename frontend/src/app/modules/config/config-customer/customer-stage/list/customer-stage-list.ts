import {
  ChangeDetectionStrategy,
  Component,
  effect,
  inject,
  OnInit,
  signal,
  ViewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Table } from 'primeng/table';
import { finalize, Observable } from 'rxjs';

import { CustomerStageModel } from '@/core/models/config/customer-stage.model';
import {
  CUSTOMER_STAGE_LIST_PAGE_SIZE,
  CustomerStageService,
} from '@/core/services/config/customer-stage.service';
import {
  CUSTOMER_STAGE_TYPE,
  CUSTOMER_STAGE_TYPE_OPTIONS,
  customerStageColorClass,
  customerStageColorLabelKey,
  PERMISSION_SELECT,
} from '@/define';
import { CustomerStageForm } from '@/modules/config/config-customer/customer-stage/form/customer-stage-form';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import { ListShareImports } from '@/share/import/list-share-imports';
import { ListApiResponse } from '@/share/models/list-api-response';

/**
 * Cấu hình → Khách hàng → Tiến trình khách hàng. Danh mục ít dòng và PHẢI hiện đủ để đổi thứ tự
 * (BE `reorder` đòi đủ mọi id) ⇒ lấy 1 trang `per-page=200`, không phân trang, không lọc.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-config-customer-stage-list',
  templateUrl: './customer-stage-list.html',
  styles: [],
  imports: [FillHeightDirective, ...ListShareImports],
  standalone: true,
})
export class CustomerStageList extends BaseListV2<CustomerStageModel> implements OnInit {
  modelService = inject(CustomerStageService);
  @ViewChild('dt') dt!: Table<CustomerStageModel>;
  protected override URL_STATE_PREFIX = 'cst';
  protected override LS_SELECTED_COLUMNS_KEY = '/config/customer-stage-list';

  displayCols: any = [
    { label: 'ID', value: 'id' },
    { label: 'Tên bước', labelKey: 'config.customer_stage.name', value: 'name' },
    { label: 'Loại', labelKey: 'common.label.type', value: 'type' },
    { label: 'Màu', labelKey: 'common.label.color', value: 'color' },
    { label: 'Thứ tự', labelKey: 'config.customer_stage.order', value: 'order' },
    { label: 'Mặc định', labelKey: 'common.label.default', value: 'is_default' },
    { label: 'Trạng thái', labelKey: 'common.label.status', value: 'status' },
  ];
  override selectedColumns = signal(['name', 'type', 'color', 'order', 'is_default', 'status']);

  override updatePermissionString = PERMISSION_SELECT.permissionManageConfig;
  override createPermissionString = PERMISSION_SELECT.permissionManageConfig;

  protected readonly typeOptions = CUSTOMER_STAGE_TYPE_OPTIONS;
  protected readonly STAGE_TYPE_OPEN = CUSTOMER_STAGE_TYPE.OPEN;
  /** Đang gọi reorder/set-default — chặn bấm dồn (mỗi lần gọi phải thấy thứ tự mới nhất của lần trước). */
  protected readonly busy = signal(false);

  constructor() {
    super();
    effect(() => {
      this.padCols.set((this.selectedColumns()?.length ?? 0) + 3);
    });
  }

  ngOnInit() {
    this.initListV2();
  }

  /** Không lọc: reorder cần thấy đủ mọi bước, lọc bớt ⇒ BE từ chối (`reorder_invalid`). */
  protected filterFields(): FilterFieldDef[] {
    return [];
  }

  protected override fetchList(): Observable<ListApiResponse<CustomerStageModel>> {
    return this.modelService.list({
      params: { page: 1, 'per-page': CUSTOMER_STAGE_LIST_PAGE_SIZE },
    });
  }

  /**
   * Không phân trang ⇒ KHÔNG dùng `loadRecords` của base: nó ghi `rows` vào localStorage
   * (`KEY_ROWS_TABLE`) làm đổi số dòng/trang mặc định của MỌI list khác.
   */
  override loadRecords(_event: any): void {
    this.getList();
  }

  override createRecord(): void {
    if (!this.permissionsService.getPermission(this.createPermissionString)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const dialogRef = this.dialogWrapperService.open(CustomerStageForm, {
      width: '700px',
      header: this.translate.instant('common.action.create'),
    });
    dialogRef.onClose
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((result: CustomerStageModel) => {
        if (result) this.getList();
      });
  }

  override updateRecord(model: CustomerStageModel) {
    if (!this.permissionsService.getPermission(this.updatePermissionString)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const dialogRef = this.dialogWrapperService.open(CustomerStageForm, {
      width: '700px',
      header: this.translate.instant('common.action.update'),
      data: { model },
    });
    dialogRef.onClose
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((result: CustomerStageModel) => {
        if (result) this.updateRow({ ...model, ...result });
      });
  }

  checkUpdatePermission() {
    return !this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig);
  }

  protected typeLabelKey(type: number | null | undefined): string {
    return this.typeOptions.find((o) => o.value === type)?.labelKey ?? '';
  }

  protected colorLabelKey(color: string | null | undefined): string {
    return customerStageColorLabelKey(color);
  }

  protected badgeClass(color: string | null | undefined): string {
    return customerStageColorClass(color).badge;
  }

  protected dotClass(color: string | null | undefined): string {
    return customerStageColorClass(color).dot;
  }

  /** Đổi chỗ bước với bước kề (lên/xuống) rồi gửi đủ mọi id theo thứ tự mới. */
  protected moveStage(model: CustomerStageModel, delta: -1 | 1): void {
    if (this.busy()) return;
    const list = this.models();
    const from = list.findIndex((m) => m.id === model.id);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= list.length) return;

    const next = [...list];
    [next[from], next[to]] = [next[to], next[from]];

    this.busy.set(true);
    this.modelService
      .reorder(next.map((m) => m.id as number))
      .pipe(
        finalize(() => this.busy.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (res) => {
          this.models.set(next.map((m, i) => ({ ...m, sort_order: i + 1 })));
          this.noti.successRes(res);
        },
      });
  }

  /** Đặt bước mặc định cho khách mới — BE đổi cờ ở 2 dòng nên tải lại list. */
  protected setDefault(model: CustomerStageModel): void {
    if (this.busy() || !model.id) return;
    this.busy.set(true);
    this.modelService
      .setDefault(model.id)
      .pipe(
        finalize(() => this.busy.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (res) => {
          this.noti.successRes(res);
          this.getList();
        },
      });
  }
}
