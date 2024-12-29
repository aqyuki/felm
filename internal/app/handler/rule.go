package handler

import "github.com/bwmarrin/discordgo"

func isBot(author *discordgo.User) bool {
	return author.Bot
}

func isSameGuild(guild string, msg *discordgo.MessageCreate) bool {
	return msg.GuildID == guild
}

func isNSFW(channel *discordgo.Channel) bool {
	return channel.NSFW
}

func isExpandable(message *discordgo.Message) bool {
	return hasContent(message) || (hasImage(message) && !hasVideo(message))
}

func hasContent(message *discordgo.Message) bool {
	return message.Content != ""
}

func hasAttachment(message *discordgo.Message) bool {
	return len(message.Attachments) != 0
}

func hasEmbed(message *discordgo.Message) bool {
	return len(message.Embeds) != 0
}

func hasImage(message *discordgo.Message) bool {
	if !hasAttachment(message) {
		return false
	}
	contentType := message.Attachments[0].ContentType
	return contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/gif"
}

func hasVideo(message *discordgo.Message) bool {
	if !hasAttachment(message) {
		return false
	}
	return message.Attachments[0].ContentType == "video/mp4"
}
