import {ChangeDetectionStrategy, Component, effect, inject, signal, untracked} from '@angular/core';
import {takeUntilDestroyed} from '@angular/core/rxjs-interop';
import {applyEach, disabled, FormField, minError, required, schema, SchemaPathTree, validate} from '@angular/forms/signals';
import {catchError, finalize, of} from 'rxjs';

import {ServiceModel} from '@/core/models/config/clinic/service.model';
import {CustomerModel} from '@/core/models/crm/customer.model';
import {OrderServiceUsedModel, OrderServiceUsedRow} from '@/core/models/crm/order-service-used.model';
import {OrderPlanModel} from '@/core/models/order-plan.model';
import {branchActiveId, isAdmin, isDevelop} from '@/core/services/auth';
import {OrderPlanService} from '@/core/services/order-plan.service';
import {ShowOtherBranch} from '@/modules/utilities/popup-show-other-branch/show-other-branch';
import {BaseSignalForm} from '@/share/base/base-signal-form';
import {
  CONFIG_OPTION_CRM_SELECT,
  PERMISSION_SELECT,
  SERVICE_DISCOUNT_TYPE_SELECT,
} from '@/define';
import {FormShareImports} from '@/share/import/form-share-imports';
import {UserCacheService, UserMultiSelectFormControl} from '@/indexeddb/config/components/user';
import {UnitHelperService} from '@/indexeddb/config/components/unit';
import {UserOptionHelperService} from '@/indexeddb/config/components/user-option';
import {ServicesHelperService} from '@/indexeddb/config/components/services';
import {CustomerRankServicesHelperService} from '@/indexeddb/config/components/customer-rank-services';
import {CustomerRanksHelperService} from '@/indexeddb/config/components/customer-ranks';
import {ChangeServiceDialog} from '@/modules/crm/order-plan/change-service-dialog/change-service-dialog';
import {isRowFieldLocked, LockableRowField} from '@/modules/crm/order-plan/form/order-plan-row-lock';

/**
 * Bộ đếm khoá dòng (`_uid`). Module-level, tăng dần trong 1 phiên tab — chỉ cần duy nhất trong phạm vi
 * 1 instance form đang mở nên không cần reset. KHÔNG dùng `id` của dòng làm khoá: dòng mới `id = null`,
 * nhiều dòng mới cùng lúc sẽ trùng khoá.
 */
let rowUidCounter = 0;

/**
 * 1 dòng dịch vụ rỗng — tương đương `fb.group({...})` trong `addServiceUsed()` bản Reactive Forms.
 *
 * MỌI field seed giá trị CỤ THỂ, KHÔNG `undefined` (gotcha #1 `skill/frontend/signal-forms.md`: Signal
 * Forms coi field `undefined` là "không tồn tại", `rootField.x` trả `undefined` VĨNH VIỄN). Field số/
 * reference → `null`; mảng → `[]`. `note`/`unitName` bind vào chỗ hiển thị/`<textarea>` chứ không phải
 * `<input pInputText>` nên `null` an toàn — riêng field text bind `pInputText` (chỉ có `coupon` ở cấp
 * form, xem `emptyOrderPlanModel()`) mới bắt buộc `''` (gotcha #9).
 */
function emptyServiceRow(seed?: Partial<OrderServiceUsedRow>): OrderServiceUsedRow {
  return {
    id: null,
    customer_id: null,
    order_plan_id: null,
    service_discount: 0,
    total: 0,
    totalEnd: 0,
    payment: 0,
    note: null,
    service_id: null,
    service_discount_type: SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT,
    unit_price: null,
    quantity: null,
    discount_other: null,
    min_price: 0,
    max_price: 0,
    updated_at: null,
    discountServiceLock: 0,
    unitName: null,
    rank_discount_percent: 0,
    _uid: ++rowUidCounter,
    unitPriceLocked: false,
    serviceLocked: false,
    ...seed,
  };
}

/**
 * Seed đủ key cấp form — Signal Forms cần KEY tồn tại thật trên object model để dựng sub-field, kể cả
 * khi BE `omitempty` bỏ key đó khỏi response. `coupon` bind `<input pInputText type="text">` nên PHẢI
 * seed `''`, KHÔNG `null` (gotcha #9: input text có giá trị `null` bị Signal Forms coi là field SỐ,
 * gõ chữ lỗi `parse`, gõ số mất số 0 đầu). `note` là `<textarea>` → `null` bình thường.
 */
function emptyOrderPlanModel(): OrderPlanModel {
  return {
    customer_id: null as any,
    appointment_id: null as any,
    branch_id: null as any,
    listRevenues: [],
    discount_other: 0,
    total: 0,
    created_time: null,
    note: null as any,
    source_id: null as any,
    updated_at: null as any,
    services_used: [],
  };
}

/**
 * Đăng ký 1 `effect()` CHỈ phản ứng khi giá trị THAY ĐỔI so với lần đọc trước — bỏ qua lần chạy đầu
 * tiên (Angular `effect()` luôn chạy ngay lúc tạo, khác `valueChanges.subscribe()` chỉ bắn khi có thay
 * đổi THẬT SỰ sau khi subscribe). Copy nguyên từ `service-form.ts`/`service-combo-form.ts` — quyết định
 * "copy riêng, đơn giản" của epic `config-signal-forms`, KHÔNG import chung.
 */
function effectOnChange<V>(read: () => V, action: (value: V) => void): void {
  let initialized = false;
  let prev: V;
  effect(() => {
    const value = read();
    if (!initialized) {
      initialized = true;
      prev = value;
      return;
    }
    if (value === prev) return;
    prev = value;
    action(value);
  });
}

/**
 * Schema 1 dòng dịch vụ — đối chiếu 1-1 validator bản Reactive Forms trong `addServiceUsed()`:
 * `customer_id: [..., Validators.required]`, `quantity: [..., [Validators.required, Validators.min(1)]]`.
 *
 * 2 lệnh `disabled()` thay cho `control.disable()/enable()` imperative (Signal Forms KHÔNG có API đó) —
 * đọc cờ khoá FE-only ghi sẵn trên chính dòng đó (xem `_computeUnitPriceLocked()` / `rowLocked`).
 * `extractValue()` VẪN lấy giá trị field `disabled` (giống `getRawValue()` cũ) nên payload không đổi.
 */
const serviceRowSchema = schema<OrderServiceUsedRow>((item) => {
  required(item.customer_id!);
  required(item.quantity!);
  // KHÔNG dùng `min(item.quantity!, 1)`: `min()` đăng ký metadata MIN_NUMBER, Signal Forms đẩy thẳng
  // xuống input `min` của `p-inputnumber` (`BaseInput.min`, xem `signals.mjs` `setInputOnDirectives`)
  // → ô Số lượng bị CLAMP lên 1 khi user gõ 0, khác bản Reactive Forms (gõ 0 được, hiện lỗi đỏ).
  // `validate()` cho cùng error kind `min` mà không sinh metadata → giữ nguyên UX cũ.
  validate(item.quantity!, ({value}) => {
    const v = value();
    return v != null && v < 1 ? minError(1) : null;
  });
  // [CRITICAL] Khoá ô PHẢI khai ở đây, KHÔNG phải bằng `[readonly]`/`[disabled]` trên template:
  // directive `FormField` đẩy ngược trạng thái field xuống chính input `readonly`/`disabled` của
  // component (`@angular/forms/fesm2022/signals.mjs` → `FIELD_STATE_KEY_TO_CONTROL_BINDING` +
  // `setInputOnDirectives`), ghi đè giá trị template gán ⇒ ô vẫn gõ được mà tsc/ngc vẫn xanh.
  // Smoke story-08 (2026-09-21) bắt đúng lỗi này ở Số lượng / Chiết khấu DV / Giảm khác.
  const lockedWhen = (field: LockableRowField) => ({valueOf}: {valueOf: (p: any) => any}) =>
    isRowFieldLocked(field, {
      unitPriceLocked: !!valueOf(item.unitPriceLocked!),
      serviceLocked: !!valueOf(item.serviceLocked!),
    });
  disabled(item.unit_price!, {when: lockedWhen('unit_price')});
  disabled(item.service_id!, {when: lockedWhen('service_id')});
});

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-order-plan-form',
  templateUrl: 'order-plan-form.html',
  standalone: true,
  imports: [...FormShareImports, FormField, UserMultiSelectFormControl],
  styleUrls: ['./order-plan-form.scss'],
})
export class OrderPlanForm extends BaseSignalForm<OrderPlanModel> {
  modelService = inject(OrderPlanService);
  public maxDate: Date = this.timezoneService.todayEnd();
  public minDate!: Date;
  optionsTimeBefore: boolean = false;
  optionsAllowPreviousDay: boolean = false;
  selectedServiceUsed: ServiceModel | null = null;
  /** `_uid` dòng → đơn giá dòng đó có sửa được không (quyết định nội dung tooltip ô đơn giá). */
  readonly displayPTooltipUnitPrice = signal<Map<number, boolean>>(new Map());
  /** `_uid` dòng → tên dịch vụ CŨ, hiện ghi chú "Đã đổi từ..." dưới dịch vụ mới sau khi đổi thành công
   * trong phiên làm việc này (order-service-change story-07) — chỉ tồn tại tạm, không lưu DB. */
  readonly changedFromServiceName = signal<Map<number, string>>(new Map());
  latestAppointment!: any;
  readonly customer = signal<CustomerModel>({});
  readonly totalRankDiscount = signal<number>(0);

  /**
   * SNAPSHOT BẤT BIẾN lúc mở dialog — `BaseSignalForm.model()` là nguồn SỐNG 2 chiều (mọi phím user gõ
   * ghi thẳng vào đó), KHÁC hẳn `BaseForm.model()` vốn chỉ set 1 lần lúc `initialForm()`. Mọi logic cần
   * "giá trị đã lưu ở DB" (rank_discount_percent gốc, discountServiceLock, tiền đã thanh toán, ngày tạo
   * đơn để tính min/maxDate) PHẢI đọc từ đây, KHÔNG đọc lại `model()`.
   * Xem `skill/bmad/hr-signal-forms/dev-notes-common.md` mục 9.
   */
  private originalServiceUseds: OrderServiceUsedModel[] = [];
  private originalPayment = 0;

  serviceInfoMap = new Map<number, ServiceModel>();
  protected readonly SERVICE_DISCOUNT_TYPE_SELECT = SERVICE_DISCOUNT_TYPE_SELECT;

  private userOptionHelper = inject(UserOptionHelperService);
  private unitHelper = inject(UnitHelperService);
  private servicesHelper = inject(ServicesHelperService);
  private customerRankServicesHelper = inject(CustomerRankServicesHelperService);
  private customerRanksHelper = inject(CustomerRanksHelperService);
  private userCacheForOrderScope = inject(UserCacheService);

  constructor() {
    super();
    const customerDialog = this.dialogConfig?.data?.customer;
    this.customer.set(customerDialog);
    // Caller (màn lịch hẹn) có thể truyền model appointment qua dialog data — chỉ dùng để điền sẵn
    // tư vấn viên/nguồn theo lịch đó. Đơn KHÔNG gắn với lịch hẹn nữa (`appointment_id` tạo mới = null).
    const appointmentDialog = this.dialogConfig?.data?.appointment;
    if (appointmentDialog) {
      this.latestAppointment = appointmentDialog;
    }
    this.optionsTimeBefore = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_TIME_BEFORE,
    )?.options;
    this.optionsAllowPreviousDay = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_ALLOW_PREVIOUS_DAY,
    )?.options;

    this.trackScopeFields('branch_id');
    this._registerRecalcEffects();
  }

  // ============ INIT ============

  /**
   * Override HOÀN TOÀN (không gọi `super.ngOnInit()`) — form tự quyết create/edit rồi `model.set()` một
   * lần qua `_applyModel()`, đúng pattern `ServiceForm`/`ServiceComboForm`.
   *
   * [CRITICAL] Điều kiện create/edit dùng `this.isEditMode()` chứ KHÔNG dùng `if (this.model())` như bản
   * Reactive Forms: `BaseForm` để `model` = `undefined` khi create, còn `BaseSignalForm` khởi tạo
   * `model = {}` (LUÔN truthy) — giữ nguyên điều kiện cũ sẽ khiến create chạy nhầm nhánh edit
   * (`new Date(undefined)` → Invalid Date cho min/maxDate).
   */
  override ngOnInit(): void {
    this.unitHelper.ensureUnitsLoaded();

    const dialogModel: OrderPlanModel | null = this.isEditMode() ? this.model() : null;

    if (dialogModel) {
      this.originalServiceUseds = dialogModel.orderServiceUseds ?? [];
      this.originalPayment = dialogModel.payment ?? 0;
      if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePlanOrder)) {
        if (!this.optionsTimeBefore) {
          const createdDate = new Date(dialogModel.created_time);
          this.minDate = new Date(createdDate);
          this.minDate.setHours(0, 0, 0, 0);
          this.maxDate = new Date(createdDate);
          this.maxDate.setHours(23, 59, 59, 999);
        }
      }
      this._applyModel(dialogModel);
    } else {
      if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePlanOrder)) {
        if (!this.optionsTimeBefore) {
          this.minDate = this.optionsAllowPreviousDay
            ? this.timezoneService.yesterdayStart()
            : this.timezoneService.todayStart();
        }
      }
      const obj: OrderPlanModel = {
        ...this.modelService.getInitialObject(),
        ...(this.initialValue ?? {}),
        customer_id: this.customer()?.id || null,
        branch_id: branchActiveId() || null,
        // Đơn không gắn lịch hẹn. Đơn cũ đã gắn (edit) giữ nguyên `appointment_id` từ BE.
        appointment_id: null as any,
        listRevenues: this.latestAppointment?.revenue_id
          ? [this.latestAppointment.revenue_id]
          : this.customer()?.payment_user_id ? [this.customer()!.payment_user_id!] : [],
        source_id: this.latestAppointment?.source_id ?? this.customer()?.source_id,
      } as OrderPlanModel;
      this._applyModel(obj);
    }
  }

  /**
   * Chuẩn hoá model rồi `model.set()` ĐÚNG 1 LẦN — hydrate mảng dòng dịch vụ từ `orderServiceUseds`
   * (bản BE) sang `services_used` (bản form), seed đủ key cho Signal Forms.
   */
  private _applyModel(model: OrderPlanModel): void {
    const source = model.orderServiceUseds;
    const rows: OrderServiceUsedRow[] =
      typeof source !== 'object' || !source || source.length === 0
        ? [this._buildRow(null, model)]
        : source.map((item) =>
            this._buildRow(
              {...item, serviceInfo: item.serviceInfo ?? this.servicesHelper.getServiceInfo(item.service_id!)} as OrderServiceUsedModel,
              model,
            ),
          );

    this.model.set({
      ...emptyOrderPlanModel(),
      ...model,
      total: 0,
      services_used: rows,
    });

    // Bản gốc gọi `void this.handleService()` ngay sau khi nạp dòng (subscription không bắn lúc init).
    // `effectOnChange` cũng BỎ QUA lần chạy đầu → phải kích hoạt tay đúng 1 lần ở đây, nếu không mở
    // "Sửa" sẽ thấy tổng tiền = 0.
    void this._recalcMoney();
  }

  /** Dựng 1 dòng form từ 1 bản ghi BE (hoặc `null` = dòng trống mới) + cập nhật side-state theo `_uid`. */
  private _buildRow(
    orderServiceItem: OrderServiceUsedModel | null,
    model: OrderPlanModel,
  ): OrderServiceUsedRow {
    // Khóa service_id theo TỪNG dòng: dòng đã có payment thì đổi dịch vụ phải đi qua dialog
    // "Đổi dịch vụ". Dòng mới/chưa payment → tự do đổi inline (BE cũng chỉ guard điều kiện này).
    const rowLocked = !!orderServiceItem?.id && Number(orderServiceItem?.payment) > 0;

    const row = emptyServiceRow({
      id: orderServiceItem?.id || null,
      customer_id: orderServiceItem?.customer_id || model.customer_id || null,
      order_plan_id: orderServiceItem?.order_plan_id || model.id || null,
      service_discount: orderServiceItem?.service_discount || 0,
      total: orderServiceItem?.total || 0,
      totalEnd: orderServiceItem?.totalEnd || 0,
      payment: orderServiceItem?.payment || 0,
      note: orderServiceItem?.note || null,
      service_id: orderServiceItem?.service_id ?? null,
      service_discount_type: orderServiceItem?.service_discount_type || SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT,
      unit_price: orderServiceItem?.unit_price ?? null,
      quantity: orderServiceItem?.quantity ?? null,
      discount_other: orderServiceItem?.discount_other ?? null,
      min_price: orderServiceItem?.serviceInfo?.min_price ?? 0,
      max_price: orderServiceItem?.serviceInfo?.max_price ?? 0,
      updated_at: orderServiceItem?.updated_at ?? null,
      discountServiceLock: orderServiceItem?.discountServiceLock || 0,
      unitName: this.unitHelper.getLabel(orderServiceItem?.serviceInfo?.unit_id) ?? null,
      rank_discount_percent: orderServiceItem?.rank_discount_percent || 0,
      serviceLocked: rowLocked,
      unitPriceLocked: false,
    });

    const uid = row._uid;
    if (orderServiceItem) {
      this.selectedServiceUsed = orderServiceItem.serviceInfo ?? null;
      this.serviceInfoMap.set(uid, orderServiceItem.serviceInfo as ServiceModel);
      row.unitPriceLocked =
        (Number(orderServiceItem?.unit_price) === Number(orderServiceItem?.serviceInfo?.min_price) &&
          Number(orderServiceItem?.unit_price) === Number(orderServiceItem?.serviceInfo?.max_price)) ||
        !(
          Number(orderServiceItem?.serviceInfo?.min_price) <= Number(orderServiceItem?.unit_price) &&
          Number(orderServiceItem?.serviceInfo?.max_price) >= Number(orderServiceItem?.unit_price)
        );
    }

    // Bản gốc gọi `setTooltipFlag()` cho MỌI dòng ngay trong `addServiceUsed()` — phải đặt ở đây (chỗ
    // dựng dòng dùng chung) chứ không chỉ ở `addServiceUsed()`, nếu không các dòng nạp lúc mở "Sửa" sẽ
    // mất cờ và ô đơn giá hiện nhầm tooltip "không sửa được" dù đang cho sửa.
    this.setTooltipFlag(uid, !row.unitPriceLocked && !!orderServiceItem);

    return row;
  }

  protected override buildSchema(p: SchemaPathTree<OrderPlanModel>): void {
    required(p.customer_id!);
    required(p.branch_id!);
    required(p.created_time!);

    applyEach(p.services_used!, serviceRowSchema);
  }

  // ============ REACTIVE EFFECTS (thay 6 valueChanges.subscribe của bản Reactive Forms) ============

  /**
   * [CRITICAL] Toàn bộ recalculation tiền gom vào effect key theo CHUỖI SERIALIZE các field ĐIỀU KHIỂN
   * (`service_discount`/`unit_price`/`quantity`/`discount_other`/`service_id`).
   *
   * Field bị GHI (`total`/`totalEnd`/`rank_discount_percent` cấp dòng và
   * `total`/`discount_other` cấp form) TUYỆT ĐỐI KHÔNG được có mặt trong key — nếu không sẽ
   * lặp vô hạn. Mọi lệnh ghi bọc `untracked()`.
   */
  private _registerRecalcEffects(): void {
    // Đọc key qua `this.model()` chứ KHÔNG qua `this.rootField.x!()`: lúc constructor chạy, `model` vẫn
    // là `{}` nên `rootField.services_used` trả `undefined` (không callable) và effect sẽ ném TypeError
    // ngay lần chạy đầu — đúng cái bẫy đã ghi trong doc `trackScopeFields()` của `BaseSignalForm`.
    // `model` là nguồn dữ liệu của `form()` nên đọc thẳng vẫn phản ứng đủ mọi thay đổi do user gõ.
    effectOnChange(
      () => {
        const rows = this.model().services_used ?? [];
        const rowKey = rows
          .map(r =>
            [r.quantity, r.unit_price, r.service_discount, r.service_discount_type, r.discount_other, r.service_id].join('|'),
          )
          .join(';');
        return `${rowKey}#${this.customer()?.customer_rank_id ?? ''}`;
      },
      () => untracked(() => void this._recalcMoney()),
    );

    effectOnChange(
      () => this.model().branch_id,
      (branchId) =>
        untracked(() => {
          // Giữ lại lựa chọn cũ nếu người đó vẫn thuộc branch mới; chỉ clear
          // những id không còn hợp lệ ở branch vừa chọn.
          this.keepValidUserList('listRevenues', branchId);
        }),
    );
  }

  // ============ SERVICE USED CRUD ============

  deleteServiceUsed(key: number) {
    // Mọi side-state key theo `_uid` → chỉ cần XOÁ đúng entry của dòng bị xoá, KHÔNG phải dịch lại
    // toàn bộ map như bản index-key (nguồn bug lệch-1-dòng: state gán nhầm sang dịch vụ khác).
    const uid = this.model().services_used?.[key]?._uid;
    this.model.update(m => ({
      ...m,
      services_used: (m.services_used ?? []).filter((_, i) => i !== key),
    }));

    if (uid != null) {
      this.serviceInfoMap.delete(uid);
      this.displayPTooltipUnitPrice.update(m => {
        if (!m.has(uid)) return m;
        const next = new Map(m);
        next.delete(uid);
        return next;
      });
      this.changedFromServiceName.update(m => {
        if (!m.has(uid)) return m;
        const next = new Map(m);
        next.delete(uid);
        return next;
      });
    }

    // Bản gốc gọi `handleServiceUsed()` (chỉ cộng lại tổng). Ở đây danh sách dòng đổi → key của effect
    // tiền đổi theo, recalc tự chạy; vẫn gọi tay để tổng cập nhật ngay trong cùng tick khi xoá dòng cuối.
    void this._recalcMoney();
  }

  addServiceUsed(orderServiceItem: OrderServiceUsedModel | null = null) {
    const dataTableWrapper = document.querySelector('.p-dialog .p-datatable-wrapper');
    if (dataTableWrapper) {
      const isScrollable = dataTableWrapper.scrollHeight > dataTableWrapper.clientHeight;
      if (isScrollable) {
        dataTableWrapper.scrollTo(0, dataTableWrapper.scrollHeight);
      }
    }

    // BE không attach serviceInfo (data đã có trong IndexedDB cache: services).
    // Resolve từ cache theo service_id để form đọc servicePosition / department_type / min-max…
    if (orderServiceItem && !orderServiceItem.serviceInfo && orderServiceItem.service_id) {
      const fromCache = this.servicesHelper.getServiceInfo(orderServiceItem.service_id);
      if (fromCache) {
        orderServiceItem = {...orderServiceItem, serviceInfo: fromCache} as OrderServiceUsedModel;
      }
    }

    const row = this._buildRow(orderServiceItem, this.model());
    this.model.update(m => ({...m, services_used: [...(m.services_used ?? []), row]}));
  }

  changeDiscountType(value: number, index: number) {
    let serviceDiscount = 0;
    const rowId = this.model().services_used?.[index]?.id;
    if (rowId) {
      const serviceUsedDb = this.originalServiceUseds.find((item: any) => item.id === rowId);
      if (serviceUsedDb && serviceUsedDb.service_discount_type === value) {
        serviceDiscount = serviceUsedDb.service_discount ?? 0;
      }
    }
    this._patchRow(index, {service_discount_type: value, service_discount: Math.round(serviceDiscount)});
  }

  // ============ CALCULATIONS ============

  /**
   * Gộp `handleService()` + `calculateRankDiscount()` + `handleServiceUsed()` của bản gốc thành 1 lượt
   * đọc-tính-ghi duy nhất (Signal Forms ghi cả mảng dòng 1 lần thay vì patch từng control).
   */
  private async _recalcMoney(): Promise<void> {
    const customerRankId = this.customer()?.customer_rank_id;

    if (customerRankId && !this.isEditMode()) {
      await this.customerRankServicesHelper.ensureCustomerRankServicesLoaded();
      await this.customerRanksHelper.ensureCustomerRanksLoaded();
    }

    const hasRankServiceConfig =
      !!customerRankId && !this.isEditMode()
        ? this.customerRankServicesHelper.getAll().some((item) => item.rank_id === customerRankId)
        : false;
    const generalRankPercent =
      !!customerRankId && !this.isEditMode() && !hasRankServiceConfig
        ? Number(this.customerRanksHelper.getRankById(customerRankId)?.discount_percent) || 0
        : 0;

    const rows = this.model().services_used ?? [];

    let rowsChanged = false;
    const updated = rows.map((row) => {
      const quantity = Number(row.quantity) || 0;
      const unitPrice = Number(row.unit_price) || 0;
      const serviceDiscount = Number(row.service_discount) || 0;

      // Rank discount percent
      let rankDiscountPercent = 0;
      if (customerRankId && row.service_id) {
        if (this.isEditMode()) {
          // Lookup by id (không dùng positional index — index lệch khi user thêm/xóa dòng)
          const originalSvc = this.originalServiceUseds.find((s: any) => s.id === row.id);
          rankDiscountPercent = originalSvc?.rank_discount_percent || 0;
        } else if (hasRankServiceConfig) {
          const rankService = this.customerRankServicesHelper.getRankServiceByRankAndService(
            customerRankId,
            row.service_id,
          );
          if (rankService) {
            rankDiscountPercent = Number(rankService.discount_percent) || 0;
          }
        } else {
          rankDiscountPercent = generalRankPercent;
        }
      }

      const total = this.totalTotalService(
        unitPrice,
        quantity,
        serviceDiscount,
        row.service_discount_type,
      );

      let totalEnd = Math.round(total - (Number(row.discount_other) || 0));
      const originalSvc = this.originalServiceUseds.find((s: any) => s.id === row.id);
      const discountServiceLock = originalSvc?.discountServiceLock ?? 0;
      if (discountServiceLock) {
        totalEnd -= discountServiceLock;
      }

      const nextTotal = Math.round(total);
      const nextTotalEnd = Math.round(totalEnd);
      // Giữ NGUYÊN tham chiếu khi 3 field tính ra không đổi — mỗi phím gõ đều chạy qua đây, tạo object
      // mới cho cả mảng làm mọi binding trong bảng phải so lại (và trước khi có `[rowTrackBy]` thì còn
      // dựng lại DOM). Chỉ dòng thực sự đổi số mới cần object mới.
      if (
        row.total === nextTotal &&
        row.totalEnd === nextTotalEnd &&
        row.rank_discount_percent === rankDiscountPercent
      ) {
        return row;
      }
      rowsChanged = true;
      return {
        ...row,
        total: nextTotal,
        totalEnd: nextTotalEnd,
        rank_discount_percent: rankDiscountPercent,
      };
    });

    // Tổng giảm giá theo hạng — tương đương `calculateRankDiscount()` gốc (cả 2 nhánh edit/create đều
    // cộng `totalEnd * phần trăm dòng / 100` cho dòng có totalEnd > 0).
    let totalRankDiscount = 0;
    let total = 0;
    let discountOther = 0;
    updated.forEach((row) => {
      const rowTotalEnd = Number(row.totalEnd) || 0;
      const pct = Number(row.rank_discount_percent) || 0;
      if (customerRankId && rowTotalEnd > 0 && pct > 0) {
        totalRankDiscount += (rowTotalEnd * pct) / 100;
      }
      total += rowTotalEnd;
      discountOther += Number(row.discount_other) || 0;
    });

    this.totalRankDiscount.set(totalRankDiscount);
    const nextTotal = Math.round(total - totalRankDiscount);
    const nextDiscountOther = Math.round(discountOther);
    const current = this.model();
    // Không ghi lại `model` khi KHÔNG có gì đổi — mỗi lần `update()` sinh object model mới, kéo theo
    // toàn bộ FieldTree/template đọc `model()` phải tính lại dù kết quả y hệt.
    if (
      rowsChanged ||
      current.total !== nextTotal ||
      current.discount_other !== nextDiscountOther
    ) {
      this.model.update(m => ({
        ...m,
        services_used: rowsChanged ? updated : m.services_used,
        total: nextTotal,
        discount_other: nextDiscountOther,
      }));
    }
  }

  totalTotalService(
    unitPrice: any = 0,
    quantity: any = 0,
    discountSerivce: any = 0,
    discountType: number | null,
  ) {
    let total = 0;
    if (!unitPrice || !quantity) {
      return unitPrice * quantity;
    }
    total = unitPrice * quantity;
    let lastDiscount = Number(discountSerivce);
    if (discountType === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT) {
      lastDiscount = Math.round((discountSerivce * total) / 100);
    }
    if (Number(lastDiscount) > 0) {
      total -= Number(lastDiscount);
    }
    return Math.round(total);
  }

  onSelectService(itemSelected: any, key: number) {
    // `key` là VỊ TRÍ dòng (autocomplete emit `index`) — quy đổi ngay sang `_uid` để mọi side-state
    // ghi theo khoá ổn định của dòng.
    const uid = this.model().services_used?.[key]?._uid;
    if (uid == null) return;

    // Bản gốc còn patch `service_type` — control đó KHÔNG tồn tại trong `buildForm()` nên Reactive
    // Forms nuốt im lặng; ở đây ghi thẳng vào model sẽ tạo key thừa (và lọt lên BE nếu quên whitelist)
    // → bỏ hẳn, đúng hành vi thực tế đang chạy.
    const unitPriceLocked =
      !!itemSelected &&
      (itemSelected?.min_price == null ||
        itemSelected?.max_price == null ||
        Number(itemSelected?.min_price) === Number(itemSelected?.max_price));

    this._patchRow(key, {
      service_id: itemSelected?.id ?? null,
      unitName: this.unitHelper.getLabel(itemSelected?.unit_id) ?? null,
      quantity: 0,
      unit_price: itemSelected?.min_price ?? itemSelected?.max_price ?? 0,
      min_price: itemSelected?.min_price ?? null,
      max_price: itemSelected?.max_price ?? null,
      unitPriceLocked,
    });
    this.setTooltipFlag(uid, !unitPriceLocked);

    this.selectedServiceUsed = itemSelected;
    this.serviceInfoMap.set(uid, itemSelected);
  }

  /**
   * Điều kiện hiện nút "Đổi dịch vụ" (order-service-change story-07) — CẢ 2 đúng mới hiện: có quyền
   * + OSU này đã có payment (per-OSU, khác khoá `serviceLocked` cấp dòng lúc dựng form). Sai bất kỳ
   * điều kiện nào → ẩn hẳn (không disabled+tooltip).
   */
  canChangeService(row: OrderServiceUsedRow | undefined): boolean {
    if (!row) return false;
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePlanOrder)) {
      return false;
    }
    return Number(row.payment) > 0;
  }

  /**
   * Mở dialog "Đổi dịch vụ" cho 1 dòng OSU — tách biệt khỏi form OSU thường (KHÔNG bật lại
   * ô service_id inline). Sau khi đổi thành công, patch lại đúng dòng (giữ nguyên các
   * dòng/state khác); recalc tiền tự chạy vì service_id/quantity đều nằm trong key effect.
   */
  openChangeServiceDialog(index: number) {
    const row = this.model().services_used?.[index];
    if (!row) return;
    const uid = row._uid;
    const currentServiceId = row.service_id;
    const currentServiceInfo = this.servicesHelper.getServiceInfo(currentServiceId!);

    const dialogRef = this.dialogWrapperService.open(ChangeServiceDialog, {
      header: this.translate.instant('crm.order_plan.change_service.action'),
      data: {
        orderServiceUsedId: row.id,
        currentServiceId,
        updated_at: row.updated_at,
      },
      width: '30%',
    });

    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((updated: any) => {
      if (!updated) return;
      // Dialog đóng bất đồng bộ — vị trí dòng có thể đã đổi (user xoá/thêm dòng khác trong lúc mở).
      // Tra lại index THEO `_uid` thay vì dùng lại `index` chụp lúc mở.
      const currentIndex = (this.model().services_used ?? []).findIndex(r => r._uid === uid);
      if (currentIndex < 0) {
        // BE ĐÃ commit đổi dịch vụ rồi mà dòng không còn trên form — phải báo, không nuốt im lặng.
        this.noti.warnMsg(this.translate.instant('crm.order_plan.change_service.row_removed'));
        return;
      }
      const newServiceInfo = this.servicesHelper.getServiceInfo(updated.service_id);
      const oldName = currentServiceInfo?.name;
      if (oldName) {
        const map = new Map(this.changedFromServiceName());
        map.set(uid, oldName);
        this.changedFromServiceName.set(map);
      }
      // Mirror onSelectService: dòng sau khi đổi dịch vụ phải "sạch" như vừa chọn dịch vụ mới
      // (đơn giá theo min/max mới, quantity clear để nhập lại — thông số dịch vụ cũ KHÔNG còn đúng
      // nghĩa khi đổi sang dịch vụ khác).
      this.onSelectService(newServiceInfo, currentIndex);
      // onSelectService không biết updated_at mới từ BE (không thuộc shape ServiceModel) — override
      // lại: PHẢI giữ đúng bản BE trả về (mất là lần lưu sau dính optimistic-lock conflict).
      this._patchRowByUid(uid, {updated_at: updated.updated_at});
    });
  }

  // ============ SUBMIT ============

  checkBeforeSubmit() {
    if (
      !isAdmin &&
      !isDevelop &&
      branchActiveId() != null &&
      branchActiveId() != this.model().branch_id
    ) {
      const dialogOtherBranch = this.dialogWrapperService.open(ShowOtherBranch, {
        header: this.translate.instant('crm.order_plan.dialog_note'),
        data: {text: 'đơn hàng'},
        width: '30%',
      });
      dialogOtherBranch.onClose
        .pipe(takeUntilDestroyed(this.destroyRef))
        .subscribe((result: any) => {
          if (result) {
            this.onSubmit();
          }
        });
    } else {
      this.onSubmit();
    }
  }

  override onSubmit() {
    if (this.isSubmitting()) return;

    // Dọn các dòng dịch vụ hoàn toàn trống (chưa chọn dịch vụ, chưa từng lưu) trước khi
    // check validity — form luôn có sẵn 1 dòng trống khi mở, để nguyên thì mảng invalid
    // (quantity required) mà không có thông báo nào, user tưởng nút Lưu không hoạt động.
    const rowsBefore = this.model().services_used ?? [];
    for (let i = rowsBefore.length - 1; i >= 0; i--) {
      const row = rowsBefore[i];
      if (!row.service_id && !row.id) {
        this.deleteServiceUsed(i);
      }
    }

    this.submitted.set(true);
    this.rootField().markAsTouched();
    if (this.rootField().invalid()) {
      // eslint-disable-next-line no-console
      console.warn('[order-plan-form] invalid controls:', this.collectInvalidControlEntries());
      const invalidLabels = this.collectInvalidControlLabels();
      const baseMsg = this.translate.instant('common.validation.please_check_required_fields');
      this.noti.warnMsg(invalidLabels.length > 0 ? `${baseMsg}: ${invalidLabels.join(', ')}` : baseMsg);
      return;
    }

    this.isSubmitting.set(true);

    const raw = this.getPayload();
    if (!raw.services_used?.length) {
      this.noti.warn(this.translate.instant('common.label.missing_service'), this.translate.instant('common.label.please_add_at_least_1_service'));
      this.isSubmitting.set(false);
      return;
    }

    const payload = {
      ...raw,
      services_used: (raw.services_used ?? []).map((s: any) => {
        const finalTotalEnd = s.totalEnd || 0;
        // CRM không còn trả góp. Vẫn gửi KEY `installment_config` (= null): BE coi có key là tín hiệu
        // tính lại payment_status của dòng đã có mỗi lần lưu (order_plan.go, nhánh `hasInstallment`).
        return {
          ...s,
          totalEnd: finalTotalEnd,
          installment_config: null,
        };
      }),
    };

    const finalizeFn = finalize(() => this.isSubmitting.set(false));
    if (this.isEditMode()) {
      this.modelService.updateOrder(this.model().id!, payload, {})
        .pipe(finalizeFn, catchError(() => of(null)))
        .subscribe({
          next: (res: any) => {
            if (res) {
              this.noti.successRes(res);
              this.dialogRef.close(res.data);
            }
          }
        });
    } else {
      this.modelService.createOrder(payload, {})
        .pipe(finalizeFn, catchError(() => of(null)))
        .subscribe({
          next: (res: any) => {
            if (res) {
              this.noti.successMsg(this.translate.instant('crm.order_plan.noti_create_success'));
              this.dialogRef.close(res);
            }
          }
        });
    }
  }

  /**
   * OVERRIDE BẮT BUỘC — `extractValue()` mặc định duyệt TOÀN BỘ key có mặt trên `model()`, mà `model()`
   * ở đây đã spread nguyên response BE (`orderServiceUseds`, `customer`, `payments`, `branch`,
   * `userCreated`…). Bản Reactive Forms chỉ gửi đúng control top-level + mảng dòng
   * qua `rfModel.getRawValue()` — whitelist dưới đây tái tạo CHÍNH XÁC tập key đó, không thừa không thiếu.
   * 3 field FE-only của dòng (`_uid`/`unitPriceLocked`/`serviceLocked`) bị loại ở đây.
   */
  protected override getPayload(): any {
    const m = this.model();
    return {
      customer_id: m.customer_id,
      appointment_id: m.appointment_id,
      branch_id: m.branch_id,
      listRevenues: m.listRevenues,
      discount_other: m.discount_other,
      total: m.total,
      created_time: m.created_time,
      note: m.note,
      source_id: m.source_id,
      updated_at: m.updated_at,
      services_used: (m.services_used ?? []).map((r) => ({
        id: r.id,
        customer_id: r.customer_id,
        order_plan_id: r.order_plan_id,
        service_discount: r.service_discount,
        total: r.total,
        totalEnd: r.totalEnd,
        payment: r.payment,
        note: r.note,
        service_id: r.service_id,
        service_discount_type: r.service_discount_type,
        unit_price: r.unit_price,
        quantity: r.quantity,
        discount_other: r.discount_other,
        min_price: r.min_price,
        max_price: r.max_price,
        updated_at: r.updated_at,
        discountServiceLock: r.discountServiceLock,
        unitName: r.unitName,
        rank_discount_percent: r.rank_discount_percent,
      })),
    };
  }

  // ============ PRIVATE HELPERS ============

  /**
   * Khoá ổn định cho `p-table` — mặc định PrimeNG track theo THAM CHIẾU object (`item ?? index`), mà
   * mỗi lần recalc `model.update()` tạo object dòng MỚI → toàn bộ dòng bị huỷ/dựng lại, user đang gõ
   * dở trong ô số lượng/đơn giá sẽ MẤT FOCUS sau mỗi ký tự. Track theo `_uid` giữ nguyên DOM.
   */
  readonly rowTrackBy = (index: number, row: OrderServiceUsedRow) => row?._uid ?? index;

  /** Ghi 1 phần giá trị vào 1 dòng theo VỊ TRÍ — thay `formGroup.patchValue()` của Reactive Forms. */
  private _patchRow(index: number, patch: Partial<OrderServiceUsedRow>): void {
    this.model.update((m) => {
      const rows = [...(m.services_used ?? [])];
      if (!rows[index]) return m;
      rows[index] = {...rows[index], ...patch};
      return {...m, services_used: rows};
    });
  }

  /** Ghi 1 phần giá trị vào dòng có `_uid` cho trước — dùng ở mọi chỗ side-state gọi ngược lại dòng. */
  private _patchRowByUid(uid: number, patch: Partial<OrderServiceUsedRow>): void {
    this.model.update((m) => {
      const rows = [...(m.services_used ?? [])];
      const index = rows.findIndex((r) => r._uid === uid);
      if (index < 0) return m;
      rows[index] = {...rows[index], ...patch};
      return {...m, services_used: rows};
    });
  }

  /**
   * Bản Signal Forms của `BaseForm.keepValidUserList()` (helper đó đọc `rfModel.controls`, không dùng
   * lại được). Giữ các id vẫn hợp lệ, chỉ lọc bỏ id không còn thuộc branch mới.
   */
  private keepValidUserList(
    key: 'listRevenues',
    branchId: number | null | undefined,
  ): void {
    const current = this.model()[key] as number[] | null | undefined;
    if (!current || current.length === 0) return;

    const validIds = new Set(
      this.userCacheForOrderScope
        .filterUsers({
          branchId: branchId ?? undefined,
          activeOnly: !this.isEditMode(),
        })
        .map((u) => u.id),
    );
    const filtered = current.filter((id) => validIds.has(id));
    if (filtered.length !== current.length) {
      this.model.update(m => ({...m, [key]: filtered}));
    }
  }

  /** Map tên field top-level → i18n key label hiển thị trên UI (đúng key đang dùng trong template). */
  private readonly TOP_LEVEL_CONTROL_LABEL_KEYS: Record<string, string> = {
    branch_id: 'common.label.branch',
    created_time: 'common.label.order_date',
    note: 'common.label.note',
    source_id: 'common.label.source',
    listRevenues: 'common.label.consultant',
  };

  /** Map tên field trong mỗi dòng services_used → i18n key label. */
  private readonly SERVICE_ROW_CONTROL_LABEL_KEYS: Record<string, string> = {
    service_id: 'common.label.service',
    quantity: 'common.label.quantity',
    unit_price: 'common.label.unit_price',
    service_discount: 'common.label.service_discount',
    discount_other: 'common.label.other_discount',
  };

  /** Field top-level có validator — dùng để quét invalid (Signal Forms không liệt kê control động được). */
  private readonly TOP_LEVEL_VALIDATED_FIELDS = ['customer_id', 'branch_id', 'created_time'] as const;
  /** Field cấp dòng có validator. */
  private readonly ROW_VALIDATED_FIELDS = ['customer_id', 'quantity'] as const;

  /**
   * Duyệt các field có validator (top-level + từng dòng) và trả về chi tiết field đang invalid — dùng
   * cho console.warn debug (order-plan-form invalid toast bug: toast hiện mà không field nào đỏ vì thủ
   * phạm là control không render trên UI).
   */
  private collectInvalidControlEntries(): { name: string; rowIndex?: number; errors: any }[] {
    const entries: { name: string; rowIndex?: number; errors: any }[] = [];
    for (const name of this.TOP_LEVEL_VALIDATED_FIELDS) {
      const field = (this.rootField as any)[name];
      if (field && field().invalid()) entries.push({name, errors: field().errors()});
    }
    const rowsField = this.rootField.services_used!;
    (this.model().services_used ?? []).forEach((_row, index) => {
      const rowField = (rowsField as any)[index];
      if (!rowField) return;
      for (const name of this.ROW_VALIDATED_FIELDS) {
        const field = rowField[name];
        if (field && field().invalid()) entries.push({name, rowIndex: index, errors: field().errors()});
      }
    });
    return entries;
  }

  /**
   * Nhãn hiển thị (đã translate) của các field đang invalid — để lộ thủ phạm khi toast
   * "vui lòng kiểm tra các trường bắt buộc" hiện dù mọi field nhìn thấy đã điền đủ.
   */
  private collectInvalidControlLabels(): string[] {
    const labels = this.collectInvalidControlEntries().map((entry) => {
      const key =
        entry.rowIndex !== undefined
          ? this.SERVICE_ROW_CONTROL_LABEL_KEYS[entry.name]
          : this.TOP_LEVEL_CONTROL_LABEL_KEYS[entry.name];
      const label = key ? this.translate.instant(key) : entry.name;
      return entry.rowIndex !== undefined ? `${label} (dòng ${entry.rowIndex + 1})` : label;
    });
    return [...new Set(labels)];
  }

  /** Tiền đã thanh toán của đơn — snapshot lúc mở dialog (xem `originalPayment`). */
  get paidAmount(): number {
    return this.originalPayment;
  }

  private setTooltipFlag(uid: number, v: boolean) {
    this.displayPTooltipUnitPrice.update((m) => {
      const next = new Map(m);
      next.set(uid, v);
      return next;
    });
  }
}
