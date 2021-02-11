package ws

import (
	"log"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/tcriess/lightspeed-chat/filter"
	"github.com/tcriess/lightspeed-chat/types"
)

func (c *Client) EvaluateFilterMessage(message *types.ChatMessage) bool {
	if message.Filter == "" {
		return true
	}
	prog, err := expr.Compile(message.Filter, expr.Env(filter.Env{}))
	if err != nil {
		log.Printf("error: could not compile filter: %s", err)
		return false
	}
	return c.RunFilterMessage(message, prog)
}

func (c *Client) RunFilterMessage(message *types.ChatMessage, prog *vm.Program) bool {
	if message == nil {
		return false
	}
	if prog == nil {
		return true
	}
	env := filter.Env{
		User: filter.User{
			Id:         c.user.Id,
			Tags:       c.user.Tags,
			IntTags:    c.user.IntTags,
			LastOnline: c.user.LastOnline.Unix(),
		},
		Client: filter.Client{
			ClientLanguage: c.Language,
		},
		Message: filter.Message{
			MessageLanguage: message.Language,
		},
	}
	res, err := expr.Run(prog, env)
	if err != nil {
		log.Printf("error: could not run filter: %s", err)
		return false
	}
	if bRes, ok := res.(bool); ok {
		if bRes {
			return true
		}
	}
	return false
}

func (c *Client) EvaluateFilterTranslation(translation *types.TranslationMessage) bool {
	if translation.Filter == "" {
		return true
	}
	prog, err := expr.Compile(translation.Filter, expr.Env(filter.Env{}))
	if err != nil {
		log.Printf("error: could not compile filter: %s", err)
		return false
	}
	return c.RunFilterTranslation(translation, prog)
}

func (c *Client) RunFilterTranslation(translation *types.TranslationMessage, prog *vm.Program) bool {
	if translation == nil {
		return false
	}
	if prog == nil {
		return true
	}
	env := filter.Env{
		User: filter.User{
			Id:         c.user.Id,
			Tags:       c.user.Tags,
			IntTags:    c.user.IntTags,
			LastOnline: c.user.LastOnline.Unix(),
		},
		Client: filter.Client{
			ClientLanguage: c.Language,
		},
		Message: filter.Message{
			MessageLanguage: translation.Language,
		},
	}
	res, err := expr.Run(prog, env)
	if err != nil {
		log.Printf("error: could not run filter: %s", err)
		return false
	}
	if bRes, ok := res.(bool); ok {
		if bRes {
			return true
		}
	}
	return false
}

func (h *Hub) EvaluateFilterEvent(event types.Event, eventFilter string) bool {
	if eventFilter == "" {
		return true
	}
	prog, err := expr.Compile(eventFilter, expr.Env(filter.Env{}))
	if err != nil {
		log.Printf("error: could not compile filter: %s", err)
		return false
	}
	return h.RunFilterEvent(event, prog)
}

func (h *Hub) RunFilterEvent(event types.Event, prog *vm.Program) bool {
	if event == nil {
		return false
	}
	if prog == nil {
		return true
	}
	env := filter.Env{}
	switch event.(type) {
	case *types.EventMessage:
		msgEvent := event.(*types.EventMessage)
		env.Message.MessageLanguage = msgEvent.Language

	case *types.EventTranslation:
		msgTranslation := event.(*types.EventTranslation)
		env.Message.MessageLanguage = msgTranslation.Language

	case *types.EventCommand:
		msgCommand := event.(*types.EventCommand)
		env.Command.Command = msgCommand.Command.Command
		env.Command.Nick = msgCommand.Command.Nick

	case *types.EventUserLogin:
		msgUserLogin := event.(*types.EventUserLogin)
		env.User.Id = msgUserLogin.User.Id
		env.User.Tags = msgUserLogin.Tags
		env.User.IntTags = msgUserLogin.IntTags
		env.User.LastOnline = msgUserLogin.LastOnline.Unix()
	}
	res, err := expr.Run(prog, env)
	if err != nil {
		log.Printf("error: could not run filter: %s", err)
		return false
	}
	if bRes, ok := res.(bool); ok {
		if bRes {
			return true
		}
	}
	return false
}
