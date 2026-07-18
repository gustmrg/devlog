package discord

import (
	"context"
	"fmt"
	"strings"

	"devlog/internal/domain"
	"github.com/bwmarrin/discordgo"
)

type Notifier struct {
	Token, ChannelID, AllowedUserID, PublicURL string
	session                                    *discordgo.Session
	OnConfirm                                  func(context.Context, string) error
	OnRegenerate                               func(context.Context, string) error
	OnAdd                                      func(context.Context, string, string) error
	OnEditSummary                              func(context.Context, string, string) error
}

func (n *Notifier) Start() error {
	if n.Token == "" || n.ChannelID == "" {
		return nil
	}
	s, err := discordgo.New("Bot " + n.Token)
	if err != nil {
		return err
	}
	n.session = s
	s.AddHandler(n.handle)
	return s.Open()
}
func (n *Notifier) Close() error {
	if n.session == nil {
		return nil
	}
	return n.session.Close()
}
func (n *Notifier) SendSummary(_ context.Context, summary domain.Summary) error {
	if n.session == nil {
		return nil
	}
	content := fmt.Sprintf("**DevLog · %s · revisão %d**\n%s", summary.Date, summary.Revision, summary.Content)
	_, err := n.session.ChannelMessageSendComplex(n.ChannelID, &discordgo.MessageSend{Content: content, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Confirmar", Style: discordgo.SuccessButton, CustomID: "devlog:confirm:" + summary.ID},
		discordgo.Button{Label: "Editar", Style: discordgo.PrimaryButton, CustomID: "devlog:edit:" + summary.ID},
		discordgo.Button{Label: "Adicionar atividade", Style: discordgo.SecondaryButton, CustomID: "devlog:add:" + summary.Date},
		discordgo.Button{Label: "Regenerar", Style: discordgo.SecondaryButton, CustomID: "devlog:regenerate:" + summary.Date},
		discordgo.Button{Label: "Revisar na web", Style: discordgo.LinkButton, URL: n.PublicURL + "/days/" + summary.Date},
	}}}})
	return err
}

func (n *Notifier) handle(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction.Type != discordgo.InteractionMessageComponent && interaction.Type != discordgo.InteractionModalSubmit {
		return
	}
	actor := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		actor = interaction.Member.User.ID
	} else if interaction.User != nil {
		actor = interaction.User.ID
	}
	if n.AllowedUserID != "" && actor != n.AllowedUserID {
		return
	}
	if interaction.Type == discordgo.InteractionModalSubmit {
		n.handleModal(session, interaction)
		return
	}
	id := interaction.MessageComponentData().CustomID
	var err error
	switch {
	case strings.HasPrefix(id, "devlog:confirm:"):
		if n.OnConfirm != nil {
			err = n.OnConfirm(context.Background(), strings.TrimPrefix(id, "devlog:confirm:"))
		}
	case strings.HasPrefix(id, "devlog:regenerate:"):
		if n.OnRegenerate != nil {
			err = n.OnRegenerate(context.Background(), strings.TrimPrefix(id, "devlog:regenerate:"))
		}
	case strings.HasPrefix(id, "devlog:add:"):
		_ = session.InteractionRespond(interaction.Interaction, modal("devlog:addmodal:"+strings.TrimPrefix(id, "devlog:add:"), "Adicionar atividade", "description", "Atividade"))
		return
	case strings.HasPrefix(id, "devlog:edit:"):
		_ = session.InteractionRespond(interaction.Interaction, modal("devlog:editmodal:"+strings.TrimPrefix(id, "devlog:edit:"), "Editar resumo", "content", "Resumo"))
		return
	}
	respond(session, interaction, err)
}
func (n *Notifier) handleModal(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	data := interaction.ModalSubmitData()
	value := ""
	for _, row := range data.Components {
		actions, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, component := range actions.Components {
			if input, ok := component.(*discordgo.TextInput); ok {
				value = input.Value
			}
		}
	}
	var err error
	if strings.HasPrefix(data.CustomID, "devlog:addmodal:") && n.OnAdd != nil {
		err = n.OnAdd(context.Background(), strings.TrimPrefix(data.CustomID, "devlog:addmodal:"), value)
	} else if strings.HasPrefix(data.CustomID, "devlog:editmodal:") && n.OnEditSummary != nil {
		err = n.OnEditSummary(context.Background(), strings.TrimPrefix(data.CustomID, "devlog:editmodal:"), value)
	}
	respond(session, interaction, err)
}
func modal(id, title, inputID, label string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: id, Title: title, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: inputID, Label: label, Style: discordgo.TextInputParagraph, Required: true}}}}}}
}
func respond(session *discordgo.Session, interaction *discordgo.InteractionCreate, err error) {
	message := "Ação concluída."
	if err != nil {
		message = "Falha: " + err.Error()
	}
	_ = session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: message, Flags: discordgo.MessageFlagsEphemeral}})
}
