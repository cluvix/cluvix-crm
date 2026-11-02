import { TranslatePipe } from '@ngx-translate/core';
import {HttpContext} from '@angular/common/http';
import {ChangeDetectionStrategy, Component, computed, ElementRef, inject, OnDestroy, OnInit, signal, ViewChild} from '@angular/core';
import {takeUntilDestroyed} from '@angular/core/rxjs-interop';
import {finalize} from 'rxjs';

import {AppointmentInCustomer} from '@/modules/crm/appointment/list/detail-customer/appointment-in-customer';
import {CustomerInfoRow} from '@/modules/crm/customer/customer-detail/customer-info-row';
import {CUSTOMER_INFO_FIELDS} from '@/modules/crm/customer/customer-detail/customer-info-config';
import {SKIP_ERROR_NOTIFICATION} from '@/core/interceptors/response-handler.interceptor';
import {consumeCustomerPreload} from '@/share/utils/customer-preload.util';
import {CustomerFormV2} from '@/modules/crm/customer/form/form-customer/customer-form-v2';
import {OrderPlanInCustomer} from '@/modules/crm/order-plan/list/order-plan-in-customer';
import {PaymentOfCustomer} from '@/modules/crm/payment/list/detail-customer/payment-of-customer';
import {ReminderOfCustomer} from '@/modules/crm/reminder/customer/reminder-of-customer';
import {CustomerModel, CustomerStageChangeResult} from '@/core/models/crm/customer.model';
import {CustomerService} from '@/core/services/crm/customer.service';
import {BaseDetail} from '@/share/base/base-detail';
import {BreadcrumbLabelService} from '@/share/services/breadcrumb-label.service';
import {AvatarLazy} from '@/share/components/app-avatar-lazy';
import {LayoutService} from '@/layout/service/layout.service';
import {CONFIG_FILE_SELECT, getGenderLabel, PERMISSION_SELECT,} from '@/define';
import {DetailShareImports} from '@/share/import/detail-share-imports';
import {isAllowExtensions} from '@/share/services/utils.service';
import {CustomerTimeline} from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import {CustomerNoteAiTabs} from '@/modules/crm/customer/customer-detail/tabs-info/customer-note-ai-tabs';
import {CustomerStageChanger} from '@/modules/crm/customer/customer-detail/customer-stage-changer';
import {applyStageChange} from '@/modules/crm/customer/customer-detail/customer-stage-change.util';
import {shade, tint} from '@/share/utils/dashboard-tint.util';
import {hasPermission} from '@/core/services/auth/auth.selectors';

/**
 * Rail stat gom 2 NHÓM ngữ nghĩa, mỗi nhóm 1 hue chủ đạo — thay cho 13 hue rời rạc
 * trước đây (màu không mã hoá thông tin gì, "Ký gửi" và "Số lần ghé" còn trùng cyan
 * dù khác nhóm hoàn toàn). Giờ màu = nhóm, độ đậm tint = mức quan trọng; chỉ tile
 * mang tín hiệu thật mới phá màu nhóm: Công nợ (đỏ).
 * Nền/chữ dựng bằng tint()/shade() nên dark-safe (B.18).
 */
const STAT_COLOR = {
  finance: '#10B981',
  debt: '#E53935',
  behavior: '#6366F1',
  lastContact: '#6366F1',
  lastVisit: '#14B8A6',
} as const;

/** 1 ô stat trên rail. `digits` undefined → DecimalPipe dùng format mặc định. */
interface StatTile {
  key: string;
  icon: string;
  color: string;
  labelKey: string;
  tooltipKey: string;
  /** Giá trị số. */
  value: number;
  digits?: string;
  /** Đơn vị đứng sau số (i18n key), vd "lần" / "ngày". */
  suffixKey?: string;
  /** Chỉ số dẫn dắt của nhóm → tint đậm hơn để mắt bám vào trước. */
  emphasis?: boolean;
  /** Giá trị 0 → làm mờ nhưng VẪN mount, giữ rail không re-flow (B.19 CLS). */
  dim?: boolean;
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-detail',
  templateUrl: './customer-detail.html',
  styleUrls: ['./customer-detail.scss'],
  standalone: true,
  imports: [
    ...DetailShareImports,
    AppointmentInCustomer,
    OrderPlanInCustomer,
    PaymentOfCustomer,
    ReminderOfCustomer,
    CustomerInfoRow,
    AvatarLazy,
    CustomerNoteAiTabs,
    CustomerStageChanger,
    TranslatePipe],
})
export class CustomerDetail extends BaseDetail<CustomerModel> implements OnInit, OnDestroy {
  modelService: CustomerService = inject(CustomerService);
  public permission = PERMISSION_SELECT;
  private readonly layoutService = inject(LayoutService);
  private readonly breadcrumbLabelService = inject(BreadcrumbLabelService);

  /** Nhóm 1 — Tài chính. Hue emerald, riêng Công nợ giữ đỏ vì là tín hiệu cần xử lý. */
  readonly financeStats = computed<StatTile[]>(() => {
    const m = this.model();
    const F = STAT_COLOR.finance;
    const num = (v: any) => Number(v ?? 0);
    return [
      {key: 'revenue', icon: 'pi pi-chart-line', color: F, emphasis: true,
       labelKey: 'common.label.revenue', tooltipKey: 'crm.customer.revenue_tooltip', value: num(m?.totalRevenue)},
      {key: 'payment', icon: 'pi pi-check-circle', color: F,
       labelKey: 'common.status.paid', tooltipKey: 'crm.customer.payment_tooltip', value: num(m?.totalPayment)},
      {key: 'debt', icon: 'pi pi-exclamation-triangle', color: STAT_COLOR.debt,
       labelKey: 'common.label.debt', tooltipKey: 'crm.customer.debt_tooltip', value: num(m?.totalDebt)},
      {key: 'deposit', icon: 'pi pi-wallet', color: F,
       labelKey: 'crm.customer.deposit_short', tooltipKey: 'crm.customer.deposit_tooltip', value: num(m?.totalDeposit)},
      {key: 'refundDeposit', icon: 'pi pi-replay', color: F,
       labelKey: 'common.label.deposit_refund', tooltipKey: 'crm.customer.refund_deposit_tooltip', value: num(m?.refundDeposit)},
      {key: 'refundService', icon: 'pi pi-undo', color: F,
       labelKey: 'crm.customer.refund_service_short', tooltipKey: 'crm.customer.refund_service_tooltip', value: num(m?.refundService)},
    ].map((t) => ({...t, dim: t.value === 0}));
  });

  /**
   * Nhóm 2 — Hành vi (số lần đến, số ngày ghé). Hue indigo. Đọc trực tiếp từ customer row.
   */
  readonly behaviorStats = computed<StatTile[]>(() => {
    const m = this.model();
    const V = STAT_COLOR.behavior;
    // Number() bọc để so sánh === 0 (làm mờ tile) không sai khi BE trả string.
    const num = (v: any) => Number(v ?? 0);
    return [
      {key: 'treatment', icon: 'pi pi-heart', color: V, suffixKey: 'common.label.times',
       labelKey: 'common.label.treatment_progress', tooltipKey: 'crm.customer.treatment_count_tooltip', value: num(m?.treatment_count)},
      {key: 'visit', icon: 'pi pi-calendar', color: V, suffixKey: 'common.label.days',
       labelKey: 'crm.customer.visit_count', tooltipKey: 'crm.customer.visit_count_tooltip', value: num(m?.visit_count)},
    ].map((t) => ({...t, dim: t.value === 0}));
  });

  readonly statGroups = computed(() => [
    {titleKey: 'common.label.finance', tiles: this.financeStats()},
    {titleKey: 'crm.customer.behavior_group', tiles: this.behaviorStats()},
  ]);
  /**
   * PrimeNG 22.0.0-rc.2 [lazy] trên p-tabs KHÔNG lazy thật (verified qua unit test cô lập:
   * tabpanel không active vẫn bị Angular tạo content ngay lần render đầu) — dùng @defer
   * (when activeTab() === 'X') ở template thay vì dựa vào lazy của PrimeNG. @defer tự có
   * "render 1 lần rồi giữ" + code-split riêng từng tab, không cần state phụ để tự quản lý.
   */
  readonly activeTab = signal('0');
  readonly avatarTs = signal(Date.now());
  readonly avatarUrl = computed(() => {
    const m = this.model();
    return m?.avatar ? `${m.avatar}?t=${this.avatarTs()}` : (m?.avatar ?? '');
  });
  readonly lastContactRelative = signal('');
  readonly lastVisitRelative = signal('');
  readonly skeletonRows = Array(15);
  readonly skeletonStats = Array(2);
  customerInfoFields = CUSTOMER_INFO_FIELDS;
  readonly tags = signal<string[]>([]);
  newTag = '';
  showCameraDialog = false;
  stream: MediaStream | null = null;
  @ViewChild('video') videoRef!: ElementRef<HTMLVideoElement>;
  @ViewChild('canvas') canvasRef!: ElementRef<HTMLCanvasElement>;
  /** Vùng cuộn riêng của tab "Thông tin" (các tab khác cuộn theo trang). */
  @ViewChild('tabScroll') tabScrollRef?: ElementRef<HTMLDivElement>;
  capturedImage: string | null = null;
  /**
   *  Hàm tạo mới nhãn khách hàng
   */
  adding = false;
  protected readonly Number = Number;
  protected readonly String = String;
  protected readonly C = STAT_COLOR;
  protected readonly tint = tint;
  protected readonly shade = shade;
  protected readonly getGenderLabel = getGenderLabel;
  /**
   * Reload lại thông tin khách hàng sau khi update ở các tab
   * @param $event
   */
  private reloadTimeout: any = null;
  /**
   * Lý do chưa đặt lịch
   * @param event
   */
  private reasonUpdateInProgress = false;
  /**
   * Guard riêng cho upload avatar (camera/file) để không clash với macro
   */
  private uploadingAvatar = false;

  getNestedValue(obj: any, path: string): any {
    return path.split('.').reduce((acc, part) => acc?.[part], obj);
  }

  /**
   * Đổi tab: giữ tab trên URL + đưa vùng nội dung về đầu.
   * Không reset scroll thì chuyển từ tab dài (đang cuộn sâu) sang tab ngắn sẽ
   * thấy khoảng trắng và tưởng tab rỗng.
   */
  protected onTabChange(value: unknown): void {
    const tab = String(value);
    if (tab === this.activeTab()) return;

    this.activeTab.set(tab);
    void this.route.navigate([], {
      relativeTo: this.activatedRoute,
      queryParams: {tab},
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });

    this.tabScrollRef?.nativeElement.scrollTo({top: 0});
    window.scrollTo({top: 0});
  }

  /**
   * Tab hợp lệ để nhận từ URL. Tab ẩn theo chuyên khoa/permission mà nhận bừa thì
   * p-tabs trỏ vào panel không tồn tại → vùng nội dung trắng trơn.
   */
  protected readonly visibleTabValues = computed(() =>
    Object.entries(this.tabPerms)
      .filter(([, perm]) => !perm || hasPermission(perm))
      .map(([tab]) => tab),
  );

  /**
   * R-4 (2026-10-18): quyền XEM của API nuôi từng tab — khớp `route_perm_map.json` BE (GET enforce):
   * nhắc lịch `reminder/reminder-of-customer`, lịch hẹn `appointment/get-by-customer`, đơn
   * `order-service-used/list-service-used-customer`, thanh toán `payment/get-payment-detail-for-customer`.
   * Thiếu quyền ⇒ ẩn tab (template `*hasPerm`), không gọi API nhận 403.
   */
  protected readonly tabPerms: Record<string, string | null> = {
    '0': null,
    '1': PERMISSION_SELECT.permissionViewReminder,
    '2': PERMISSION_SELECT.permissionViewAppointment,
    '4': PERMISSION_SELECT.permissionViewServiceUsed,
    '5': PERMISSION_SELECT.permissionViewPayment,
  };

  ngOnInit() {
    this.activatedRoute.params
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((queryParams: any) => {
        this.id.set(queryParams.id);
        this.hydrateFromPreload(queryParams.id);
        this.getModel(queryParams.id);
      });

    // Khôi phục tab từ ?tab= (F5 / mở link ở tab mới / link gửi cho đồng nghiệp).
    // Đọc snapshot 1 lần: onTabChange ghi lại URL bằng replaceUrl nên không có
    // vòng lặp queryParams → set signal → navigate.
    const tab = this.activatedRoute.snapshot.queryParamMap.get('tab');
    if (tab && this.visibleTabValues().includes(tab)) {
      this.activeTab.set(tab);
    }

    // Trang chi tiết khách hàng mặc định menu Overlay để tối đa không gian nội dung.
    // Dùng menuModeOverride (không persist localStorage) thay vì mutate layoutConfig trực tiếp —
    // tránh ghi đè cài đặt layout đã lưu của user nếu ngOnDestroy không kịp chạy (đóng tab đột ngột...).
    this.layoutService.menuModeOverride.set('overlay');
  }

  /**
   * Paint instant từ preload nếu có:
   *  - history.state.customer cho in-app navigate (cùng tab)
   *  - localStorage `customer:preload:{id}` cho new tab nav (target="_blank")
   * Fresh HTTP vẫn fire sau để cập nhật field server-computed.
   */
  private hydrateFromPreload(id: number | string): void {
    const preloaded = history.state?.customer ?? consumeCustomerPreload(id);
    if (!preloaded || typeof preloaded !== 'object' || !preloaded.id) return;

    const prepared = this.preparePermissions(preloaded);
    const labeled = this.modelService.addLabel(prepared);
    this.model.set(labeled as any);
    this.afterModelLoaded(labeled as any);
  }

  override getModel = (id: number) => {
    this.loading.set(true);
    // Fire helper cache load song song với HTTP fetch để giảm wall-clock latency.
    const cachePromise = this.modelService.ensureCacheLoaded();
    this.modelService
      // SKIP_ERROR_NOTIFICATION: `response-handler.interceptor` biến MỌI 404 của API
      // thành `router.navigate(['/auth/notfound'])` (điều hướng toàn trang, mù, không
      // biết API nào hỏng). Với khách nằm ngoài phạm vi leadscope, BE cố ý trả 404
      // (story-32) ⇒ user bị ném ra màn "404 Not Found" vô nghĩa thay vì được báo đúng
      // lý do. Tự xử lý ở nhánh `error` bên dưới.
      .getCustomerById(id, {context: new HttpContext().set(SKIP_ERROR_NOTIFICATION, true)})
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: async (model) => {
          if (!model) {
            this.noti.error(this.translate.instant('crm.customer.not_found_title'), this.translate.instant('crm.customer.not_found_message'));
            void this.route.navigateByUrl('/customer');
            return;
          }

          await cachePromise;

          const prepared = this.preparePermissions(model);
          const labeled = this.modelService.addLabel(prepared);

          if (labeled.relatives && typeof labeled.relatives === 'string') {
            try {
              labeled.relatives = JSON.parse(labeled.relatives);
            } catch {
              labeled.relatives = [];
            }
          }

          this.model.set(labeled as any);
          if (labeled.id) {
            this.resetForm.set(labeled.id);
          }
          this.afterModelLoaded(labeled as any);
        },
        error: (err) => {
          console.error('[CustomerDetail] Error:', err);
          // 404 = khách không tồn tại HOẶC nằm ngoài phạm vi leadscope của user
          // (telesale chỉ đọc được khách có `payment_user_id` = mình, convert theo
          // `convert_id` — xem `sale/leadscope/scope.go`, bật `user_option` id=5 để tắt lọc).
          // BE cố ý dùng 404 thay 403 để không tiết lộ khách đó có tồn tại hay không,
          // nên thông báo ở đây cũng phải mập mờ đúng như vậy.
          if (err?.status === 404) {
            this.noti.error(
              this.translate.instant('crm.customer.not_found_title'),
              this.translate.instant('crm.customer.not_found_message'),
            );
          } else {
            this.noti.errorMsg(this.translate.instant('crm.customer.load_failed'));
          }
          void this.route.navigateByUrl('/customer');
        },
      });
  };

  /**
   * timeline từng khách hàng
   * @param id
   * @param fullname
   */
  timelineCustomer(id: number, fullname: string) {
    this.dialogWrapperService.open(CustomerTimeline, {
      header: fullname,
      data: {
        customerId: id,
      },
      width: '960px',
      height: '80vh',
    });
  }

  addTag() {
    if (this.adding) return;

    const value = (this.newTag ?? '').trim();
    if (!value || this.tags().includes(value)) return;

    if (value.length > 50) {
      this.noti.errorMsg(this.translate.instant('crm.customer.tag_too_long'));
      return;
    }

    if (this.tags().length >= 10) {
      this.noti.errorMsg(this.translate.instant('crm.customer.tag_max_count'));
      return;
    }

    this.adding = true;
    this.modelService
      .addTag(this.model().id!, value)
      .pipe(
        finalize(() => (this.adding = false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (res: string[]) => {
          this.noti.successMsg(this.translate.instant('crm.customer.tag_attach_success'));
          this.setTag(res);
          this.newTag = '';
        },
        error: () => {
          this.noti.errorMsg(this.translate.instant('crm.customer.tag_attach_failed'));
        },
      });
  }

  removeTag(tag: string) {
    if (this.adding) return;

    this.adding = true;
    this.modelService
      .removeTag(this.model().id!, tag)
      .pipe(
        finalize(() => (this.adding = false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (res: string[]) => {
          this.noti.successMsg(this.translate.instant('crm.customer.tag_remove_success'));
          this.setTag(res);
        },
        error: () => {
          this.noti.errorMsg(this.translate.instant('crm.customer.tag_remove_failed'));
        },
      });
  }

  setTag(tags: string | string[]) {
    const parsed =
      typeof tags === 'string'
        ? (() => {
          try {
            return JSON.parse(tags);
          } catch {
            return [];
          }
        })()
        : tags || [];

    const uniq = [
      ...new Set(
        parsed
          .map((t: any) => String(t).trim())
          .filter((t: string) => t.length > 0 && t.length <= 50),
      ),
    ] as string[];

    this.tags.set(uniq);
  }

  /**
   * Hàm xử lý cập nhật thông tin khách hàng
   */
  updateCustomer(customer: CustomerModel): void {
    this.dialogRef = this.dialogWrapperService.open(CustomerFormV2, {
      width: '960px',
      header: this.translate.instant('common.action.update'),
      data: {customer},
    });

    this.dialogRef.onClose
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((res: any) => {
        if (res) {
          // preparePermissions chạy trước addLabel để giữ allowedPerms.
          const prepared = this.preparePermissions(res);
          const dataWithLabels = this.modelService.addLabel(prepared);
          this.model.set({...dataWithLabels} as any);
          this.afterModelLoaded(dataWithLabels as any);
        }
      });
  }

  /**
   * Hàm upload ảnh
   * @param event
   */
  onFileSelected(event: any) {
    const file: File = event.target.files[0];
    if (!file) return;

    if (file.size > CONFIG_FILE_SELECT.CONFIG_FILE_MAX_SIZE) {
      this.noti.errorMsg(this.translate.instant('crm.customer.file_too_large'));
      event.target.value = '';
      return;
    }

    if (!isAllowExtensions(file.name, CONFIG_FILE_SELECT.CONFIG_FILE_ALLOW_EXT, file.type)) {
      this.noti.errorMsg(this.translate.instant('common.label.allowed_formats') + CONFIG_FILE_SELECT.CONFIG_FILE_ALLOW_EXT.join(', '));
      event.target.value = '';
      return;
    }

    const formData = new FormData();
    formData.append('fileImage', file);

    this.modelService
      .updateAvatar(this.model().id!, formData)
      .pipe(
        finalize(() => (event.target.value = '')),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (res: any) => {
          this.handleAvatarUpdated(res);
          this.noti.successRes(res);
        },
      });
  }

  reloadCustomerInfo($event: boolean) {
    if (!$event) return;

    // Debounce reload to avoid multiple calls
    if (this.reloadTimeout) {
      clearTimeout(this.reloadTimeout);
    }

    this.reloadTimeout = setTimeout(() => {
      this.getModel(this.model().id!);
      this.reloadTimeout = null;
    }, 300);
  }

  ngOnDestroy() {
    if (this.reloadTimeout) {
      clearTimeout(this.reloadTimeout);
    }
    this.stream?.getTracks().forEach((t) => t.stop());
    this.stream = null;
    if (this.dialogRef) {
      this.dialogRef.close();
    }
    this.layoutService.menuModeOverride.set(null);
  }

  selectedAppointmentNotBookedReasons(event) {
    if (this.reasonUpdateInProgress || this.model().appointmentNotBookedReasons?.id === event) {
      return;
    }

    this.reasonUpdateInProgress = true;
    this.modelService
      .updateAppointmentNotBookedReasons(this.model().id!, event)
      .pipe(
        finalize(() => (this.reasonUpdateInProgress = false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: () => {
          this.noti.successMsg(this.translate.instant('crm.customer.no_appointment_reason_added'));
          this.reloadCustomerInfo(true);
        },
        error: () => {
          this.noti.errorMsg(this.translate.instant('crm.customer.no_appointment_reason_failed'));
        },
      });
  }

  async openCamera() {
    try {
      this.showCameraDialog = true;
      this.stream = await navigator.mediaDevices.getUserMedia({video: true});
      this.videoRef.nativeElement.srcObject = this.stream;
    } catch (error) {
      this.noti.errorMsg(this.translate.instant('crm.customer.camera_access_failed'));
      this.showCameraDialog = false;
    }
  }

  capture() {
    const video = this.videoRef.nativeElement;
    const canvas = this.canvasRef.nativeElement;
    const ctx = canvas.getContext('2d')!;
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    ctx.drawImage(video, 0, 0, canvas.width, canvas.height);

    this.capturedImage = canvas.toDataURL('image/jpeg');
  }

  uploadCaptured() {
    if (this.uploadingAvatar) return;

    const canvas = this.canvasRef.nativeElement;

    // toBlob() KHÔNG truyền type → mặc định encode PNG (nặng hơn JPEG cho ảnh chụp, lại lệch
    // với filename/type 'image/jpeg' gán bên dưới) — truyền tường minh, mirror `capture()`
    // phía trên đã dùng `toDataURL('image/jpeg')` đúng.
    canvas.toBlob(
      (blob) => {
        if (!blob) {
          this.noti.errorMsg(this.translate.instant('crm.customer.create_image_failed'));
          return;
        }

        const file = new File([blob], `captured-${Date.now()}.jpg`, {type: 'image/jpeg'});

        if (file.size > CONFIG_FILE_SELECT.CONFIG_FILE_MAX_SIZE) {
          this.noti.errorMsg(this.translate.instant('crm.customer.file_too_large'));
          return;
        }

        const formData = new FormData();
        formData.append('fileImage', file);

        this.uploadingAvatar = true;

        this.modelService
          .updateAvatar(this.model().id!, formData)
          .pipe(
            finalize(() => {
              this.uploadingAvatar = false;
              this.cleanupCamera();
            }),
            takeUntilDestroyed(this.destroyRef),
          )
          .subscribe({
            next: (res: any) => {
              this.handleAvatarUpdated(res);
              this.noti.successRes(res);
            },
          });
      },
      'image/jpeg',
      CONFIG_FILE_SELECT.CONFIG_CAMERA_JPEG_QUALITY,
    );
  }

  async retake() {
    this.capturedImage = null;

    setTimeout(async () => {
      if (this.videoRef?.nativeElement) {
        if (!this.stream || this.stream.getTracks().every((t) => t.readyState === 'ended')) {
          try {
            this.stream = await navigator.mediaDevices.getUserMedia({video: true});
            this.videoRef.nativeElement.srcObject = this.stream;
          } catch (error) {
            this.noti.errorMsg(this.translate.instant('crm.customer.camera_access_failed'));
          }
        } else {
          this.videoRef.nativeElement.srcObject = this.stream;
        }
      }
    }, 100);
  }

  cancelCaptured() {
    this.cleanupCamera();
  }

  onHide() {
    this.cleanupCamera();
  }

  preparePermissions(model: CustomerModel) {
    // Lead/Ticket đã gộp vào Customer (story-33 + story-22): chỉ còn 1 quyền updateCustomer.
    // Nút sửa + badge đổi bước gate thẳng bằng quyền ở template — không phụ thuộc tiến trình khách.
    const allowedPerms: string[] = [this.permission.permissionUpdateCustomer];

    return {
      ...model,
      allowedPerms,
    };
  }

  /**
   * BE đã xác nhận đổi bước: thay reference model (OnPush) với `stage_id` + `updated_at` mới —
   * `updated_at` PHẢI nhận về, không thì lần sửa form kế tiếp bị 409 (optlock). Timeline là dialog tải
   * lại mỗi lần mở nên tự thấy sự kiện mới.
   */
  protected onStageChanged(result: CustomerStageChangeResult): void {
    const current = this.model();
    const merged = applyStageChange(current, result);
    if (!merged || merged === current) return;
    this.model.set(this.modelService.addLabel(merged) as any);
  }

  getRelativeTime(timestamp: number | Date | undefined): string {
    if (!timestamp) return this.translate.instant('crm.customer.no_value');

    const date = typeof timestamp === 'number' ? new Date(timestamp * 1000) : new Date(timestamp);
    const now = new Date();

    // So sánh ngày thực tế, không phải milliseconds
    const dateOnly = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    const nowOnly = new Date(now.getFullYear(), now.getMonth(), now.getDate());

    const diffMs = nowOnly.getTime() - dateOnly.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffDays === 0) return this.translate.instant('common.label.today');
    if (diffDays === 1) return this.translate.instant('common.label.yesterday');
    if (diffDays < 30) return this.translate.instant('crm.customer.days_ago', {days: diffDays});
    if (diffDays < 365) return this.translate.instant('crm.customer.months_ago', {months: Math.floor(diffDays / 30)});
    return this.translate.instant('crm.customer.years_ago', {years: Math.floor(diffDays / 365)});
  }

  protected override afterModelLoaded(model: CustomerModel): void {
    this.title.setTitle(model.customerDisplayName || this.translate.instant('common.label.customer'));
    // Breadcrumb hiện tên khách thay nhãn tĩnh "Chi tiết khách hàng"; service tự xoá khi đổi path.
    this.breadcrumbLabelService.set(model.customerDisplayName);
    this.setTag(model.tags ?? []);
    this.lastContactRelative.set(this.getRelativeTime(model.last_contact_date));
    this.lastVisitRelative.set(this.getRelativeTime(model.last_visit_date));
  }

  private cleanupCamera() {
    this.showCameraDialog = false;
    this.stream?.getTracks().forEach((track) => track.stop());
    this.stream = null;
    this.capturedImage = null;
  }

  private handleAvatarUpdated(res: any) {
    const newAvatar = res?.data?.avatar || res?.avatar;

    if (newAvatar) {
      this.model.update((m) => ({ ...m, avatar: newAvatar }));
      this.avatarTs.set(Date.now());
      return;
    }

    // Fallback cho trường hợp API không trả avatar trong payload upload.
    this.getModel(this.model().id!);
  }
}
