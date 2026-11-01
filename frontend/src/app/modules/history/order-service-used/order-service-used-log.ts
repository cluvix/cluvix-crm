import { ChangeDetectionStrategy, Component, effect, inject, OnInit, ViewChild } from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { Table } from 'primeng/table';
import { Observable, switchMap } from 'rxjs';

import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import { ListShareImports } from '@/share/import/list-share-imports';
import { LogsService } from '@/core/services/history/logs.service';
import { ActivatedRoute } from '@angular/router';
import { BranchHelperService } from '@/indexeddb/config/components/branch';
import { UserHelperService } from '@/indexeddb/config/components/user';
import { ServicesHelperService } from '@/indexeddb/config/components/services';
import { SourceHelperService } from '@/indexeddb/config/components/source';
import { LocaleService } from '@/share/services/locale.service';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import { LOG_ACTION_SELECT } from '@/define';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-order-service-used-log',
  templateUrl: './order-service-used-log.html',
  standalone: true,
  imports: [FillHeightDirective, ...ListShareImports],
})
export class OrderServiceUsedLog extends BaseListV2<any> implements OnInit {
  modelService = inject(LogsService);
  router = inject(ActivatedRoute);
  @ViewChild('dt') dt!: Table;
  protected override URL_STATE_PREFIX = 'lo';
  /** Dòng xoá: không liệt kê snapshot `giá trị → null`, chỉ hiện 'Dữ liệu đã bị xóa'. */
  protected readonly LOG_ACTION_DELETE = LOG_ACTION_SELECT.LOG_ACTION_DELETE;
  private userHelper = inject(UserHelperService);
  private branchHelper = inject(BranchHelperService);
  private servicesHelper = inject(ServicesHelperService);
  private sourceHelper = inject(SourceHelperService);
  private localeService = inject(LocaleService);

  private moneyFields = new Set(['unit_price', 'service_discount', 'discount', 'discount_other', 'totalEnd', 'vat_amount']);

  private fieldLabels: Record<string, string> = {
    service_id: 'Dịch vụ',
    quantity: 'Số lượng',
    unit_price: 'Đơn giá',
    service_discount: 'Giảm dịch vụ',
    service_discount_type: 'Loại giảm dịch vụ',
    discount: 'Giảm coupon',
    discount_other: 'Giảm khác',
    totalEnd: 'Thành tiền',
    vat_amount: 'Tiền VAT',
    expires_at: 'Hiệu lực đến',
    note: 'Ghi chú',
    branch_id: 'Chi nhánh',
    created_day: 'Ngày lên đơn',
    appointment_id: 'Lịch hẹn',
    source_id: 'Nguồn khách',
    listRevenues: 'Tư vấn viên',
    listDoctors: 'Nhân viên tư vấn',
    listDirectSales: 'Direct sale',
  };

  constructor() {
    super();
    effect(() => {
      this.padCols.set(7);
    });
  }

  ngOnInit() {
    this.requiredSearch.set({ paramUrl: this.router.snapshot.url.at(-1)?.path });
    this.initListV2();
  }

  protected filterFields(): FilterFieldDef[] {
    return [{ name: '_keyword', kind: 'text' }]; // urlKey tự suy → 'keyword'
  }

  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService.listLogs({ params: this.searchParams() }).pipe(
      switchMap(async (response: any) => {
        if (response.items) {
          const items = response.items ?? [];
          response.items = await this.modelService.addLabels(items);
          response.items.forEach((item: any) => {
            item.changed_fields = item.changed_fields.map((field: any) => ({
              key: this.fieldLabels[field.key] || field.key,
              value: {
                from: this.formatFieldValue(field.key, field.value?.from),
                to: this.formatFieldValue(field.key, field.value?.to),
              },
            }));
          });
        }
        return response;
      }),
    );
  }

  /**
   * D-12: bản cũ dùng new Date(timestamp*1000).getHours()/getDate() — GIỜ MÁY trình duyệt, lệch
   * với bộ lọc ngày (đã chuyển sang giờ công ty ở §4.4). Đổi sang cùng kỹ thuật với
   * TimezoneService.toTz(): Intl.DateTimeFormat theo tz() công ty, giữ nguyên format hiển thị.
   */
  private formatTimestamp(timestamp: number): string {
    const formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: this.timezoneService.tz(),
      year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
    });
    const parts = formatter.formatToParts(new Date(timestamp * 1000));
    const get = (type: string) => parts.find((p) => p.type === type)?.value ?? '00';
    return `${get('day')}-${get('month')}-${get('year')} ${get('hour')}:${get('minute')}`;
  }

  private formatFieldValue(key: string, value: any): any {
    if (value == null) return value;
    if (key === 'service_id' && typeof value === 'number') {
      return this.servicesHelper.getServiceName(value);
    }
    if ((key === 'listRevenues' || key === 'listDoctors') && Array.isArray(value)) {
      return value.map((id: number) => this.userHelper.getLabel(id)).join(', ');
    }
    if (key === 'branch_id' && typeof value === 'number') {
      return this.branchHelper.getBranchName(value);
    }
    if (key === 'source_id' && typeof value === 'number') {
      return this.sourceHelper.getSourceName(value);
    }
    if ((key === 'expires_at' || key === 'created_day') && typeof value === 'number') {
      return this.formatTimestamp(value);
    }
    if (this.moneyFields.has(key) && value !== null && value !== '-' && !isNaN(value)) {
      const decimalPipe = new DecimalPipe(this.localeService.getLocale());
      return decimalPipe.transform(parseFloat(value), '1.0');
    }
    return value;
  }
}
