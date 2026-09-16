package mail

import (
	"fmt"
	"mime"
	"strings"
	"time"
)

// Notification is one event, in words, before it is addressed to anyone.
type Notification struct {
	Event          string
	Subject        string
	Lines          []string
	Path           string
	PresentationID string
}

// Render is the plain-text body: the lines, then a link when the deployment
// knows its own address. Plain text, because a company relay and a company
// mail client both take it, and because nobody wants HTML from a robot.
func (n Notification) Render(config Config) string {
	body := strings.Join(n.Lines, "\n")
	if base := strings.TrimSpace(config.BaseURL); base != "" && n.Path != "" {
		body += "\n\n" + base + n.Path
	}
	return body + "\n\n— " + firstNonEmpty(config.FromName, defaultFromName) + " 알림\n"
}

// GenerationCompleted is the deck somebody asked for being done.
func GenerationCompleted(title, presentationID string, slides int) Notification {
	return Notification{
		Event:          EventGenerationCompleted,
		Subject:        fmt.Sprintf("[Ptium] 덱이 완성되었습니다: %s", shorten(title, 80)),
		Lines:          []string{fmt.Sprintf("요청하신 덱 \"%s\" 이(가) 완성되었습니다 (슬라이드 %d장).", title, slides), "편집기에서 열어 확인하세요."},
		Path:           "/presentations/" + presentationID + "/editor",
		PresentationID: presentationID,
	}
}

// GenerationFailed is writing or rewriting a deck stopping, with what the
// author was told — never the operator's cause, which may name an internal
// service.
func GenerationFailed(title, presentationID, said string) Notification {
	return Notification{
		Event:          EventGenerationFailed,
		Subject:        fmt.Sprintf("[Ptium] 덱 생성이 멈췄습니다: %s", shorten(title, 80)),
		Lines:          []string{fmt.Sprintf("덱 \"%s\" 을(를) 만들지 못했습니다.", title), strings.TrimSpace(said), "편집기에서 다시 시도할 수 있습니다."},
		Path:           "/presentations/" + presentationID + "/editor",
		PresentationID: presentationID,
	}
}

// CommentLeft is a reviewer saying something about a deck through its share
// link. The remark itself stays out of the mail: it is one click away, and a
// mail is a copy that outlives the deck.
func CommentLeft(title, presentationID, author string, slide int) Notification {
	who := strings.TrimSpace(author)
	if who == "" {
		who = "익명"
	}
	where := "덱"
	if slide > 0 {
		where = fmt.Sprintf("%d번 슬라이드", slide)
	}
	return Notification{
		Event:          EventComment,
		Subject:        fmt.Sprintf("[Ptium] %s 님이 \"%s\" 에 의견을 남겼습니다", shorten(who, 40), shorten(title, 60)),
		Lines:          []string{fmt.Sprintf("%s 님이 \"%s\" 의 %s에 의견을 남겼습니다.", who, title, where), "편집기의 의견 탭에서 읽고 답할 수 있습니다."},
		Path:           "/presentations/" + presentationID + "/editor",
		PresentationID: presentationID,
	}
}

// TestMessage proves the relay works before anything depends on it.
func TestMessage(at time.Time) Notification {
	return Notification{
		Event:   EventTest,
		Subject: "[Ptium] 메일 알림 시험 발송",
		Lines: []string{"이 메일이 도착했다면 SMTP 릴레이 설정이 맞습니다.",
			"보낸 시각: " + at.Format("2006-01-02 15:04:05 MST")},
	}
}

// compose writes the RFC 5322 message: the subject encoded so Korean survives
// a relay that folds headers, and a body in UTF-8 as it is.
func compose(config Config, message Message) string {
	var builder strings.Builder
	builder.WriteString("From: " + config.Address() + "\r\n")
	builder.WriteString("To: " + strings.TrimSpace(message.To) + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", message.Subject) + "\r\n")
	builder.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	builder.WriteString("Auto-Submitted: auto-generated\r\n")
	builder.WriteString("\r\n")
	// A line that is only a dot ends the DATA dialogue; the client library
	// escapes it, but a bare LF in the body is not a line at all to SMTP.
	builder.WriteString(strings.ReplaceAll(strings.ReplaceAll(message.Body, "\r\n", "\n"), "\n", "\r\n"))
	return builder.String()
}

func shorten(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
