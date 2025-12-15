package notifiers

import (
	_ "embed"

	"github.com/dkar-dev/hitpipe/internal/domain/notification"
)

// Встраиваем HTML-шаблон прямо в бинарник при компиляции
// Путь относительно ЭТОГО файла (welcome_send_request.go)
var welcomeMessageTemplate string

func NewWelcomeSendRequest(email, token string) (*notification.SendRequest, error) {
	verificationLink := "http://localhost:8080/verify?token=" + token

	// Теперь передаём содержимое шаблона напрямую, а не путь к файлу
	htmlBody, err := notification.RenderTemplate(
		"welcome_message.html",
		notification.WelcomeEmailData{
			VerificationLink: verificationLink,
		},
	)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"From":         "hitpipe.app@gmail.com",
		"Subject":      "✨ Привет от HitPipe!",
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	message := ""
	for k, v := range headers {
		message += k + ": " + v + "\r\n"
	}
	message += "\r\n" + htmlBody

	return &notification.SendRequest{
		To:      []string{email},
		Message: []byte(message),
	}, nil
}
