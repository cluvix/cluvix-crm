import { AfterViewInit, Directive, inject, Input, OnChanges, SimpleChanges } from '@angular/core';
import { EChartsOption } from 'echarts';

import { EchartsDirective } from '@/share/directive/echart/echarts.directive';
import { LocaleFormatService } from '@/share/services/locale-format.service';

/**
 * Funnel Chart Directive
 * // use
 * <div appFunnelChart [data]="funnelData" [sort]="'descending'" class="w-[400px] h-[400px]"></div>
 *
 * // 📁 data
 * funnelData = [
 *   { value: 60, name: 'Visit' },
 *   { value: 40, name: 'Inquiry' },
 *   { value: 20, name: 'Order' },
 *   { value: 80, name: 'Click' },
 *   { value: 100, name: 'Show' }
 * ];
 * Source: https://echarts.apache.org/examples/en/editor.html?c=funnel
 */
@Directive({
  selector: '[appFunnelChart]',
  standalone: true,
})
export class FunnelChartDirective extends EchartsDirective implements OnChanges, AfterViewInit {
  private localeFormat = inject(LocaleFormatService);
  @Input({ required: true }) data!: any[];
  @Input() sort: 'ascending' | 'descending' | 'none' = 'descending';
  /**
   * Bề rộng tối thiểu (% khung vẽ) của tầng. ECharts tính bề rộng theo thang [min=0 … max] nên cạnh DƯỚI của tầng cuối
   * luôn bằng `minSize`: để 0% thì tầng cuối (và cả tầng gần cuối) hẹp tới mức nhãn trắng tràn khỏi hình.
   * Mặc định giữ 0% (phễu nhọn); nơi có nhãn trong tầng nên truyền mức đủ rộng.
   */
  @Input() minSize: string = '0%';
  @Input() colors: string[] = [
    '#7B3F9E', '#5E35B1', '#1565C0', '#0277BD', '#00838F', '#2E7D32', '#E65100', '#BF360C',
  ];
  @Input() override renderer: 'canvas' | 'svg' = 'canvas';
  @Input() override theme: 'light' | 'dark' | string = 'light';
  @Input() override useDecal = false;
  @Input() override showToolbox = false;

  override ngAfterViewInit() {
    super.ngAfterViewInit();
    if (this.data?.length) {
      this.updateChart(this.getChartOptions());
    }
  }

  override ngOnChanges(changes: SimpleChanges) {
    super.ngOnChanges(changes);
    if (this.chart && this.data?.length && changes['data']) {
      this.updateChart(this.getChartOptions());
    }
  }

  private getChartOptions(): EChartsOption {
    const maxTiers = this.data?.length || 1;

    const dataNormalized = this.data.map((d: any, idx: number) => ({
      ...d,
      _realValue: d.value,
      value: maxTiers - idx,
    }));

    return {
      color: this.colors,
      tooltip: {
        trigger: 'item',
        formatter: (params: any) => {
          const realValue = params.data._realValue ?? params.value;
          const formatted = this.localeFormat.formatNumber(realValue);
          return `<strong>${params.name}</strong><br/>Số lượng: ${formatted}`;
        },
      },
      series: [
        {
          name: 'Funnel',
          type: 'funnel',
          sort: this.sort,
          left: '10%',
          top: 40,
          bottom: 40,
          width: '80%',
          minSize: this.minSize,
          maxSize: '100%',
          gap: 2,
          label: {
            show: true,
            position: 'inside',
            fontSize: 14,
            fontWeight: 700,
            color: '#fff',
            textShadowBlur: 4,
            textShadowColor: 'rgba(0,0,0,0.45)',
            formatter: (params: any) => {
              const realValue = params.data._realValue ?? params.value;
              const formatted = this.localeFormat.formatNumber(realValue);
              return `${params.name}\n${formatted}`;
            },
          },
          labelLine: {
            show: false,
          },
          itemStyle: {
            opacity: 1,
            borderColor: '#fff',
            borderWidth: 2,
          },
          emphasis: {
            label: {
              fontSize: 16,
              fontWeight: 'bold',
              textShadowBlur: 6,
              textShadowColor: 'rgba(0,0,0,0.5)',
            },
            itemStyle: {
              opacity: 1,
            },
          },
          data: dataNormalized,
        },
      ],
    } as const;
  }
}
