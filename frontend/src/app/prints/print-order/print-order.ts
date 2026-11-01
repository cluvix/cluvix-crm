import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, ViewEncapsulation } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { PrintService } from '@/prints/print.service';
import { PrintHeaderLayout } from '@/layout/print/customer-info.layout';
import { BasePrint } from '@/share/base/base-print';
import { PrintShareImport } from '@/share/import/print-share-import';
import { SERVICE_DISCOUNT_TYPE_SELECT } from '@/define';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './print-order.html',
  styleUrls: ['../../../assets/overrides/print-order.scss'],
  encapsulation: ViewEncapsulation.None,
  standalone: true,
  imports: [...PrintShareImport, PrintHeaderLayout],
})
export class PrintOrder extends BasePrint implements OnInit {
  private printService = inject(PrintService);
  totalPrice: number = 0;
  totalDiscount: number = 0;
  totalSerices: number = 0;
  totalPayment: number = 0;
  totalConLai: number = 0;
  protected readonly SERVICE_DISCOUNT_TYPE_SELECT = SERVICE_DISCOUNT_TYPE_SELECT;
  private destroyRef = inject(DestroyRef);

  ngOnInit() {
    this.activatedRoute.params.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((queryParams: any) => {
      // Thử đọc từ sessionStorage trước (parent đã fetch sẵn)
      const cacheKey = this.activatedRoute.snapshot.queryParams['cacheKey'];
      const cached = this.tryLoadFromCache(cacheKey);
      if (cached) {
        this.applyData(cached);
        return;
      }
      this.printService.orderPlan(queryParams.id)
        .subscribe((model: any) => this.applyData(model));
    });
  }

  applyData(model: any) {
    this.model.set(model);
    const orderPlan = model?.orderPlan;
    this.printData.setModel({
      branch: orderPlan?.branch_id,
      customer: orderPlan?.customer,
    });
    this.totalPrice = 0;
    this.totalDiscount = 0;
    this.totalSerices = 0;
    this.totalPayment = 0;
    this.totalConLai = 0;
    (orderPlan?.orderServiceUseds ?? []).forEach((item: any) => {
      this.totalPrice += +item.unit_price * +item.quantity;
      this.totalDiscount += +item.discount_other;
      if (item.service_discount_type === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT) {
        this.totalDiscount += item.quantity * item.unit_price * (item.service_discount / 100);
      } else {
        this.totalDiscount += +item.service_discount;
      }
      this.totalSerices += +item.totalEnd;
    });
    (this.model()?.historyPayment ?? []).forEach((item: any) => {
      this.totalPayment += +item.amount;
    });
    this.totalConLai = this.totalSerices - this.totalPayment;
    setTimeout(() => void this.handlePrintAfterInit());
  }

  protected override async handlePrintAfterInit(): Promise<void> {
    const d = this.model()?.orderPlan?.created_time;
    await this.formatDate(d ? new Date(d * 1000) : new Date());
  }
}
