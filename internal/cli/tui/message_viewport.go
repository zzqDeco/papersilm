package tui

import "strings"

type MessageViewport struct {
	width    int
	rendered []string
	keys     []string
}

func (v *MessageViewport) Reset() {
	v.width = 0
	v.rendered = nil
	v.keys = nil
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
	v.keys = nil
	v.rendered = make([]string, 0, count)
	for i := 0; i < count; i++ {
		v.rendered = append(v.rendered, render(i, width))
	}
	return strings.Join(v.rendered, "\n\n")
}

func (v *MessageViewport) ContentByKey(width int, keys []string, render func(index int, width int) string) string {
	if len(keys) == 0 {
		v.Reset()
		return ""
	}
	if v.width == width && sameStrings(v.keys, keys) && len(v.rendered) == len(keys) {
		return strings.Join(v.rendered, "\n\n")
	}
	previous := make(map[string]string, len(v.keys))
	if v.width == width {
		for i, key := range v.keys {
			if key == "" || i >= len(v.rendered) {
				continue
			}
			previous[key] = v.rendered[i]
		}
	}
	v.width = width
	v.keys = append(v.keys[:0], keys...)
	v.rendered = make([]string, 0, len(keys))
	for i, key := range keys {
		if cached, ok := previous[key]; ok {
			v.rendered = append(v.rendered, cached)
			continue
		}
		v.rendered = append(v.rendered, render(i, width))
	}
	return strings.Join(v.rendered, "\n\n")
}

func (v *MessageViewport) Append(width int, previousCount int, rendered string) (string, bool) {
	if previousCount < 0 {
		return "", false
	}
	if v.width != width || len(v.rendered) != previousCount || len(v.keys) > 0 {
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

func (v *MessageViewport) ReplaceLastByKey(width int, key string, rendered string) (string, bool) {
	if key == "" || v.width != width || len(v.rendered) == 0 || len(v.rendered) != len(v.keys) {
		return "", false
	}
	if v.keys[len(v.keys)-1] != key {
		return "", false
	}
	v.rendered[len(v.rendered)-1] = rendered
	return strings.Join(v.rendered, "\n\n"), true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
