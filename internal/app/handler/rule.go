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
	return hasContent(message) || hasImage(message)
}

func hasContent(message *discordgo.Message) bool {
	return message.Content != ""
}

func hasImage(message *discordgo.Message) bool {
	return len(message.Attachments) != 0
}
