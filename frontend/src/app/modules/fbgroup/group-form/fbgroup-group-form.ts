import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { DynamicDialogConfig } from 'primeng/dynamicdialog';
import { finalize } from 'rxjs';

import { FbGroupDetail } from '@/core/models/fbgroup/fbgroup.model';
import { FbGroupService } from '@/core/services/fbgroup/fbgroup.service';
import { FB_GROUP_PRIVACY, FB_GROUP_WORK_STATUS_OPTIONS } from '@/define/fbgroup.define';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';
import { LocaleFormatService } from '@/share/services/locale-format.service';
import { translateOptions } from '@/share/utils/translate-options.util';

const NOTE_MAX = 5000;

/** Chi tiết 1 group + sửa trạng thái làm việc / ghi chú. Đóng với `true` khi đã lưu. */
@Component({
  selector: 'app-fbgroup-group-form',
  templateUrl: './fbgroup-group-form.html',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [...FormShareImports],
})
export class FbGroupForm extends BaseApp implements OnInit {
  private readonly api = inject(FbGroupService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly config = inject(DynamicDialogConfig);
  private readonly localeFormat = inject(LocaleFormatService);

  protected readonly noteMax = NOTE_MAX;
  protected readonly PRIVACY = FB_GROUP_PRIVACY;
  protected readonly canUpdate: boolean = !!this.config.data?.canUpdate;
  protected readonly detail = signal<FbGroupDetail | null>(null);
  protected readonly workStatus = signal<number>(0);
  protected readonly note = signal('');
  protected readonly workStatusOptions = translateOptions(FB_GROUP_WORK_STATUS_OPTIONS);

  ngOnInit() {
    const id = Number(this.config.data?.id);
    this.loading.set(true);
    this.api
      .getGroup(id)
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loading.set(false)),
      )
      .subscribe((d) => {
        this.detail.set(d);
        this.workStatus.set(d.work_status);
        this.note.set(d.note ?? '');
      });
  }

  protected fmt(n: number | null | undefined): string {
    return n === null || n === undefined ? '—' : new Intl.NumberFormat(this.localeFormat.currentLocale()).format(n);
  }

  protected cancel() {
    this.dialogRef.close(false);
  }

  protected onSubmit() {
    const d = this.detail();
    if (!d || !this.canUpdate) return;
    this.isSubmitting.set(true);
    this.api
      .updateGroup(d.id, { work_status: this.workStatus(), note: this.note() })
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.isSubmitting.set(false)),
      )
      .subscribe((res) => {
        this.noti.successRes(res);
        this.dialogRef.close(true);
      });
  }
}
