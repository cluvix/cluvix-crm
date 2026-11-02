import { Identifiable } from '@/core/interface/identifiable';
import { UserProfileModel } from '@/core/models/user-profile.model';

export interface UnitModel extends Identifiable {
  id?: number;
  name?: string;
  description?: string;
  status?: number;
  categoryId?: number[];
  SortOrder?: number;
  created_by?: number;
  updated_by?: number;
  created_at?: number;
  updated_at?: number;

  userCreated?: UserProfileModel;
  userUpdated?: UserProfileModel;
}
