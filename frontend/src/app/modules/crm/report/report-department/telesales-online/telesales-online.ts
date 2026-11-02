import {ChangeDetectionStrategy, Component, effect, inject, Input, OnInit, signal, ViewChild} from '@angular/core';
import {Table} from 'primeng/table';
import {Observable, map} from 'rxjs';
import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {ListShareImports} from '@/share/import/list-share-imports';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import {ReportCrmService} from "@/core/services/report/report-crm.service";

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-report-marketing-telesales-online',
  templateUrl: './telesales-online.html',
  styles: [],
  imports: [
    FillHeightDirective,
    ...ListShareImports,
  ],
  standalone: true,
})
export class TelesalesOnline extends BaseListV2<any> implements OnInit {
  @ViewChild('dt') dt!: Table;
  protected override URL_STATE_PREFIX = 'tso';
  modelsTotal = signal<any>({
    totalTuongTac: 0,
    totalComment: 0,
    totalLead: 0,
    totalChamLai: 0,
    totalAppointmentNew: 0,
    totalAppointmentNewPlus: 0,
    totalAppointment: 0,
    totalAppointmentDen: 0,
    totalAppointmentDongY: 0,
    totalDeposit: 0,
    leadTuongTac: 0,
    lichMoiLead: 0,
    lichMoiPlusChamLai: 0,
    lichMoiMinusLead: 0,
    denLichHen: 0,
    dongYDen: 0,
    denTuongTac: 0,
    dongYTuongTac: 0,
    totalOrderPlan: 0,
    totalThanhToan: 0,
    totalDepositMoney: 0,
    tbKhach: 0,
  }); // khởi tạo giá trị ban đầu
  displayCols: any = [
    {label: 'ID', value: 'id'},
    {label: 'Nhân viên', labelKey: 'common.label.staff', value: 'fullname'},
    {label: 'Tương tác', labelKey: 'common.label.interaction', value: 'totalTuongTac'},
    {label: 'Bình luận', labelKey: 'common.label.comment', value: 'totalComment'},
    {label: 'Lead', labelKey: 'common.label.lead', value: 'totalLead'},
    {label: 'Chăm lại', labelKey: 'common.label.re_care', value: 'totalChamLai'},
    {label: 'Lịch mới', labelKey: 'common.label.new_appointment', value: 'totalAppointmentNew'},
    {label: 'Lịch chăm lại', labelKey: 'common.label.re_care_appointment', value: 'totalAppointmentNewPlus'},
    {label: this.translate.instant('common.label.appointment'), value: 'totalAppointment'},
    {label: 'Khách đến', labelKey: 'common.status.arrived', value: 'totalAppointmentDen'},
    {label: 'Đồng ý', labelKey: 'common.label.accepted', value: 'totalAppointmentDongY'},
    {label: 'Khách cọc', labelKey: 'common.label.deposit_customer', value: 'totalDeposit'},
    {label: 'Lead/ Tương tác', labelKey: 'crm.telesales_online.lead_per_interaction', value: 'leadTuongTac'},
    {label: 'Lịch mới/ Lead', labelKey: 'common.label.new_appointment_lead', value: 'lichMoiLead'},
    {label: 'Lịch mới-/ Lead', labelKey: 'crm.telesales_online.appointment_minus_per_lead', value: 'lichMoiMinusLead'},
    {label: 'Lịch mới+/ Chăm lại', labelKey: 'crm.telesales_online.appointment_plus_per_cham_lai', value: 'lichMoiPlusChamLai'},
    {label: 'Đến/ Lead', labelKey: 'crm.telesales_online.arrived_per_lead', value: 'denLead'},
    {label: 'Đến/ Lịch hẹn', labelKey: 'common.label.arrived_appointment', value: 'denLichHen'},
    {label: 'Đồng ý/ Đến', labelKey: 'common.label.accepted_arrived', value: 'dongYDen'},
    {label: 'Đến/ Tương tác', labelKey: 'crm.telesales_online.arrived_per_interaction', value: 'denTuongTac'},
    {label: 'Đồng ý/ Tương tác', labelKey: 'crm.telesales_online.accepted_per_interaction', value: 'dongYTuongTac'},
    {label: 'Doanh số', labelKey: 'common.label.total_sales', value: 'totalOrderPlan'},
    {label: this.translate.instant('crm.order_plan.form.total_to_pay'), value: 'totalThanhToan'},
    {label: 'Số tiền cọc', labelKey: 'common.label.deposit_amount', value: 'totalDepositMoney'},
    {label: 'Trung bình', labelKey: 'common.label.average', value: 'tbKhach'},
  ]; // dùng để hiển thị column table
  override selectedColumns = signal([
    'fullname',
    'totalTuongTac',
    'totalComment',
    'totalLead',
    'totalChamLai',
    'totalAppointmentNew',
    'totalAppointmentNewPlus',
    'totalAppointment',
    'totalAppointmentDen',
    'totalAppointmentDongY',
    'totalDeposit',
    'leadTuongTac',
    'lichMoiLead',
    'lichMoiMinusLead',
    'lichMoiPlusChamLai',
    'denLead',
    'denLichHen',
    'dongYDen',
    'denTuongTac',
    'dongYTuongTac',
    'totalOrderPlan',
    'totalThanhToan',
    'totalDepositMoney',
    'tbKhach',
  ]); // dùng để hiển thị column mặc định table
  modelService = inject(ReportCrmService);
  protected override LS_SELECTED_COLUMNS_KEY = '/statistic/app-report-marketing-telesales-online';

  constructor() {
    super();
    this.sort.set('-totalLead');
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 2);
    });
  }

  _selectedCols!: any[];

  @Input() get selectedCols(): any[] {
    return this._selectedCols;
  }

  ngOnInit() {
    this.initListV2();
  }

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {
        name: 'created_at',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
      },
    ];
  }

  /**
   * Lấy danh sách hiệu quả telesale
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService
      .telesalesOnline({
        params: this.searchParams(),
      })
      .pipe(
        map((response: any) => {
          const items = response.items ?? [];
          const totals = response.totals ?? {};

          const modelsTemp = items.map((item: any) => ({
            id: item.id,
            fullname: item.fullname,
            totalTuongTac: item.totalTuongTac,
            totalComment: item.totalComment,
            totalLead: item.totalLead,
            totalChamLai: item.totalChamLai,
            totalAppointmentNew: item.totalAppointmentNew,
            totalAppointmentNewPlus: item.totalAppointmentNewPlus,
            totalAppointment: item.totalAppointment,
            totalAppointmentDen: item.totalAppointmentDen,
            totalAppointmentDongY: item.totalAppointmentDongY,
            totalDeposit: item.totalDeposit,
            totalOrderPlan: item.totalOrderPlan,
            totalThanhToan: item.totalThanhToan,
            totalDepositMoney: item.totalDepositMoney,
            leadTuongTac: +item.totalTuongTac == 0 ? 0 : (+item.totalLead * 100) / +item.totalTuongTac,
            lichMoiLead: +item.totalLead == 0 ? 0 : (+item.totalAppointmentNew * 100) / +item.totalLead,
            lichMoiMinusLead: +item.totalLead == 0 ? 0 : ((+item.totalAppointmentNew - +item.totalAppointmentNewPlus) * 100) / +item.totalLead,
            lichMoiPlusChamLai: +item.totalChamLai == 0 ? 0 : (+item.totalAppointmentNewPlus * 100) / +item.totalChamLai,
            denLead: +item.totalLead == 0 ? 0 : (+item.totalAppointmentDen * 100) / +item.totalLead,
            denLichHen: +item.totalAppointment == 0 ? 0 : (+item.totalAppointmentDen * 100) / +item.totalAppointment,
            dongYDen: +item.totalAppointmentDen == 0 ? 0 : (+item.totalAppointmentDongY * 100) / +item.totalAppointmentDen,
            denTuongTac: +item.totalTuongTac == 0 ? 0 : (+item.totalAppointmentDen * 100) / +item.totalTuongTac,
            dongYTuongTac: +item.totalTuongTac == 0 ? 0 : (+item.totalAppointmentDongY * 100) / +item.totalTuongTac,
            tbKhach: +item.totalAppointmentDongY == 0 ? 0 : +item.totalOrderPlan / +item.totalAppointmentDongY,
          }));

          this.modelsTotal.set({
            totalTuongTac: +totals.totalTuongTac || 0,
            totalComment: +totals.totalComment || 0,
            totalLead: +totals.totalLead || 0,
            totalChamLai: +totals.totalChamLai || 0,
            totalAppointmentNew: +totals.totalAppointmentNew || 0,
            totalAppointmentNewPlus: +totals.totalAppointmentNewPlus || 0,
            totalAppointment: +totals.totalAppointment || 0,
            totalAppointmentDen: +totals.totalAppointmentDen || 0,
            totalAppointmentDongY: +totals.totalAppointmentDongY || 0,
            totalDeposit: +totals.totalDeposit || 0,
            totalOrderPlan: +totals.totalOrderPlan || 0,
            totalThanhToan: +totals.totalThanhToan || 0,
            totalDepositMoney: +totals.totalDepositMoney || 0,
            leadTuongTac: +totals.leadTuongTac || 0,
            lichMoiLead: +totals.lichMoiLead || 0,
            lichMoiMinusLead: +totals.lichMoiMinusLead || 0,
            lichMoiPlusChamLai: +totals.lichMoiPlusChamLai || 0,
            denLead: +totals.denLead || 0,
            denLichHen: +totals.denLichHen || 0,
            dongYDen: +totals.dongYDen || 0,
            denTuongTac: +totals.denTuongTac || 0,
            dongYTuongTac: +totals.dongYTuongTac || 0,
            tbKhach: +totals.tbKhach || 0,
          });

          return {
            ...response,
            items: modelsTemp,
            _meta: response._meta ?? {
              totalCount: modelsTemp.length,
              pageCount: 1,
              currentPage: 1,
              perPage: modelsTemp.length,
            },
          } as ListApiResponse<any>;
        }),
      );
  }

  onQuickSelect({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({created_at: [start, end]});
  }

  /**
   * Nút xoá ngày KHÔNG được để bộ lọc rỗng: BE nay bắt buộc có dải `created_at` và chặn trần
   * 1 năm — báo cáo này có 12 bảng dẫn xuất, thiếu ngày là 12 lượt quét cạn (riêng nhánh
   * "chăm lại" đã đọc trọn bảng `note` 3,37 triệu dòng). Xoá về null chỉ đổi một màn treo lấy
   * một toast 422; xoá = quay về mặc định (đầu tháng → hôm nay).
   */
  clearStartTime() {
    this.setFilterValue('created_at', [
      this.timezoneService.monthStart(),
      this.timezoneService.todayStart(),
    ]);
  }
}
