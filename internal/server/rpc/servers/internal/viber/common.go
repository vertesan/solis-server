package viber

import "github.com/vertesan/solis-server/internal/proto/papi"

func NewCommonResponse(updatedData *papi.UpdatedData) *papi.CommonResponse {
	if updatedData == nil {
		updatedData = &papi.UpdatedData{}
	}
	return &papi.CommonResponse{
		UpdatedData: updatedData,
		DeletedData: &papi.DeletedData{},
	}
}
