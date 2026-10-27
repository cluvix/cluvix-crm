import { ChangeDetectionStrategy, Component, computed, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TableLazyLoadEvent } from 'primeng/table';
import { debounceTime, finalize, Subject, Subscription } from 'rxjs';

import { FbGroup, FbGroupListQuery, FbGroupStatsProvince, FbGroupTopic } from '@/core/models/fbgroup/fbgroup.model';
import { FbGroupService } from '@/core/services/fbgroup/fbgroup.service';
import { PERMISSION_SELECT } from '@/define';
import {
  FB_GROUP_BIG_MEMBERS,
  FB_GROUP_PRIVACY,
  FB_GROUP_PRIVACY_OPTIONS,
  FB_GROUP_WORK_STATUS,
  FB_GROUP_WORK_STATUS_OPTIONS,
} from '@/define/fbgroup.define';
import { FbGroupForm } from '@/modules/fbgroup/group-form/fbgroup-group-form';
import { FbGroupImportDialog } from '@/modules/fbgroup/import-dialog/fbgroup-import-dialog';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';
import { ListShareImports } from '@/share/import/list-share-imports';
import { PaginationInfo } from '@/share/models/list-api-response';
import { LocaleFormatService } from '@/share/services/locale-format.service';
import { translateOptions } from '@/share/utils/translate-options.util';

const PER_PAGE = 50;

/**
 * Danh sách group Facebook (/fb-group/groups). Bộ lọc gửi thẳng query param BE (topicId, workStatus…) —
 * không đi qua BaseListV2 vì BE module này không dùng cú pháp filter[...].
 */
@Component({
  selector: 'app-fbgroup-group-list',
  templateUrl: './fbgroup-group-list.html',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [...ListShareImports, ...FormShareImports],
})
export class FbGroupGroupList extends BaseApp implements OnInit {
  private readonly api = inject(FbGroupService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly localeFormat = inject(LocaleFormatService);
  private listSub?: Subscription;
  private readonly keyword$ = new Subject<string>();

  protected readonly perPage = PER_PAGE;
  protected readonly topics = signal<FbGroupTopic[]>([]);
  protected readonly provinces = signal<FbGroupStatsProvince[]>([]);
  protected readonly models = signal<FbGroup[]>([]);
  protected readonly pagination = signal<PaginationInfo | null>(null);
  protected readonly selected = signal<FbGroup[]>([]);
  protected readonly first = signal(0);

  // Bộ lọc
  protected readonly keyword = signal('');
  protected readonly topicId = signal<number | null>(null);
  protected readonly branchId = signal<number | null>(null);
  protected readonly workStatuses = signal<number[]>([FB_GROUP_WORK_STATUS.NEW, FB_GROUP_WORK_STATUS.TARGET, FB_GROUP_WORK_STATUS.WATCH]);
  protected readonly privacy = signal<number | null>(null);
  protected readonly minMembers = signal<number | null>(null);
  protected readonly province = signal<string | null>(null);
  protected readonly qualifiedOnly = signal(false);
  private sort = '-member_count';
  private page = 1;

  // Thao tác hàng loạt
  protected readonly bulkTopicId = signal<number | null>(null);

  protected readonly canUpdate = computed(() => this.rbacService.hasPermission(PERMISSION_SELECT.permissionUpdateFbGroup));
  protected readonly workStatusOptions = translateOptions(FB_GROUP_WORK_STATUS_OPTIONS);
  protected readonly privacyOptions = translateOptions(FB_GROUP_PRIVACY_OPTIONS);
  protected readonly PRIVACY = FB_GROUP_PRIVACY;
  protected readonly WORK = FB_GROUP_WORK_STATUS;
  protected readonly branchOptions = computed(() => this.topics().find((t) => t.id === this.topicId())?.branches ?? []);
  protected readonly maxMembers = computed(() => Math.max(FB_GROUP_BIG_MEMBERS, ...this.models().map((m) => m.member_count ?? 0)));

  ngOnInit() {
    this.api
      .listTopics()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((t) => this.topics.set(t));
    this.api
      .statsProvinces(null)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((p) => this.provinces.set(p));
    this.keyword$.pipe(debounceTime(400), takeUntilDestroyed(this.destroyRef)).subscribe((v) => {
      this.keyword.set(v);
      this.reload();
    });
    this.reload();
  }

  private query(): FbGroupListQuery {
    return {
      page: this.page,
      'per-page': PER_PAGE,
      sort: this.sort,
      _keyword: this.keyword().trim(),
      topicId: this.topicId(),
      branchId: this.topicId() ? this.branchId() : null,
      workStatus: this.workStatuses().join(','),
      privacy: this.privacy(),
      minMembers: this.minMembers(),
      province: this.province(),
      qualified: this.qualifiedOnly() ? '1' : '',
    };
  }

  private fetch() {
    this.listSub?.unsubscribe(); // huỷ request cũ khi đổi lọc nhanh
    this.loading.set(true);
    this.listSub = this.api
      .listGroups(this.query())
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loading.set(false)),
      )
      .subscribe((res) => {
        this.models.set(res.items ?? []);
        this.pagination.set(res._meta ?? null);
      });
  }

  /** Đổi bộ lọc ⇒ về trang 1, bỏ chọn. */
  protected reload() {
    this.page = 1;
    this.first.set(0);
    this.selected.set([]);
    this.fetch();
  }

  protected onLazyLoad(e: TableLazyLoadEvent) {
    const rows = e.rows ?? PER_PAGE;
    this.page = Math.floor((e.first ?? 0) / rows) + 1;
    this.first.set(e.first ?? 0);
    if (e.sortField) {
      const field = Array.isArray(e.sortField) ? e.sortField[0] : e.sortField;
      this.sort = (e.sortOrder === 1 ? '' : '-') + field;
    }
    this.fetch();
  }

  protected onKeyword(v: string) {
    this.keyword$.next(v);
  }

  protected setTopic(id: number | null) {
    this.topicId.set(id);
    this.branchId.set(null);
    this.reload();
  }

  protected setFilter<T>(sig: { set(v: T): void }, v: T) {
    sig.set(v);
    this.reload();
  }

  protected clearFilters() {
    this.keyword.set('');
    this.topicId.set(null);
    this.branchId.set(null);
    this.workStatuses.set([FB_GROUP_WORK_STATUS.NEW, FB_GROUP_WORK_STATUS.TARGET, FB_GROUP_WORK_STATUS.WATCH]);
    this.privacy.set(null);
    this.minMembers.set(null);
    this.province.set(null);
    this.qualifiedOnly.set(false);
    this.reload();
  }

  protected statusOption(v: number) {
    return FB_GROUP_WORK_STATUS_OPTIONS.find((o) => o.value === v) ?? FB_GROUP_WORK_STATUS_OPTIONS[0];
  }

  protected fmt(n: number | null | undefined): string {
    return n === null || n === undefined ? '' : new Intl.NumberFormat(this.localeFormat.currentLocale()).format(n);
  }

  protected barWidth(n: number | null): number {
    return Math.max(3, Math.round((56 * (n ?? 0)) / this.maxMembers()));
  }

  /** Đặt trạng thái / gán / bỏ chủ đề cho các group đang chọn. */
  protected bulk(action: { work_status?: number; add_topic_id?: number; remove_topic_id?: number }) {
    const ids = this.selected().map((g) => g.id);
    if (!ids.length) return;
    this.isSubmitting.set(true);
    this.api
      .bulkGroups({ ids, ...action })
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.isSubmitting.set(false)),
      )
      .subscribe((res) => {
        this.noti.successRes(res);
        this.selected.set([]);
        this.fetch();
      });
  }

  protected bulkTopic(mode: 'add' | 'remove') {
    const id = this.bulkTopicId();
    if (!id) return;
    this.bulk(mode === 'add' ? { add_topic_id: id } : { remove_topic_id: id });
  }

  protected openGroup(g: FbGroup) {
    const ref = this.dialogWrapperService.open(FbGroupForm, {
      header: g.name,
      width: '640px',
      data: { id: g.id, canUpdate: this.canUpdate() },
    });
    ref.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((changed) => {
      if (changed) this.fetch();
    });
  }

  protected exportExcel() {
    this.isSubmitting.set(true);
    this.api
      .exportGroups(this.query())
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.isSubmitting.set(false)),
      )
      .subscribe((url) => this.downloadExport(url));
  }

  protected openImport() {
    const ref = this.dialogWrapperService.open(FbGroupImportDialog, {
      header: this.translate.instant('fbgroup.import.title'),
      width: '560px',
    });
    ref.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((changed) => {
      if (changed) this.reload();
    });
  }
}
