package tools

// SendMessageFunc is a callback for sending mid-conversation messages.
type SendMessageFunc func(chatID int64, channel, message string) error
