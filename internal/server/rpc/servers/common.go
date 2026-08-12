package servers

import pb "github.com/vertesan/solis-server/internal/proto/papi"

func newCommonResponse(updatedData *pb.UpdatedData) *pb.CommonResponse {
	if updatedData == nil {
		updatedData = &pb.UpdatedData{}
	}
	return &pb.CommonResponse{
		UpdatedData: updatedData,
		DeletedData: &pb.DeletedData{},
	}
}
