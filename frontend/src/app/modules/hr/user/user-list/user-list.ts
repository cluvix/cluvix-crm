import {ChangeDetectionStrategy, Component, effect, inject, OnDestroy, OnInit, signal, ViewChild } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Table } from 'primeng/table';
import { finalize, Observable, switchMap } from 'rxjs';

import { UserForm } from '@/modules/hr/user/form/user-form/user-form';
import { UserModel } from '@/core/models/hr/user.model';
import { UserService } from '@/core/services/hr/user.service';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { PERMISSION_SELECT, USER_STATUS_OPTIONS } from '@/define';
import { ListShareImports } from '@/share/import/list-share-imports';
import { translateOptions } from '@/share/utils/translate-options.util';
import { ListApiResponse } from '@/share/models/list-api-response';
import { DepartementMultiselectFromControl } from '@/indexeddb/config/components/hr-department';
import { UserImportDialog } from './user-import-dialog/user-import-dialog';
import { UserResetPasswordDialog } from './user-reset-password-dialog/user-reset-password-dialog';
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-users-user-list',
  templateUrl: './user-list.html',
  styleUrls: ['./user-list.scss'],
  imports: [FillHeightDirective, ...ListShareImports, DepartementMultiselectFromControl],
  standalone: true,
})
export class UserList extends BaseListV2<UserModel> implements OnInit, OnDestroy {
  modelService = inject(UserService);
  @ViewChild('dt') dt!: Table<UserModel>;
  childRole = signal([]);
  // dùng để hiển thị column table
  displayCols: any = [
    { label: 'Avatar', labelKey: 'common.label.avatar', value: 'avatar' },
    { label: 'ID', value: 'id' },
    { label: 'Nhân viên', labelKey: 'common.label.staff', value: 'name' },
    { label: 'Mã chấm công', labelKey: 'hr.user_list.code', value: 'code' },
    { label: 'Bí danh', labelKey: 'hr.user_list.nickname', value: 'nickname' },
    { label: 'Giới tính', labelKey: 'common.label.gender', value: 'genderUser' },
    { label: 'Số điện thoại', labelKey: 'common.label.phone', value: 'phone' },
    { label: 'Email', labelKey: 'common.label.email', value: 'email' },
    { label: 'Tên đăng nhập', labelKey: 'common.label.username', value: 'username' },
    { label: 'Vai trò', labelKey: 'common.label.role', value: 'description' },
    { label: 'Trạng thái', labelKey: 'common.label.status', value: 'userStatus' },
    { label: 'Khoa', labelKey: 'common.label.department', value: 'department' },
    { label: 'Chi nhánh', labelKey: 'common.label.branch', value: 'branch' },
    { label: 'Người tạo', labelKey: 'common.label.user_created', value: 'created_by' },
    { label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at' },
    { label: 'Người cập nhật', labelKey: 'common.label.user_updated', value: 'updated_by' },
    { label: 'Ngày cập nhật', labelKey: 'common.label.updated_at', value: 'updated_at' },
  ];
  // dùng để hiển thị column mặc định table
  override selectedColumns = signal([
    'avatar',
    'name',
    'code',
    'username',
    'description',
    'userStatus',
    'department',
    'branch',
  ]);

  protected override LS_SELECTED_COLUMNS_KEY = '/users/app-users-user-list';
  protected override URL_STATE_PREFIX = 'us';
  protected readonly userStatusOptions = translateOptions(USER_STATUS_OPTIONS);

  constructor() {
    super();
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 2);
    });
  }

  ngOnInit() {
    this.loadChildRole();
    this.initListV2();
  }

  loadChildRole() {
    this.rbacService.getChildRoleForDropdown().subscribe({
      next: (res: any) => this.childRole.set(Array.isArray(res) ? res : res?.data || []),
      error: () => this.childRole.set([]),
    });
  }

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      { name: '_keyword', kind: 'text' },
      { name: 'branch_id', kind: 'in' },
      { name: '_department', kind: 'in', urlKey: 'department' },
      { name: 'status', kind: 'in' },
      { name: '_role', kind: 'in', urlKey: 'role' },
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<UserModel>> {
    return this.modelService.list({ params: this.searchParams() }).pipe(
      switchMap(async (response: any) => {
        if (response.items) {
          const items = response.items ?? [];
          response.items = await this.modelService.addLabels(items);
        }
        return response;
      }),
    );
  }

  resetPassword(_event: Event, id: number): void {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdateUser)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.dialogWrapperService.open(UserResetPasswordDialog, {
      width: '400px',
      header: this.translate.instant('hr.user_list_page.reset_password_dialog_title'),
      data: { userId: id },
    });
  }

  /**
   * Import nhân sự từ Excel — mở dialog chuẩn (tải file mẫu → chọn file → tổng kết).
   * File mẫu và file Export dùng CHUNG bộ cột nên export ra sửa rồi import lại được luôn.
   */
  openImportDialog() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtUser)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.dialogWrapperService.open(UserImportDialog, {
      width: '700px',
      header: this.translate.instant('hr.user_list_page.import_dialog_title'),
      data: { onImported: () => this.getList() },
    });
  }

  /**
   * Export danh sách nhân sự — hỏi trước: toàn bộ cột hay chỉ các cột đang hiển thị.
   *
   * `columns` chỉ gửi ở chế độ 'selected'; không gửi = BE export toàn bộ. BE luôn ép giữ cột `id`
   * nên file ở cả 2 chế độ đều import ngược lại được.
   */
  exportUser() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtUser)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const ref = this.dialogWrapperService.open(ExportOptionsDialog, {
      header: this.translate.instant('utilities.export_options_dialog.header'),
      width: '480px',
      data: {
        selectedCount: this.selectedColumns().length,
        totalCount: this.displayCols.length,
      },
    });

    ref.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((mode: ExportMode | null) => {
      if (!mode) return;
      // exportParams() (KHÔNG tự dựng lại từ search()) để giữ `sort` — thiếu nó thì file export ra
      // khác thứ tự bảng user đang nhìn.
      const params = { ...this.exportParams() } as any;
      if (mode === 'selected') {
        params['columns'] = this.selectedColumns().join(',');
      }
      this.isSubmitting.set(true);
      this.loadingService.showLoading('exportUser');
      this.modelService
        .exportUser({ params })
        .pipe(
          finalize(() => {
            this.isSubmitting.set(false);
            this.loadingService.hideLoading('exportUser');
          }),
        )
        .subscribe({
          next: (res: any) => {
            this.noti.successMsg(this.translate.instant('common.message.export_success'));
            this.downloadExport(res);
          },
        });
    });
  }

  /**
   * Tạo mới nhân viên
   */
  override createRecord(): void {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionCreateUser)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const dialogRef = this.dialogWrapperService.open(UserForm, {
      width: '760px',
      header: this.translate.instant('common.action.create'),
      data: { childRole: this.childRole() },
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: UserModel) => {
      if (result) {
        this.getList();
      }
    });
  }

  /**
   * Cập nhật nhân viên
   */
  override updateRecord(model: UserModel) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdateUser)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const dialogRef = this.dialogWrapperService.open(UserForm, {
      width: '760px',
      header: this.translate.instant('common.action.update'),
      data: { model, childRole: this.childRole() },
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
      if (result) {
        this.getList();
      }
    });
  }

}
