import { Identifiable } from '@/core/interface/identifiable';
import { BranchModel } from '@/core/models/config/system/branch.model';
import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { SourceModel } from '@/core/models/config/marketing/source.model';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { OrderServiceUsedModel, OrderServiceUsedRow } from '@/core/models/crm/order-service-used.model';
import { PaymentModel } from '@/core/models/crm/payment.model';
import { UserProfileModel } from '@/core/models/user-profile.model';
import { UserModel } from '@/core/models/hr/user.model';
import { AppointmentModel } from '@/core/models/crm/appointment.model';

export interface OrderPlanModel extends Identifiable {
  id?: number;
  code?: string;
  orderPlanType?: number; // Đơn hàng mới hay đơn hàng cũ
  customer_id?: number;
  appointment_id?: number;
  branch_id?: number;
  revenue_id?: number;
  // number[] — form/BE đều dùng mảng id user (khai `string` trước đây là SAI, Reactive Forms untyped
  // nên không lộ ra; Signal Forms strict-type bind multiselect sẽ nổ TS2322).
  listRevenues?: number[];
  source_id?: number;
  broker_id?: number; // CTV giới thiệu (từ customer.affiliate_id hoặc coupon, staff override được)
  status?: number;
  payment?: number;
  total?: number;
  promotion?: number;
  paymentStatus?: string;
  discount_other?: number;
  order_plan_id?: number;
  quantity?: number;
  unit_price?: number;
  service_discount?: number;
  service_discount_type?: number;
  note_discount?: string;
  note?: string;
  paymentPlan?: number;
  debtsPlan?: number;
  created_time?: any;
  created_by?: number;
  updated_by?: number;
  created_at?: number;
  updated_at?: number;
  coupon?: string; // Mã coupon đơn hàng sử dụng
  service?: ServiceModel;
  orderServiceUseds?: OrderServiceUsedModel[];
  /**
   * Mảng dòng dịch vụ Ở TRONG FORM (Signal Forms) — KHÁC `orderServiceUseds` (bản BE trả về, chỉ đọc).
   * Chỉ tồn tại phía FE lúc mở `OrderPlanForm`; payload gửi lên BE vẫn dùng đúng key `services_used`
   * như bản Reactive Forms cũ.
   */
  services_used?: OrderServiceUsedRow[];
  customer?: CustomerModel;
  appointment?: AppointmentModel;
  payments?: PaymentModel[];
  branch?: BranchModel;
  source?: SourceModel;
  revenue?: UserModel;
  orderServiceUsed?: OrderServiceUsedModel;
  refundServiceTotal?: number;
  // Nhãn + thông tin dựng ở FE bởi `OrderPlanService.addLabel` (cache IndexedDB) — không phải cột DB.
  customerDisplayName?: string;
  branchLabel?: string;
  sourceLabel?: string;
  createdByInfo?: UserProfileModel | null;
  updatedByInfo?: UserProfileModel | null;
  // Chuỗi ngày giờ đã format sẵn (`addFormattedTimestampFields`)
  created_timeFormatted?: string;
  created_atFormatted?: string;
  updated_atFormatted?: string;
  userCreated?: UserProfileModel;
  userUpdated?: UserProfileModel;
  created_day?: number;
}
