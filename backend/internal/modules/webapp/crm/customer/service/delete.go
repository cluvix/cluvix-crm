package service

import (
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	history "erpCluvix/internal/modules/webapp/history/log/service"
)

// Story-36 (D15): KHÔNG còn tự chụp `company_id` vào log_customer — history.insertLogRow tự điền
// từ bản ghi gốc khi còn cột (schema cũ), story-40 bỏ hẳn phía history.

// nullBeforeDelete — NULL các FK vòng tròn và cross-customer trước khi xoá.
//
// appointment.order_service_used_id ↔ order_service_used.appointment_id đều RESTRICT
// → phải NULL 1 chiều trước, không thể xoá theo thứ tự bình thường.
var nullBeforeDelete = []string{
	`UPDATE appointment SET order_service_used_id = NULL WHERE customer_id = ?`,
}

// subTableDeletes — xoá / NULL các bảng KHÔNG có customer_id nhưng sẽ block xoá bảng cha.
// Thực hiện trước customerChildTables.
//
// KHÔNG có `payment` ở đây: khách còn phiếu thu/chi thì KHÔNG được xoá — crmService.DeleteCustomer chặn trước
// (user chốt 2026-11-01); FK payment.customer_id / serviceUsed_id / parent_id (RESTRICT) là lưới cuối.
var subTableDeletes = []string{
	// log tables
	`DELETE FROM log_order_service_used WHERE order_service_used_id IN (SELECT id FROM order_service_used WHERE customer_id = ?)`,
	`DELETE FROM log_appointment WHERE appointment_id IN (SELECT id FROM appointment WHERE customer_id = ?)`,
}

// childDeleteLogTables — dòng log XOÁ cũ của entity con (lịch hẹn/thanh toán/dòng dịch vụ đã
// bị xoá TRƯỚC khi xoá khách): fk đã NULL nên subTableDeletes (lọc theo fk IN …) không bắt được, chỉ
// nhận ra qua snapshot customer_id (history.LogDeleteSnapshot). Xoá khách ⇒ chỉ giữ 1 dòng log xoá
// khách (user chốt 2026-09-22), để lại thì các dòng này hiện cột khách trống. Không có index cho
// biểu thức JSON ⇒ quét toàn bảng log (~0.1–0.3s ở 153k dòng) — chấp nhận vì xoá khách hiếm.
var childDeleteLogTables = []struct{ table, fkCol string }{
	{"log_appointment", "appointment_id"},
	{"log_payment", "payment_id"},
	{"log_order_service_used", "order_service_used_id"},
}

// customerChildTables — bảng có customer_id, xoá theo dependency order.
// log_customer: các dòng log CŨ bị xoá (sau khi xoá khách, customer_id NULL ⇒ không còn biết là
// của ai, chỉ thành rác). Dòng log XOÁ được ghi SAU bước này nên sống sót — xem DeleteWithCleanup.
// Quy tắc: A phải xoá trước B nếu A.fk_col → B.id (RESTRICT).
var customerChildTables = []string{
	// ── Logs / audit ─────────────────────────────────────────────────────────
	"log_customer", "note",
	// ── Nhắc việc — chỉ phụ thuộc customer_id ────────────────────────────────
	"reminder",
	// ── Chuỗi chính (thứ tự bắt buộc) ───────────────────────────────────────
	"order_service_used", // trước order_plan
	"order_plan",         // trước appointment (order_plan.appointment_id RESTRICT)
	"appointment",        // sau khi order_plan đã xoá
}

// DeleteWithCleanup — DeleteWithCleanupTx trong transaction riêng. ⚠ KHÔNG xoá payment: khách còn
// phiếu thu/chi thì lỗi FK 1451. Đường xoá khách của handler là crmService.DeleteCustomer (chặn khách đã có
// đơn hàng / phiếu rồi mới gọi DeleteWithCleanupTx, cùng 1 tx).
func (s *CustomerService) DeleteWithCleanup(id uint, userID int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		return s.DeleteWithCleanupTx(tx, id, userID)
	})
}

// DeleteWithCleanupTx xoá toàn bộ dữ liệu liên quan của customer trên `tx` của caller.
// Không guard nghiệp vụ — tất cả FK được dọn sạch theo dependency order. Payment phải đã được xoá
// trước (xem subTableDeletes).
// Ghi 1 dòng log_customer action_type=delete (userID = người xoá) — sống sót nhờ FK SET NULL.
// Trả gorm.ErrRecordNotFound nếu customer không tồn tại.
func (s *CustomerService) DeleteWithCleanupTx(tx *gorm.DB, id uint, userID int) error {
	// ── Step 0: Snapshot trước khi dọn — nguồn duy nhất nhận diện khách sau khi xoá ──
	var snap map[string]interface{}
	if err := tx.Table("customer").Select(history.CustomerDeleteFields).Where("id = ?", id).Take(&snap).Error; err != nil {
		return err // gorm.ErrRecordNotFound đi thẳng lên handler → 404
	}

	// ── Step 1: NULL circular / cross-customer FKs ────────────────────────
	for _, q := range nullBeforeDelete {
		if err := tx.Exec(q, id).Error; err != nil {
			return fmt.Errorf("null fk: %w", err)
		}
	}

	// ── Step 2: Xoá bảng không có customer_id nhưng block bảng cha ────────
	for _, q := range subTableDeletes {
		if err := tx.Exec(q, id).Error; err != nil {
			return fmt.Errorf("clean sub-table: %w", err)
		}
	}

	// ── Step 2.1: Dòng log xoá cũ của entity con — xem childDeleteLogTables ──
	for _, l := range childDeleteLogTables {
		q := fmt.Sprintf("DELETE FROM `%s` WHERE %s IS NULL AND action_type = ? AND "+
			"JSON_VALUE(changed_fields, '$.customer_id.from' RETURNING UNSIGNED) = ?", l.table, l.fkCol)
		if err := tx.Exec(q, history.ActionDelete, id).Error; err != nil {
			return fmt.Errorf("clean %s delete rows: %w", l.table, err)
		}
	}

	// ── Step 3: Xoá bảng có customer_id theo dependency order ────────────
	for _, table := range customerChildTables {
		if err := tx.Exec("DELETE FROM `"+table+"` WHERE customer_id = ?", id).Error; err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 {
				continue // table không tồn tại ở DB này (legacy)
			}
			return fmt.Errorf("clean %s: %w", table, err)
		}
	}

	// ── Step 4.5: Log xoá — PHẢI sau Step 3 (đã xoá log_customer cũ) và trước Step 5 ──
	// Lỗi ghi log ⇒ rollback cả lần xoá: hard-delete không hoàn tác được, không để mất dấu vết.
	if err := history.LogDelete(tx, "log_customer", "customer_id", int(id), snap,
		history.CustomerDeleteFields, userID); err != nil {
		return err
	}

	// ── Step 5: Xoá customer (FK SET NULL gỡ customer_id trên dòng log xoá) ──
	res := tx.Exec("DELETE FROM `customer` WHERE id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
