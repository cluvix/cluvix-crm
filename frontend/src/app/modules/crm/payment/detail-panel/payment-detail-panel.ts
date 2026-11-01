import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { TranslateService } from '@ngx-translate/core';

import { PaymentModel } from '@/core/models/crm/payment.model';
import { PaymentService } from '@/core/services/crm/payment.service';
import { PAYMENT_FOR, getPaymentForKey } from '@/define';
import { ServicesHelperService } from '@/indexeddb/config/components/services';
import { BaseDetailPanel } from '@/share/base/base-detail-panel';
import { SLIDE_OVER_IMPORTS } from '@/share/components/slide-over';
import { WhitespaceControlDirective } from '@/share/directive/whitespace-control.directive';
import { DetailShareImports } from '@/share/import/detail-share-imports';

/** Một dòng trong bảng phân bổ (thực chất là một phiếu thu khác của cùng đơn hàng). */
export type PaymentAllocationRow = {
  id: number;
  code?: string;
  serviceName: string;
  amount: number;
  paymentFor: number;
  paymentForKey: string;
  timestamp?: number | null;
  /** Dòng ứng với chính phiếu đang mở — tô nổi để người dùng định vị được mình đang ở đâu. */
  isCurrent: boolean;
};

/**
 * Loại phiếu được tính vào "đã thu" của ĐƠN: thanh toán + chuyển cọc.
 * Phải TRÙNG với công thức của `GetOrderPlanDetail` (`payment_for IN (4,5)`) — hai màn cùng nói về
 * số tiền đã thu của một đơn mà tính khác nhau là lỗi nặng nhất ở màn tiền.
 */
const COLLECTED_PAYMENT_FOR: number[] = [PAYMENT_FOR.ORDER, PAYMENT_FOR.TRANSFER_DEPOSIT];

/** Loại phiếu hoàn tiền — DB lưu `amount` ÂM (negate ở `beforeSave` bên BE). */
const REFUND_PAYMENT_FOR: number[] = [
  PAYMENT_FOR.SERVICE_REFUND,
  PAYMENT_FOR.DEPOSIT_REFUND,
];

/**
 * Chi tiết PHIẾU THU dạng SlideOver.
 *
 * ⚠ Về bảng phân bổ: hệ thống KHÔNG có bảng phân bổ riêng — thu tiền cho đơn sẽ tạo NHIỀU dòng
 * `payment`, mỗi dòng gắn một dịch vụ và có mã riêng (xem docblock `GetPaymentDetail` bên BE). Nên
 * "phân bổ" hiển thị ở đây là **toàn bộ phiếu thu của cùng đơn hàng**, dòng đang xem được đánh dấu.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-payment-detail-panel',
  standalone: true,
  templateUrl: './payment-detail-panel.html',
  imports: [...DetailShareImports, ...SLIDE_OVER_IMPORTS, WhitespaceControlDirective],
})
export class PaymentDetailPanel extends BaseDetailPanel<PaymentModel> {
  readonly paymentId = input<number | null>(null);
  readonly seed = input<PaymentModel | null>(null);

  private readonly paymentService = inject(PaymentService);
  private readonly servicesHelper = inject(ServicesHelperService);
  private readonly translateSvc = inject(TranslateService);

  // Hook của BaseDetailPanel — máy móc nạp/race/lỗi/đóng nằm ở base.
  protected currentId() {
    return this.paymentId();
  }
  protected currentSeed() {
    return this.seed();
  }
  protected fetchDetail(id: number) {
    return this.paymentService.getDetail(id);
  }
  protected override ensureCache() {
    return this.paymentService.ensureCacheLoaded();
  }
  protected override decorate(detail: PaymentModel, seed: PaymentModel | null): PaymentModel {
    // Giữ nhãn/quan hệ mà dòng list đã có nhưng API chi tiết không trả, rồi addLabel lại trên bản
    // đã gộp để nhãn khớp dữ liệu mới.
    return this.paymentService.addLabel({ ...(seed ?? {}), ...detail }) as PaymentModel;
  }

  readonly paymentForKey = computed(() => getPaymentForKey(Number(this.model().payment_for ?? 0)));

  /** Tên dịch vụ của chính phiếu này — tra từ cache theo `service_id` của dịch vụ đã gắn. */
  readonly serviceName = computed(
    () => this.servicesHelper.getServiceInfo(this.model().serviceUsed?.service_id as number)?.name ?? '',
  );

  private serviceNameOf = (serviceId: unknown) =>
    this.servicesHelper.getServiceInfo(Number(serviceId))?.name ?? '';

  readonly allocations = computed<PaymentAllocationRow[]>(() => {
    const currentId = Number(this.model().id ?? 0);
    return (this.model().allocations ?? []).map((row) => ({
      id: Number(row.id),
      code: row.code,
      // BE trả kèm `service_id` cho từng dòng (JOIN order_service_used) nên mọi dòng đều có tên
      // dịch vụ, không riêng dòng đang xem.
      serviceName: this.serviceNameOf((row as { service_id?: number }).service_id),
      amount: Number(row.amount ?? 0),
      paymentFor: Number(row.payment_for ?? 0),
      paymentForKey: getPaymentForKey(Number(row.payment_for ?? 0)),
      timestamp: row.payment_time,
      isCurrent: Number(row.id) === currentId,
    }));
  });

  private round2 = (value: number) => Math.round(value * 100) / 100;

  /**
   * "Đã thu trên đơn" — CHỈ cộng phiếu loại thanh toán + chuyển cọc, đúng công thức
   * `GetOrderPlanDetail`. KHÔNG cộng đặt cọc / hoàn tiền / thu-chi phòng khám, dù các dòng đó vẫn
   * được liệt kê bên dưới để nhìn được toàn cảnh dòng tiền của đơn.
   */
  readonly collectedTotal = computed(() =>
    this.round2(
      this.allocations()
        .filter((row) => COLLECTED_PAYMENT_FOR.includes(row.paymentFor))
        .reduce((sum, row) => sum + row.amount, 0),
    ),
  );

  /** Tổng đã hoàn — DB lưu số âm nên lấy trị tuyệt đối để hiển thị. */
  readonly refundedTotal = computed(() =>
    this.round2(
      Math.abs(
        this.allocations()
          .filter((row) => REFUND_PAYMENT_FOR.includes(row.paymentFor))
          .reduce((sum, row) => sum + row.amount, 0),
      ),
    ),
  );

  /** Phiếu thu/chi phòng khám không gắn đơn/dịch vụ/lịch hẹn nào — khối "Thu cho" khi đó chỉ toàn
   *  dấu gạch, ẩn hẳn đi. */
  readonly hasTarget = computed(() => {
    const m = this.model();
    return !!(m.orderPlan?.id || m.serviceUsed?.id || m.appointment?.id);
  });

  /** Nội dung nút "Sao chép" — computed, không gọi method trong binding. */
  readonly copyText = computed(() => {
    const m = this.model();
    const t = this.translateSvc;
    return [
      `${t.instant('common.label.code')}: ${m.code ?? ''}`,
      `${t.instant('common.label.amount')}: ${m.amountFormatted ?? m.amount ?? ''}`,
      `${t.instant('common.label.payment_method')}: ${m.paymentMethodLabel ?? ''}`,
      `${t.instant('common.label.collection_time')}: ${m.payment_timeFormatted ?? ''}`,
      `${t.instant('common.label.customer')}: ${m.customerDisplayName ?? ''}`,
    ].join('\n');
  });

}
