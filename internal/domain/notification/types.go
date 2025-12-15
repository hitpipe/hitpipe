package notification

type SendRequest struct {
	Message []byte
	To      []string
}

type WelcomeEmailData struct {
	VerificationLink string
}
