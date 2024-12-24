package handler

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aqyuki/felm/internal/app/rule"
	"github.com/aqyuki/felm/pkg/cache"
	"github.com/aqyuki/felm/pkg/discord"
	"github.com/aqyuki/felm/pkg/logging"
	"github.com/aqyuki/felm/pkg/trace"
	"github.com/bwmarrin/discordgo"
	"github.com/samber/lo"
	"github.com/samber/oops"
	"go.uber.org/zap"
)

var _ discord.MessageCreateHandler = (*CitationService)(nil).On

var ErrMessageLinkNotFound = errors.New("message link not found")

type CitationService struct {
	channelCache *cache.Cache[discordgo.Channel]
	messageRegex *regexp.Regexp
}

func NewCitationService() *CitationService {
	return &CitationService{
		channelCache: cache.New[discordgo.Channel](24 * time.Hour),
		messageRegex: regexp.MustCompile(`https://(?:ptb\.|canary\.)?discord\.com/channels/(?P<guild_id>\d+)/(?P<channel_id>\d+)/(?P<message_id>\d+)`),
	}
}

func (srv *CitationService) On(ctx context.Context, session *discordgo.Session, message *discordgo.MessageCreate) error {
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

	ids, err := srv.parseMessageLink(message.Content)
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

	sourceChannel, err := srv.channelCache.Get(message.ChannelID)
	if err != nil {
		if !errors.Is(err, cache.ErrNotFound) {
			return oops.
				Trace(trace.AcquireTraceID(ctx)).
				With("message_detail",
					oops.With("guild_id", message.GuildID),
					oops.With("channel_id", message.ChannelID),
					oops.With("message_id", message.ID)).
				Wrapf(err, "error occurred while fetching channel information from cache (channel_id = %s)", message.ChannelID)
		}

		logger.Debug("cache not found, fetching channel information from API", zap.String("channel_id", message.ChannelID))
		channel, err := session.Channel(message.ChannelID)
		if err != nil {
			return oops.
				Trace(trace.AcquireTraceID(ctx)).
				With("message_detail",
					oops.With("guild_id", message.GuildID),
					oops.With("channel_id", message.ChannelID),
					oops.With("message_id", message.ID)).
				Wrapf(err, "error occurred while fetching channel information (channel_id = %s)", message.ChannelID)
		}
		if err := srv.channelCache.Set(channel.ID, lo.FromPtr(channel)); err != nil {
			return oops.
				Trace(trace.AcquireTraceID(ctx)).
				With("message_detail",
					oops.With("guild_id", message.GuildID),
					oops.With("channel_id", message.ChannelID),
					oops.With("message_id", message.ID)).
				Wrapf(err, "error occurred while caching channel information (channel_id = %s)", message.ChannelID)
		}
		logger.Debug("channel information was cached successfully", zap.String("channel_id", channel.ID))
		sourceChannel = lo.FromPtr(channel)
	}

	if rule.IsNSFW(&sourceChannel) {
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

	embed := emptyEmbed(&sourceChannel, sourceMessage)
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

type messageLink struct {
	guildID   string
	channelID string
	messageID string
}

func (srv *CitationService) parseMessageLink(message string) (*messageLink, error) {
	matches := srv.messageRegex.FindStringSubmatch(message)
	if len(matches) == 0 {
		return nil, ErrMessageLinkNotFound
	}

	return &messageLink{
		guildID:   matches[srv.messageRegex.SubexpIndex("guild_id")],
		channelID: matches[srv.messageRegex.SubexpIndex("channel_id")],
		messageID: matches[srv.messageRegex.SubexpIndex("message_id")],
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
