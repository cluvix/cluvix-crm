package service

import "testing"

// Story-12 (customer-stage-pipeline, AC1): cột `customer.status`/`is_patient` đã bỏ ⇒ form tạo/sửa khách gửi
// 2 khoá này phải bị whitelist loại (không thì INSERT/UPDATE lỗi Unknown column); `stage_id` cũng KHÔNG
// được đi qua form (đổi bước chỉ qua ChangeStage).
func TestFilterAllowed_DropsLegacyProgressAndStage(t *testing.T) {
	s := &CustomerService{}
	got := s.FilterAllowed(map[string]interface{}{
		"fullname":   "Nguyễn Văn A",
		"status":     12,
		"is_patient": 1,
		"stage_id":   3,
	})
	for _, k := range []string{"status", "is_patient", "stage_id"} {
		if _, ok := got[k]; ok {
			t.Errorf("FilterAllowed giữ lại khoá %q — phải bị loại", k)
		}
	}
	if got["fullname"] != "Nguyễn Văn A" {
		t.Errorf("FilterAllowed làm mất field hợp lệ: %v", got)
	}
}
