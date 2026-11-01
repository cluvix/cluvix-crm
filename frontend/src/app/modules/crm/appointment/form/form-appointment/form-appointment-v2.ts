import { ChangeDetectionStrategy, Component, inject, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormField, required, SchemaPathTree } from '@angular/forms/signals';
import { catchError, finalize, of, switchMap } from 'rxjs';

import { AppointmentDuplicate } from '@/modules/crm/appointment/form/appointment-duplicate-dialog/appointment-duplicate';
import { AppointmentModel } from '@/core/models/crm/appointment.model';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { AppointmentService } from '@/core/services/crm/appointment.service';
import { branchActiveId } from '@/core/services/auth';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { FormShareImports } from '@/share/import/form-share-imports';
import { APPOINTMENT_TYPE_SELECT } from '@/define';
import { NoteTemplateHelperService } from '@/indexeddb/config/components/note-template';
import { ServicesGroupedAutocompleteFormControl } from '@/indexeddb/config/components/services';

/**
 * Signal Forms cần KEY tồn tại thật + giá trị KHÔNG `undefined` trên object model để dựng sub-field
 * (`rootField.x` trả `undefined`, không phải FieldTree, nếu giá trị hiện tại của field đó là `undefined`
 * — kể cả khi object CÓ key đó). Seed toàn bộ field `buildForm()` gốc (dòng 259-282) bằng `null`/`[]`,
 * KHÔNG field nào `undefined` — xem gotcha #1 `skill/frontend/angular22.md`.
 * `schedule_time`/`convert_id`/`bookType` đã bỏ khỏi form (2026-11-01) ⇒ không seed: tạo mới để NULL,
 * sửa thì giá trị cũ đi theo model gốc (BE chỉ ghi field khác nil) nên không bị xoá.
 */
function emptyAppointmentModel(): AppointmentModel {
  return {
    content: null, contentPrivate: null, service_id: null,
    customer_id: null, appointment_type_id: null, branch_id: null, appointment_date: null,
    type_service_id: null, source_id: null,
    revenue_id: null, customer_data: [], updated_at: null,
  } as unknown as AppointmentModel;
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-appointment-appointment-form-v2',
  templateUrl: './form-appointment-v2.html',
  imports: [...FormShareImports, FormField, ServicesGroupedAutocompleteFormControl],
  standalone: true,
})
export class FormAppointmentV2 extends BaseSignalForm<AppointmentModel> {
  modelService = inject(AppointmentService);
  private noteTemplateHelper = inject(NoteTemplateHelperService);

  appointment!: AppointmentModel;
  customer!: CustomerModel;
  customerServicesData = signal<any[]>([]);

  constructor() {
    super();
    this.appointment = this.dialogConfig?.data?.appointment;
    this.customer = this.dialogConfig?.data?.customer; // Dùng cho tạo mới customer
    if (this.customer == null) {
      this.customer = this.appointment?.customer ?? {};
    }
    const serviceTreatments = this.dialogConfig?.data?.serviceTreatments || [];
    this.customerServicesData.set(serviceTreatments);

    // Seed model NGAY trong constructor (khác ServiceForm seed ở ngOnInit) — mọi input cần thiết
    // (dialogConfig, branchActiveId()) đã sẵn có ở đây, không cần chờ bất đồng bộ nào. `trackScopeFields()`
    // giờ tra field theo tên bên trong closure `effect()` (lazy, xem base-signal-form.ts) nên không còn
    // bắt buộc seed sớm vì lý do đó — giữ seed ở constructor vì vẫn hợp lý (không có input bất đồng bộ
    // nào cần chờ, seed sớm giúp field hiện đúng ngay khung hình đầu).
    if (this.appointment) {
      this.model.set({ ...emptyAppointmentModel(), ...this.appointment });
    } else {
      // Record<string, any> (không phải Partial<AppointmentModel>) — nhiều field model khai `number`
      // (không `number | null`) nên gán `null` (đúng ngữ nghĩa "rỗng" cho gotcha #1) bị TS chặn nếu ép
      // kiểu chặt; cùng cách emptyAppointmentModel() dùng `as unknown as AppointmentModel` ở cuối.
      const seeded: Record<string, any> = {
        ...emptyAppointmentModel(),
        ...this.initialValue,
        branch_id: branchActiveId(),
        appointment_date: this.today,
        customer_id: this.customer?.id ?? null,
        source_id: this.customer?.source_id ?? null,
        revenue_id: this.customer?.payment_user_id ?? null,
        type_service_id: this.customer?.type_service_id ?? null,
      };
      this.model.set(seeded as AppointmentModel);
    }

    this.trackScopeFields('branch_id');
  }

  override ngOnInit() {
    // Không gọi super.ngOnInit() — model đã seed đủ trong constructor (đối chiếu ghi chú ở BaseSignalForm
    // + gotcha #8 `angular22.md`: form override ngOnInit() tự chịu trách nhiệm seed model đủ field).

    if (!this.appointment) {
      // Load note template nền — không block render (chỉ create mode, đối chiếu dòng 87-94 gốc).
      this.noteTemplateHelper
        .getNoteTemplateByTypeIdNewest(APPOINTMENT_TYPE_SELECT.APPOINTMENT_TYPE_TU_VAN)
        .then((data) => {
          const content = data?.content;
          if (content) {
            untracked(() => this.rootField.content!().value.set(content));
          }
        });
    }

  }

  override onSubmit(): void {
    if (this.isSubmitting()) return;
    this.submitted.set(true);
    this.rootField().markAsTouched();

    if (this.rootField().invalid()) return;

    this.isSubmitting.set(true);

    // `content`/`contentPrivate` bind `p-editor` — chặn base64 nhúng lọt xuống DB, upload S3 trước.
    // Field được ghi ngược vào rootField ngay trong uploadRichTextImages$() nên mọi lượt getPayload()
    // phía sau (kể cả trong openDuplicateDialog() lúc user xác nhận trùng lịch) đều đọc URL đã upload.
    this.submitWithRichTextImages(['content', 'contentPrivate'], () => this.submitAppointment());
  }

  private submitAppointment(): void {
    // Extract service_id from object if needed (dòng 133-136 gốc), áp dụng trên getPayload().
    const formValue: any = { ...this.getPayload() };
    if (formValue.service_id && typeof formValue.service_id === 'object') {
      formValue.service_id = formValue.service_id.value;
    }

    if (this.isEditMode()) {
      this.modelService
        .updateAppointment(this.model().id!, formValue, {})
        .pipe(
          finalize(() => this.isSubmitting.set(false)),
          catchError(() => of(null)),
        )
        .subscribe({
          next: (res) => {
            if (res) {
              this.noti.successRes(res);
              this.dialogRef.close(res.data || res);
            }
          },
        });
    } else {
      this.modelService
        .getNextAppointmentsByCustomer(this.customer?.id ?? 0)
        .pipe(
          switchMap((appointment) => {
            if (appointment) {
              this.openDuplicateDialog(appointment, false, true);
              return of(null);
            }
            return this.modelService.createAppointment(formValue, {});
          }),
          finalize(() => this.isSubmitting.set(false)),
          catchError(() => of(null)),
        )
        .subscribe({
          next: (res) => {
            if (res) {
              this.noti.successRes(res);
              this.dialogRef.close(res.data);
            }
          },
        });
    }
  }

  /**
   * Show popup này nếu như book lịch hẹn bị trùng
   * @param appointment
   * @param isDuplicate
   * @param isCreate
   */
  openDuplicateDialog(
    appointment: AppointmentModel,
    isDuplicate: boolean = false,
    isCreate: boolean = false,
  ) {
    const currentValue: any = this.getPayload();
    const dialogRef = this.dialogWrapperService.open(AppointmentDuplicate, {
      width: '700px',
      autoZIndex: true,
      showHeader: true,
      header: this.translate.instant('common.label.possible_duplicate_appointment'),
      closeOnEscape: false,
      data: {
        appointment: {
          ...appointment,
          branchLabel: appointment.branchLabel || this.modelService.branchHelper.getBranchName(appointment.branch_id),
        },
        appointmentCreating: {
          ...currentValue,
          branchLabel: this.modelService.branchHelper.getBranchName(currentValue.branch_id),
        },
        isDuplicate: isDuplicate,
        isCreate: isCreate,
      },
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((confirmDuplicate: any) => {
      switch (confirmDuplicate) {
        case 'save':
          this.modelService
            .createAppointment(this.getPayload(), {})
            .pipe(
              takeUntilDestroyed(this.destroyRef),
              finalize(() => this.isSubmitting.set(false)),
              catchError(() => of(null)),
            )
            .subscribe({
              next: (res: any) => {
                if (res) {
                  this.noti.successRes(res);
                  this.dialogRef.close(res.data);
                }
              },
            });
          break;
        case 'update':
          untracked(() => this.rootField.updated_at!().value.set((appointment.updated_at ?? null) as any));
          this.modelService
            .updateAppointment(appointment.id!, this.getPayload(), {})
            .pipe(
              takeUntilDestroyed(this.destroyRef),
              finalize(() => this.isSubmitting.set(false)),
              catchError(() => of(null)),
            )
            .subscribe({
              next: (res) => {
                if (res) {
                  this.noti.successRes(res);
                  this.dialogRef.close(res.data);
                }
              },
            });
          break;
        default:
          break;
      }
    });
  }

  protected override buildSchema(p: SchemaPathTree<AppointmentModel>): void {
    required(p.customer_id!);
    required(p.appointment_type_id!);
    required(p.branch_id!);
    required(p.appointment_date!);
  }
}
