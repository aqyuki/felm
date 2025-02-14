package handler

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aqyuki/felm/pkg/cache"
	"github.com/aqyuki/felm/pkg/discord"
	"github.com/aqyuki/felm/pkg/logging"
	"github.com/aqyuki/felm/pkg/trace"
	"github.com/bwmarrin/discordgo"
	"github.com/samber/lo"
	"github.com/samber/oops"
	"go.uber.org/zap"
)

const embedColor = 0x7fffff

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

	if message.Author.Bot {
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

	if message.GuildID != ids.guildID {
		logger.Debug("skip processing message because it was sent from different guild")
		return nil
	}

	citationChannel, err := srv.fetchChannel(ctx, session, ids.channelID)
	if err != nil {
		return oops.
			Trace(trace.AcquireTraceID(ctx)).
			With("message_detail",
				oops.With("guild_id", message.GuildID),
				oops.With("channel_id", message.ChannelID),
				oops.With("message_id", message.ID)).
			Wrapf(err, "error occurred while fetching channel information (channel_id = %s)", ids.channelID)
	}

	if citationChannel.NSFW {
		logger.Debug("skip processing message because it was sent from NSFW channel", zap.String("message_id", message.ID))
		return nil
	}

	citationMessage, err := session.ChannelMessage(ids.channelID, ids.messageID)
	if err != nil {
		return oops.
			Trace(trace.AcquireTraceID(ctx)).
			With("message_detail",
				oops.With("guild_id", message.GuildID),
				oops.With("channel_id", message.ChannelID),
				oops.With("message_id", message.ID)).
			Wrapf(err, "error occurred while fetching message information (channel_id = %s, message_id = %s)", ids.channelID, ids.messageID)
	}

	var embed *discordgo.MessageEmbed

	// メッセージ本文が空で画像等も含まれていない場合の処理
	if citationMessage.Content == "" && len(citationMessage.Attachments) == 0 {

		// Embedも含まれていない場合は何もせずに処理を終了する
		if len(citationMessage.Embeds) == 0 {
			logger.Debug("skip processing message because it was not contains expandable content", zap.String("message_id", message.ID))
			return nil
		}

		// Embedが含まれている場合はEmbedをそのまま返す
		if err := srv.sendReply(session, message.ChannelID, srv.buildReply(message.Message, citationMessage.Embeds[0])); err != nil {
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

	logger.Debug("expandable content detected.", zap.String("message_id", message.ID))

	var image *discordgo.MessageEmbedImage
	if len(citationMessage.Attachments) != 0 {
		attachment := citationMessage.Attachments[0]
		if attachment.ContentType == "image/jpeg" || attachment.ContentType == "image/png" || attachment.ContentType == "image/gif" {
			logger.Debug("image content detected.", zap.String("message_id", message.ID))
			image = &discordgo.MessageEmbedImage{
				URL: attachment.URL,
			}
		} else if citationMessage.Content == "" {
			// もし､メッセージ本文が空で画像が含まれていない場合は何もしない
			// e.g. 動画のみの場合や添付ファイルのみの場合
			logger.Debug("skip processing message because it does not contain expandable content", zap.String("message_id", message.ID))
			return nil
		}
	}

	embed = &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    message.Author.Username,
			IconURL: message.Author.AvatarURL(""),
		},
		Color:       embedColor,
		Description: citationMessage.Content,
		Image:       image,
		Timestamp:   message.Timestamp.Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("from %s", citationChannel.Name),
		},
	}

	if err := srv.sendReply(session, message.ChannelID, srv.buildReply(message.Message, embed)); err != nil {
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

func (srv *CitationService) fetchChannel(ctx context.Context, session *discordgo.Session, channelID string) (*discordgo.Channel, error) {
	logger := logging.FromContext(ctx)

	citationChannel, err := srv.channelCache.Get(channelID)
	if err == nil {
		logger.Debug("channel information fetched from cache (cache hit)", zap.String("channel_id", channelID))
		return lo.ToPtr(citationChannel), nil
	}
	if !errors.Is(err, cache.ErrNotFound) {
		return nil, fmt.Errorf("error occurred while fetching channel information from cache (channel_id = %s)", channelID)
	}

	channel, err := session.Channel(channelID)
	if err != nil {
		return nil, fmt.Errorf("error occurred while fetching channel information (channel_id = %s)", channelID)
	}
	logger.Debug("channel information fetched from API (cache miss)", zap.String("channel_id", channelID))

	if err := srv.channelCache.Set(channelID, lo.FromPtr(channel)); err != nil {
		return nil, fmt.Errorf("error occurred while caching channel information (channel_id = %s)", channelID)
	}
	logger.Debug("channel information cached", zap.String("channel_id", channelID))

	return channel, nil
}

func (srv *CitationService) buildReply(message *discordgo.Message, embed *discordgo.MessageEmbed) *discordgo.MessageSend {
	return &discordgo.MessageSend{
		Embed:           embed,
		Reference:       message.Reference(),
		AllowedMentions: &discordgo.MessageAllowedMentions{RepliedUser: true},
	}
}

func (srv *CitationService) sendReply(session *discordgo.Session, channelID string, replyMsg *discordgo.MessageSend) error {
	if _, err := session.ChannelMessageSendComplex(channelID, replyMsg); err != nil {
		return fmt.Errorf("error occurred while sending message (channel_id = %s)", channelID)
	}
	return nil
}
