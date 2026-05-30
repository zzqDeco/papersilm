package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type MessageRenderFunc func(index int, width int) string

type MessageListModel struct {
	viewport   viewport.Model
	cache      MessageViewport
	autoScroll bool
	unread     int
}

func NewMessageListModel(vp viewport.Model, cache MessageViewport) MessageListModel {
	return MessageListModel{
		viewport:   vp,
		cache:      cache,
		autoScroll: true,
	}
}

func (m MessageListModel) Viewport() viewport.Model {
	return m.viewport
}

func (m MessageListModel) Cache() MessageViewport {
	return m.cache
}

func (m *MessageListModel) SetViewport(vp viewport.Model) {
	m.viewport = vp
}

func (m *MessageListModel) SetCache(cache MessageViewport) {
	m.cache = cache
}

func (m MessageListModel) View() string {
	return m.viewport.View()
}

func (m MessageListModel) Width() int {
	return m.viewport.Width
}

func (m MessageListModel) Height() int {
	return m.viewport.Height
}

func (m *MessageListModel) SetSize(width, height int) {
	m.viewport.Width = width
	m.viewport.Height = height
}

func (m *MessageListModel) SetMouseWheelEnabled(enabled bool) {
	m.viewport.MouseWheelEnabled = enabled
}

func (m *MessageListModel) SetContentByKeyVersion(width int, keys []string, versions []string, render MessageRenderFunc) string {
	content := m.cache.ContentByKeyVersion(width, keys, versions, render)
	m.viewport.SetContent(content)
	return content
}

func (m *MessageListModel) ReplaceLastByKey(width int, key string, content string, fallback func() string) {
	if rendered, ok := m.cache.ReplaceLastByKey(width, key, content); ok {
		m.viewport.SetContent(rendered)
		return
	}
	if fallback != nil {
		m.viewport.SetContent(fallback())
	}
}

func (m *MessageListModel) AnchorAt(offset int) (ViewportAnchor, bool) {
	return m.cache.AnchorAt(offset)
}

func (m *MessageListModel) OffsetForAnchor(anchor ViewportAnchor) (int, bool) {
	return m.cache.OffsetForAnchor(anchor)
}

func (m *MessageListModel) ResetCache() {
	m.cache.Reset()
}

func (m *MessageListModel) GotoBottom() {
	m.viewport.GotoBottom()
	m.autoScroll = true
	m.unread = 0
}

func (m *MessageListModel) AtBottom() bool {
	return m.viewport.AtBottom()
}

func (m *MessageListModel) AutoScroll() bool {
	return m.autoScroll
}

func (m *MessageListModel) SetAutoScroll(autoScroll bool) {
	m.autoScroll = autoScroll
	if autoScroll {
		m.unread = 0
	}
}

func (m *MessageListModel) Unread() int {
	return m.unread
}

func (m *MessageListModel) SetUnread(unread int) {
	m.unread = max(0, unread)
}

func (m *MessageListModel) IncrementUnread() {
	if !m.autoScroll {
		m.unread++
	}
}

func (m *MessageListModel) SetYOffset(offset int) {
	m.viewport.SetYOffset(offset)
}

func (m MessageListModel) YOffset() int {
	return m.viewport.YOffset
}

func (m MessageListModel) TotalLineCount() int {
	return m.viewport.TotalLineCount()
}

func (m *MessageListModel) Update(msg tea.Msg) (MessageListModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	m.autoScroll = m.viewport.AtBottom()
	if m.autoScroll {
		m.unread = 0
	}
	return *m, cmd
}
