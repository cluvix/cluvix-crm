import { Identifiable } from '@/core/interface/identifiable';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { UserProfileModel } from '@/core/models/user-profile.model';

export interface NoteModel extends Identifiable {
  id?: number;
  note?: string;
  parent_table?: string;
  parent_id?: number;
  customer_id?: number;
  version?: number;
  pinned?: boolean | 1 | '1';
  pinned_by?: number;
  pinned_at?: number;
  created_at?: number;
  updated_at?: number;
  created_by?: number;
  updated_by?: number;

  createdByInfo?: UserProfileModel;

  customer?: CustomerModel;
  userCreated?: UserProfileModel;
  createdBy?: any;
  pinnedBy?: UserProfileModel;
}

/** NoteModel đã qua addLabel() — có thêm các display fields */
export interface NoteDisplayModel extends NoteModel {
  customerDisplayName?: string;
  created_atFormatted?: string;
  stageName?: string;
  stageColorClass?: string;
  createdByInfo?: UserProfileModel;
  branchLabel?: string;
  provinceLabel?: string;
  sourceCustomerLabel?: string;
}
