import {
  ChangeDetectionStrategy,
  Component,
  computed,
  DestroyRef,
  inject,
  OnInit,
  signal,
  ViewEncapsulation,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { authBusinessProfile } from '@/core/services/auth';
import { PrintService } from '@/prints/print.service';
import { PrintHeaderLayout } from '@/layout/print/customer-info.layout';
import { BasePrint } from '@/share/base/base-print';
import { PrintShareImport } from '@/share/import/print-share-import';
import { SERVICE_DISCOUNT_TYPE_SELECT } from '@/define';
import { PrintTemplateRenderService } from '@/prints/print-template-render.service';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './print-payment.html',
  styleUrls: ['../../../assets/overrides/print-order.scss'],
  encapsulation: ViewEncapsulation.None,
  standalone: true,
  imports: [...PrintShareImport, PrintHeaderLayout],
})
export class PrintPayment extends BasePrint implements OnInit {
  private printService = inject(PrintService);
  private templateRenderer = inject(PrintTemplateRenderService);
  protected readonly SERVICE_DISCOUNT_TYPE_SELECT = SERVICE_DISCOUNT_TYPE_SELECT;
  private destroyRef = inject(DestroyRef);

  /** HTML đã render từ template cấu hình; null = dùng layout mặc định */
  readonly renderedHtml = signal<string | null>(null);

  /** Tiền VAT của dịch vụ trên phiếu (0 với dịch vụ trước khi có VAT). */
  readonly vatAmount = computed(() => Number(this.model().payment?.orderServiceUsed?.vat_amount) || 0);
  /** Phải thu = Thành tiền (`totalEnd`, CHƯA VAT) + VAT. */
  readonly payable = computed(() => (Number(this.model().payment?.orderServiceUsed?.totalEnd) || 0) + this.vatAmount());
  /**
   * Còn lại = phải thu − tổng đã thu. KHÔNG trừ thêm `refundServiceUsed`: BE tính nợ = phải thu − tiền đã thu ròng,
   * tiền hoàn đã nằm trong số thu ròng đó — trừ lần nữa là đếm hoàn hai lần, phiếu in ra nợ thấp hơn sổ sách (D11b).
   */
  readonly outstanding = computed(() => this.payable() - (Number(this.model().totalPayment) || 0));

  ngOnInit() {
    this.activatedRoute.params.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((queryParams: any) => {
      const cacheKey = this.activatedRoute.snapshot.queryParams['cacheKey'];
      const cached = this.tryLoadFromCache(cacheKey);
      if (cached) { this.applyData(cached); return; }
      this.printService.payment(queryParams.id)
        .subscribe((model: any) => this.applyData(model));
    });
  }

  applyData(model: any) {
    this.model.set(model);
    const branchId = this.model().payment?.branch_id;

    this.printData.setModel({
      branch: branchId,
      customer: this.model().payment?.customer,
      layoutType: 'with-accountant',
    });
    this.models.set([this.model()]);

    // Thử load template cấu hình; fall back hardcode nếu không có
    this.templateRenderer.getDefault('payment', branchId)
      .subscribe((tpl) => {
        if (tpl) {
          const html = this.templateRenderer.render(tpl, this.buildTemplateData());
          this.renderedHtml.set(html);
        }
        setTimeout(() => void this.handlePrintAfterInit());
      });
  }

  protected override async handlePrintAfterInit(): Promise<void> {
    const d = this.model()?.payment?.payment_of_date;
    await this.formatDate(d ? new Date(d * 1000) : new Date());
  }

  private buildTemplateData(): Record<string, any> {
    const m = this.model();
    const p = m?.payment;
    const now = p?.payment_of_date ? new Date(p.payment_of_date * 1000) : new Date();
    const profile = authBusinessProfile();
    return {
      company: {
        name: profile?.name ?? '',
        hotline: profile?.hotline ?? '',
        website: profile?.website ?? '',
        logo: profile?.logo_image ?? '',
      },
      branch: {
        name: p?.branch?.name ?? '',
        address: p?.branch?.address ?? '',
        hotline: p?.branch?.hotline ?? '',
        email: p?.branch?.email ?? '',
      },
      customer: {
        fullname: p?.customer?.fullname ?? '',
        phone: p?.customer?.phone ?? '',
        email: p?.customer?.email ?? '',
        address: p?.customer?.address ?? '',
        code: p?.customer?.code ?? '',
      },
      payment: {
        code: p?.code ?? '',
        date: now.toLocaleDateString('vi-VN'),
        note: p?.note ?? '',
        total: m?.totalPayment ?? 0,
        outstanding: this.outstanding(),
        vat: this.vatAmount(),
        total_after_vat: this.payable(),
      },
      services: p?.orderServiceUsed
        ? [
            {
              name: p.orderServiceUsed.service?.name ?? '',
              quantity: p.orderServiceUsed.quantity ?? 0,
              unit_price: p.orderServiceUsed.unit_price ?? 0,
              subtotal: (p.orderServiceUsed.unit_price ?? 0) * (p.orderServiceUsed.quantity ?? 0),
              discount: 0,
              total: p.orderServiceUsed.totalEnd ?? 0,
              vat: this.vatAmount(),
              total_after_vat: this.payable(),
            },
          ]
        : [],
      print: {
        date: now.toLocaleDateString('vi-VN'),
        day: now.getDate(),
        month: now.getMonth() + 1,
        year: now.getFullYear(),
      },
    };
  }
}
