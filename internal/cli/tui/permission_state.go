package tui

import (
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type PermissionState struct {
	Active       bool
	Request      protocol.PermissionRequest
	Selection    int
	FeedbackMode string
	Feedback     string
	DetailsOpen  bool

	requestKey string
}

func (s *PermissionState) Sync(active bool, request protocol.PermissionRequest) {
	if !active {
		*s = PermissionState{}
		return
	}
	key := permissionRequestKey(request)
	if !s.Active || s.requestKey != key {
		s.Selection = 0
		s.FeedbackMode = ""
		s.Feedback = ""
		s.DetailsOpen = false
	}
	s.Active = true
	s.Request = request
	s.requestKey = key
	s.Selection = clamp(s.Selection, 0, len(s.Options())-1)
}

func (s PermissionState) Options() []protocol.PermissionOption {
	return s.Request.Options
}

func (s PermissionState) SelectedOption() (protocol.PermissionOption, bool) {
	options := s.Options()
	if len(options) == 0 {
		return protocol.PermissionOption{}, false
	}
	return options[clamp(s.Selection, 0, len(options)-1)], true
}

func (s *PermissionState) MoveSelection(delta int) {
	options := s.Options()
	if len(options) == 0 {
		s.Selection = 0
		return
	}
	next := s.Selection + delta
	if next < 0 {
		next = len(options) - 1
	}
	if next >= len(options) {
		next = 0
	}
	s.Selection = next
	s.FeedbackMode = ""
}

func (s *PermissionState) SelectValue(value string) {
	for i, option := range s.Options() {
		if option.Value == value {
			s.Selection = i
			return
		}
	}
	s.Selection = 0
}

func (s *PermissionState) ToggleFeedback() bool {
	option, ok := s.SelectedOption()
	if !ok || strings.TrimSpace(option.Feedback) == "" {
		return false
	}
	if s.FeedbackMode == option.Feedback {
		s.FeedbackMode = ""
		return true
	}
	s.FeedbackMode = option.Feedback
	return true
}

func (s *PermissionState) CycleScope(acceptFeedback string) bool {
	options := s.Options()
	if len(options) == 0 {
		return false
	}
	current := options[clamp(s.Selection, 0, len(options)-1)]
	if current.Feedback == acceptFeedback {
		for i := 1; i <= len(options); i++ {
			next := (s.Selection + i) % len(options)
			if options[next].Feedback == acceptFeedback && options[next].Scope != current.Scope {
				s.Selection = next
				s.FeedbackMode = ""
				return true
			}
		}
	}
	for i := 1; i <= len(options); i++ {
		next := (s.Selection + i) % len(options)
		if options[next].Value == current.Value && options[next].Scope != current.Scope {
			s.Selection = next
			s.FeedbackMode = ""
			return true
		}
	}
	return false
}

func (s *PermissionState) AppendText(text string) {
	s.Feedback += text
}

func (s *PermissionState) Backspace() {
	if len(s.Feedback) == 0 {
		return
	}
	runes := []rune(s.Feedback)
	s.Feedback = string(runes[:len(runes)-1])
}

func (s *PermissionState) Newline() {
	s.Feedback += "\n"
}

func (s *PermissionState) CancelFeedback() {
	s.FeedbackMode = ""
	s.Feedback = ""
}

func (s *PermissionState) ConsumeFeedback() string {
	feedback := strings.TrimSpace(s.Feedback)
	s.Feedback = ""
	s.FeedbackMode = ""
	return feedback
}

func permissionRequestKey(request protocol.PermissionRequest) string {
	parts := []string{
		request.RequestID,
		request.InterruptID,
		request.PlanID,
		request.NodeID,
		request.Tool,
		request.Operation,
		request.Title,
		request.Subtitle,
		request.Question,
		request.Summary,
		request.TargetPath,
		request.Command,
		request.Preview.Kind,
		request.Preview.Summary,
		request.Preview.Diff,
		request.Preview.OldContentHash,
		request.Preview.NewContent,
		request.Preview.CommandPrefix,
		request.Preview.ConflictMessage,
		request.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	return strings.Join(parts, "\x00")
}
