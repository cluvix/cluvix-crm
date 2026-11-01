import { Identifiable } from '@/core/interface/identifiable';
import { ServiceTypeModel } from '@/core/models/config/clinic/service-type.model';
import { UnitModel } from '@/core/models/config/system/unit.model';
import { UserProfileModel } from '@/core/models/user-profile.model';

export interface ServiceCategory extends Identifiable {
  label?: string;
  value?: number;
}

export interface InstallmentConfig {
  periods: number[]; // Các kỳ hạn: [3, 6, 12]
  min_deposit_percent: number; // % đặt cọc tối thiểu
  interest_rate: number; // Lãi suất
  period_interval_days: number; // Khoảng cách giữa các kỳ (ngày)
}

export interface ServiceModel extends Identifiable {
  id?: number;
  id_old?: number;
  name?: string;
  code?: string;
  code_old?: string;
  min_price?: number;
  max_price?: number;
  unit_id?: number;
  servicePosition?: string[] | null;
  serviceCategory_id?: number;
  service_type_id?: number;
  child?: any;
  is_installment?: number; // Dịch vụ có được trả góp hay không?
  installment_config?: InstallmentConfig; // Cấu hình trả góp
  isUpdateMaterial?: number; // Dịch vụ có được chỉnh sửa nguyên vật liệu khi điều trị hay không. Mặc định 0 là không được chỉnh sửa, 1 là được chỉnh sửa
  status?: number;
  time_reminder?: number;
  commission_telesale?: number;
  commission_telesale_new?: number;
  commission_telesale_type?: number; // 1 là VND, 2 là %
  commission_directSale?: number;
  commission_directSale_new?: number;
  commission_directSale_type?: number; // 1 là VND, 2 là %
  commission_technical?: number;
  commission_technical_new?: number;
  commission_technical_type?: number; // 1 là VND, 2 là %
  commission_assistant?: number;
  commission_assistant_new?: number;
  commission_assistant_type?: number; // 1 là VND, 2 là %
  commission_doctor?: number;
  commission_doctor_new?: number;
  commission_doctor_type?: number; // 1 là VND, 2 là %
  commission_doctor_treatment?: number; // Hoa hồng bác sĩ ĐIỀU TRỊ (khác commission_doctor = bác sĩ tư vấn)
  commission_doctor_treatment_new?: number;
  commission_doctor_treatment_type?: number; // 1 là VND, 2 là %
  SortOrder?: number;
  tk_no?: string; // Dành cho kế toán
  tk_co?: string; // Dành cho kế toán
  created_at?: null;
  updated_at?: number | null;
  created_by?: number;
  updated_by?: number;

  serviceType?: ServiceTypeModel;
  unit?: UnitModel;
  userCreated?: UserProfileModel;
  userUpdated?: UserProfileModel;
  serviceCategory?: ServiceCategory;
}
