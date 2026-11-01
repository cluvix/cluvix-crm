import { ChangeDetectionStrategy, Component, computed, DestroyRef, inject, OnInit, signal, ViewEncapsulation } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { PrintService } from '@/prints/print.service';
import { BasePrint } from '@/share/base/base-print';
import { PrintShareImport } from '@/share/import/print-share-import';
import { PrintHeaderLayout } from '@/layout/print/customer-info.layout';
import { CustomerModel } from '@/core/models/crm/customer.model';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './print-multi-payment.html',
  styleUrls: ['../../../assets/overrides/print-order.scss'],
  encapsulation: ViewEncapsulation.None,
  standalone: true,
  imports: [...PrintShareImport, PrintHeaderLayout],
})
export class PrintMultiPayment extends BasePrint implements OnInit {
  private printService = inject(PrintService);
  readonly customer = signal<CustomerModel>({});
  private destroyRef = inject(DestroyRef);

  /** Chỉ in cột VAT khi có dòng nào mang VAT — phiếu của dịch vụ cũ không VAT in y như trước. */
  readonly hasVat = computed(() => this.models().some((m) => (Number(m.vat_amount) || 0) !== 0));

  ngOnInit() {
    this.activatedRoute.queryParams.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((queryParams: any) => {
      const cacheKey = queryParams['cacheKey'];
      const cached = this.tryLoadFromCache(cacheKey);
      if (cached) { this.applyData(cached); return; }

      const ids = queryParams.ids;
      const customerId = queryParams.customerId;
      if (!ids) { console.error('Không tìm thấy IDs trong URL. Dùng: ?ids=1,2,3&customerId=X'); return; }
      this.printService.multiPayment({ ids, customerId })
        .subscribe((data: any) => this.applyData(data));
    });
  }

  applyData(data: any) {
    this.models.set(data?.payment ?? []);
    this.customer.set(data?.customer ?? {});
    this.printData.setModel({
      branch: this.customer()?.branch_id ?? undefined,
      customer: this.customer(),
      layoutType: 'with-header-footer',
    });
    setTimeout(() => void this.handlePrintAfterInit());
  }

  calculatorTotalEnd(models: any) {
    let totalEnd = 0;
    for (let i = 0; i < models.length; i++) {
      totalEnd += +models[i].totalEnd;
    }
    return totalEnd;
  }

  calculatorVat(models: any) {
    let totalVat = 0;
    for (let i = 0; i < models.length; i++) {
      totalVat += Number(models[i].vat_amount) || 0;
    }
    return totalVat;
  }

  /** Phải thu của 1 dòng: BE trả `total_after_vat`, thiếu thì tự cộng Thành tiền (chưa VAT) + VAT. */
  payable(model: any): number {
    return Number(model.total_after_vat ?? +model.totalEnd + (Number(model.vat_amount) || 0));
  }

  /** Còn lại của 1 dòng = phải thu − đã thanh toán. */
  residual(model: any) {
    return this.payable(model) - +model.amount;
  }

  calculatorAmount(models: any) {
    let totalAmount = 0;
    for (let i = 0; i < models.length; i++) {
      totalAmount += +models[i].amount;
    }
    return totalAmount;
  }

  residualTotal(models: any) {
    let totalResidual = 0;
    for (let i = 0; i < models.length; i++) {
      totalResidual += this.residual(models[i]);
    }
    return totalResidual;
  }
}
