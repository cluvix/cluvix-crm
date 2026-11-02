import {ChangeDetectionStrategy, Component,
  computed,
  effect,
  inject,
  OnInit,
  signal,
  ViewChild,
} from '@angular/core';
import { Table } from 'primeng/table';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Observable, finalize, switchMap } from 'rxjs';

import { CustomerTimeline } from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import { CustomerFormV2 } from '@/modules/crm/customer/form/form-customer/customer-form-v2';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { CustomerService } from '@/core/services/crm/customer.service';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import {
  CUSTOMER_TYPE_OPTION,
  GENDER_OPTIONS,
  PERMISSION_SELECT,
} from '@/define';
import {
  displayColsContactList,
  selectedColumnsContactList,
} from '@/share/displaycols/displayColsContact.list';
import { ListShareImports } from '@/share/import/list-share-imports';
import { translateOptions } from '@/share/utils/translate-options.util';
import { InputNumberModule } from 'primeng/inputnumber';
import { CustomerCategoryMultiselectFormControl } from '@/indexeddb/config/components/customer-category';
import { CustomerStageMultiselectFormControl } from '@/indexeddb/config/components/customer-stage';
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-contact-list',
  templateUrl: './contact-list.html',
  styleUrls: ['./contact-list.scss'],
  standalone: true,
  imports: [
    ...ListShareImports,
    FillHeightDirective,
    InputNumberModule,
    CustomerCategoryMultiselectFormControl,
    CustomerStageMultiselectFormControl,
  ],
})
export class ContactList extends BaseListV2<CustomerModel> implements OnInit {
  modelService = inject(CustomerService);
  @ViewChild('dt') dt!: Table;
  displayCols: any = displayColsContactList; // dùng để hiển thị column table
  override selectedColumns = signal<string[]>(selectedColumnsContactList);
  keysToFilter = ['user', 'customerCare', 'telesale'];

  // Cache visibility checks for better performance
  visibleColumns = computed(() => {
    const selected = this.selectedColumns();
    return {
      id: selected.includes('id'),
      avatar: selected.includes('avatar'),
      code: selected.includes('code'),
      old_code: selected.includes('old_code'),
      fullname: selected.includes('fullname'),
      phone: selected.includes('phone'),
      email: selected.includes('email'),
      genderCustomer: selected.includes('genderCustomer'),
      birthday: selected.includes('birthday'),
      age: selected.includes('age'),
      statusName: selected.includes('statusName'),
      affiliate: selected.includes('affiliate'),
      source: selected.includes('source'),
      serviceType: selected.includes('serviceType'),
      payment_user_id: selected.includes('payment_user_id'),
      totalRevenue: selected.includes('totalRevenue'),
      totalPayment: selected.includes('totalPayment'),
      totalDebt: selected.includes('totalDebt'),
      totalDeposit: selected.includes('totalDeposit'),
      refundDeposit: selected.includes('refundDeposit'),
      refundService: selected.includes('refundService'),
      treatment_count: selected.includes('treatment_count'),
      visit_count: selected.includes('visit_count'),
      country: selected.includes('country'),
      province: selected.includes('province'),
      address: selected.includes('address'),
      branch: selected.includes('branch'),
      created_by: selected.includes('created_by'),
      createdDay: selected.includes('createdDay'),
    };
  });
  protected override LS_SELECTED_COLUMNS_KEY = '/app/crm/customer/contact-list';
  protected override URL_STATE_PREFIX = 'cu';
  protected readonly genderOptions = translateOptions(GENDER_OPTIONS);
  protected readonly customerTypeOptions = translateOptions(CUSTOMER_TYPE_OPTION);

  /**
   * Cột lưu trong localStorage từ bản cũ (order_count, predicted_clv, churn_risk_level… — tính năng
   * CLV đã gỡ) không còn trong `displayCols`: lọc bỏ để không lọt vào `?columns=` của export.
   */
  override loadSelectedColumns() {
    super.loadSelectedColumns();
    const valid = new Set<string>(this.displayCols.map((c: { value: string }) => c.value));
    this.selectedColumns.update((cols) => cols.filter((c) => valid.has(c)));
  }

  constructor() {
    super();
    // Sort mặc định — đặt ở constructor (TRƯỚC initListV2() chạy trong ngOnInit) để
    // BaseListV2 chốt làm initialSort; đặt trong fetchList() (như bản V1 cũ) sẽ ghi đè
    // sort người dùng chọn về mặc định sau MỖI lần tải, gãy cả URL state lẫn AC-R4.
    this.sort.set('-created_day');
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 2);
    });
  }

  ngOnInit() {
    this.initListV2();
  }

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {name: '_keyword', kind: 'text'},
      {name: 'created_day', kind: 'dateRange'},
      {name: 'created_at', kind: 'dateRange'},
      {name: 'customerType', kind: 'equals'},
      {name: 'created_by', kind: 'in'},
      {name: 'branch_id', kind: 'in'},
      // Tiến trình = bước cấu hình (customer_stage). Tên không có `_` đầu ⇒ gửi `filter[stage_id][i]=…`
      // (BE story-04); URL `cu_stage_id=…` (quy ước urlKeyOf — không trùng field nào nên khỏi khai urlKey).
      {name: 'stage_id', kind: 'in'},
      {name: 'source_id', kind: 'in'},
      {name: 'gender', kind: 'in'},
      {name: 'category', kind: 'in'},
      {name: 'type_service_id', kind: 'in'},
      {
        name: 'payment_user_id',
        kind: 'in',
        // id=0 là sentinel "chưa có tư vấn viên" → đổi thành điều kiện IS NULL — copy nguyên luật cũ
        toFilter: (value: any) => {
          if (value == null || (Array.isArray(value) && !value.length)) return null;
          if (Array.isArray(value) && value.includes(0)) return {value: '1', matchMode: 'is'};
          return {value, matchMode: Array.isArray(value) ? 'in' : 'equals'};
        },
      },
      {
        name: '_ageFrom',
        kind: 'custom',
        debounce: 500,
        // BE đọc raw c.Query("_ageFrom") (không qua query.ApplyFilter) → matchMode 'custom'
        // giữ nguyên tên param `_ageFrom`, KHÔNG đổi thành filter[_ageFrom][eq] (AC-R2).
        toFilter: (value: any) => (value == null ? null : {value, matchMode: 'custom'}),
      },
      {
        name: '_ageTo',
        kind: 'custom',
        debounce: 500,
        toFilter: (value: any) => (value == null ? null : {value, matchMode: 'custom'}),
      },
    ];
  }

  /**
   * So sánh để hiển thị màu sắc ra ngoài template cho dễ nhìn
   * @param totalDoanhSo
   * @param totalPayment
   */
  displayColorRow(totalDoanhSo: number, totalPayment: number) {
    //Thanh toán 1 phần
    if (totalDoanhSo - totalPayment > 0 && totalPayment != 0) {
      return '#f8f3e1';
    }
    //Chưa thanh toán
    if (totalDoanhSo - totalPayment > 0 && totalPayment == 0) {
      return '#f5e9e9';
    }
  }

  /**
   * lấy danh sách khách hàng
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<CustomerModel>> {
    return this.modelService
      .getCustomer({
        params: this.searchParams(),
      })
      .pipe(
        switchMap(async (response: any) => {
          if (response.items) {
            const items = response.items ?? [];
            try {
              response.items = await this.modelService.addLabels(items);
            } catch (e) {
              console.error('[ContactList] addLabels failed:', e);
              response.items = items;
            }
          }
          return response;
        }),
      );
  }

  /**
   * timeline từng khách hàng
   * @param event
   * @param id
   * @param fullname
   */
  timelineCustomer(event: Event, id: number, fullname: string) {
    this.dialogWrapperService.open(CustomerTimeline, {
      header: fullname,
      data: {
        customerId: id,
      },
      width: '960px',
      height: '80vh',
    });
  }

  /**
   * Cập nhật thông tin khách hàng
   * @param customer
   */
  async updateCustomer(customer: CustomerModel) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdateCustomer)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    if (customer.relatives && typeof customer.relatives === 'string') {
      customer.relatives = JSON.parse(customer.relatives);
    }
    await this.modelService.ensureCacheLoaded();
    this.dialogRef = this.dialogWrapperService.open(CustomerFormV2, {
      width: '960px',
      data: { customer },
      header: this.translate.instant('common.label.update_customer'),
    });

    this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: CustomerModel) => {
      if (result) {
        const dataWithLabels = this.modelService.addLabel(result);
        this.updateRow(dataWithLabels as any);
      }
    });
  }

  onCreatedDaySelect({ start, end }: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({ created_day: [start, end] });
  }

  onQuickSelect({ start, end }: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({ created_at: [start, end] });
  }

  /**
   * export khách hàng csv
   */
  exportCustomerToExcel() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtCustomer)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const ref = this.dialogWrapperService.open(ExportOptionsDialog, {
      header: this.translate.instant('utilities.export_options_dialog.header'),
      width: '520px',
      data: {
        selectedCount: this.selectedColumns().length,
        totalCount: this.displayCols.length,
      },
    });
    ref.onClose.subscribe((mode: ExportMode | null) => {
      if (!mode) return;
      this.doExportCustomer(mode);
    });
  }

  private doExportCustomer(mode: ExportMode) {
    this.isSubmitting.set(true);
    this.loadingService.showLoading('exportCustomer');
    const params = {...this.exportParams()} as any;
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    this.modelService
      .exportCustomer({
        params,
      })
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportCustomer');
        }),
      )
      .subscribe({
        next: (res: any) => {
          this.noti.successMsg(this.translate.instant('common.message.export_success'));
          this.downloadExport(res);
        },
      });
  }

  clearStartTime() {
    this.setFilterValue('created_day', null);
  }
}
