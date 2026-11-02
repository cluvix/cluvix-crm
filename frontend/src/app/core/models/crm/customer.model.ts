import { Identifiable } from '@/core/interface/identifiable';
import { AppointmentModel } from '@/core/models/crm/appointment.model';
import { AppointmentNotBookedReasonsModel } from '@/core/models/config/appointment/appointment-not-booked-reasons.model';
import { CustomerCategoryModel } from '@/core/models/config/marketing/customer-category.model';
import { ServiceTypeModel } from '@/core/models/config/clinic/service-type.model';
import { SourceModel } from '@/core/models/config/marketing/source.model';
import { NoteModel } from '@/core/models/crm/note.model';
import { UserProfileModel } from '@/core/models/user-profile.model';
import { LocationWardModel } from '@/core/models/config/location/location-ward.model';
import { LocationCountryModel } from '@/core/models/config/location/location-country.model';

export interface CustomerModel extends Identifiable {
  id?: number;
  code?: string;
  category?: number;
  fullname?: string;
  nickname?: string;
  gender?: number;
  genderCustomer?: string;
  tags?: string[];
  birthday?: Date;
  phone?: string;
  other_phone?: string;
  avatar?: string;
  social?: any;
  address?: string;
  job_id?: number;
  ethnicity_id?: number;
  address_other?: string;
  totalRevenue?: number;
  totalPayment?: number;
  totalDebt?: number;
  totalDeposit?: number;
  totalBonus?: number;
  refundDeposit?: number;
  refundService?: number;
  province_id?: number;
  country_id?: any;
  ward_id?: number;
  branch_id?: number;
  status?: any;
  source_id?: number;
  loai_message?: string;
  chat_message?: string;
  link_message?: string;
  facebook?: string;
  instagram?: string;
  zalo?: string;
  email?: string;
  viber?: string;
  old_code?: string;
  owner_id?: number;
  payment_user_id?: number;
  data_id?: number;
  connect_id?: number;
  customerType?: number;
  appointment_not_booked_reasons_id?: number;
  appointment_date?: Date;
  code_affiliate?: string;
  affiliate_id?: number;
  affiliate_fullname?: string;
  type_service_id?: number;
  year_born?: number;
  last_contact_date?: number | Date;
  last_visit_date?: number | Date;

  // EMR Fields
  is_patient?: number; // 0=Chưa là bệnh nhân, 1=Đã là bệnh nhân
  emr_patient_code?: string; // Mã bệnh nhân (BN2025000001)
  emr_house_number?: string; // Số nhà
  emr_street?: string; // Thôn, phố
  emr_id_card_number?: string; // Số CCCD/Hộ chiếu/Định danh
  emr_permanent_address?: string; // Địa chỉ thường trú
  emr_insurance_number?: string; // Số thẻ BHYT (15 ký tự)
  emr_insurance_expiry_date?: number; // Ngày hết hạn BHYT
  emr_insurance_place_code?: string; // Mã nơi đăng ký KCB ban đầu
  emr_insurance_place_name?: string; // Tên nơi đăng ký KCB
  emr_insurance_valid_from?: number; // BHYT giá trị từ ngày
  emr_insurance_valid_to?: number; // BHYT giá trị đến ngày
  emr_insurance_benefit_ratio?: number; // Tỷ lệ hưởng BHYT (%)
  emr_family_history?: string; // Tiền sử gia đình

  created_day?: number | Date;
  created_at?: number | Date;
  updated_at?: number | Date;
  created_by?: null;
  updated_by?: null;

  customerTypeLabel?: string;
  sourceLabel?: string;
  serviceTypeLabel?: string;
  customerDisplayName?: string;
  branchLabel?: string;
  statusLabel?: string;
  statusKey?: string;
  provinceLabel?: string;
  created_dayFormatted?: string;
  created_atFormatted?: string;
  updated_atFormatted?: string;
  updatedByInfo?: UserProfileModel;
  createdByInfo?: UserProfileModel;
  countryInfo?: LocationCountryModel;
  paymentUserInfo?: UserProfileModel;

  connect?: string;
  data?: string;
  appointmentNotBookedReasons?: AppointmentNotBookedReasonsModel;
  notes?: NoteModel[];
  latestNote?: string;
  notesTotal?: number;
  appointmentsTotal?: number;
  appointments?: AppointmentModel;
  source?: SourceModel;
  ward?: LocationWardModel;
  serviceType?: ServiceTypeModel;
  customerCategory?: CustomerCategoryModel;
  relatives?: any;

  canShowUpdateBtn?: unknown;
  allowedPerms?: string[]; // Permission

  // Aggregate đếm lượt (giữ lại — BE vẫn serve).
  treatment_count?: number;
  visit_count?: number;
}
