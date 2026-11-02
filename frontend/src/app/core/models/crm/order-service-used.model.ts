import { Identifiable } from '@/core/interface/identifiable';
import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { SourceModel } from '@/core/models/config/marketing/source.model';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { UserProfileModel } from '@/core/models/user-profile.model';
import { OrderPlanModel } from '@/core/models/order-plan.model';
import { AppointmentModel } from '@/core/models/crm/appointment.model';

/**
 * 1 DÒNG dịch vụ bên trong `OrderPlanForm` (Signal Forms) — tương đương các control của
 * `fb.group()` trong `addServiceUsed()` bản Reactive Forms cũ (trừ `discount`/`expires_at` đã bỏ),
 * cộng 3 field FE-only.
 *
 * KHÁC `OrderServiceUsedModel` (shape BE): có thêm `unitName`/`min_price`/`max_price` (hiển thị). Payload gửi BE vẫn giữ nguyên tập
 * key như bản cũ — 3 field FE-only dưới đây bị loại ở `getPayload()`.
 *
 * Mọi field seed giá trị CỤ THỂ (`null`/`[]`/`''`), KHÔNG `undefined` — Signal Forms không bind được
 * field `undefined` (gotcha #1 `skill/frontend/signal-forms.md`).
 */
export interface OrderServiceUsedRow {
  id: number | null;
  customer_id: number | null;
  order_plan_id: number | null;
  service_discount: number | null;
  total: number | null;
  totalEnd: number | null;
  payment: number | null;
  note: string | null;
  service_id: number | null;
  service_discount_type: number | null;
  unit_price: number | null;
  quantity: number | null;
  discount_other: number | null;
  min_price: number | null;
  max_price: number | null;
  updated_at: number | null;
  discountServiceLock: number | null;
  unitName: string | null;
  /** Tiền VAT của dòng = round(totalEnd × % VAT đơn / 100) — BE tính lại và đối chiếu (lệch > 1đ ⇒ chặn). */
  vat_amount: number | null;

  // ===== FE-only, KHÔNG gửi BE (loại ở getPayload) =====
  /** Khoá ổn định của dòng — `dataKey` cho p-table. `id` không dùng được vì dòng mới có id = null. */
  _uid: number;
  /** Khoá ô Đơn giá (min==max / giá ngoài khoảng) — thay `control.disable()` imperative. */
  unitPriceLocked: boolean;
  /** Khoá ô Dịch vụ (dòng đã có payment) — đổi dịch vụ phải qua dialog riêng. */
  serviceLocked: boolean;
}

export interface OrderServiceUsedModel extends Identifiable {
  id?: number;
  code?: string;
  customer_id?: number;
  appointment_id?: number;
  order_plan_id?: number;
  serviceType?: number;
  service_id?: number;
  listRevenues?: number[];
  listDoctors?: number[];
  listDirectSales?: number[];
  branch_id?: number;
  source_id?: number;
  quantity?: number;
  unit_price?: number;
  service_discount_type?: number;
  service_discount?: number; // Giảm trừ trên dịch vụ
  discount_other?: number; // Giảm trừ khác
  note_discount?: string;
  ListApiResponse?: number;
  refundServiceUsed?: number; // Tổng tiền hoàn dịch vụ
  total?: number;
  discountServiceLock?: number; // Số tiền giảm trừ vào doanh số do khoá dịch vụ
  totalEnd?: number; // Thành tiền TRƯỚC thuế — phải thu = totalEnd + vat_amount
  vat_amount?: number; // Tiền VAT của dòng (theo mức VAT của đơn)
  payment?: number;
  broker_id?: number; // CTV giới thiệu (copy từ OTP, NULL = không tính hoa hồng)
  status_commission?: number;
  note?: string;
  status?: number;
  treatmentPlanCode?: string;
  /** Ngày bắt đầu điều trị. // ⚠ SAU afterLoad là `Date`, không còn là số giây (BaseApiService.unixTimesTampFields) */
  activated_at?: number;
  dayComplete?: number; // Ngày dịch vụ hoàn thành
  /** Ngày có thể tái khai thác. // ⚠ SAU afterLoad là `Date`, không còn là số giây (BaseApiService.unixTimesTampFields) */
  dayReExploitation?: number;
  created_day?: number; // Ngày tạo dịch vụ điều trị
  isLock?: number;
  created_by?: number;
  updated_by?: number;
  created_at?: number;
  updated_at?: number;

  orderPlanTypeLabel?: string; // Đơn hàng cũ hay mới
  // Nhãn + thông tin dựng ở FE bởi `OrderServiceUsedService.addLabel` (cache IndexedDB) — không phải cột DB.
  customerDisplayName?: string;
  branchLabel?: string;
  provinceLabel?: string;
  sourceLabel?: string;
  sourceCustomerLabel?: string;
  serviceTypeLabel?: string;
  appointmentTypeLabel?: string;
  unitInfo?: { id?: number; name?: string } | null;
  countryInfo?: { id?: number; name?: string } | null;
  doctor?: UserProfileModel | null; // Bác sĩ điều trị cuối
  createdByInfo?: UserProfileModel | null;
  updatedByInfo?: UserProfileModel | null;
  // Chuỗi ngày giờ đã format sẵn (`addFormattedTimestampFields`)
  created_dayFormatted?: string;
  dayCompleteFormatted?: string;
  created_atFormatted?: string;
  updated_atFormatted?: string;

  revenueList?: UserProfileModel[];
  doctorList?: UserProfileModel[];
  treatmentCourseDoctor?: any;
  serviceInfo?: ServiceModel;
  treatmentPlan?: OrderPlanModel;
  source?: SourceModel;
  customer?: CustomerModel;
  appointment?: AppointmentModel;
  userCreated?: UserProfileModel;
  userUpdated?: UserProfileModel;
}
