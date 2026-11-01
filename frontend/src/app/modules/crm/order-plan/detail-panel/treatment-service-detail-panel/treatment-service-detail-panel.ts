import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { HttpContext } from '@angular/common/http';
import { SKIP_GLOBAL_404 } from '@/core/interceptors/response-handler.interceptor';
import { TagSeverity } from 'primeng/tag';
import { Observable } from 'rxjs';

import { OrderServiceUsedModel } from '@/core/models/crm/order-service-used.model';
import { OrderServiceUsedService } from '@/core/services/crm/order-service-used.service';
import { ORDER_PLAN_TYPE_OPTION } from '@/define';
import { SLIDE_OVER_IMPORTS } from '@/share/components/slide-over';
import { BaseDetailPanel } from '@/share/base/base-detail-panel';
import { WhitespaceControlDirective } from '@/share/directive/whitespace-control.directive';
import { DetailShareImports } from '@/share/import/detail-share-imports';

/** Trạng thái thu tiền suy từ số liệu, khớp đúng cách màn danh sách đang hiển thị. */
export type TreatmentPaymentState = 'free' | 'pending' | 'partial' | 'paid';

/**
 * Tình trạng khoá dịch vụ (`isLock`: 1 bình thường, 2 khoá, 3 tạm ngưng).
 *
 * CHỈ map 2 trạng thái bất thường: tag này để cảnh báo, bình thường thì không cần nói. Quan trọng
 * hơn, map cả giá trị 1 rồi lọc bằng `isLock !== 1` ở template là sai — `isLock` chưa có giá trị
 * (undefined) cũng lọt qua điều kiện đó và hiện tag "Bình thường", đúng thứ định giấu.
 */
const LOCK_STATE: Record<number, { labelKey: string; severity: TagSeverity }> = {
  2: { labelKey: 'common.status.locked', severity: 'danger' },
  3: { labelKey: 'crm.treatment_service.detail.lock_paused', severity: 'warn' },
};

/**
 * Chi tiết dịch vụ điều trị dạng SlideOver — dùng chung khung `share/components/slide-over` với panel
 * chi tiết lịch hẹn.
 *
 * ```html
 * <app-treatment-service-detail-panel
 *   [(visible)]="detail.visible"
 *   [serviceUsedId]="detail.id()"
 *   [seed]="detail.seed()" />
 * ```
 *
 * `seed` = dòng đang bấm ở danh sách (đã qua `addLabel`) để vẽ ngay; dữ liệu hiển thị vẫn lấy từ
 * `GET /crm/order-service-used/{id}` vì dòng list thiếu ghi chú, hiệu lực, trả góp và lịch sử buổi
 * điều trị.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-treatment-service-detail-panel',
  standalone: true,
  templateUrl: './treatment-service-detail-panel.html',
  imports: [...DetailShareImports, ...SLIDE_OVER_IMPORTS, WhitespaceControlDirective],
})
export class TreatmentServiceDetailPanel extends BaseDetailPanel<OrderServiceUsedModel> {
  readonly serviceUsedId = input<number | null>(null);
  readonly seed = input<OrderServiceUsedModel | null>(null);

  private readonly serviceUsedService = inject(OrderServiceUsedService);

  // Hook của BaseDetailPanel — máy móc nạp/race/lỗi/đóng nằm ở base.
  protected currentId() {
    return this.serviceUsedId();
  }
  protected currentSeed() {
    return this.seed() as OrderServiceUsedModel | null;
  }
  protected fetchDetail(id: number) {
    // 404 = bản ghi đã bị xoá → panel tự báo; KHÔNG để interceptor đá cả trang sang /auth/notfound
    // (mất danh sách + bộ lọc người dùng đang mở).
    return this.serviceUsedService.get(id, {
      context: new HttpContext().set(SKIP_GLOBAL_404, true),
    }) as unknown as Observable<OrderServiceUsedModel>;
  }
  protected override ensureCache() {
    return this.serviceUsedService.ensureCacheLoaded();
  }
  protected override decorate(detail: OrderServiceUsedModel, seed: OrderServiceUsedModel | null): OrderServiceUsedModel {
    // Giữ nhãn/quan hệ mà dòng list đã có nhưng API chi tiết không trả, rồi addLabel lại trên bản
    // đã gộp để nhãn khớp dữ liệu mới.
    return this.serviceUsedService.addLabel({ ...(seed ?? {}), ...detail }) as OrderServiceUsedModel;
  }

  // ── Trạng thái ────────────────────────────────────────────────────────────
  /** `undefined` = không có gì bất thường để cảnh báo (bình thường hoặc chưa có dữ liệu). */
  readonly lockState = computed<{ labelKey: string; severity: TagSeverity } | undefined>(
    () => LOCK_STATE[Number(this.model().isLock)],
  );

  /**
   * [3] Loại đơn (mới/cũ) qua `labelKey`. `addLabel` dựng sẵn `orderPlanTypeLabel` nhưng đó là chuỗi
   * tiếng Việt hardcode trong `ORDER_PLAN_TYPE_MAP` — bind thẳng là lộ tiếng Việt ở 3 locale còn lại.
   */
  readonly orderPlanTypeLabelKey = computed(
    () =>
      ORDER_PLAN_TYPE_OPTION.find((o) => o.value === this.model().treatmentPlan?.orderPlanType)
        ?.labelKey ?? '',
  );

  // ── Tiền ──────────────────────────────────────────────────────────────────
  /**
   * Làm tròn 2 số lẻ mọi phép tính tiền ở FE. Đây chỉ là số HIỂN THỊ — nguồn sự thật là `decimal`
   * bên BE; làm tròn để phép cộng float không đẻ ra đuôi kiểu `...0000000004` ngay cạnh con số BE.
   */
  private round2 = (value: number) => Math.round(value * 100) / 100;

  /** Tạm tính = đơn giá × số lượng. BE không lưu cột tương ứng (cột `total` đã trừ chiết khấu). */
  readonly subtotal = computed(() =>
    this.round2(Number(this.model().unit_price ?? 0) * Number(this.model().quantity ?? 0)),
  );

  readonly totalDiscount = computed(() => {
    const m = this.model();
    return this.round2(
      Number(m.service_discount ?? 0) +
        Number(m.discount_other ?? 0) +
        Number(m.rank_discount_amount ?? 0) +
        Number(m.discountServiceLock ?? 0),
    );
  });

  /** Tiền VAT của dòng (0 với dịch vụ trước khi có VAT). */
  readonly vatAmount = computed(() => Number(this.model().vat_amount) || 0);

  /** Phải thu = Thành tiền (`totalEnd`, CHƯA VAT) + VAT — tiền khách thực trả đã gồm VAT. */
  readonly payable = computed(() => this.round2(Number(this.model().totalEnd ?? 0) + this.vatAmount()));

  readonly collected = computed(() => Number(this.model().payment ?? 0));
  readonly refunded = computed(() => Number(this.model().refundServiceUsed ?? 0));

  /**
   * Còn phải thu = phải thu (gồm VAT) − đã thu. KHÔNG trừ thêm phần đã hoàn trả: BE tính nợ = phải thu − tiền đã
   * thu ròng, tiền hoàn đã nằm trong số thu ròng — trừ lần nữa là đếm hoàn hai lần, panel hiện nợ thấp hơn BE và
   * lệch cả công thức form thanh toán (D11b). Dòng "Đã hoàn trả" ở template chỉ để tham khảo.
   * KHÔNG kẹp về 0: giá trị âm nghĩa là **thu vượt** (đổi giá sau khi đã thu, gán nhầm phiếu) — đó
   * đúng là tín hiệu kế toán cần thấy, kẹp lại là biến nó thành "đã thu đủ" y như ca bình thường.
   */
  readonly remaining = computed(() => this.round2(this.payable() - this.collected()));

  readonly isOverpaid = computed(() => this.remaining() < 0);

  readonly paymentState = computed<TreatmentPaymentState>(() => {
    // So với số PHẢI THU (gồm VAT): trả đúng Thành tiền mà còn VAT thì vẫn là thu một phần.
    const total = this.payable();
    const paid = this.collected();
    if (total === 0) return 'free';
    if (paid === 0) return 'pending';
    return paid >= total ? 'paid' : 'partial';
  });

}
