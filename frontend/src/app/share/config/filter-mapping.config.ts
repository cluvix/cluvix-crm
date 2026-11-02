export type FilterFieldType = 'text' | 'multiSelect' | 'date' | 'dateRange';

export interface FilterFieldMappingEntry {
  queryKey: string;
  type: FilterFieldType;
}

export type FilterFieldMapping = {
  [formKey: string]: FilterFieldMappingEntry;
};
export const FilterMappings: { [module: string]: FilterFieldMapping } = {
  appointment: {
    appointment_date: { queryKey: 'appointment', type: 'dateRange' },
    created_at: { queryKey: 'created', type: 'dateRange' },
    branch_id: { queryKey: 'branch', type: 'multiSelect' },
    convert_id: { queryKey: 'convert', type: 'multiSelect' },
    is_accepted: { queryKey: 'is_accepted', type: 'multiSelect' },
    type_service_id: { queryKey: 'type_service', type: 'multiSelect' },
    type_service_accepted_id: { queryKey: 'type_service_accepted', type: 'multiSelect' },
    appointment_type_id: { queryKey: 'appointment_type', type: 'multiSelect' },
    status: { queryKey: 'status', type: 'multiSelect' },
    _keyword: { queryKey: '_keyword', type: 'text' },
    _groupLocal: { queryKey: 'group_local', type: 'multiSelect' },
    _groupCountry: { queryKey: 'group_country', type: 'multiSelect' },
    _groupSource: { queryKey: 'group_source', type: 'multiSelect' },
    revenue_id: { queryKey: 'revenue', type: 'multiSelect' },
    _stage_id: { queryKey: '_stage_id', type: 'multiSelect' },
    stage_id: { queryKey: 'stage_id', type: 'multiSelect' },
    _gender: { queryKey: '_gender', type: 'multiSelect' },
    source_id: { queryKey: 'source', type: 'multiSelect' },
    created_by: { queryKey: 'created_by', type: 'multiSelect' },
    owner_id: { queryKey: 'owner', type: 'multiSelect' },
    category: { queryKey: 'category', type: 'multiSelect' },
  },
};
