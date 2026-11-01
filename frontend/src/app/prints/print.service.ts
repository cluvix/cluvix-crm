import { inject, Service} from '@angular/core';
import { HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { ApiService } from '@/share/services/api.service';

@Service()
export class PrintService {
  private api = inject(ApiService);

  laboOrder(id: number): Observable<any> {
    return this.api.get(`print/labo-order/${id}`);
  }

  orderPlan(id: number): Observable<any> {
    return this.api.get(`print/order-plan/${id}`);
  }

  payment(id: number): Observable<any> {
    return this.api.get(`print/payment/${id}`);
  }

  deposit(id: number): Observable<any> {
    return this.api.get(`print/deposit/${id}`);
  }

  refundDeposit(id: number): Observable<any> {
    return this.api.get(`print/refund-deposit/${id}`);
  }

  refundService(id: number): Observable<any> {
    return this.api.get(`print/refund-service/${id}`);
  }

  multiPayment(params: { ids: number[] | string; customerId?: number | string }): Observable<any> {
    const idsString = Array.isArray(params.ids) ? params.ids.join(',') : params.ids;
    let httpParams = new HttpParams().set('ids', idsString);
    if (params.customerId) httpParams = httpParams.set('customerId', String(params.customerId));
    return this.api.get('print/multi-payment', { params: httpParams });
  }

  receptionPayment(params: any = {}): Observable<any> {
    return this.api.get('print/reception-payment', { params });
  }

  paymentInDay(options: any = {}): Observable<any> {
    return this.api.get('print/payment-in-day', options);
  }

  prescription(id: number): Observable<any> {
    return this.api.get(`print/prescription/${id}`).pipe(
      map((res: any) => res?.data ?? res),
    );
  }

  treatment(params: { idCustomer: number; [key: string]: any } = { idCustomer: 0 }): Observable<any> {
    const { idCustomer, ...rest } = params;
    let httpParams = new HttpParams();
    Object.entries(rest).forEach(([k, v]) => {
      if (v !== undefined && v !== null) httpParams = httpParams.set(k, String(v));
    });
    return this.api.get(`print/treatment/${idCustomer}`, { params: httpParams });
  }

  appointmentByhourList(params: any = {}): Observable<any> {
    return this.api.get('print/appointment-byhour-list', params);
  }

  appointmentBydoctorList(params: any = {}): Observable<any> {
    return this.api.get('print/appointment-bydoctor-list', params);
  }
}
