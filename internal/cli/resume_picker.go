package cli

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"reasonix/internal/agent"
	"reasonix/internal/i18n"
)

// resumePicker is an in-chat overlay for "/resume" that lets the user pick a
// saved session by navigating with ↑/↓ and confirming with Enter. It mirrors
// the rewindPicker pattern: keys route through handleResumePickerKey and it
// renders via renderResumePicker while m.resumePick is set.
type resumePicker struct {
	sessions []agent.SessionInfo
	sel      int // selected index
	active   int // index of the currently-active session (-1 when none)
}

// openResumePicker populates the picker from the session directory and opens it.
// A no-op (with a notice) when there are no saved sessions.
func (m *chatTUI) openResumePicker() {
	sessions := recentSessions(m.ctrl.SessionDir())
	if len(sessions) == 0 {
		m.notice(i18n.M.NoSessionToResume)
		return
	}
	active := m.ctrl.SessionPath()
	activeIdx := -1
	for i, s := range sessions {
		if s.Path == active {
			activeIdx = i
			break
		}
	}
	// Default selection: the first session after the active one, else 0.
	sel := 0
	if activeIdx >= 0 && activeIdx+1 < len(sessions) {
		sel = activeIdx + 1
	}
	m.resumePick = &resumePicker{sessions: sessions, sel: sel, active: activeIdx}
}

func (m chatTUI) handleResumePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := m.resumePick
	if r == nil {
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		if r.sel > 0 {
			r.sel--
		}
	case "down", "j":
		if r.sel < len(r.sessions)-1 {
			r.sel++
		}
	case "x":
		return m.deleteResumePickEntry()
	case "enter":
		return m.applyResumePick()
	case "esc":
		m.resumePick = nil
	}
	return m, nil
}

// deleteResumePickEntry moves the selected session into the local trash
// (recoverable from <sessionDir>/.trash/<key>/) and removes it from the
// picker list. It refuses to delete the active session or act while a turn
// is in flight, in line with applyResumePick. The picker stays open so the
// user can keep navigating; if the list becomes empty it closes.
func (m chatTUI) deleteResumePickEntry() (tea.Model, tea.Cmd) {
	r := m.resumePick
	if r == nil || r.sel < 0 || r.sel >= len(r.sessions) {
		return m, nil
	}
	target := r.sessions[r.sel]
	if target.Path == m.ctrl.SessionPath() || r.sel == r.active {
		m.notice(i18n.M.ResumePickCannotDeleteActive)
		return m, nil
	}
	if m.ctrl.Running() {
		m.notice(i18n.M.ResumeBusy)
		return m, nil
	}
	if err := m.ctrl.DeleteSession(target.Path); err != nil {
		m.notice(fmt.Sprintf(i18n.M.ResumePickDeleteFailedFmt, err))
		return m, nil
	}
	m.notice(fmt.Sprintf(i18n.M.ResumePickDeletedFmt, sessionPickerLabel(target)))

	// Drop the entry and adjust the cursor + active index.
	r.sessions = append(r.sessions[:r.sel], r.sessions[r.sel+1:]...)
	if r.active > r.sel {
		r.active--
	}
	if len(r.sessions) == 0 {
		m.resumePick = nil
		return m, nil
	}
	if r.sel >= len(r.sessions) {
		r.sel = len(r.sessions) - 1
	}
	// If the active row was removed (it shouldn't be — we refuse to delete
	// the active one above — but guard anyway), leave the cursor on sel=0
	// so the user lands on a real entry.
	if r.active < 0 {
		r.active = -1
	}
	return m, nil
}

func (m chatTUI) applyResumePick() (tea.Model, tea.Cmd) {
	r := m.resumePick
	if r == nil || r.sel < 0 || r.sel >= len(r.sessions) {
		return m, nil
	}
	target := r.sessions[r.sel]
	m.resumePick = nil
	if target.Path == m.ctrl.SessionPath() {
		m.notice(i18n.M.ResumeAlreadyActive)
		return m, nil
	}
	if m.ctrl.Running() {
		m.notice(i18n.M.ResumeBusy)
		return m, nil
	}
	loaded, err := agent.LoadSession(target.Path)
	if err != nil {
		m.notice("resume: " + err.Error())
		return m, nil
	}
	_ = m.ctrl.Snapshot()
	m.ctrl.Resume(loaded, target.Path)
	m.replayActiveBranch(i18n.M.ResumedTitle)
	return m, nil
}

func (m chatTUI) renderResumePicker() string {
	r := m.resumePick
	if r == nil {
		return ""
	}
	w := max(m.width, 10)
	var b strings.Builder
	b.WriteString(accent(i18n.M.ResumePickTitle) + "\n")
	for i, s := range r.sessions {
		label := sessionPickerLabel(s)
		if i == r.active {
			label = dim(label) + " " + dim("(active)")
		}
		b.WriteString(rowLine(i == r.sel, i+1, "", label, false) + "\n")
	}
	b.WriteString(dim(i18n.M.ResumePickHint))
	return choicePanelStyle.Width(w).Render(b.String())
}

// sessionPickerLabel is the "N turns · topicTitle/first message" line, truncated to fit.
// When a TopicTitle is set (via /rename or desktop), it is shown instead of the raw preview.
func sessionPickerLabel(s agent.SessionInfo) string {
	preview := s.Preview
	if s.TopicTitle != "" {
		preview = s.TopicTitle
	}
	if preview == "" {
		preview = "(no user message yet)"
	}
	return fmt.Sprintf("%d turns · %s", s.Turns, ansi.Truncate(preview, 60, "…"))
}
