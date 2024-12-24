package handler

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aqyuki/felm/internal/app/rule"
	"github.com/aqyuki/felm/pkg/discord"
	"github.com/aqyuki/felm/pkg/logging"
	"github.com/aqyuki/felm/pkg/trace"
	"github.com/bwmarrin/discordgo"
	"github.com/samber/oops"
	"go.uber.org/zap"
)

var _ discord.MessageCreateHandler = ExpandMessageLink

var ErrMessageLinkNotFound = errors.New("message link not found")

func ExpandMessageLink(ctx context.Context, session *discordgo.Session, message *discordgo.MessageCreate) error {
	logger := logging.FromContext(ctx)

	logger.Info("handler called",
		zap.String("trace_id", trace.AcquireTraceID(ctx)),
		zap.Dict("message",
			zap.String("guild_id", message.GuildID),
			zap.String("channel_id", message.ChannelID),
			zap.String("message_id", message.ID),
			zap.Dict("author",
				zap.String("id", message.Author.ID),
				zap.String("username", message.Author.Username),
				zap.Bool("is_bot", message.Author.Bot),
			)))

	if rule.IsBot(message.Author) {
		logger.Debug("skip processing message because it was sent by bot")
		return nil
	}

	ids, err := parseMessageLink(message.Content)
	if err != nil {
		if errors.Is(err, ErrMessageLinkNotFound) {
			logger.Debug("skip processing message because message link not found")
			return nil
		}
		return oops.
			Trace(trace.AcquireTraceID(ctx)).
			With("message",
				oops.With("guild_id", message.GuildID),
				oops.With("channel_id", message.ChannelID),
				oops.With("message_id", message.ID)).
			Wrapf(err, "error occurred while parsing message link (message_id = %s)", message.ID)
	}

	logger.Info("message link detected",
		zap.Dict("message_link",
			zap.String("guild_id", ids.guildID),
			zap.String("channel_id", ids.channelID),
			zap.String("message_id", ids.messageID)))

	if !rule.IsSameGuild(ids.guildID, message) {
		logger.Debug("skip processing message because it was sent from different guild")
		return nil
	}

	sourceChannel, err := session.Channel(message.ChannelID)
	if err != nil {
		return oops.
			Trace(trace.AcquireTraceID(ctx)).
			With("message_detail",
				oops.With("guild_id", message.GuildID),
				oops.With("channel_id", message.ChannelID),
				oops.With("message_id", message.ID)).
			Wrapf(err, "error occurred while fetching channel information (channel_id = %s)", message.ChannelID)
	}

	if rule.IsNSFW(sourceChannel) {
		logger.Debug("skip processing message because it was sent from NSFW channel", zap.String("message_id", message.ID))
		return nil
	}

	sourceMessage, err := session.ChannelMessage(ids.channelID, ids.messageID)
	if err != nil {
		return oops.
			Trace(trace.AcquireTraceID(ctx)).
			With("message_detail",
				oops.With("guild_id", message.GuildID),
				oops.With("channel_id", message.ChannelID),
				oops.With("message_id", message.ID)).
			Wrapf(err, "error occurred while fetching message information (channel_id = %s, message_id = %s)", ids.channelID, ids.messageID)
	}

	if !rule.IsExpandable(sourceMessage) {
		logger.Debug("skip processing message because it was not expandable", zap.String("message_id", message.ID))
		return nil
	}

	embed := emptyEmbed(sourceChannel, sourceMessage)
	if rule.HasContent(sourceMessage) {
		embed.Description = sourceMessage.Content
	}
	if rule.HasImage(sourceMessage) {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: sourceMessage.Attachments[0].URL,
		}
	}

	replyMsg := &discordgo.MessageSend{
		Embed:           embed,
		Reference:       message.Reference(),
		AllowedMentions: &discordgo.MessageAllowedMentions{RepliedUser: true},
	}
	if _, err := session.ChannelMessageSendComplex(message.ChannelID, replyMsg); err != nil {
		return oops.
			Trace(trace.AcquireTraceID(ctx)).
			With("message_detail",
				oops.With("guild_id", message.GuildID),
				oops.With("channel_id", message.ChannelID),
				oops.With("message_id", message.ID)).
			Wrapf(err, "error occurred while sending message (channel_id = %s)", message.ChannelID)
	}
	return nil
}

var messageRegex = regexp.MustCompile(`https://(?:ptb\.|canary\.)?discord\.com/channels/(?P<guild_id>\d+)/(?P<channel_id>\d+)/(?P<message_id>\d+)`)

type messageLink struct {
	guildID   string
	channelID string
	messageID string
}

func parseMessageLink(message string) (*messageLink, error) {
	matches := messageRegex.FindStringSubmatch(message)
	if len(matches) == 0 {
		return nil, ErrMessageLinkNotFound
	}

	return &messageLink{
		guildID:   matches[messageRegex.SubexpIndex("guild_id")],
		channelID: matches[messageRegex.SubexpIndex("channel_id")],
		messageID: matches[messageRegex.SubexpIndex("message_id")],
	}, nil
}

func emptyEmbed(channel *discordgo.Channel, message *discordgo.Message) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    message.Author.Username,
			IconURL: message.Author.AvatarURL(""),
		},
		Color:     0x7fffff,
		Timestamp: message.Timestamp.Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("from %s", channel.Name),
		},
	}
}
