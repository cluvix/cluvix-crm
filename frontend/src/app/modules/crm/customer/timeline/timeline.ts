import { TranslatePipe } from '@ngx-translate/core';
import {ChangeDetectionStrategy, Component, computed, effect, inject, input, OnInit, signal } from '@angular/core';
import { TimelineModule } from 'primeng/timeline';

import { CustomerService } from '@/core/services/crm/customer.service';
import { TimezoneService } from '@/core/services/timezone.service';
import { CustomerStageHelperService } from '@/indexeddb/config/components/customer-stage';
import { DetailShareImports } from '@/share/import/detail-share-imports';
import { GlobalLoadingService } from '@/share/services/global-loading.service';
import { toStageChangeView } from './stage-change.util';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-contact-timeline',
  templateUrl: './timeline.html',
  styleUrl: 'timeline.scss',
  standalone: true,
  imports: [...DetailShareImports, TimelineModule, TranslatePipe],
})
export class Timeline implements OnInit {
  customerId = input.required<number>();
  align = input<'left' | 'right' | 'alternate' | 'top' | 'bottom'>('alternate');
  private customerService = inject(CustomerService);
  private loadingService = inject(GlobalLoadingService);
  private timelineData = signal<any>(null);
  private timezoneService = inject(TimezoneService);
  /** Tra tên/màu bước cho sự kiện `stage_change` (đọc signal ⇒ tự cập nhật khi cache sync). */
  protected stageHelper = inject(CustomerStageHelperService);

  events = computed(() => {
    const data = this.timelineData();
    if (!data?.timeline) return [];

    return data.timeline.map((item: any) => ({
      ...item,
      iconDirection: item.iconDirection || 'pi pi-circle',
      stageView: toStageChangeView(item),
    }));
  });

  customer = computed(() => this.timelineData()?.customer || null);

  async ngOnInit() {
    // F5 xong store `customerStages` chưa nạp vào bộ nhớ — không nạp thì tên bước hiện '-'.
    await this.stageHelper.ensureLoaded();
  }

  /** Giờ đổi bước theo múi giờ công ty (TimezoneService) — dd/MM/yyyy HH:mm. */
  protected stageChangeTime(unix: number): string {
    return this.timezoneService.format(unix, {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    });
  }

  constructor() {
    effect(() => {
      const id = this.customerId();
      if (id) {
        this.loadingService.showLoading('timeline');
        this.customerService.getTimeLine(id).subscribe({
          next: (res) => {
            this.timelineData.set(res);
            this.loadingService.hideLoading('timeline');
          },
          error: () => {
            this.loadingService.hideLoading('timeline');
          },
        });
      }
    });
  }
}
