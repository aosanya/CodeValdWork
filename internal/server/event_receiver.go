package server

import (
	"context"
	"log"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EventDispatcher is called asynchronously after each new event is ACKed.
type EventDispatcher interface {
	Dispatch(ctx context.Context, topic, payload string)
}

// EventReceiverServer implements sharedev1.EventReceiverServiceServer.
// Cross calls NotifyEvent to push a subscribed event; the handler writes a
// ReceivedEvent record for deduplication before returning.
// If dispatcher is non-nil, dispatch is fired asynchronously after the ACK.
type EventReceiverServer struct {
	sharedev1.UnimplementedEventReceiverServiceServer
	backend    entitygraph.DataManager
	agencyID   string
	dispatcher EventDispatcher
}

// NewEventReceiver constructs an EventReceiverServer.
func NewEventReceiver(backend entitygraph.DataManager, agencyID string, dispatcher EventDispatcher) *EventReceiverServer {
	return &EventReceiverServer{backend: backend, agencyID: agencyID, dispatcher: dispatcher}
}

// NotifyEvent receives a pushed event from Cross.
// Deduplicates by event_id; fires dispatcher asynchronously on new events.
func (s *EventReceiverServer) NotifyEvent(ctx context.Context, req *sharedev1.NotifyEventRequest) (*sharedev1.NotifyEventResponse, error) {
	eventID := req.GetEventId()

	if eventID != "" {
		existing, err := s.backend.ListEntities(ctx, entitygraph.EntityFilter{
			AgencyID: s.agencyID,
			TypeID:   "ReceivedEvent",
			Properties: map[string]any{
				"event_id": eventID,
			},
		})
		if err == nil && len(existing) > 0 {
			log.Printf("codevaldwork: NotifyEvent: duplicate event_id=%s topic=%s — skipping", eventID, req.GetTopic())
			return &sharedev1.NotifyEventResponse{}, nil
		}
	}

	_, err := s.backend.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID: s.agencyID,
		TypeID:   "ReceivedEvent",
		Properties: map[string]any{
			"event_id":    eventID,
			"topic":       req.GetTopic(),
			"agency_id":   req.GetAgencyId(),
			"source":      req.GetSource(),
			"payload":     req.GetPayload(),
			"received_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		log.Printf("codevaldwork: NotifyEvent: write ReceivedEvent: %v", err)
		return nil, status.Errorf(codes.Internal, "log received event: %v", err)
	}
	log.Printf("codevaldwork: NotifyEvent: ACK event_id=%s topic=%s source=%s",
		eventID, req.GetTopic(), req.GetSource())

	if s.dispatcher != nil {
		go s.dispatcher.Dispatch(context.Background(), req.GetTopic(), req.GetPayload())
	}

	return &sharedev1.NotifyEventResponse{}, nil
}
