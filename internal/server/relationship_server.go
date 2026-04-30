package server

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// CreateRelationship implements pb.TaskServiceServer.
func (s *Server) CreateRelationship(ctx context.Context, req *pb.CreateRelationshipRequest) (*pb.CreateRelationshipResponse, error) {
	rel, err := s.mgr.CreateRelationship(ctx, req.AgencyId, codevaldwork.Relationship{
		Label:      req.Label,
		FromID:     req.FromId,
		ToID:       req.ToId,
		Properties: structToMap(req.Properties),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.CreateRelationshipResponse{Relationship: relationshipToProto(rel)}, nil
}

// DeleteRelationship implements pb.TaskServiceServer.
func (s *Server) DeleteRelationship(ctx context.Context, req *pb.DeleteRelationshipRequest) (*pb.DeleteRelationshipResponse, error) {
	if err := s.mgr.DeleteRelationship(ctx, req.AgencyId, req.FromId, req.ToId, req.Label); err != nil {
		return nil, mapError(err)
	}
	return &pb.DeleteRelationshipResponse{}, nil
}

// TraverseRelationships implements pb.TaskServiceServer.
func (s *Server) TraverseRelationships(ctx context.Context, req *pb.TraverseRelationshipsRequest) (*pb.TraverseRelationshipsResponse, error) {
	dir := protoToDirection(req.Direction)
	edges, err := s.mgr.TraverseRelationships(ctx, req.AgencyId, req.VertexId, req.Label, dir)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.Relationship, 0, len(edges))
	for _, e := range edges {
		out = append(out, relationshipToProto(e))
	}
	return &pb.TraverseRelationshipsResponse{Relationships: out}, nil
}

// ── Conversion helpers ────────────────────────────────────────────────────────

func relationshipToProto(r codevaldwork.Relationship) *pb.Relationship {
	pt := &pb.Relationship{
		Id:         r.ID,
		AgencyId:   r.AgencyID,
		Label:      r.Label,
		FromId:     r.FromID,
		ToId:       r.ToID,
		Properties: mapToStruct(r.Properties),
	}
	if r.CreatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
			pt.CreatedAt = timestamppb.New(ts)
		}
	}
	return pt
}

// structToMap converts a google.protobuf.Struct into the map[string]any shape
// the domain layer expects. nil → nil so the manager can distinguish "no
// properties supplied" from "empty map".
func structToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return nil
	}
	return s.AsMap()
}

// mapToStruct is the inverse of structToMap. Returns nil for an empty/nil
// input so callers don't see an empty Struct payload.
func mapToStruct(m map[string]any) *structpb.Struct {
	if len(m) == 0 {
		return nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		// structpb.NewStruct rejects values it cannot marshal (e.g. time.Time).
		// Fall back to a stringified copy so callers always get *something*.
		safe := make(map[string]any, len(m))
		for k, v := range m {
			switch x := v.(type) {
			case bool, float64, float32, int, int32, int64, uint, uint32, uint64, string, nil:
				safe[k] = x
			default:
				safe[k] = ""
			}
		}
		s, _ = structpb.NewStruct(safe)
	}
	return s
}

func protoToDirection(d pb.Direction) codevaldwork.Direction {
	if d == pb.Direction_DIRECTION_INBOUND {
		return codevaldwork.DirectionInbound
	}
	return codevaldwork.DirectionOutbound
}
