package handler

import "testing"

func TestResolveUnitImportColumns(t *testing.T) {
	t.Run("header mới (sau khi bỏ cột Loại)", func(t *testing.T) {
		cols, ok := resolveUnitImportColumns([]string{
			"ID", "Tên đơn vị", "Mô tả", "Danh mục (categoryId)", "Tên danh mục", "Vị trí", "Trạng thái", "Trạng thái (text)",
		})
		if !ok {
			t.Fatal("ok = false, muốn true")
		}
		want := unitImportCols{id: 0, name: 1, description: 2, categoryID: 3, sortOrder: 5, status: 6}
		if cols != want {
			t.Fatalf("cols = %+v, muốn %+v", cols, want)
		}
	})

	t.Run("header cũ còn cột Loại ở vị trí 2 — field vẫn map đúng cột", func(t *testing.T) {
		cols, ok := resolveUnitImportColumns([]string{
			"ID", "Tên đơn vị", "Loại", "Mô tả", "Danh mục (categoryId)", "Tên danh mục", "Vị trí", "Trạng thái", "Trạng thái (text)",
		})
		if !ok {
			t.Fatal("ok = false, muốn true")
		}
		want := unitImportCols{id: 0, name: 1, description: 3, categoryID: 4, sortOrder: 6, status: 7}
		if cols != want {
			t.Fatalf("cols = %+v, muốn %+v", cols, want)
		}
	})

	t.Run("thiếu cột Tên đơn vị", func(t *testing.T) {
		_, ok := resolveUnitImportColumns([]string{"ID", "Mô tả", "Vị trí", "Trạng thái"})
		if ok {
			t.Fatal("ok = true, muốn false khi thiếu Tên đơn vị")
		}
	})

	t.Run("nhãn có khoảng trắng thừa, cột tuỳ chọn vắng = -1", func(t *testing.T) {
		cols, ok := resolveUnitImportColumns([]string{" Tên đơn vị ", "Trạng thái"})
		if !ok {
			t.Fatal("ok = false, muốn true")
		}
		want := unitImportCols{id: -1, name: 0, description: -1, categoryID: -1, sortOrder: -1, status: 1}
		if cols != want {
			t.Fatalf("cols = %+v, muốn %+v", cols, want)
		}
	})

	t.Run("nhãn trùng lấy cột đầu", func(t *testing.T) {
		cols, _ := resolveUnitImportColumns([]string{"Tên đơn vị", "Tên đơn vị"})
		if cols.name != 0 {
			t.Fatalf("name = %d, muốn 0", cols.name)
		}
	})
}

func TestUnitImportColsCell(t *testing.T) {
	var cols unitImportCols
	row := []string{"a", "b"}
	if got := cols.cell(row, 1); got != "b" {
		t.Fatalf("cell(1) = %q", got)
	}
	if got := cols.cell(row, 5); got != "" {
		t.Fatalf("cell ngoài dòng = %q, muốn rỗng", got)
	}
	if got := cols.cell(row, -1); got != "" {
		t.Fatalf("cell cột vắng = %q, muốn rỗng", got)
	}
}
