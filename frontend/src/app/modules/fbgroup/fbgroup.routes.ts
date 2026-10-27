import { Routes } from '@angular/router';

import { requirePerm } from '@/core/guards/has-perm.guard';
import { PERMISSION_SELECT } from '@/define';

/** Module Group Facebook — /fb-group/* (đăng ký trong crm.routes, nằm dưới AppLayout + authGuard). */
export default [
  { path: '', pathMatch: 'full', redirectTo: 'stats' },
  {
    path: 'stats',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewFbGroup)],
    data: { breadcrumb: 'fbgroup.menu.stats', title: 'fbgroup.menu.stats' },
    loadComponent: () => import('@/modules/fbgroup/stats/fbgroup-stats').then((c) => c.FbGroupStats),
  },
  {
    path: 'groups',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewFbGroup)],
    data: { breadcrumb: 'fbgroup.menu.groups', title: 'fbgroup.menu.groups' },
    loadComponent: () => import('@/modules/fbgroup/group-list/fbgroup-group-list').then((c) => c.FbGroupGroupList),
  },
  {
    path: 'topics',
    // Xem danh sách chủ đề chỉ cần quyền xem; nút sửa/xoá + khối kết nối NextCore tự ẩn khi thiếu manageFbGroupTopic.
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewFbGroup)],
    data: { breadcrumb: 'fbgroup.menu.topics', title: 'fbgroup.menu.topics' },
    loadComponent: () => import('@/modules/fbgroup/topics/fbgroup-topics').then((c) => c.FbGroupTopics),
  },
] as Routes;
