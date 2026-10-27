import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs';

import { FbGroupUpsertResult } from '@/core/models/fbgroup/fbgroup.model';
import { FbGroupService } from '@/core/services/fbgroup/fbgroup.service';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';

/** Nhập tệp "Xuất CSV" của NextCore GROUPCRAWL. Đóng với `true` nếu đã nhập. */
@Component({
  selector: 'app-fbgroup-import-dialog',
  templateUrl: './fbgroup-import-dialog.html',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [...FormShareImports],
})
export class FbGroupImportDialog extends BaseApp {
  private readonly api = inject(FbGroupService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly file = signal<File | null>(null);
  protected readonly result = signal<FbGroupUpsertResult | null>(null);

  protected onFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    this.file.set(input.files?.[0] ?? null);
  }

  protected cancel() {
    this.dialogRef.close(this.result() !== null);
  }

  protected onSubmit() {
    const f = this.file();
    if (!f) {
      this.noti.warnMsg(this.translate.instant('fbgroup.import.no_file'));
      return;
    }
    this.isSubmitting.set(true);
    this.api
      .importCsv(f)
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.isSubmitting.set(false)),
      )
      .subscribe((res) => {
        this.result.set(res.data);
        this.noti.successRes(res);
      });
  }
}
