import { Identifiable } from '@/core/interface/identifiable';
import { LocationProvinceModel } from '@/core/models/config/location/location-province.model';
import { UserProfileModel } from '@/core/models/user-profile.model';

export interface LocationWardModel extends Identifiable {
  id?: number;
  codeWard?: string;
  name?: string;
  path?: string;
  Type?: string;
  province_id?: number;
  SortOrder?: number;
  status?: boolean;
  updated_at?: any;
  created_at?: any;
  created_by?: any;
  updated_by?: any;

  province?: LocationProvinceModel;
  userCreated?: UserProfileModel;
  userUpdated?: UserProfileModel;
}
