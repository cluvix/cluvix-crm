import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { Avatar } from 'primeng/avatar';
import { StyleClass } from 'primeng/styleclass';

import { UserUpdateProfile } from '@/modules/hr/user/form/user-update-profile/user-update-profile';
import { UserChangePassword } from '@/modules/hr/user/form/change-password/user-change-password';
import { UserDevices } from '@/modules/hr/user/form/devices/user-devices';
import { AuthService, isSuperUser, userAvatar, userFullname, userId } from '@/core/services/auth';
import { AvatarWordPipe } from '@/share/pipes/avatar-word/avatar-word.pipe';
import { DialogWrapperService } from '@/share/services/dialog-wrapper.service';
import { NotificationService } from '@/share/services/notification.service';
import { SOURCE_CODE_URL } from '@/define';
import { SwitchBranch } from '@/modules/utilities/switch-branch/switch-branch';
import { SwitchLanguage } from '@/modules/utilities/switch-language/switch-language';
import { BranchHelperService } from '@/indexeddb/config/components/branch';
import { LayoutService } from '@/layout/service/layout.service';

// Trích từ app.topbar.ts <li> avatar dropdown gốc — dùng chung cho ERP topbar (app.topbar.ts, bọc
// trong <li>) + omnichat-topbar.ts (đặt thẳng trong .seg, KHÔNG có <ul>/<li> cha). Tự chứa style
// (KHÔNG dựa vào `.layout-topbar-items > li` ambient CSS của _topbar_main.scss — selector đó chỉ
// khớp khi <a>/<div> là con TRỰC TIẾP của <li>, extract ra component chèn thêm 1 lớp DOM sẽ làm gãy
// `>` combinator) — mirror pattern dropdown "system module" tự style bằng Tailwind ngay trong
// app.topbar.ts (KHÔNG phải bản dropdown avatar cũ dựa ambient CSS).
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-user-account-menu',
  standalone: true,
  imports: [RouterLink, TranslatePipe, Avatar, StyleClass],
  template: `
    <!-- B.19: PHẢI là <button> + có aria-label. Trước đây là <a> KHÔNG href (⇒ ngoài tab order —
         bàn phím không tới được menu đăng xuất/đổi thương hiệu) và accessible name RỖNG (chỉ có
         <p-avatar> không truyền ariaLabel nên alt rỗng). Đo 2026-09-04 trên rail teamchat.
         Class .tb-btn-reset gỡ nền/viền mặc định của UA; sàn 24px cho WCAG 2.2 AA 2.5.8 vì
         w-10/h-10 (2.5rem) chỉ ra 22.5px ở khung 375 (root font-size fluid 9px).
         ⚠ KHÔNG dùng dấu backtick trong comment này — cả template là template literal, một backtick
         lạc vào là cắt đứt chuỗi và tsc báo lỗi ở tận dòng khác. -->
    <button
      type="button"
      pStyleClass="@next"
      enterFromClass="hidden"
      enterActiveClass="animate-scalein"
      leaveToClass="hidden"
      leaveActiveClass="animate-fadeout"
      [hideOnOutsideClick]="true"
      [attr.aria-label]="
        (displayName() ? ('layout.app_topbar.account_menu_tooltip' | translate) + ': ' + userFullname() : ('layout.app_topbar.account_menu_tooltip' | translate))
      "
      [attr.title]="showName() ? userFullname() : null"
      class="tb-btn-reset tb-user-btn cursor-pointer flex items-center h-10 rounded-full"
      [class.w-10]="!showName()"
      [class.justify-center]="!showName()"
      [class.tb-user-btn-named]="showName()"
    >
      @defer (on idle) {
        @if (userAvatar()) {
          <p-avatar
            [image]="userAvatar()"
            [style]="avatarRingStyle"
            class="p-avatar-topbar w-10 h-10"
            shape="circle"
          ></p-avatar>
        } @else {
          <!-- Chưa có ảnh: chữ cái tên (cùng quy ước app-user-avatar / AvatarWordPipe) trên nền primary ĐẶC
               hình tròn, thay cho no-image.svg (icon ảnh hỏng, không nhận ra là "tài khoản của tôi"). PrimeNG
               ưu tiên label hơn image nên 2 nhánh tách riêng, chỉ truyền đúng 1 input. -->
          <p-avatar
            [label]="initial()"
            [style]="initialStyle"
            class="p-avatar-topbar w-10 h-10"
            shape="circle"
          ></p-avatar>
        }
      } @placeholder {
        <span class="inline-block w-10 h-10 rounded-full bg-surface-200 animate-pulse"></span>
      } @loading {
        <span class="inline-block w-10 h-10 rounded-full bg-surface-200 animate-pulse"></span>
      }
      @if (showName() && displayName()) {
        <!-- Tên (từ đầu + từ cuối của họ tên) cạnh avatar để người dùng biết đang đăng nhập bằng ai. Chỉ topbar chính
             bật (showName); ẩn dưới 992px vì topbar đã chật (xem .tb-user-name trong styles). -->
        <span class="tb-user-name">{{ displayName() }}</span>
        <!-- Mũi tên báo trạng thái menu: đóng trỏ lên, mở xoay xuống (user chốt 2026-10-18). Xoay bằng CSS :has
             trên panel kế bên (xem .tb-user-caret) — pStyleClass tự đóng khi bấm ra ngoài nên KHÔNG giữ state bằng signal. -->
        <i class="pi pi-chevron-up tb-user-caret" aria-hidden="true"></i>
      }
    </button>
    <div
      class="hidden absolute right-0 top-full origin-top mt-4 min-w-[220px] py-2 rounded-lg shadow-lg border border-surface-200 dark:border-surface-700 bg-white dark:bg-surface-800 z-50 text-[color:var(--text-color)]"
    >
      <ul class="list-none p-0 m-0">
        <li>
          <a
            (click)="showUserProfile()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-cog tb-mi tb-mi-account"></i>
            <span>{{ 'common.label.update_profile' | translate }}</span>
          </a>
        </li>
        <li>
          <a
            (click)="changePasswordDialog()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-file-o tb-mi tb-mi-account"></i>
            <span>{{ 'common.label.change_password' | translate }}</span>
          </a>
        </li>
        <!-- Chuyển từ app-menu-profile (đã gỡ khỏi sidebar): topbar là nơi DUY NHẤT còn lối vào 2 chức năng bảo mật này. -->
        <li>
          <a
            (click)="devicesDialog()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-desktop tb-mi tb-mi-account"></i>
            <span>{{ 'hr.user_devices.title' | translate }}</span>
          </a>
        </li>
        <li>
          <a
            routerLink="/2fa-setup"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-shield tb-mi tb-mi-account"></i>
            <span>{{ 'auth.twofa.menu' | translate }}</span>
          </a>
        </li>

        <li class="my-1 border-t border-surface-200 dark:border-surface-700"></li>

        <li>
          <a
            (click)="switchBranch()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-arrow-right-arrow-left tb-mi tb-mi-settings"></i>
            <span>{{ 'layout.app_topbar.switch_branch_tooltip' | translate }}</span>
          </a>
        </li>
        <li>
          <a
            (click)="switchLanguage()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-language tb-mi tb-mi-settings"></i>
            <span>{{ 'common.label.switch_language' | translate }}</span>
          </a>
        </li>
        <li>
          <a
            (click)="toggleTheme()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
          >
            <i class="pi tb-mi tb-mi-settings" [class.pi-sun]="isDark()" [class.pi-moon]="!isDark()"></i>
            <!-- Nhãn là theme sẽ chuyển SANG (cùng quy ước teamchat-rail): 2 khoá có sẵn. -->
            <span>{{ (isDark() ? 'common.label.theme_light' : 'common.label.theme_dark') | translate }}</span>
          </a>
        </li>

        <li class="my-1 border-t border-surface-200 dark:border-surface-700"></li>

        @if (isSuperUser()) {
          <li>
            <a
              (click)="deleteCache()"
              class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
            >
              <i class="pi pi-compass tb-mi tb-mi-tools"></i>
              <span>{{ 'common.label.clear_cache' | translate }}</span>
            </a>
          </li>
        }
        @if (sourceCodeUrl) {
          <li>
            <a
              [href]="sourceCodeUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#7B3F9E] duration-150 transition-all px-4 py-2"
            >
              <i class="pi pi-code tb-mi tb-mi-tools"></i>
              <span>{{ 'common.label.source_code_agpl' | translate }}</span>
            </a>
          </li>
        }

        <li class="my-1 border-t border-surface-200 dark:border-surface-700"></li>

        <li>
          <a
            (click)="logout()"
            class="group cursor-pointer flex items-center gap-3 hover:bg-emphasis hover:text-[#E53935] duration-150 transition-all px-4 py-2"
          >
            <i class="pi pi-power-off tb-mi tb-mi-danger"></i>
            <span>{{ 'common.action.logout' | translate }}</span>
          </a>
        </li>
      </ul>
    </div>
  `,
  styleUrl: './user-account-menu.scss',
  host: { class: 'relative inline-flex items-center' },
})
export class UserAccountMenu {
  private authService = inject(AuthService);
  private dialogWrapperService = inject(DialogWrapperService);
  private translate = inject(TranslateService);
  private noti = inject(NotificationService);
  private branchHelper = inject(BranchHelperService);
  private layoutService = inject(LayoutService);

  /** Hiện tên cạnh avatar — chỉ topbar chính bật; omnichat-topbar (nền gradient) và rail teamchat (cột 4rem) giữ chỉ avatar. */
  readonly showName = input(false);

  protected readonly isDark = this.layoutService.isDarkTheme;
  protected readonly userAvatar = userAvatar;
  protected readonly userFullname = userFullname;
  /** Tên hiển thị = từ ĐẦU + từ CUỐI của họ tên (Đào Văn Mong → Đào Mong; Nguyễn Trần Bảo Ngân → Nguyễn Ngân);
   *  họ tên 1-2 từ giữ nguyên; rỗng khi chưa nạp hồ sơ ('---'). */
  protected readonly displayName = computed(() => {
    const name = userFullname();
    if (!name || name === '---') return '';
    const words = name.trim().split(/\s+/);
    return words.length > 2 ? `${words[0]} ${words[words.length - 1]}` : words.join(' ');
  });
  /** Chữ cái đại diện khi chưa có ảnh; `userFullname` mặc định '---' lúc chưa nạp hồ sơ ⇒ hiện '?'. */
  protected readonly initial = computed(() => {
    const name = userFullname();
    return (name !== '---' && new AvatarWordPipe().transform(name)) || '?';
  });
  // Nút avatar là điểm nhấn của topbar: tròn, primary ĐẶC. Chữ dùng `primary-contrast-color` (trắng ở
  // sáng, surface.900 ở tối — primary tối là bậc 300 nhạt nên chữ trắng sẽ không đọc được).
  protected readonly initialStyle = {
    background: 'var(--p-primary-color)',
    color: 'var(--p-primary-contrast-color)',
    fontWeight: '700',
  };
  /** Có ảnh: vòng primary 2px quanh ảnh để nút vẫn nổi bật như trường hợp chưa có ảnh. */
  protected readonly avatarRingStyle = { boxShadow: '0 0 0 2px var(--p-primary-color)' };
  protected readonly isSuperUser = isSuperUser;
  /** AGPL-3.0 §13 — rỗng ⇒ ẩn mục "Mã nguồn" (xem brand.define.ts). */
  protected readonly sourceCodeUrl = SOURCE_CODE_URL;
  loggingOut = signal(false);

  showUserProfile() {
    document.body.click();

    setTimeout(() => {
      this.dialogWrapperService.open(UserUpdateProfile, {
        header: this.translate.instant('common.label.update_personal_info'),
        data: {
          userId: userId(),
        },
      });
    }, 100);
  }

  changePasswordDialog() {
    document.body.click();

    setTimeout(() => {
      this.dialogWrapperService.open(UserChangePassword, {
        width: '520px',
        header: this.translate.instant('common.label.change_password'),
      });
    }, 100);
  }

  devicesDialog() {
    document.body.click();

    setTimeout(() => {
      this.dialogWrapperService.open(UserDevices, {
        width: '520px',
        header: this.translate.instant('hr.user_devices.title'),
      });
    }, 100);
  }

  /**
   * Chuyển đổi chi nhánh cho user (story-42: bỏ khái niệm thương hiệu).
   */
  async switchBranch() {
    document.body.click();

    await this.branchHelper.ensureBranchesLoaded();
    this.dialogWrapperService.open(SwitchBranch, {
      width: '520px',
      header: this.translate.instant('layout.app_topbar.switch_dialog_header'),
    });
  }

  /**
   * Chuyển đổi ngôn ngữ
   */
  switchLanguage() {
    document.body.click();

    this.dialogWrapperService.open(SwitchLanguage, {
      width: '400px',
      header: this.translate.instant('common.label.switch_language'),
    });
  }

  toggleTheme() {
    this.layoutService.toggleDarkTheme();
  }

  deleteCache() {
    this.authService.deleteCache().subscribe({
      next: (res: any) => {
        if (res) {
          this.noti.successRes(res);
        }
      },
      error: () => this.noti.errorMsg(this.translate.instant('common.message.clear_cache_failed')),
    });
  }

  logout() {
    if (this.loggingOut()) return;
    this.loggingOut.set(true);
    this.authService.logout().subscribe();
  }
}
