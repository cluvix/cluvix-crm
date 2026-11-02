import {Routes} from '@angular/router';

export default [
  // story-20: relocated từ config-marketing (đã xoá — architecture.md §1 xoá module marketing,
  // nhưng customer_category/source/group_source/job/ethnicity/note_template nằm ở BE
  // `webapp/config` keep-list, KHÔNG thuộc marketing — chỉ từng bị gom nhầm thư mục).
  {
    path: 'customer-category',
    loadComponent: () =>
      import('./customer-category/list/customer-category-list').then((c) => c.CustomerCategoryList),
    data: { breadcrumb: 'common.label.customer_category' },
  },
  {
    path: 'customer-source',
    loadComponent: () => import('./source/list/source-list').then((c) => c.SourceList),
    data: { breadcrumb: 'common.label.source' },
  },
  {
    path: 'group-source',
    loadComponent: () =>
      import('./group-source/list/group-source-list').then((c) => c.GroupSourceList),
    data: { breadcrumb: 'common.label.customer_source_group' },
  },
  {
    path: 'customer-job',
    loadComponent: () => import('./job/job-list/job-list').then((c) => c.JobList),
    data: { breadcrumb: 'config.nav.marketing.customer_job' },
  },
  {
    path: 'customer-ethnicity',
    loadComponent: () =>
      import('./ethnicity/ethnicity-list/ethnicity-list').then((c) => c.EthnicityList),
    data: { breadcrumb: 'config.nav.marketing.customer_ethnicity' },
  },
  {
    path: 'note-template',
    loadComponent: () =>
      import('./note-template/list/note-template-list').then((c) => c.NoteTemplateList),
    data: { breadcrumb: 'config.nav.marketing.note_template' },
  },
] as Routes;
