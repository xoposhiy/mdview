package viewer

// histEntry records a document and the scroll position it was left at.
type histEntry struct {
	path    string
	scrollY int
}

// history is a stack of visited documents, most recently visited last.
type history []histEntry

func (h *history) push(path string, scrollY int) {
	*h = append(*h, histEntry{path: path, scrollY: scrollY})
}

// pop removes and returns the most recent entry. ok is false if empty.
func (h *history) pop() (histEntry, bool) {
	if len(*h) == 0 {
		return histEntry{}, false
	}
	last := (*h)[len(*h)-1]
	*h = (*h)[:len(*h)-1]
	return last, true
}

func (h history) canGoBack() bool { return len(h) > 0 }
