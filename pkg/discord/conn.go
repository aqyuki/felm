package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/aqyuki/felm/pkg/logging"
	"github.com/aqyuki/felm/pkg/trace"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// MessageCreateHandler is a function that handles a message creation event.
type MessageCreateHandler func(context.Context, *discordgo.Session, *discordgo.MessageCreate) error

// MinimumHandlerTimeout is the minimum timeout for the handler.
const MinimumHandlerTimeout = 5 * time.Second

// Option is a function that configures a Conn.
type Option func(*Conn)

// WithHandlerTimeout sets the timeout for the handler.
func WithHandlerTimeout(timeout time.Duration) Option {
	return func(c *Conn) {
		if timeout < MinimumHandlerTimeout {
			timeout = MinimumHandlerTimeout
		}
		c.handlerDeadline = timeout
	}
}

// WithMessageCreateHandler adds a handler for the message creation event.
func WithMessageCreateHandler(handler MessageCreateHandler) Option {
	return func(c *Conn) {
		c.messageCreateHandlers = append(c.messageCreateHandlers, handler)
	}
}

// WithBaseContext sets the base context for the handler.
func WithBaseContext(ctx context.Context) Option {
	return func(c *Conn) {
		if ctx != nil {
			c.baseContext = ctx
		}
	}
}

// Conn manages the session with the Discord API.
type Conn struct {
	session               *discordgo.Session
	messageCreateHandlers []MessageCreateHandler
	preClose              []func()

	// handlerDeadline is the timeout for the handler.
	handlerDeadline time.Duration

	// baseContext is the base context for the handler.
	baseContext context.Context
}

func defaultConn() *Conn {
	return &Conn{
		session:               nil,
		messageCreateHandlers: make([]MessageCreateHandler, 0),
		handlerDeadline:       MinimumHandlerTimeout,
		baseContext:           context.Background(),
	}
}

func NewConn(token string, option ...Option) *Conn {
	conn := defaultConn()

	// discordgo#New does not return an error, so don't handle it.
	session, _ := discordgo.New("Bot " + token)
	conn.session = session

	// apply options
	for _, opt := range option {
		opt(conn)
	}
	return conn
}

// Open opens a connection to the Discord API.
func (c *Conn) Open() error {
	// register handlers and save the function to unregister them later.
	for _, handler := range c.messageCreateHandlers {
		fn := c.session.AddHandler(buildMessageCreateHandler(c.baseContext, handler))
		c.preClose = append(c.preClose, fn)
	}

	if err := c.session.Open(); err != nil {
		return fmt.Errorf("error was occurred when trying to connect to discord: %w", err)
	}
	return nil
}

// Close closes the connection to the Discord API.
func (c *Conn) Close() error {
	// unregister handlers
	for _, fn := range c.preClose {
		fn()
	}
	if err := c.session.Close(); err != nil {
		return fmt.Errorf("error was occurred when trying to disconnect from discord: %w", err)
	}
	return nil
}

// buildMessageCreateHandler creates a handler for the message creation event.
func buildMessageCreateHandler(ctx context.Context, handler MessageCreateHandler) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		start := time.Now()

		// attach trace id to the context
		ctx = trace.WithTraceID(ctx)
		traceID := trace.AcquireTraceID(ctx)

		// debug information
		logger := logging.FromContext(ctx)
		logger.Debug("MessageCreate event received",
			zap.String("trace_id", traceID),
			zap.Dict("message",
				zap.String("guild_id", m.GuildID),
				zap.String("channel_id", m.ChannelID),
				zap.String("message_id", m.ID),
				zap.String("author_id", m.Author.ID),
			),
		)

		// create a new context from the base context with the deadline.
		ctx, cancel := context.WithTimeout(ctx, MinimumHandlerTimeout)
		defer cancel()

		// execute the handler in a other goroutine.
		errCh := make(chan error)
		go func() {
			errCh <- handler(ctx, s, m)
		}()

		// wait for the handler to finish.
		// if the handler does not finish in time, cancel the context.
		select {
		case <-ctx.Done():
			logger.Warn("handler timed out",
				zap.String("trace_id", traceID),
			)
		case err := <-errCh:
			if err != nil {
				logger.Error("error occurred in handler",
					zap.String("trace_id", traceID),
					zap.Error(err),
				)
			}
		}

		// debug information
		latency := time.Since(start)
		logger.Debug("MessageCreate event handled",
			zap.String("trace_id", traceID),
			zap.Duration("latency", latency),
		)
	}
}
