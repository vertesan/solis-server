package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllSalaries(ctx context.Context) ([]*pmaster.Salary, error) {
	return mcommon.FindMany[mmodel.Salary, pmaster.Salary](ctx, mname.Salary, bson.M{})
}

func FindCompanySalarySetting(ctx context.Context) (*pmaster.CompanySalarySetting, error) {
	return mcommon.FindOne[mmodel.CompanySalarySetting, pmaster.CompanySalarySetting](ctx, mname.CompanySalarySetting, bson.M{"Id": "company_salary_setting"})
}
