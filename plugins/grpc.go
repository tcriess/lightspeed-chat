package plugins

import (
	"bytes"
	"encoding/gob"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/tcriess/lightspeed-chat/globals"
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

func eventNative2Proto(inEvent *types.Event) *proto.Event {
	outEvent := &proto.Event{
		Id:   inEvent.Id,
		Room: roomNative2Proto(inEvent.Room),
		Source: &proto.Source{
			User:       userNative2Proto(inEvent.Source.User),
			PluginName: inEvent.Source.PluginName,
		},
		Created:      inEvent.Created.Unix(),
		Language:     inEvent.Language,
		Name:         inEvent.Name,
		Tags:         inEvent.Tags,
		Sent:         inEvent.Sent.Unix(),
		TargetFilter: inEvent.TargetFilter,
		History:      inEvent.History,
	}

	return outEvent
}

func eventProto2Native(inEvent *proto.Event) *types.Event {
	outEvent := &types.Event{
		Id:   inEvent.Id,
		Room: roomProto2Native(inEvent.Room),
		Source: &types.Source{
			User:       userProto2Native(inEvent.Source.User),
			PluginName: inEvent.Source.PluginName,
		},
		Created:      time.Unix(inEvent.Created, 0).In(time.UTC),
		Language:     inEvent.Language,
		Name:         inEvent.Name,
		Tags:         inEvent.Tags,
		Sent:         time.Unix(inEvent.Sent, 0).In(time.UTC),
		TargetFilter: inEvent.TargetFilter,
		History:      inEvent.History,
	}

	return outEvent
}

func userNative2Proto(inUser *types.User) *proto.User {
	outUser := &proto.User{
		Id:         inUser.Id,
		Nick:       inUser.Nick,
		Language:   inUser.Language,
		Tags:       inUser.Tags,
		LastOnline: inUser.LastOnline.Unix(),
	}

	return outUser
}

func userProto2Native(inUser *proto.User) *types.User {
	outUser := &types.User{
		Id:         inUser.Id,
		Nick:       inUser.Nick,
		Language:   inUser.Language,
		Tags:       inUser.Tags,
		LastOnline: time.Unix(inUser.LastOnline, 0).In(time.UTC),
	}

	return outUser
}

func roomNative2Proto(inRoom *types.Room) *proto.Room {
	outRoom := &proto.Room{
		Id:    inRoom.Id,
		Owner: userNative2Proto(inRoom.Owner),
		Tags:  inRoom.Tags,
	}
	globals.AppLogger.Debug("converted native to proto:", "native", *inRoom, "proto", *outRoom)
	return outRoom
}

func roomProto2Native(inRoom *proto.Room) *types.Room {
	outRoom := &types.Room{
		Id:    inRoom.Id,
		Owner: userProto2Native(inRoom.Owner),
		Tags:  inRoom.Tags,
	}
	globals.AppLogger.Debug("converted proto to native:", "proto", *inRoom, "native", *outRoom)
	return outRoom
}

func (c *GRPCClient) HandleEvents(inEvents []*types.Event) ([]*types.Event, error) {
	events := make([]*proto.Event, len(inEvents))
	for i, inEvent := range inEvents {
		events[i] = eventNative2Proto(inEvent)
	}
	req := &proto.HandleEventsRequest{Events: events}
	resp, err := c.client.HandleEvents(context.Background(), req)
	if err != nil {
		return nil, err
	}
	log.Printf("in GRPC handleEvents: %+v", resp.Events)
	outEvents := make([]*types.Event, len(resp.Events))
	for i, event := range resp.Events {
		log.Printf("in GRPC handleEvents: %+v", *event)
		outEvents[i] = eventProto2Native(event)
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

func (c *GRPCClient) Cron(room *types.Room) ([]*types.Event, error) {
	resp, err := c.client.Cron(context.Background(), &proto.CronRequest{
		Room: roomNative2Proto(room),
	})
	if err != nil {
		return nil, err
	}
	outEvents := make([]*types.Event, len(resp.Events))
	for i, event := range resp.Events {
		outEvents[i] = eventProto2Native(event)
	}
	return outEvents, nil
}

// InitEmitEvents is called once per hub from the main process and it opens a permanent data stream
// from the plugin to the main process (via GRPC/ EmitEventsHelper).
// It must be called from a go routine as it never returns.
func (c *GRPCClient) InitEmitEvents(room *types.Room, eh EmitEventsHelper) error {
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
		Room:             roomNative2Proto(room),
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
	inEvents := make([]*types.Event, len(req.Events))
	for i, event := range req.Events {
		inEvents[i] = eventProto2Native(event)
	}
	outEvents, err := s.Impl.HandleEvents(inEvents)
	if err != nil {
		return nil, err
	}
	events := make([]*proto.Event, len(outEvents))
	for i, inEvent := range outEvents {
		events[i] = eventNative2Proto(inEvent)
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
	room := roomProto2Native(req.Room)
	outEvents, err := s.Impl.Cron(room)
	if err != nil {
		return nil, err
	}
	events := make([]*proto.Event, len(outEvents))
	for i, inEvent := range outEvents {
		events[i] = eventNative2Proto(inEvent)
	}
	return &proto.CronResponse{Events: events}, nil
}

func (s *GRPCServer) InitEmitEvents(ctx context.Context, req *proto.InitEmitEventsRequest) (*proto.InitEmitEventsResponse, error) {
	conn, err := s.broker.Dial(req.EmitEventsServer)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	room := roomProto2Native(req.Room)

	c := &GRPCEmitEventsHelperClient{client: proto.NewEmitEventsHelperClient(conn)}
	err = s.Impl.InitEmitEvents(room, c) // this is supposed to run forever
	if err != nil {
		return nil, err
	}
	return &proto.InitEmitEventsResponse{}, nil
}

type GRPCEmitEventsHelperClient struct {
	client proto.EmitEventsHelperClient
}

func (c *GRPCEmitEventsHelperClient) EmitEvents(events []*types.Event) error {
	emitEvents := make([]*proto.Event, len(events))
	for i, event := range events {
		emitEvents[i] = eventNative2Proto(event)
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

func (c *GRPCEmitEventsHelperClient) AuthenticateUser(idToken, provider string) (*types.User, error) {
	req := &proto.AuthenticateUserRequest{
		IdToken:  idToken,
		Provider: provider,
	}
	resp, err := c.client.AuthenticateUser(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return userProto2Native(resp.User), nil
}

func (c *GRPCEmitEventsHelperClient) GetUser(userId string) (*types.User, error) {
	req := &proto.GetUserRequest{
		UserId: userId,
	}
	resp, err := c.client.GetUser(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return userProto2Native(resp.User), nil
}

func (c *GRPCEmitEventsHelperClient) GetRoom(roomId string) (*types.Room, error) {
	req := &proto.GetRoomRequest{
		RoomId: roomId,
	}
	resp, err := c.client.GetRoom(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return roomProto2Native(resp.Room), nil
}

type GRPCEmitEventsHelperServer struct {
	proto.UnimplementedEmitEventsHelperServer

	// This is the real implementation
	Impl EmitEventsHelper
}

func (s *GRPCEmitEventsHelperServer) EmitEvents(ctx context.Context, req *proto.EmitEventsRequest) (resp *proto.EmitEventsResponse, err error) {
	events := make([]*types.Event, len(req.Events))
	for i, event := range req.Events {
		events[i] = eventProto2Native(event)
	}
	err = s.Impl.EmitEvents(events)
	if err != nil {
		return nil, err
	}
	return &proto.EmitEventsResponse{}, nil
}

func (s *GRPCEmitEventsHelperServer) AuthenticateUser(ctx context.Context, req *proto.AuthenticateUserRequest) (resp *proto.AuthenticateUserResponse, err error) {
	user, err := s.Impl.AuthenticateUser(req.IdToken, req.Provider)
	if err != nil {
		return nil, err
	}
	return &proto.AuthenticateUserResponse{User: userNative2Proto(user)}, nil
}

func (s *GRPCEmitEventsHelperServer) GetUser(ctx context.Context, req *proto.GetUserRequest) (resp *proto.GetUserResponse, err error) {
	user, err := s.Impl.GetUser(req.UserId)
	if err != nil {
		return nil, err
	}
	return &proto.GetUserResponse{User: userNative2Proto(user)}, nil
}

func (s *GRPCEmitEventsHelperServer) GetRoom(ctx context.Context, req *proto.GetRoomRequest) (resp *proto.GetRoomResponse, err error) {
	room, err := s.Impl.GetRoom(req.RoomId)
	if err != nil {
		return nil, err
	}
	return &proto.GetRoomResponse{Room: roomNative2Proto(room)}, nil
}