import { Identifiable } from '@/core/interface/identifiable';
import { UserProfileModel } from '@/core/models/user-profile.model';
import { RoleModel } from '@/core/models/hr/role.model';

export interface UserModel extends Identifiable {
  id?: number;
  id_old?: number;
  avatar_id?: number;
  username?: string;
  fullname?: string;
  device?: string;
  status?: number;
  branchId?: number[];
  created_at?: number;
  updated_at?: number;
  created_by?: number;
  updated_by?: number;
  statusTmp?: boolean;
  gender?: number;
  role?: string;
  password?: string;
  phone?: string;
  avatar?: string;
  pancake?: any;
  code?: string;
  about?: string;

  birthday?: Date;
  userRole?: RoleModel;
  userCreated?: UserProfileModel;
  userProfile?: UserProfileModel;
  /**
   * IP/CIDR được phép truy cập (JSON column BE) — bản gốc Reactive Forms đọc qua `model: any`
   * (KHÔNG khai trên interface này), UI hiển thị dạng text nhiều dòng qua `ipsToText()`/`parseIps()`
   * (`user-form.ts`). Thêm field vào type ở đây để Signal Forms bind được (gotcha #1 angular22.md —
   * cần key thật tồn tại trên model). `ListUser` (BE) KHÔNG trả field này (chỉ `GetUser` có) — xem
   * `deferred-work.md` entry story-08.
   */
  allowed_ips?: string[] | null;
  /**
   * S-4/D-1 — chỉ có trên user của phiên (login + `/auth/me`): true ⇒ bắt buộc đổi mật khẩu trước khi
   * dùng hệ thống (admin seed mật khẩu mặc định, hoặc mật khẩu do người khác đặt lại). `authGuard` đưa
   * vào `/change-password-required`; BE chặn mọi API nghiệp vụ (403 `password_change_required`).
   */
  mustChangePassword?: boolean;
}
