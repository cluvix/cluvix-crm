import {ChangeDetectionStrategy, ChangeDetectorRef, Component, inject, OnInit } from '@angular/core';

import { OptionsService } from '@/core/services/config/system/options.service';
import { BaseApp } from '@/share/base/BaseApp';
import { FormShareImports } from '@/share/import/form-share-imports';
import { finalize } from 'rxjs';
import { CONFIG_OPTION_CRM_SELECT, PERMISSION_SELECT } from '@/define';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-help-option-crm',
  templateUrl: './option-crm.html',
  imports: [...FormShareImports],
  standalone: true,
})
export class OptionCrm extends BaseApp implements OnInit {
  permissionManageConfig = PERMISSION_SELECT.permissionManageConfig;
  selectedPaymentUpdate!: number;
  selectedOrderPlanUpdate!: number;
  selectedLeadOfTelesale!: number;
  selectedNoteDelete!: number;
  selectedUpdateCustomerForTelesale!: number;
  selectedHidePhoneNumber!: number;
  selectedTimeBefore!: number;
  selectedAllowPreviousDay!: number;
  selectedCrossBranch!: number;
  selectedExportPhone!: number;
  categoryOptionUpdate: any[] = [
    { nameKey: 'config.option_crm.opt_deny', key: 0 },
    { nameKey: 'config.option_crm.opt_allow_change', key: 1 },
  ];
  categoryOptionDelete: any[] = [
    { nameKey: 'config.option_crm.opt_deny', key: 0 },
    { nameKey: 'config.option_crm.opt_allow_change', key: 1 },
  ];
  categoryOptionLeadView: any[] = [
    { nameKey: 'config.option_crm.opt_lead_own_only', key: 0 },
    { nameKey: 'config.option_crm.opt_lead_all', key: 1 },
  ];
  categoryUpdateCustomerForTelesale: any[] = [
    { nameKey: 'config.option_crm.opt_deny', key: 0 },
    { nameKey: 'config.option_crm.opt_allow_change', key: 1 },
  ];
  categoryHidePhoneNumber: any[] = [
    { nameKey: 'config.option_crm.opt_export_phone_all', key: 0 },
    { nameKey: 'config.option_crm.opt_show_last4_phone', key: 1 },
  ];
  categoryTimeBefore: any[] = [
    { nameKey: 'config.option_crm.opt_deny', key: 0 },
    { nameKey: 'config.option_crm.opt_allow_change', key: 1 },
  ];
  categoryAllowPreviousDay: any[] = [
    { nameKey: 'config.option_crm.opt_deny', key: 0 },
    { nameKey: 'config.option_crm.allow_previous_day_title', key: 1 },
  ];
  categoryCrossBranch: any[] = [
    { nameKey: 'config.option_crm.opt_own_branch_only', key: 0 },
    { nameKey: 'config.option_crm.opt_all_branch_company', key: 1 },
  ];
  categoryExportPhone: any[] = [
    { nameKey: 'config.option_crm.opt_export_phone_manager_only', key: 0 },
    { nameKey: 'config.option_crm.opt_export_phone_all', key: 1 },
  ];
  modelService = inject(OptionsService);
  protected readonly CONFIG_OPTION_CRM_SELECT = CONFIG_OPTION_CRM_SELECT;
  private cdr = inject(ChangeDetectorRef);

  constructor() {
    super();
  }

  ngOnInit() {
    this.loadingService.showLoading('option-crm');
    this.modelService
      .getOptions()
      .pipe(finalize(() => this.loadingService.hideLoading('option-crm')))
      .subscribe({
        next: (res: any) => {
          for (const option of res) {
            switch (option.id) {
              case CONFIG_OPTION_CRM_SELECT.OPTION_UPDATE_PAYMENT:
                this.selectedPaymentUpdate = this.categoryOptionUpdate[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_UPDATE_ORDER_PLAN:
                this.selectedOrderPlanUpdate = this.categoryOptionUpdate[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_VIEW_LEAD_OF_TELE:
                this.selectedLeadOfTelesale = this.categoryOptionLeadView[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_DELETE_NOTE:
                this.selectedNoteDelete = this.categoryOptionDelete[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_UPDATE_CUSTOMER_FOR_TELESALE:
                this.selectedUpdateCustomerForTelesale =
                  this.categoryUpdateCustomerForTelesale[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_HIDE_NUMBER_PHONE:
                this.selectedHidePhoneNumber = this.categoryHidePhoneNumber[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_TIME_BEFORE:
                this.selectedTimeBefore = this.categoryTimeBefore[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_ALLOW_PREVIOUS_DAY:
                this.selectedAllowPreviousDay = this.categoryAllowPreviousDay[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_CUSTOMER_CROSS_BRANCH:
                this.selectedCrossBranch = this.categoryCrossBranch[option.options];
                break;
              case CONFIG_OPTION_CRM_SELECT.OPTION_EXPORT_PHONE:
                this.selectedExportPhone = this.categoryExportPhone[option.options];
                break;
            }
          }
          this.cdr.markForCheck();
        },
        error: (err: any) => {
          this.loadingService.hideLoading('option-crm');
          console.error(err);
        },
      });
  }

  optionUpdate(id: number, idOption: number) {
    const params = {
      options: idOption,
    };
    this.modelService.optionUpdate(id, params).subscribe(() => {
      this.noti.successMsg(this.translate.instant('common.message.update_success'));
    });
  }
}
