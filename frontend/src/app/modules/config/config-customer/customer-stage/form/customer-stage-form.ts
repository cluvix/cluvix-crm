import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { disabled, FormField, required, SchemaPathTree } from '@angular/forms/signals';

import { CustomerStageModel } from '@/core/models/config/customer-stage.model';
import { CustomerStageService } from '@/core/services/config/customer-stage.service';
import {
  CUSTOMER_STAGE_COLOR_OPTIONS,
  CUSTOMER_STAGE_TYPE_OPTIONS,
  customerStageColorClass,
} from '@/define';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { FormShareImports } from '@/share/import/form-share-imports';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-stage-form',
  templateUrl: './customer-stage-form.html',
  styles: [],
  imports: [...FormShareImports, FormField],
  standalone: true,
})
export class CustomerStageForm extends BaseSignalForm<CustomerStageModel> {
  modelService = inject(CustomerStageService);

  protected readonly typeOptions = CUSTOMER_STAGE_TYPE_OPTIONS;
  protected readonly colorOptions = CUSTOMER_STAGE_COLOR_OPTIONS;
  protected readonly colorDotClass = (color: string) => customerStageColorClass(color).dot;

  /** Bước mặc định: BE khoá "phải là loại đang mở + đang hiện" ⇒ UI cũng khoá loại và trạng thái. */
  protected get isDefaultStage(): boolean {
    return this.model().is_default === 1;
  }

  protected override buildSchema(p: SchemaPathTree<CustomerStageModel>): void {
    required(p.name!);
    required(p.type!);
    required(p.color!);
    // Bước mặc định: BE khoá "loại đang mở + đang hiện" ⇒ khoá loại/trạng thái ở UI (BE vẫn là lưới).
    // Khoá qua schema, KHÔNG `[disabled]` trên control có `[formField]` (directive ghi đè về false).
    disabled(p.type!, { when: () => this.model().is_default === 1 });
    disabled(p.status!, { when: () => this.model().is_default === 1 });
  }
}
