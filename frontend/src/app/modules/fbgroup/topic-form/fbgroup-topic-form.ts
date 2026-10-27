import { ChangeDetectionStrategy, Component, computed, DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { DynamicDialogConfig } from 'primeng/dynamicdialog';
import { finalize } from 'rxjs';

import { FbGroupPreview, FbGroupTopic, FbGroupTopicInput } from '@/core/models/fbgroup/fbgroup.model';
import { FbGroupService } from '@/core/services/fbgroup/fbgroup.service';
import { FB_GROUP_TOPIC_COLORS } from '@/define/fbgroup.define';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';

/** Nhánh đang sửa — `key` chỉ để track trong @for (nhánh mới chưa có id). */
interface BranchDraft {
  key: number;
  id: number;
  name: string;
  keywords: string;
}

/** Tách chuỗi "a, b; c" / xuống dòng thành mảng từ khoá, bỏ rỗng. */
function splitKeywords(s: string): string[] {
  return s
    .split(/[,;\n]/)
    .map((k) => k.trim())
    .filter((k) => k.length > 0);
}

/**
 * Thêm/sửa chủ đề: tên, màu, từ khoá gồm/loại trừ, nhánh, ngưỡng mục tiêu, xem trước số group khớp.
 * Lưu xong BE phân loại lại toàn kho. Đóng với chủ đề đã lưu (hoặc undefined khi huỷ).
 */
@Component({
  selector: 'app-fbgroup-topic-form',
  templateUrl: './fbgroup-topic-form.html',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [...FormShareImports],
})
export class FbGroupTopicForm extends BaseApp {
  private readonly api = inject(FbGroupService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly config = inject(DynamicDialogConfig);
  private readonly original: FbGroupTopic | null = this.config.data?.topic ?? null;
  private nextKey = 1;

  protected readonly colors = FB_GROUP_TOPIC_COLORS;
  protected readonly name = signal(this.original?.name ?? '');
  protected readonly color = signal(this.original?.color ?? FB_GROUP_TOPIC_COLORS[0]);
  protected readonly include = signal((this.original?.include_keywords ?? []).join(', '));
  protected readonly exclude = signal((this.original?.exclude_keywords ?? []).join(', '));
  protected readonly minMembers = signal<number>(this.original?.min_members ?? 10000);
  protected readonly minPosts = signal<number>(this.original?.min_posts_per_day ?? 10);
  protected readonly publicOnly = signal<boolean>(this.original?.public_only ?? true);
  protected readonly active = signal<boolean>((this.original?.status ?? 1) === 1);
  protected readonly branches = signal<BranchDraft[]>(
    (this.original?.branches ?? []).map((b) => ({ key: this.nextKey++, id: b.id, name: b.name, keywords: b.keywords.join(', ') })),
  );
  protected readonly preview = signal<FbGroupPreview | null>(null);
  protected readonly previewing = signal(false);
  protected readonly includeList = computed(() => splitKeywords(this.include()));
  protected readonly valid = computed(() => this.name().trim().length > 0 && this.includeList().length > 0);

  protected addBranch() {
    this.branches.update((list) => [...list, { key: this.nextKey++, id: 0, name: '', keywords: '' }]);
  }

  protected removeBranch(key: number) {
    this.branches.update((list) => list.filter((b) => b.key !== key));
  }

  protected moveBranch(key: number, dir: -1 | 1) {
    this.branches.update((list) => {
      const i = list.findIndex((b) => b.key === key);
      const j = i + dir;
      if (i < 0 || j < 0 || j >= list.length) return list;
      const copy = [...list];
      [copy[i], copy[j]] = [copy[j], copy[i]];
      return copy;
    });
  }

  /** Sửa 1 trường của 1 nhánh — thay object mới (OnPush, không mutate). */
  protected patchBranch(key: number, field: 'name' | 'keywords', value: string) {
    this.branches.update((list) => list.map((b) => (b.key === key ? { ...b, [field]: value } : b)));
  }

  protected runPreview() {
    this.previewing.set(true);
    this.api
      .previewTopic(this.includeList(), splitKeywords(this.exclude()))
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.previewing.set(false)),
      )
      .subscribe((p) => this.preview.set(p));
  }

  protected cancel() {
    this.dialogRef.close();
  }

  protected onSubmit() {
    if (!this.valid()) return;
    const body: FbGroupTopicInput = {
      name: this.name().trim(),
      color: this.color(),
      include_keywords: this.includeList(),
      exclude_keywords: splitKeywords(this.exclude()),
      min_members: this.minMembers() ?? 0,
      min_posts_per_day: this.minPosts() ?? 0,
      public_only: this.publicOnly(),
      sort_order: this.original?.sort_order ?? 0,
      status: this.active() ? 1 : 0,
      branches: this.branches()
        .filter((b) => b.name.trim().length > 0)
        .map((b, i) => ({ id: b.id, name: b.name.trim(), keywords: splitKeywords(b.keywords), sort_order: i + 1 })),
    };
    this.isSubmitting.set(true);
    const call = this.original ? this.api.updateTopic(this.original.id, body) : this.api.createTopic(body);
    call
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.isSubmitting.set(false)),
      )
      .subscribe((res) => {
        this.noti.successRes(res);
        this.dialogRef.close(res.data);
      });
  }
}
