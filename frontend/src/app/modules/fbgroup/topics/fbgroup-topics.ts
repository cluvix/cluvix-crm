import { ChangeDetectionStrategy, Component, computed, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs';

import { FbGroupIngestKeyInfo, FbGroupTopic } from '@/core/models/fbgroup/fbgroup.model';
import { FbGroupService } from '@/core/services/fbgroup/fbgroup.service';
import { PERMISSION_SELECT } from '@/define';
import { FbGroupTopicForm } from '@/modules/fbgroup/topic-form/fbgroup-topic-form';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';
import { LocaleFormatService } from '@/share/services/locale-format.service';
import { environment } from '@env/environment';

/**
 * Chủ đề group Facebook (/fb-group/topics): danh sách chủ đề, thêm/sửa/xoá, phân loại lại toàn kho
 * và khối "Kết nối NextCore" (khoá API để NextCore đẩy group vào CRM).
 */
@Component({
  selector: 'app-fbgroup-topics',
  templateUrl: './fbgroup-topics.html',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [...FormShareImports],
})
export class FbGroupTopics extends BaseApp implements OnInit {
  private readonly api = inject(FbGroupService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly localeFormat = inject(LocaleFormatService);

  protected readonly topics = signal<FbGroupTopic[]>([]);
  protected readonly keyInfo = signal<FbGroupIngestKeyInfo | null>(null);
  /** Khoá vừa tạo — chỉ giữ trong bộ nhớ trang này, BE không trả lại lần hai. */
  protected readonly newKey = signal<string | null>(null);
  protected readonly reclassifying = signal(false);
  protected readonly canManage = computed(() => this.rbacService.hasPermission(PERMISSION_SELECT.permissionManageFbGroupTopic));

  /** URL đầy đủ NextCore cần gọi — origin của API CRM + đường BE trả về. */
  protected readonly ingestUrl = computed(() => {
    const path = this.keyInfo()?.ingest_path ?? '/api/fbgroup/ingest';
    try {
      return new URL(environment.apiUrl, window.location.origin).origin + path;
    } catch {
      return window.location.origin + path;
    }
  });

  ngOnInit() {
    this.loadTopics();
    if (this.canManage()) {
      this.api
        .getIngestKey()
        .pipe(takeUntilDestroyed(this.destroyRef))
        .subscribe((k) => this.keyInfo.set(k));
    }
  }

  private loadTopics() {
    this.loading.set(true);
    this.api
      .listTopics()
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loading.set(false)),
      )
      .subscribe((t) => this.topics.set(t));
  }

  protected fmt(n: number | null | undefined): string {
    return new Intl.NumberFormat(this.localeFormat.currentLocale()).format(n ?? 0);
  }

  protected openForm(topic: FbGroupTopic | null) {
    const ref = this.dialogWrapperService.open(FbGroupTopicForm, {
      header: this.translate.instant(topic ? 'fbgroup.topic.edit_title' : 'fbgroup.topic.create_title'),
      width: '720px',
      data: { topic },
    });
    ref.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((saved) => {
      if (saved) this.loadTopics();
    });
  }

  protected deleteTopic(event: Event, topic: FbGroupTopic) {
    this.confirmationService.confirm({
      target: event.currentTarget as EventTarget,
      message: this.translate.instant('fbgroup.topic.confirm_delete', { name: topic.name }),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: { severity: 'danger', size: 'small' },
      rejectButtonProps: { severity: 'secondary', outlined: true, size: 'small' },
      accept: () => {
        this.api
          .deleteTopic(topic.id)
          .pipe(takeUntilDestroyed(this.destroyRef))
          .subscribe((res) => {
            this.noti.successRes(res);
            this.loadTopics();
          });
      },
    });
  }

  protected reclassify(event: Event) {
    this.confirmationService.confirm({
      target: event.currentTarget as EventTarget,
      message: this.translate.instant('fbgroup.topic.confirm_reclassify'),
      icon: 'pi pi-refresh',
      acceptButtonProps: { size: 'small' },
      rejectButtonProps: { severity: 'secondary', outlined: true, size: 'small' },
      accept: () => {
        this.reclassifying.set(true);
        this.api
          .reclassify()
          .pipe(
            takeUntilDestroyed(this.destroyRef),
            finalize(() => this.reclassifying.set(false)),
          )
          .subscribe((res) => {
            this.noti.successRes(res);
            this.loadTopics();
          });
      },
    });
  }

  protected createKey(event: Event) {
    const run = () =>
      this.api
        .createIngestKey()
        .pipe(takeUntilDestroyed(this.destroyRef))
        .subscribe((res) => {
          this.newKey.set(res.data.key);
          this.noti.successRes(res);
          this.api
            .getIngestKey()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe((k) => this.keyInfo.set(k));
        });
    if (!this.keyInfo()?.has_key) {
      run();
      return;
    }
    // Đã có khoá ⇒ tạo mới sẽ thu hồi khoá NextCore đang dùng: hỏi trước.
    this.confirmationService.confirm({
      target: event.currentTarget as EventTarget,
      message: this.translate.instant('fbgroup.connect.confirm_rotate'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: { severity: 'danger', size: 'small' },
      rejectButtonProps: { severity: 'secondary', outlined: true, size: 'small' },
      accept: run,
    });
  }

  protected copy(text: string) {
    navigator.clipboard
      ?.writeText(text)
      .then(() => this.noti.successMsg(this.translate.instant('fbgroup.connect.copied')))
      .catch(() => this.noti.errorMsg(this.translate.instant('fbgroup.connect.copy_failed')));
  }
}
