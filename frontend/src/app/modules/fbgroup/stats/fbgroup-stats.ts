import { ChangeDetectionStrategy, Component, computed, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { finalize, forkJoin } from 'rxjs';

import {
  FbGroupStatsBranch,
  FbGroupStatsKeyword,
  FbGroupStatsOverview,
  FbGroupStatsProvince,
  FbGroupTopic,
} from '@/core/models/fbgroup/fbgroup.model';
import { FbGroupService } from '@/core/services/fbgroup/fbgroup.service';
import { FB_GROUP_WORK_STATUS_OPTIONS } from '@/define/fbgroup.define';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';
import { LocaleFormatService } from '@/share/services/locale-format.service';

/** Màu thanh trạng thái làm việc — semantic color (ui.md), khớp severity của tag. */
const WORK_STATUS_COLORS: Record<number, string> = { 0: '#9CA3AF', 1: '#10B981', 2: '#3B82F6', 3: '#E53935' };

/**
 * Thống kê group Facebook (/fb-group/stats). Biểu đồ vẽ bằng thanh HTML theo token (tự đúng sáng/tối,
 * screen reader đọc được) — đi kèm số trong bảng theo rule chart (ui.md).
 */
@Component({
  selector: 'app-fbgroup-stats',
  templateUrl: './fbgroup-stats.html',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [...FormShareImports, RouterLink],
})
export class FbGroupStats extends BaseApp implements OnInit {
  private readonly api = inject(FbGroupService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly localeFormat = inject(LocaleFormatService);

  protected readonly topics = signal<FbGroupTopic[]>([]);
  protected readonly topicId = signal<number | null>(null);
  protected readonly overview = signal<FbGroupStatsOverview | null>(null);
  protected readonly branches = signal<FbGroupStatsBranch[]>([]);
  protected readonly provinces = signal<FbGroupStatsProvince[]>([]);
  protected readonly keywords = signal<FbGroupStatsKeyword[]>([]);

  protected readonly topicBars = computed(() => {
    const rows = this.overview()?.by_topic ?? [];
    const max = Math.max(1, ...rows.map((r) => r.members));
    return rows.map((r) => ({ ...r, pct: (100 * r.members) / max, dim: this.topicId() !== null && this.topicId() !== r.topic_id }));
  });

  protected readonly statusBars = computed(() => {
    const rows = this.overview()?.by_work_status ?? [];
    const total = rows.reduce((s, r) => s + r.groups, 0) || 1;
    return FB_GROUP_WORK_STATUS_OPTIONS.map((o) => {
      const groups = rows.find((r) => r.work_status === o.value)?.groups ?? 0;
      return { ...o, groups, pct: (100 * groups) / total, color: WORK_STATUS_COLORS[o.value] };
    });
  });

  protected readonly provinceBars = computed(() => {
    const rows = this.provinces().slice(0, 10);
    const max = Math.max(1, ...rows.map((r) => r.members));
    return rows.map((r) => ({ ...r, pct: (100 * r.members) / max }));
  });

  ngOnInit() {
    this.api
      .listTopics()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((t) => this.topics.set(t));
    this.api
      .statsKeywords()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((k) => this.keywords.set(k));
    this.load();
  }

  protected pickTopic(id: number | null) {
    if (this.topicId() === id) return;
    this.topicId.set(id);
    this.load();
  }

  private load() {
    const id = this.topicId();
    this.loading.set(true);
    forkJoin({
      overview: this.api.statsOverview(id),
      branches: this.api.statsBranches(id),
      provinces: this.api.statsProvinces(id),
    })
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loading.set(false)),
      )
      .subscribe((r) => {
        this.overview.set(r.overview);
        this.branches.set(r.branches);
        this.provinces.set(r.provinces);
      });
  }

  /** Số nguyên theo locale thương hiệu (rule B.19 — không tự ghép chuỗi). */
  protected fmt(n: number | null | undefined): string {
    return new Intl.NumberFormat(this.localeFormat.currentLocale()).format(n ?? 0);
  }

  /** Số lớn gọn (63,8 Tr) cho thẻ KPI và nhãn biểu đồ. */
  protected compact(n: number | null | undefined): string {
    return new Intl.NumberFormat(this.localeFormat.currentLocale(), { notation: 'compact', maximumFractionDigits: 1 }).format(
      n ?? 0,
    );
  }

  /** Đánh giá từ khoá: TB thành viên thấp hoặc lạc đề nhiều ⇒ nên bỏ ở lượt quét sau. */
  protected keywordVerdict(k: FbGroupStatsKeyword): { labelKey: string; severity: 'success' | 'warn' | 'danger' } {
    const offRatio = k.on_topic + k.off_topic > 0 ? k.off_topic / (k.on_topic + k.off_topic) : 0;
    if (k.avg_members < 2000 || offRatio > 0.3) return { labelKey: 'fbgroup.stats.verdict_drop', severity: 'danger' };
    if (k.avg_members < 10000) return { labelKey: 'fbgroup.stats.verdict_review', severity: 'warn' };
    return { labelKey: 'fbgroup.stats.verdict_keep', severity: 'success' };
  }
}
