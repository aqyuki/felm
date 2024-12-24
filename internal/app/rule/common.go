package rule

import "github.com/bwmarrin/discordgo"

func IsBot(author *discordgo.User) bool {
	return author.Bot
}

func IsSameGuild(guild string, msg *discordgo.MessageCreate) bool {
	return msg.GuildID == guild
}

func IsNSFW(channel *discordgo.Channel) bool {
	return channel.NSFW
}

func IsExpandable(message *discordgo.Message) bool {
	return HasContent(message) || HasImage(message)
}

func HasContent(message *discordgo.Message) bool {
	return message.Content != ""
}

func HasImage(message *discordgo.Message) bool {
	return len(message.Attachments) != 0
}
