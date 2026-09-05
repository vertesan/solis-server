package notice

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const noticePageSize = 20

func findNoticeCatalog(ctx context.Context) (*smodel.NoticeCatalog, error) {
	catalog, err := opsolis.FindNoticeCatalog(ctx)
	if err == nil {
		return catalog, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &smodel.NoticeCatalog{}, nil
	}
	return nil, err
}

func noticePage(notices []*papi.NoticeInfo, offset int) ([]*papi.NoticeInfo, bool) {
	if offset >= len(notices) {
		return []*papi.NoticeInfo{}, false
	}
	end := offset + noticePageSize
	if end < len(notices) {
		return notices[offset:end], true
	}
	return notices[offset:], false
}
