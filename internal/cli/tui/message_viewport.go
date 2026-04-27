package tui

import "strings"

type MessageViewport struct {
	width    int
	rendered []string
}

func (v *MessageViewport) Reset() {
	v.width = 0
	v.rendered = nil
}

func (v *MessageViewport) Content(width, count int, render func(index int, width int) string) string {
	if count <= 0 {
		v.Reset()
		return ""
	}
	if v.width == width && len(v.rendered) == count {
		return strings.Join(v.rendered, "\n\n")
	}
	v.width = width
	v.rendered = make([]string, 0, count)
	for i := 0; i < count; i++ {
		v.rendered = append(v.rendered, render(i, width))
	}
	return strings.Join(v.rendered, "\n\n")
}

func (v *MessageViewport) Append(width int, previousCount int, rendered string) (string, bool) {
	if previousCount < 0 {
		return "", false
	}
	if v.width != width || len(v.rendered) != previousCount {
		return "", false
	}
	v.rendered = append(v.rendered, rendered)
	return strings.Join(v.rendered, "\n\n"), true
}

func (v *MessageViewport) ReplaceLast(width int, count int, rendered string) (string, bool) {
	if count <= 0 || v.width != width || len(v.rendered) != count {
		return "", false
	}
	v.rendered[count-1] = rendered
	return strings.Join(v.rendered, "\n\n"), true
}
