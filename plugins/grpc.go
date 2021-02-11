package plugins

import (
	"bytes"
	"encoding/gob"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/tcriess/lightspeed-chat/proto"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// GRPCClient is the client part of the plugin implementation via GRPC
type GRPCClient struct {
	broker *plugin.GRPCBroker
	client proto.EventHandlerClient
}

func (c *GRPCClient) HandleEvents(inEvents []types.Event) ([]types.Event, error) {
	events := make([]*proto.Event, len(inEvents))
	for i, inEvent := range inEvents {
		switch inEvent.GetEventType() {
		case types.EventTypeMessage:
			msgEvent := inEvent.(*types.EventMessage)
			events[i] = &proto.Event{EventType: &proto.Event_Message{Message: &proto.EventMessage{
				Id:        msgEvent.Id,
				Timestamp: msgEvent.Timestamp.Unix(),
				Nick:      msgEvent.Nick,
				Text:      msgEvent.Message,
				Language:  msgEvent.Language,
				Filter:    msgEvent.Filter,
			}}}

		case types.EventTypeTranslation:
			transEvent := inEvent.(*types.EventTranslation)
			events[i] = &proto.Event{EventType: &proto.Event_Translation{Translation: &proto.EventTranslation{
				SourceId:  transEvent.SourceId,
				Timestamp: transEvent.Timestamp.Unix(),
				Language:  transEvent.Language,
				Text:      transEvent.Message,
				Filter:    transEvent.Filter,
			}}}

		case types.EventTypeCommand:
			cmdEvent := inEvent.(*types.EventCommand)
			events[i] = &proto.Event{EventType: &proto.Event_Command{Command: &proto.EventCommand{
				Command: cmdEvent.Command.Command,
				Nick:    cmdEvent.Nick,
			}}}

		case types.EventTypeUserLogin:
			ulEvent := inEvent.(*types.EventUserLogin)
			events[i] = &proto.Event{EventType: &proto.Event_UserLogin{UserLogin: &proto.EventUserLogin{
				Id:         ulEvent.Id,
				Nick:       ulEvent.Nick,
				Tags:       ulEvent.Tags,
				IntTags:    ulEvent.IntTags,
				LastOnline: ulEvent.LastOnline.Unix(),
			}}}
		}
	}
	req := &proto.HandleEventsRequest{Events: events}
	resp, err := c.client.HandleEvents(context.Background(), req)
	if err != nil {
		return nil, err
	}
	outEvents := make([]types.Event, len(resp.Events))
	for i, event := range resp.Events {
		switch event.EventType.(type) {
		case *proto.Event_Message:
			msgEvent := event.EventType.(*proto.Event_Message)
			chatMessage := types.ChatMessage{
				Id:        msgEvent.Message.Id,
				Nick:      msgEvent.Message.Nick,
				Timestamp: time.Unix(msgEvent.Message.Timestamp, 0),
				Message:   msgEvent.Message.Text,
				Language:  msgEvent.Message.Language,
				Filter:    msgEvent.Message.Filter,
			}
			outEvents[i] = types.NewMessageEvent(chatMessage)

		case *proto.Event_Translation:
			transEvent := event.EventType.(*proto.Event_Translation)
			translationMessage := types.TranslationMessage{
				SourceId:  transEvent.Translation.SourceId,
				Timestamp: time.Unix(transEvent.Translation.Timestamp, 0),
				Message:   transEvent.Translation.Text,
				Language:  transEvent.Translation.Language,
				Filter:    transEvent.Translation.Filter,
			}
			outEvents[i] = types.NewTranslationEvent(translationMessage)

		case *proto.Event_Command:
			cmdEvent := event.EventType.(*proto.Event_Command)
			command := types.Command{
				Command: cmdEvent.Command.Command,
				Nick:    cmdEvent.Command.Nick,
			}
			outEvents[i] = types.NewCommandEvent(command)

		case *proto.Event_UserLogin:
			ulEvent := event.EventType.(*proto.Event_UserLogin)
			user := types.User{
				Id:         ulEvent.UserLogin.Id,
				Nick:       ulEvent.UserLogin.Nick,
				Tags:       ulEvent.UserLogin.Tags,
				IntTags:    ulEvent.UserLogin.IntTags,
				LastOnline: time.Unix(ulEvent.UserLogin.LastOnline, 0),
			}
			outEvents[i] = types.NewUserLoginEvent(user)
		}
	}
	return outEvents, nil
}

func (c *GRPCClient) GetSpec() (*hcldec.BlockSpec, error) {
	s, err := c.client.GetSpec(context.Background(), &proto.GetSpecRequest{})
	if err != nil {
		log.Printf("error: could not get spec: %s", err)
		return nil, err
	}
	var spec hcldec.BlockSpec
	buf := bytes.NewBuffer(s.Data)
	enc := gob.NewDecoder(buf)
	err = enc.Decode(&spec)
	if err != nil {
		log.Printf("error: could not decode: %s", err)
		return nil, err
	}
	return &spec, nil
}

func (c *GRPCClient) Configure(val cty.Value) (string, string, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(val)
	if err != nil {
		return "", "", err
	}
	resp, err := c.client.Configure(context.Background(), &proto.ConfigureRequest{Data: buf.Bytes()})
	if err != nil {
		return "", "", err
	}
	return resp.CronSpec, resp.EventsFilter, err
}

func (c *GRPCClient) Cron() ([]types.Event, error) {
	resp, err := c.client.Cron(context.Background(), &proto.CronRequest{})
	if err != nil {
		return nil, err
	}
	outEvents := make([]types.Event, len(resp.Events))
	for i, event := range resp.Events {
		switch event.EventType.(type) {
		case *proto.Event_Message:
			msgEvent := event.EventType.(*proto.Event_Message)
			outEvents[i] = &types.EventMessage{
				EventBase: &types.EventBase{EventType: types.EventTypeMessage},
				ChatMessage: &types.ChatMessage{
					Id:        msgEvent.Message.Id,
					Nick:      msgEvent.Message.Nick,
					Timestamp: time.Unix(msgEvent.Message.Timestamp, 0),
					Message:   msgEvent.Message.Text,
					Language:  msgEvent.Message.Language,
					Filter:    msgEvent.Message.Filter,
				},
			}

		case *proto.Event_Translation:
			transEvent := event.EventType.(*proto.Event_Translation)
			outEvents[i] = &types.EventTranslation{
				EventBase: &types.EventBase{EventType: types.EventTypeTranslation},
				TranslationMessage: &types.TranslationMessage{
					SourceId:  transEvent.Translation.SourceId,
					Timestamp: time.Unix(transEvent.Translation.Timestamp, 0),
					Message:   transEvent.Translation.Text,
					Language:  transEvent.Translation.Language,
					Filter:    transEvent.Translation.Filter,
				},
			}

		case *proto.Event_Command:
			cmdEvent := event.EventType.(*proto.Event_Command)
			outEvents[i] = &types.EventCommand{
				EventBase: &types.EventBase{EventType: types.EventTypeCommand},
				Command: &types.Command{
					Command: cmdEvent.Command.Command,
					Nick:    cmdEvent.Command.Nick,
				},
			}
		case *proto.Event_UserLogin:
			ulEvent := event.EventType.(*proto.Event_UserLogin)
			outEvents[i] = &types.EventUserLogin{
				EventBase: &types.EventBase{EventType: types.EventTypeUserLogin},
				User: &types.User{
					Id:         ulEvent.UserLogin.Id,
					Nick:       ulEvent.UserLogin.Nick,
					Tags:       ulEvent.UserLogin.Tags,
					IntTags:    ulEvent.UserLogin.IntTags,
					LastOnline: time.Unix(ulEvent.UserLogin.LastOnline, 0),
				},
			}
		}
	}
	return outEvents, nil
}

// InitEmitEvents is called once per hub from the main process and it opens a permanent data stream
// from the plugin to the main process (via GRPC/ EmitEventsHelper).
// It must be called from a go routine as it never returns.
func (c *GRPCClient) InitEmitEvents(eh EmitEventsHelper) error {
	emitEventsServer := &GRPCEmitEventsHelperServer{Impl: eh}

	var wg sync.WaitGroup
	wg.Add(1)
	var s *grpc.Server
	serverFunc := func(opts []grpc.ServerOption) *grpc.Server {
		defer wg.Done()
		s = grpc.NewServer(opts...)
		proto.RegisterEmitEventsHelperServer(s, emitEventsServer)
		return s
	}

	brokerID := c.broker.NextId()
	go c.broker.AcceptAndServe(brokerID, serverFunc)

	wg.Wait()
	// this is supposed to run forever! if it stops, the main process should call here again.
	_, err := c.client.InitEmitEvents(context.Background(), &proto.InitEmitEventsRequest{
		EmitEventsServer: brokerID,
	})

	s.Stop()
	return err
}

// Here is the gRPC server that GRPCClient talks to.
type GRPCServer struct {
	proto.UnimplementedEventHandlerServer

	// This is the real implementation
	Impl EventHandler

	broker *plugin.GRPCBroker
}

func (s *GRPCServer) HandleEvents(ctx context.Context, req *proto.HandleEventsRequest) (*proto.HandleEventsResponse, error) {
	inEvents := make([]types.Event, len(req.Events))
	for i, event := range req.Events {
		switch event.EventType.(type) {
		case *proto.Event_Message:
			msgEvent := event.EventType.(*proto.Event_Message)
			inEvents[i] = &types.EventMessage{
				EventBase: &types.EventBase{EventType: types.EventTypeMessage},
				ChatMessage: &types.ChatMessage{
					Id:        msgEvent.Message.Id,
					Nick:      msgEvent.Message.Nick,
					Timestamp: time.Unix(msgEvent.Message.Timestamp, 0),
					Message:   msgEvent.Message.Text,
					Language:  msgEvent.Message.Language,
					Filter:    msgEvent.Message.Filter,
				},
			}

		case *proto.Event_Translation:
			transEvent := event.EventType.(*proto.Event_Translation)
			inEvents[i] = &types.EventTranslation{
				EventBase: &types.EventBase{EventType: types.EventTypeTranslation},
				TranslationMessage: &types.TranslationMessage{
					SourceId:  transEvent.Translation.SourceId,
					Timestamp: time.Unix(transEvent.Translation.Timestamp, 0),
					Message:   transEvent.Translation.Text,
					Language:  transEvent.Translation.Language,
					Filter:    transEvent.Translation.Filter,
				},
			}

		case *proto.Event_Command:
			cmdEvent := event.EventType.(*proto.Event_Command)
			inEvents[i] = &types.EventCommand{
				EventBase: &types.EventBase{EventType: types.EventTypeCommand},
				Command: &types.Command{
					Command: cmdEvent.Command.Command,
					Nick:    cmdEvent.Command.Nick,
				},
			}
		case *proto.Event_UserLogin:
			ulEvent := event.EventType.(*proto.Event_UserLogin)
			inEvents[i] = &types.EventUserLogin{
				EventBase: &types.EventBase{EventType: types.EventTypeUserLogin},
				User: &types.User{
					Id:         ulEvent.UserLogin.Id,
					Nick:       ulEvent.UserLogin.Nick,
					Tags:       ulEvent.UserLogin.Tags,
					IntTags:    ulEvent.UserLogin.IntTags,
					LastOnline: time.Unix(ulEvent.UserLogin.LastOnline, 0),
				},
			}
		}
	}
	outEvents, err := s.Impl.HandleEvents(inEvents)
	if err != nil {
		return nil, err
	}
	events := make([]*proto.Event, len(outEvents))
	for i, inEvent := range outEvents {
		switch inEvent.GetEventType() {
		case types.EventTypeMessage:
			msgEvent := inEvent.(*types.EventMessage)
			events[i] = &proto.Event{EventType: &proto.Event_Message{Message: &proto.EventMessage{
				Id:        msgEvent.Id,
				Timestamp: msgEvent.Timestamp.Unix(),
				Nick:      msgEvent.Nick,
				Text:      msgEvent.Message,
				Language:  msgEvent.Language,
				Filter:    msgEvent.Filter,
			}}}

		case types.EventTypeTranslation:
			transEvent := inEvent.(*types.EventTranslation)
			events[i] = &proto.Event{EventType: &proto.Event_Translation{Translation: &proto.EventTranslation{
				SourceId:  transEvent.SourceId,
				Timestamp: transEvent.Timestamp.Unix(),
				Language:  transEvent.Language,
				Text:      transEvent.Message,
				Filter:    transEvent.Filter,
			}}}

		case types.EventTypeCommand:
			cmdEvent := inEvent.(*types.EventCommand)
			events[i] = &proto.Event{EventType: &proto.Event_Command{Command: &proto.EventCommand{
				Command: cmdEvent.Command.Command,
				Nick:    cmdEvent.Nick,
			}}}

		case types.EventTypeUserLogin:
			ulEvent := inEvent.(*types.EventUserLogin)
			events[i] = &proto.Event{EventType: &proto.Event_UserLogin{UserLogin: &proto.EventUserLogin{
				Id:         ulEvent.Id,
				Nick:       ulEvent.Nick,
				Tags:       ulEvent.Tags,
				IntTags:    ulEvent.IntTags,
				LastOnline: ulEvent.LastOnline.Unix(),
			}}}
		}
	}
	return &proto.HandleEventsResponse{Events: events}, nil
}

func (s *GRPCServer) GetSpec(ctx context.Context, req *proto.GetSpecRequest) (*proto.GetSpecResponse, error) {
	spec, err := s.Impl.GetSpec()
	if err != nil {
		log.Printf("error: could not get spec: %s", err)
		return nil, err
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err = enc.Encode(spec)
	if err != nil {
		log.Printf("error: could not encode spec: %s", err)
		return nil, err
	}
	return &proto.GetSpecResponse{Data: buf.Bytes()}, nil
}

func (s *GRPCServer) Configure(ctx context.Context, req *proto.ConfigureRequest) (*proto.ConfigureResponse, error) {
	var val cty.Value
	buf := bytes.NewBuffer(req.Data)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&val)
	if err != nil {
		return nil, err
	}
	cronSpec, eventsFilter, err := s.Impl.Configure(val)
	if err != nil {
		return nil, err
	}
	return &proto.ConfigureResponse{CronSpec: cronSpec, EventsFilter: eventsFilter}, nil
}

func (s *GRPCServer) Cron(ctx context.Context, req *proto.CronRequest) (*proto.CronResponse, error) {
	outEvents, err := s.Impl.Cron()
	if err != nil {
		return nil, err
	}
	events := make([]*proto.Event, len(outEvents))
	for i, inEvent := range outEvents {
		switch inEvent.GetEventType() {
		case types.EventTypeMessage:
			msgEvent := inEvent.(*types.EventMessage)
			events[i] = &proto.Event{EventType: &proto.Event_Message{Message: &proto.EventMessage{
				Id:        msgEvent.Id,
				Timestamp: msgEvent.Timestamp.Unix(),
				Nick:      msgEvent.Nick,
				Text:      msgEvent.Message,
				Language:  msgEvent.Language,
				Filter:    msgEvent.Filter,
			}}}

		case types.EventTypeTranslation:
			transEvent := inEvent.(*types.EventTranslation)
			events[i] = &proto.Event{EventType: &proto.Event_Translation{Translation: &proto.EventTranslation{
				SourceId:  transEvent.SourceId,
				Timestamp: transEvent.Timestamp.Unix(),
				Language:  transEvent.Language,
				Text:      transEvent.Message,
				Filter:    transEvent.Filter,
			}}}

		case types.EventTypeCommand:
			cmdEvent := inEvent.(*types.EventCommand)
			events[i] = &proto.Event{EventType: &proto.Event_Command{Command: &proto.EventCommand{
				Command: cmdEvent.Command.Command,
				Nick:    cmdEvent.Nick,
			}}}

		case types.EventTypeUserLogin:
			ulEvent := inEvent.(*types.EventUserLogin)
			events[i] = &proto.Event{EventType: &proto.Event_UserLogin{UserLogin: &proto.EventUserLogin{
				Id:         ulEvent.Id,
				Nick:       ulEvent.Nick,
				Tags:       ulEvent.Tags,
				IntTags:    ulEvent.IntTags,
				LastOnline: ulEvent.LastOnline.Unix(),
			}}}
		}
	}
	return &proto.CronResponse{Events: events}, nil
}

func (s *GRPCServer) InitEmitEvents(ctx context.Context, req *proto.InitEmitEventsRequest) (*proto.InitEmitEventsResponse, error) {
	conn, err := s.broker.Dial(req.EmitEventsServer)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	c := &GRPCEmitEventsHelperClient{client: proto.NewEmitEventsHelperClient(conn)}
	err = s.Impl.InitEmitEvents(c) // this is supposed to run forever
	if err != nil {
		return nil, err
	}
	return &proto.InitEmitEventsResponse{}, nil
}

type GRPCEmitEventsHelperClient struct {
	client proto.EmitEventsHelperClient
}

func (c *GRPCEmitEventsHelperClient) EmitEvents(events []types.Event) error {
	emitEvents := make([]*proto.Event, len(events))
	for i, event := range events {
		switch event.(type) {
		case *types.EventMessage:
			msgEvt := event.(*types.EventMessage)
			emitEvents[i] = &proto.Event{
				EventType: &proto.Event_Message{
					Message: &proto.EventMessage{
						Id:        msgEvt.Id,
						Timestamp: msgEvt.Timestamp.Unix(),
						Nick:      msgEvt.Nick,
						Text:      msgEvt.Message,
						Language:  msgEvt.Language,
						Filter:    msgEvt.Filter,
					},
				},
			}

		case *types.EventTranslation:
			transEvt := event.(*types.EventTranslation)
			emitEvents[i] = &proto.Event{
				EventType: &proto.Event_Translation{
					Translation: &proto.EventTranslation{
						SourceId:  transEvt.SourceId,
						Timestamp: transEvt.Timestamp.Unix(),
						Text:      transEvt.Message,
						Language:  transEvt.Language,
						Filter:    transEvt.Filter,
					},
				},
			}

		case *types.EventCommand:
			cmdEvt := event.(*types.EventCommand)
			emitEvents[i] = &proto.Event{
				EventType: &proto.Event_Command{
					Command: &proto.EventCommand{
						Command: cmdEvt.Command.Command,
						Nick:    cmdEvt.Nick,
					},
				},
			}

		case *types.EventUserLogin:
			ulEvt := event.(*types.EventUserLogin)
			emitEvents[i] = &proto.Event{
				EventType: &proto.Event_UserLogin{
					UserLogin: &proto.EventUserLogin{
						Id:         ulEvt.Id,
						Nick:       ulEvt.Nick,
						Tags:       ulEvt.Tags,
						IntTags:    ulEvt.IntTags,
						LastOnline: ulEvt.LastOnline.Unix(),
					},
				},
			}

			//default:
			//log.Println("event type not implemented")
			//continue streamSendLoop
		}
	}
	req := &proto.EmitEventsRequest{
		Events: emitEvents,
	}
	_, err := c.client.EmitEvents(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

type GRPCEmitEventsHelperServer struct {
	proto.UnimplementedEmitEventsHelperServer

	// This is the real implementation
	Impl EmitEventsHelper
}

func (s *GRPCEmitEventsHelperServer) EmitEvents(ctx context.Context, req *proto.EmitEventsRequest) (resp *proto.EmitEventsResponse, err error) {
	events := make([]types.Event, len(req.Events))
	for i, event := range req.Events {
		switch event.EventType.(type) {
		case *proto.Event_Message:
			msgEvt := event.EventType.(*proto.Event_Message)
			events[i] = &types.EventMessage{
				EventBase: &types.EventBase{EventType: types.EventTypeMessage},
				ChatMessage: &types.ChatMessage{
					Id:        msgEvt.Message.Id,
					Nick:      msgEvt.Message.Nick,
					Timestamp: time.Unix(msgEvt.Message.Timestamp, 0),
					Message:   msgEvt.Message.Text,
					Language:  msgEvt.Message.Language,
					Filter:    msgEvt.Message.Filter,
				},
			}

		case *proto.Event_Translation:
			transEvt := event.EventType.(*proto.Event_Translation)
			events[i] = &types.EventTranslation{
				EventBase: &types.EventBase{EventType: types.EventTypeTranslation},
				TranslationMessage: &types.TranslationMessage{
					SourceId:  transEvt.Translation.SourceId,
					Timestamp: time.Unix(transEvt.Translation.Timestamp, 0),
					Language:  transEvt.Translation.Language,
					Message:   transEvt.Translation.Text,
					Filter:    transEvt.Translation.Filter,
				},
			}

		case *proto.Event_Command:
			cmdEvt := event.EventType.(*proto.Event_Command)
			events[i] = &types.EventCommand{
				EventBase: &types.EventBase{EventType: types.EventTypeCommand},
				Command: &types.Command{
					Command: cmdEvt.Command.Command,
					Nick:    cmdEvt.Command.Nick,
				},
			}

		case *proto.Event_UserLogin:
			ulEvt := event.EventType.(*proto.Event_UserLogin)
			events[i] = &types.EventUserLogin{
				EventBase: &types.EventBase{EventType: types.EventTypeUserLogin},
				User: &types.User{
					Id:         ulEvt.UserLogin.Id,
					Nick:       ulEvt.UserLogin.Nick,
					Tags:       ulEvt.UserLogin.Tags,
					IntTags:    ulEvt.UserLogin.IntTags,
					LastOnline: time.Unix(ulEvt.UserLogin.LastOnline, 0),
				},
			}
		}
	}
	err = s.Impl.EmitEvents(events)
	if err != nil {
		return nil, err
	}
	return &proto.EmitEventsResponse{}, nil
}
