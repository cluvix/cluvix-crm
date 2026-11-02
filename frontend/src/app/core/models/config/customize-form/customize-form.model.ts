import { Identifiable } from '@/core/interface/identifiable';

export interface CustomizeFormModel extends Identifiable {
  id?: number;
  name?: string;
  control?: string;
}
