package ai

import "context"

// StreamProvider is an optional interface. Providers may implement streaming chat.
type StreamProvider interface {
	StreamChat(ctx context.Context, messages []Message) (<-chan string, <-chan error)
}
