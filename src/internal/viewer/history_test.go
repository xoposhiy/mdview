package viewer

import "testing"

func TestHistoryPopReturnsEntriesInReverseOrder(t *testing.T) {
	var h history
	h.push("a.md", 10)
	h.push("b.md", 20)

	got, ok := h.pop()
	if !ok {
		t.Fatal("pop() on 2-entry history returned ok=false")
	}
	if got.path != "b.md" || got.scrollY != 20 {
		t.Errorf("first pop = %+v, want {b.md 20}", got)
	}

	got, ok = h.pop()
	if !ok {
		t.Fatal("pop() on 1-entry history returned ok=false")
	}
	if got.path != "a.md" || got.scrollY != 10 {
		t.Errorf("second pop = %+v, want {a.md 10}", got)
	}
}

func TestHistoryPopOnEmptyReportsNotOk(t *testing.T) {
	var h history

	got, ok := h.pop()
	if ok {
		t.Errorf("pop() on empty history returned ok=true, entry %+v", got)
	}
}

func TestHistoryCanGoBack(t *testing.T) {
	var h history
	if h.canGoBack() {
		t.Error("canGoBack() on empty history = true, want false")
	}

	h.push("a.md", 0)
	if !h.canGoBack() {
		t.Error("canGoBack() after push = false, want true")
	}

	h.pop()
	if h.canGoBack() {
		t.Error("canGoBack() after popping the only entry = true, want false")
	}
}
