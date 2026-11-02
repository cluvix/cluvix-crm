import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { TranslatePipe } from '@ngx-translate/core';

import { CrmDashboardService } from '@/core/services/crm/crm-dashboard.service';
import { FunnelChartDirective } from '@/share/directive/echart/chart/funnelChart.directive';
import { LocaleFormatService } from '@/share/services/locale-format.service';

import { injectChartTheme } from '../shell/chart-theme';
import { createWidgetData, injectWidgetContext } from '../shell/widget-data';
import { isEmptyRows } from '../shell/widget-empty';
import { DashboardWidgetFrame } from '../shell/widget-frame';
import { widgetChartHeight } from '../shell/dashboard-widgets';
import { buildCustomerStageFunnelView, CUSTOMER_STAGE_FUNNEL_MIN_SIZE, type CustomerStageFunnelRow } from './customer-stage-funnel.util';

/**
 * Biểu đồ khối `customer_status_overview` (key giữ nguyên để layout đã lưu của người dùng vẫn hiện khối): phễu
 * theo BƯỚC đã cấu hình (customer_stage) — tầng = bước loại đang mở/thành công theo thứ tự, màu theo khoá màu bước;
 * bước loại thất bại tách riêng thành danh sách dưới phễu. Dựng lại khi đổi theme.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-status-chart-overview',
  imports: [DashboardWidgetFrame, FunnelChartDirective, TranslatePipe],
  template: `
    <app-dashboard-widget-frame
      titleKey="dashboard.crm.widgets.customer_status_overview.title"
      infoKey="dashboard.crm.widgets.customer_status_overview.desc"
      [status]="data.status()" skeleton="chart" [bodyHeight]="chartHeight"
      [canHide]="ctx?.canHide() ?? false" (hide)="ctx?.hide()"
      (retry)="data.retry()" (refresh)="data.refresh()">
      @if (view(); as v) {
        <div class="flex flex-col gap-3 w-full" [style.height.px]="chartHeight">
          @if (v.tiers.length) {
            @for (t of [theme()]; track t) {
              <div appFunnelChart [data]="v.tiers" [theme]="t" [minSize]="funnelMinSize" class="w-full min-h-0 flex-1"></div>
            }
          }
          @if (v.lost.length) {
            <div class="shrink-0 border-t border-[var(--p-content-border-color)] pt-2">
              <h4 class="m-0 mb-1 text-sm font-semibold text-[var(--p-text-muted-color)]">
                {{ 'config.customer_stage.type.lost' | translate }}
              </h4>
              <ul class="m-0 p-0 list-none flex flex-col gap-1">
                @for (l of v.lost; track l.stageId) {
                  <li class="flex items-center gap-2 text-sm">
                    <span class="inline-block w-2.5 h-2.5 rounded-full shrink-0" [class]="l.dotClass" aria-hidden="true"></span>
                    <span class="flex-1 min-w-0 truncate text-[var(--p-text-color)]" [attr.title]="l.name">{{ l.name }}</span>
                    <span class="font-semibold text-[var(--p-text-color)]">{{ formatNumber(l.value) }}</span>
                  </li>
                }
              </ul>
            </div>
          }
        </div>
      }
    </app-dashboard-widget-frame>
  `,
})
export class CustomerStatusChartOverview {
  private readonly svc = inject(CrmDashboardService);
  private readonly localeFormat = inject(LocaleFormatService);
  protected readonly ctx = injectWidgetContext();
  protected readonly theme = injectChartTheme();
  protected readonly funnelMinSize = CUSTOMER_STAGE_FUNNEL_MIN_SIZE;
  protected readonly chartHeight = widgetChartHeight('customer_status_overview');
  protected readonly data = createWidgetData<CustomerStageFunnelRow[]>('customer_status_overview', (q) => this.svc.statusCustomerOverview(q), isEmptyRows);
  /** Tầng phễu + khối thất bại — tính 1 lần mỗi lần có số mới (reference ổn định cho `[data]` của directive). */
  protected readonly view = computed(() => {
    const rows = this.data.value();
    return rows ? buildCustomerStageFunnelView(rows) : null;
  });

  protected formatNumber(value: number): string {
    return this.localeFormat.formatNumber(value);
  }
}
