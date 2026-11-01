import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import { Observable } from 'rxjs';

import { OrderServiceUsedModel } from '@/core/models/crm/order-service-used.model';
import { OrderPlanModel } from '@/core/models/order-plan.model';
import { OrderServiceUsedService } from '@/core/services/crm/order-service-used.service';
import { OrderPlanService } from '@/core/services/order-plan.service';
import { ORDER_PLAN_TYPE_OPTION } from '@/define';
import { UnitHelperService } from '@/indexeddb/config/components/unit';
import { SLIDE_OVER_IMPORTS } from '@/share/components/slide-over';
import { BaseDetailPanel } from '@/share/base/base-detail-panel';
import { WhitespaceControlDirective } from '@/share/directive/whitespace-control.directive';
import { DetailShareImports } from '@/share/import/detail-share-imports';
import { vatRateLabel } from '@/share/utils/vat-label.util';

/** 1 dịch vụ con trong đơn, sau khi qua `OrderServiceUsedService.addLabel`. */
export type OrderPlanServiceRow = OrderServiceUsedModel;

/**
 * Response của `GET /crm/order-plan/{id}/detail` sau khi qua `addLabel`.
 *
 * Khai tường minh thay vì để `any`: template panel chỉ được `ngc` kiểm tra khi model có kiểu — gõ sai
 * một tên field trên `any` thì build vẫn xanh và ô đó rỗng câm lặng (đúng lớp lỗi mà `ngc` đã bắt 23
 * và 29 chỗ ở hai panel trước).
 */
export type OrderPlanDetailView = OrderPlanModel & {
  orderServiceUseds?: OrderPlanServiceRow[];
};

/**
 * Chi tiết ĐƠN HÀNG dạng SlideOver.
 *
 * Khác panel dịch vụ điều trị (`treatment-service-detail-panel`): ở đó một bản ghi = MỘT dịch vụ lẻ,
 * còn đơn hàng là bản ghi CHA gồm nhiều dịch vụ con — nên panel này lấy dữ liệu từ endpoint riêng
 * `GET /crm/order-plan/{id}/detail` và trọng tâm là danh sách dịch vụ + tổng tiền toàn đơn.
 *
 * ```html
 * <app-order-plan-detail-panel [(visible)]="detail.visible" [orderPlanId]="detail.id()" />
 * ```
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-order-plan-detail-panel',
  standalone: true,
  templateUrl: './order-plan-detail-panel.html',
  imports: [
    ...DetailShareImports,
    ...SLIDE_OVER_IMPORTS,
    WhitespaceControlDirective,
  ],
})
export class OrderPlanDetailPanel extends BaseDetailPanel<OrderPlanDetailView> {
  readonly orderPlanId = input<number | null>(null);
  /**
   * Seed MỘT PHẦN cho header: dòng ở tab Đơn hàng là dòng *dịch vụ*, nhưng có kèm quan hệ
   * `treatmentPlan` (mã đơn, loại đơn, ngày đặt) — đủ để hiện ngay thay vì spinner trắng.
   * Số tiền cấp đơn KHÔNG có trong seed nên vẫn chờ API, không hiện số sai.
   */
  readonly seed = input<OrderPlanModel | null>(null);

  /** Người dùng bấm xem chi tiết 1 dịch vụ con — trang gọi tự quyết mở panel dịch vụ. */
  readonly serviceSelected = output<OrderPlanServiceRow>();

  private readonly orderPlanService = inject(OrderPlanService);
  private readonly serviceUsedService = inject(OrderServiceUsedService);
  private readonly unitHelper = inject(UnitHelperService);

  // Hook của BaseDetailPanel — máy móc nạp/race/lỗi/đóng nằm ở base.
  protected currentId() {
    return this.orderPlanId();
  }
  protected currentSeed() {
    return this.seed() as OrderPlanDetailView | null;
  }
  protected fetchDetail(id: number) {
    return this.orderPlanService.getDetail(id) as unknown as Observable<OrderPlanDetailView>;
  }
  protected override ensureCache() {
    return Promise.all([
      this.orderPlanService.ensureCacheLoaded(),
      this.serviceUsedService.ensureCacheLoaded(),
      this.unitHelper.ensureUnitsLoaded(),
    ]);
  }
  protected override decorate(detail: OrderPlanDetailView): OrderPlanDetailView {
    // KHÔNG gộp seed: seed chỉ là dòng DỊCH VỤ kèm quan hệ đơn, gộp vào model cấp đơn sẽ trộn hai
    // thực thể khác nhau. Seed chỉ dùng để vẽ header trong lúc chờ (base đã lo).
    const labeled = this.orderPlanService.addLabel(detail) as OrderPlanDetailView;
    // Mỗi dịch vụ con chạy addLabel của chính module dịch vụ để có tên dịch vụ, đơn vị, trạng
    // thái điều trị… — không tự dựng lại nhãn ở đây cho khỏi lệch với màn dịch vụ.
    labeled.orderServiceUseds = (labeled.orderServiceUseds ?? []).map((svc) =>
      this.serviceUsedService.addLabel(svc) as OrderPlanServiceRow,
    );
    return labeled;
  }

  readonly services = computed<OrderPlanServiceRow[]>(() => this.model().orderServiceUseds ?? []);

  readonly orderPlanTypeLabelKey = computed(
    () =>
      ORDER_PLAN_TYPE_OPTION.find((o) => o.value === this.model().orderPlanType)?.labelKey ?? '',
  );

  private round2 = (value: number) => Math.round(value * 100) / 100;

  /** Tổng giá trị đơn CHƯA VAT — BE đã cộng từ `totalEnd` của các dịch vụ con. */
  readonly orderTotal = computed(() => Number(this.model().total ?? 0));
  /** Σ VAT các dịch vụ con (`vat_total` do BE cộng); đơn trước khi có VAT = 0. */
  readonly vatTotal = computed(() => Number(this.model().vat_total) || 0);
  /** Có VAT ⇒ hiện thêm dòng VAT + Tổng sau VAT; đơn cũ không VAT giữ nguyên giao diện. */
  readonly hasVat = computed(() => this.vatTotal() > 0 || !!this.model().vat_rate);
  readonly vatRateText = computed(() =>
    vatRateLabel(this.model().vat_rate, (key) => this.translate.instant(key)),
  );
  /** Số phải thu của đơn = Σ(totalEnd + vat_amount); BE trả `total_after_vat`, thiếu thì tự cộng. */
  readonly totalAfterVat = computed(() =>
    this.round2(Number(this.model().total_after_vat ?? this.orderTotal() + this.vatTotal())),
  );
  readonly collected = computed(() => Number(this.model().payment ?? 0));
  readonly refunded = computed(() => Number(this.model().refundServiceTotal ?? 0));

  /**
   * Còn phải thu = phải thu (đã gồm VAT) − tổng đã thu. KHÔNG trừ thêm `refundServiceTotal`: BE tính nợ đơn =
   * phải thu − tiền đã thu ròng, tiền hoàn đã nằm trong con số thu ròng đó — trừ lần nữa là đếm hoàn hai lần
   * và hiện số nợ thấp hơn BE (D11b). Dòng "Đã hoàn trả" ở template chỉ để tham khảo.
   * KHÔNG kẹp về 0: âm = thu vượt, đó là tín hiệu kế toán cần thấy chứ không phải "đã thu đủ".
   */
  readonly remaining = computed(() => this.round2(this.totalAfterVat() - this.collected()));
  readonly isOverpaid = computed(() => this.remaining() < 0);
}
