package tui

func keepVisible(offset, cursor, visible, total int) int {
	visible = max(visible, 1)
	offset = max(min(offset, total-visible), 0)
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return offset
}

func (m Model) scrolled() Model {
	m.offset = keepVisible(m.offset, m.cursor, m.visibleRows(), len(m.rows()))
	if current, ok := m.accessShare(); ok {
		m.access.offset = keepVisible(m.access.offset, m.access.cursor, m.visibleAccesses(), len(current.accesses))
	}
	return m
}
